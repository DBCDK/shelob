package main

import (
	"github.com/Sirupsen/logrus"
	"github.com/dbcdk/shelob/logging"
	"github.com/dbcdk/shelob/marathon"
	"github.com/dbcdk/shelob/signals"
	"github.com/dbcdk/shelob/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vulcand/oxy/roundrobin"
	"go.uber.org/zap"
	"gopkg.in/alecthomas/kingpin.v2"
	"time"
	"github.com/dbcdk/shelob/proxy"
	"github.com/vulcand/oxy/forward"
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


func backendManager(config *util.Config, forwarder *forward.Forwarder, updateChan chan time.Time) error {
	for {
		backends, err := marathon.UpdateBackends(config)

		if err != nil {
			println("Error:")
			println(err.Error())
		} else {
			config.Backends = backends
			config.RrbBackends = proxy.CreateRoundRobinBackends(forwarder, backends)
			config.Counters.Reloads.Inc()
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

func main() {
	defer log.Sync()

	config := util.Config{
		HttpPort:        *httpPort,
		MetricsPort:     *metricsPort,
		ReuseHttpPort:   *reuseHttpPort,
		IgnoreSSLErrors: *insecureSSL,
		InstanceName:    *instanceName,
		Logging: util.Logging{
			AccessLog: *accessLogEnabled,
		},
		State: util.State{
			ShutdownInProgress: false,
		},
		Counters: util.Counters{
			Requests: *request_counter,
			Reloads: reload_counter,
		},
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

	signals.RegisterSignals(&config)

	forwarder := proxy.CreateForwarder()

	// messages to this channel will trigger instant updates
	updateChan := make(chan time.Time)

	go proxy.StartProxyServer(&config)
	go proxy.StartAdminServer(&config)

	// start main loop
	backendManager(&config, forwarder, updateChan)
}
