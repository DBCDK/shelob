package main

import (
	"github.com/Sirupsen/logrus"
	"github.com/dbcdk/shelob/handlers"
	"github.com/dbcdk/shelob/logging"
	"github.com/dbcdk/shelob/marathon"
	"github.com/dbcdk/shelob/signals"
	"github.com/dbcdk/shelob/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/viki-org/dnscache"
	"github.com/vulcand/oxy/forward"
	"github.com/vulcand/oxy/roundrobin"
	"go.uber.org/zap"
	"gopkg.in/alecthomas/kingpin.v2"
	"net"
	"net/http"
	"net/http/pprof"
	"strconv"
	"strings"
	"time"
	"github.com/kavu/go_reuseport"
)

var (
	app                 = kingpin.New("shelob", "Automatically updated HTTP reverse proxy for Marathon").Version("1.0")
	httpPort            = kingpin.Flag("port", "Http port to listen on").Default("8080").Int()
	metricsPort         = kingpin.Flag("metrics-port", "Http port to serve Prometheus metrics on").Default("8081").Int()
	reuseHttpPort       = kingpin.Flag("reuse-port", "Enable SO_REUSEPORT for the main http port").Default("false").Bool()
	instanceName        = kingpin.Flag("name", "Instance name. Used in headers and on status pages.").String()
	masterDomain        = kingpin.Flag("domain", "This will enable all apps to by default be exposed as a subdomain to this domain.").String()
	marathons           = kingpin.Flag("marathon", "url to marathon (repeatable for multiple instances of marathon)").Required().Strings()
	marathonAuth        = kingpin.Flag("marathon-auth", "username:password for marathon").String()
	marathonLabelPrefix = kingpin.Flag("marathon-label-prefix", "prefix for marathon labels used for configuration").Default("expose").String()
	updateInterval      = kingpin.Flag("update-interval", "Force updates this often [s]").Default("5").Int()
	shutdownDelay       = kingpin.Flag("shutdown-delay", "Delay shutdown by this many seconds [s]").Int()
	insecureSSL         = kingpin.Flag("insecureSSL", "Ignore SSL errors").Default("false").Bool()
	accessLogEnabled    = kingpin.Flag("access-log", "Enable accesslog to stdout").Default("true").Bool()
	shelobItself        = http.NewServeMux()
	adminMux            = http.NewServeMux()
	shutdownInProgress  = false
	request_counter     = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_server_requests_total",
		Help: "Total number of http requests",
	}, []string{"domain", "code", "method", "type"})
	reload_counter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "shelob_reloads_total",
		Help: "Number of times the service definitions have been reloaded",
	})
	log = logging.GetInstance()
)

func init() {
	kingpin.Parse()
	logrus.SetLevel(logrus.ErrorLevel)
	logrus.SetFormatter(&logrus.JSONFormatter{
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime: "timestamp",
		},
	})

	prometheus.MustRegister(request_counter)
	prometheus.MustRegister(reload_counter)
}

func createForwarder() *forward.Forwarder {
	resolver := dnscache.New(time.Minute * 1)

	dialFn := func(network string, address string) (net.Conn, error) {
		separator := strings.LastIndex(address, ":")
		ip, _ := resolver.FetchOneString(address[:separator])
		dialer := &net.Dialer{
			Timeout: 1 * time.Second,
		}
		return dialer.Dial(network, ip+address[separator:])
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConnsPerHost:   10,
		MaxIdleConns:          100,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   2 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		Dial: dialFn,
	}

	forwarder, err := forward.New(forward.PassHostHeader(true), forward.RoundTripper(transport))

	if err != nil {
		panic(err)
	}

	return forwarder
}

func createRoundRobinBackends(forwarder *forward.Forwarder, backends map[string][]util.Backend) map[string]*roundrobin.RoundRobin {
	rrbBackends := make(map[string]*roundrobin.RoundRobin)

	for domain, backendList := range backends {
		rrbBackends[domain], _ = roundrobin.New(forwarder)

		for _, backend := range backendList {
			rrbBackends[domain].UpsertServer(backend.Url)
		}
	}

	return rrbBackends
}

func backendManager(config *util.Config, backendChan chan map[string][]util.Backend, updateChan chan time.Time) error {
	for {
		backends, err := marathon.UpdateBackends(config)

		if err != nil {
			println("Error:")
			println(err.Error())
		} else {
			backendChan <- backends
		}

		select {
		case eventTime := <-updateChan:
			delay := time.Now().Sub(eventTime)
			log.Info("Update requested",
				zap.String("event", "reload"),
				zap.String("delay", delay.String()),
			)
		case <-time.After(time.Second * time.Duration(*updateInterval)):
			log.Info("No changes for a while, forcing reload",
				zap.String("event", "reload"),
			)
		}
	}
}

func routeToSelf(req *http.Request) bool {
	return (req.Host == "localhost") || (req.Host == *masterDomain)
}

func createListener(proto string, laddr string, reuse bool) (net.Listener, error) {
	if *reuseHttpPort {
		return reuseport.Listen(proto, laddr)
	} else {
		return net.Listen(proto, laddr)
	}
}

func main() {
	defer log.Sync()

	config := util.Config{
		HttpPort:        *httpPort,
		IgnoreSSLErrors: *insecureSSL,
		InstanceName:    *instanceName,
		Marathon: util.MarathonConfig{
			Urls:        *marathons,
			Auth:        *marathonAuth,
			LabelPrefix: *marathonLabelPrefix,
		},
		Domain:         *masterDomain,
		ShutdownDelay:  *shutdownDelay,
		UpdateInterval: *updateInterval,
		Backends:       make(map[string][]util.Backend, 0),
		RrbBackends:    make(map[string]*roundrobin.RoundRobin),
	}

	shutdownChan := make(chan bool, 1)
	go func() {
		for {
			select {
			case shutdownState := <-shutdownChan:
				shutdownInProgress = shutdownState
			}
		}
	}()

	signals.RegisterSignals(&config, shutdownChan)

	forwarder := createForwarder()

	backendChan := make(chan map[string][]util.Backend)
	updateChan := make(chan time.Time)

	shelobItself.Handle("/", http.HandlerFunc(handlers.CreateListApplicationsHandler(&config)))
	shelobItself.Handle("/api/applications", http.HandlerFunc(handlers.CreateListApplicationsHandlerJson(&config)))
	shelobItself.Handle("/status", http.HandlerFunc(handlers.CreateStatusHandler(&config, &shutdownInProgress)))
	shelobItself.Handle("/metrics", promhttp.Handler())

	adminMux.Handle("/metrics", promhttp.Handler())
	adminMux.HandleFunc("/debug/pprof/", pprof.Index)
	adminMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	adminMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	adminMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)

	go func() {
		for {
			select {
			case bs := <-backendChan:
				config.Backends = bs
				config.RrbBackends = createRoundRobinBackends(forwarder, bs)
				reload_counter.Inc()
			}
		}
	}()

	go backendManager(&config, backendChan, updateChan)
	// go trackUpdates(updateChan)

	redirect := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		t__start := time.Now().UnixNano()
		domain := util.StripPortFromDomain(req.Host)
		status := http.StatusOK
		request_type := "internal"

		tooManyXForwardedHostHeaders := false

		if xForwardedHostHeader, ok := req.Header["X-Forwarded-Host"]; ok {
			// The XFH-header must not be repeated
			if len(xForwardedHostHeader) == 1 {
				xForwardedHost := xForwardedHostHeader[0]
				// .. but it can contain a list of hosts. Pick the first one in the list, if that's the case
				if strings.Contains(xForwardedHost, ",") {
					parts := strings.Split(xForwardedHost, ",")
					xForwardedHost = strings.TrimSpace(parts[0])
				}

				req.Host = xForwardedHost
			} else {
				tooManyXForwardedHostHeaders = true
			}
			delete(req.Header, "X-Forwarded-Host")
		}

		if tooManyXForwardedHostHeaders {
			status = http.StatusBadRequest
			http.Error(w, "X-Forwarded-Host must not be repeated", status)
		} else if (domain == "localhost") || (domain == *masterDomain) {
			shelobItself.ServeHTTP(w, req)
		} else if backend := config.RrbBackends[domain]; backend != nil {
			request_type = "proxy"
			backend.ServeHTTP(w, req)
		} else {
			status = http.StatusNotFound
			http.Error(w, http.StatusText(status), status)
		}

		duration := float64(time.Now().UnixNano()-t__start) / 1000000

		promLabels := prometheus.Labels{
			"domain": domain,
			"code":   strconv.Itoa(status),
			"method": req.Method,
			"type":   request_type,
		}
		request_counter.With(promLabels).Inc()

		if *accessLogEnabled {
			log.Info("request",
				zap.String("event", "request"),
				zap.Any("marathon", map[string]interface{}{
					"duration": duration,
					"user": map[string]interface{}{
						"addr":  req.RemoteAddr,
						"agent": req.UserAgent(),
					},
					"domain":   domain,
					"method":   req.Method,
					"protocol": req.Proto,
					"status":   status,
					"url":      req.URL.String(),
				}),
			)

		}
	})

	log.Info("shelob started on port "+strconv.Itoa(*httpPort),
		zap.String("event", "started"),
		zap.Int("port", *httpPort),
	)

	go func() {
		metricsServer := &http.Server{
			Addr:    ":" + strconv.Itoa(*metricsPort),
			Handler: adminMux,
		}

		metricsServer.ListenAndServe()
	}()

	proto := "tcp"
	httpAddr := ":" + strconv.Itoa(*httpPort)

	listener, err := createListener(proto, httpAddr, *reuseHttpPort)
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	s := &http.Server{
		Handler: redirect,
	}

	log.Fatal(s.Serve(listener).Error(),
		zap.String("event", "shutdown"),
		zap.Int("port", *httpPort),
	)

}
