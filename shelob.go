package main

import (
	log "github.com/Sirupsen/logrus"
	"github.com/dbcdk/shelob/handlers"
	"github.com/dbcdk/shelob/marathon"
	"github.com/dbcdk/shelob/signals"
	"github.com/dbcdk/shelob/util"
	"github.com/vulcand/oxy/forward"
	"github.com/vulcand/oxy/roundrobin"
	"gopkg.in/alecthomas/kingpin.v2"
	"net/http"
	"strconv"
	"time"
	"strings"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	app                 = kingpin.New("shelob", "Automatically updated HTTP reverse proxy for Marathon").Version("1.0")
	httpPort            = kingpin.Flag("port", "Http port to listen on").Default("8080").Int()
	instanceName        = kingpin.Flag("name", "Instance name. Used in headers and on status pages.").String()
	masterDomain        = kingpin.Flag("domain", "This will enable all apps to by default be exposed as a subdomain to this domain.").String()
	marathons           = kingpin.Flag("marathon", "url to marathon (repeatable for multiple instances of marathon)").Required().Strings()
	marathonAuth        = kingpin.Flag("marathon-auth", "username:password for marathon").String()
	marathonLabelPrefix = kingpin.Flag("marathon-label-prefix", "prefix for marathon labels used for configuration").Default("expose").String()
	updateInterval      = kingpin.Flag("update-interval", "Force updates this often [s]").Default("5").Int()
	shutdownDelay       = kingpin.Flag("shutdown-delay", "Delay shutdown by this many seconds [s]").Int()
	insecureSSL         = kingpin.Flag("insecureSSL", "Ignore SSL errors").Default("false").Bool()
	shelobItself        = http.NewServeMux()
	forwarder, _        = forward.New(forward.PassHostHeader(true))
	shutdownInProgress  = false
	request_counter     = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "shelob_requests_total",
		Help: "Total number of requests served"},
	)
)

func init() {
	log.SetFormatter(&log.JSONFormatter{
		FieldMap: log.FieldMap{
			log.FieldKeyTime: "timestamp",
		},
	})
	log.SetLevel(log.DebugLevel)

	prometheus.Register(request_counter)
}

func createRoundRobinBackends(backends map[string][]util.Backend) map[string]*roundrobin.RoundRobin {
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
			log.WithFields(log.Fields{
				"app":   "shelob",
				"event": "reload",
				"delay": delay.String(),
			}).Info("Update requested")
		case <-time.After(time.Second * time.Duration(*updateInterval)):
			log.WithFields(log.Fields{
				"app":   "shelob",
				"event": "reload",
			}).Info("No changes for a while, forcing reload")
		}
	}
}

func routeToSelf(req *http.Request) bool {
	return (req.Host == "localhost") || (req.Host == *masterDomain)
}

func main() {
	kingpin.Parse()

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

	backendChan := make(chan map[string][]util.Backend)
	updateChan := make(chan time.Time)

	shelobItself.Handle("/", http.HandlerFunc(handlers.CreateListApplicationsHandler(&config)))
	shelobItself.Handle("/api/applications", http.HandlerFunc(handlers.CreateListApplicationsHandlerJson(&config)))
	shelobItself.Handle("/status", http.HandlerFunc(handlers.CreateStatusHandler(&config, &shutdownInProgress)))
	shelobItself.Handle("/metrics", promhttp.Handler())

	go func() {
		for {
			select {
			case bs := <-backendChan:
				config.Backends = bs
				config.RrbBackends = createRoundRobinBackends(bs)
			}
		}
	}()

	go backendManager(&config, backendChan, updateChan)
	// go trackUpdates(updateChan)

	redirect := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		t__start := time.Now().UnixNano()
		domain := util.StripPortFromDomain(req.Host)
		status := http.StatusOK

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
			backend.ServeHTTP(w, req)
		} else {
			status = http.StatusNotFound
			http.Error(w, http.StatusText(status), status)
		}

		duration := float64(time.Now().UnixNano()-t__start) / 1000000
		log.WithFields(log.Fields{
			"app":   "shelob",
			"event": "requets",
			"request": log.Fields{
				"duration": duration,
				"user": log.Fields{
					"addr":  req.RemoteAddr,
					"agent": req.UserAgent(),
				},
				"domain":   domain,
				"method":   req.Method,
				"protocol": req.Proto,
				"status":   status,
				"url":      req.URL.String(),
			},
		}).Info("")
	})

	log.WithFields(log.Fields{
		"app":   "shelob",
		"event": "started",
		"port":  *httpPort,
	}).Info("shelob started")

	s := &http.Server{
		Addr:    ":" + strconv.Itoa(*httpPort),
		Handler: handlers.MetricsCountRequest(redirect, []prometheus.Counter{request_counter}),
	}
	log.WithFields(log.Fields{
		"app":   "shelob",
		"event": "shutdown",
	}).Fatal(s.ListenAndServe())
}
