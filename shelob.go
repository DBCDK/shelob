package main

import (
	"github.com/Sirupsen/logrus"
	"github.com/dbcdk/shelob/backends"
	"github.com/dbcdk/shelob/logging"
	"github.com/dbcdk/shelob/proxy"
	"github.com/dbcdk/shelob/signals"
	"github.com/dbcdk/shelob/util"
	"github.com/vulcand/oxy/roundrobin"
	"gopkg.in/alecthomas/kingpin.v2"
	"os"
	"strconv"
	"time"
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
	acceptableUpdateLag = kingpin.Flag("acceptable-update-lag", "Mark Shelob as down when not receiving updates for this many seconds (0=disabled)").Default("0").Int()
	shutdownDelay       = kingpin.Flag("shutdown-delay", "Delay shutdown by this many seconds [s]").Int()
	insecureSSL         = kingpin.Flag("insecureSSL", "Ignore SSL errors").Default("false").Bool()
	accessLogEnabled    = kingpin.Flag("access-log", "Enable accesslog to stdout").Default("true").Bool()
	log                 = logging.GetInstance()
)

func init() {
	kingpin.Parse()
	logrus.SetLevel(logrus.ErrorLevel)
	logrus.SetFormatter(&logrus.JSONFormatter{
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime: "timestamp",
		},
	})
}

func main() {
	defer log.Sync()

	name := *instanceName
	if name == "" {
		hostname, err := os.Hostname()
		if err != nil {
			log.Warn("Could not resolve own hostname: " + err.Error())
		} else {
			name = hostname + ":" + strconv.Itoa(*httpPort)
		}
	}

	config := util.Config{
		HttpPort:        *httpPort,
		MetricsPort:     *metricsPort,
		ReuseHttpPort:   *reuseHttpPort,
		IgnoreSSLErrors: *insecureSSL,
		InstanceName:    name,
		Logging: util.Logging{
			AccessLog: *accessLogEnabled,
		},
		State: util.State{
			ShutdownInProgress: false,
		},
		Counters: util.CreateAndRegisterCounters(),
		Marathon: util.MarathonConfig{
			Urls:        *marathons,
			Auth:        *marathonAuth,
			LabelPrefix: *marathonLabelPrefix,
		},
		Domain:              *masterDomain,
		ShutdownDelay:       *shutdownDelay,
		UpdateInterval:      *updateInterval,
		AcceptableUpdateLag: *acceptableUpdateLag,
		Backends:            make(map[string][]util.Backend, 0),
		RrbBackends:         make(map[string]*roundrobin.RoundRobin),
	}

	signals.RegisterSignals(&config)

	go proxy.StartProxyServer(&config)
	go proxy.StartAdminServer(&config)

	// messages to this channel will trigger instant updates
	updateChan := make(chan time.Time)

	// start main loop
	forwarder := proxy.CreateForwarder()
	backends.BackendManager(&config, forwarder, updateChan)
}
