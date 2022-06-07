package main

import (
	"github.com/dbcdk/shelob/backends"
	"github.com/dbcdk/shelob/certs"
	"github.com/dbcdk/shelob/kubernetes"
	"github.com/dbcdk/shelob/logging"
	"github.com/dbcdk/shelob/proxy"
	"github.com/dbcdk/shelob/signals"
	"github.com/dbcdk/shelob/util"
	"github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
)

var (
	app                 = kingpin.New("shelob", "Automatically updated HTTP reverse proxy").Version("1.0")
	httpPort            = kingpin.Flag("port", "Http port to listen on").Default("8080").Int()
	httpsPort           = kingpin.Flag("tlsport", "Https port to listen on").Default("8443").Int()
	metricsPort         = kingpin.Flag("metrics-port", "Http port to serve Prometheus metrics on").Default("8081").Int()
	reuseHttpPort       = kingpin.Flag("reuse-port", "Enable SO_REUSEPORT for the main http port").Default("false").Bool()
	instanceName        = kingpin.Flag("name", "Instance name. Used in headers and on status pages.").String()
	masterDomain        = kingpin.Flag("domain", "This will enable all apps to by default be exposed as a subdomain to this domain.").String()
	kubeConfig          = kingpin.Flag("kubeconfig", "Path to kubeconfig file with kubernets connection details").ExistingFile()
	reloadEvery         = kingpin.Flag("reload-every", "Force updates this often [s]").Default("30").Int()
	reloadRollup        = kingpin.Flag("reload-rollup", "Limit number of reloads by merging them every n seconds").Default("1").Int()
	acceptableUpdateLag = kingpin.Flag("acceptable-update-lag", "Mark Shelob as down when not receiving updates for this many seconds (0=disabled)").Default("0").Int()
	shutdownDelay       = kingpin.Flag("shutdown-delay", "Delay shutdown by this many seconds [s]").Int()
	insecureSSL         = kingpin.Flag("insecureSSL", "Ignore SSL errors").Default("false").Bool()
	accessLogEnabled    = kingpin.Flag("access-log", "Enable accesslog to stdout").Default("true").Bool()
	disableWatch        = kingpin.Flag("disable-watch", "Disables the kubernetes watch-api feature, causing updates to only happen once per 'reload-every' interval.").Default("false").Bool()
	ignoreNamespaces    = kingpin.Flag("ignore-namespaces", "Ignore endpoint watch-events from one or more (comma-separated) namespaces").Default("default,kube-system").String()
	certFilePairs       = kingpin.Flag("cert-file-pairs", "Comma-separated list of keypair paths in local fs - format: 'hostname1:path-to-pubkey1:path-to-privkey1,hostname2:path-to-pubkey2:path-to-privkey2' etc., mutually excusive with 'cert-namespace'").String()
	certNamespace       = kingpin.Flag("cert-namespace", "Kubernetes Namespace in which to search for issued certificates, mutually excusive with 'cert-file-pairs'").String()
	wildcardCertPrefix  = kingpin.Flag("wildcard-cert-prefix", "The name prefix to use for wildcard certificates in Kubernetes, e.g. (prefix).wildcardexample.com.").Default("").String()
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

	kubeconfig, err := kubernetes.GetKubeConfig(kubeConfig)
	if err != nil {
		log.Error("Cannot start without a valid kubeconfig: " + err.Error())
		os.Exit(1)
	}

	ignoreNamespacesMap := make(map[string]bool)
	for _, n := range strings.Split(*ignoreNamespaces, ",") {
		if n := strings.TrimSpace(n); n != "" {
			ignoreNamespacesMap[n] = true
		}
	}

	certFilePairMap := make(map[string]util.KeyPairPaths)
	if *certFilePairs != "" {
		for _, pair := range strings.Split(*certFilePairs, ",") {
			parts := strings.Split(pair, ":")
			certHostName := parts[0]
			pubkey := parts[1]
			privkey := parts[2]
			if _, exists := certFilePairMap[certHostName]; exists || certHostName == "" {
				log.Error("Cert: Invalid or duplicate hostname: " + certHostName)
				os.Exit(1)
			}
			pubRaw, err := ioutil.ReadFile(pubkey)
			if err != nil {
				log.Error("Invalid public key file: " + pubkey + " err: " + err.Error())
				os.Exit(1)
			}
			privRaw, err := ioutil.ReadFile(privkey)
			if err != nil {
				log.Error("Invalid private key file: " + pubkey + " err: " + err.Error())
				os.Exit(1)
			}
			_, err = util.ParseX509(pubRaw, privRaw)
			if err != nil {
				log.Error("Unable to parse x509 cert: " + pubkey + ", key: " + privkey + " err: " + err.Error())
				os.Exit(1)
			}
			certFilePairMap[certHostName] = util.KeyPairPaths{
				PublicKey:  pubkey,
				PrivateKey: privkey,
			}
		}
	}
	config := util.Config{
		HttpPort:        *httpPort,
		HttpsPort:       *httpsPort,
		MetricsPort:     *metricsPort,
		ReuseHttpPort:   *reuseHttpPort,
		IgnoreSSLErrors: *insecureSSL,
		InstanceName:    name,
		Logging: util.Logging{
			AccessLog: *accessLogEnabled,
		},
		State: util.State{
			ShutdownInProgress: false,
			ShutdownChan:       make(chan bool),
		},
		Counters:            util.CreateAndRegisterCounters(),
		Kubeconfig:          kubeconfig,
		Domain:              *masterDomain,
		ShutdownDelay:       *shutdownDelay,
		ReloadEvery:         *reloadEvery,
		ReloadRollup:        *reloadRollup,
		AcceptableUpdateLag: *acceptableUpdateLag,
		Frontends:           make(map[string]*util.Frontend, 0),
		Forwarder:           proxy.CreateForwarder(),
		DisableWatch:        *disableWatch,
		IgnoreNamespaces:    ignoreNamespacesMap,
		CertFilePairMap:     certFilePairMap,
		CertNamespace:       *certNamespace,
		WildcardCertPrefix:  *wildcardCertPrefix,
	}

	signals.RegisterSignals(&config)

	// messages to these channels will trigger instant updates
	backendsChan := make(chan util.Reload)
	certsChan := make(chan util.Reload)

	certHandler, err := certs.New(&config, certsChan)
	if err != nil {
		log.Error("Couldn't start certHandler, exitting... err: " + err.Error())
		os.Exit(1)
	}
	certHandler.RegisterValidityMonitoring()

	go proxy.StartProxyServer(&config)
	go proxy.StartTLSProxyServer(&config, certHandler)
	go proxy.StartAdminServer(&config)

	// start main loop
	backends.BackendManager(&config, backendsChan)
}
