package main

import (
	"encoding/json"
	"github.com/uber-go/zap"
	"github.com/vulcand/oxy/forward"
	"github.com/vulcand/oxy/roundrobin"
	"gopkg.in/alecthomas/kingpin.v2"
	"html/template"
	"net/http"
	"strconv"
	"time"
)

var (
	app            = kingpin.New("timeattack", "Replays http requests").Version("1.0")
	httpPort       = kingpin.Flag("port", "Http port to listen on").Default("8080").Int()
	httpPortAlias  = kingpin.Flag("port-alias", "Http port Shelob is actually exposed on").Int()
	masterDomain   = kingpin.Flag("domain", "All apps will by default be exposed as a subdomain to this domain").Default("localhost").String()
	marathons      = kingpin.Flag("marathon", "url to marathon (repeatable for multiple instances of marathon)").Required().Strings()
	marathonAuth   = kingpin.Flag("marathon-auth", "username:password for marathon").String()
	updateInterval = kingpin.Flag("update-interval", "Force updates this often [s]").Default("5").Int()
	insecureSSL    = kingpin.Flag("insecureSSL", "Ignore SSL errors").Default("false").Bool()
	shelobItself   = http.NewServeMux()
	forwarder, _   = forward.New()
	backends       = make(map[string][]Backend)
	rrbBackends    = make(map[string]*roundrobin.RoundRobin)
	logger         = zap.New(
		zap.NewJSONEncoder(zap.RFC3339Formatter("timestamp")),
	)
)

func createRoundRobinBackends(backends map[string][]Backend) map[string]*roundrobin.RoundRobin {
	rrbBackends := make(map[string]*roundrobin.RoundRobin)

	for domain, backendList := range backends {
		rrbBackends[domain], _ = roundrobin.New(forwarder)

		for _, backend := range backendList {
			rrbBackends[domain].UpsertServer(backend.Url)
		}
	}

	return rrbBackends
}

func backendManager(backendChan chan map[string][]Backend, updateChan chan time.Time) error {
	for {
		backends, err := updateBackends()

		if err != nil {
			println("Error:")
			println(err.Error())
		} else {
			backendChan <- backends
		}

		select {
		case eventTime := <-updateChan:
			delay := time.Now().Sub(eventTime)
			logger.Info("Update requested",
				appField,
				eventField("reload"),
				zap.Duration("delay", delay),
			)
		case <-time.After(time.Second * time.Duration(*updateInterval)):
			logger.Info("No changes for a while, forcing reload",
				appField,
				eventField("reload"),
			)
		}
	}
}

func listApplicationsHandler(w http.ResponseWriter, r *http.Request) {
	data := make(map[string][]Backend)

	port := *httpPort
	if *httpPortAlias != 0 {
		port = *httpPortAlias
	}

	for domain, backends := range backends {
		if port != 80 {
			domain = domain + ":" + strconv.Itoa(port)
		}

		data[domain] = backends
	}

	var page = `
<!DOCTYPE html>
<html>
	<head>
		<meta charset="UTF-8">
		<title>{{.Domain}}</title>
	</head>
	<body>
		<h1>Available applications:</h1>
		<ul>
			{{range $domain, $backends := . }}<li><a href="http://{{ $domain }}">{{ $domain }}</a></li>
			{{ end }}
		</ul>
	</body>
</html>`

	t, err := template.New("t").Parse(page)
	if err != nil {
		panic(err)
	}

	err = t.Execute(w, data)

	if err != nil {
		panic(err)
	}
}

func listApplicationsHandlerJson(w http.ResponseWriter, r *http.Request) {
	json, err := json.Marshal(backends)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(json)
}

func routeToSelf(req *http.Request) bool {
	return (req.Host == "localhost") || (req.Host == *masterDomain)
}

func main() {
	kingpin.Parse()
	backendChan := make(chan map[string][]Backend)
	updateChan := make(chan time.Time)

	shelobItself.Handle("/", http.HandlerFunc(listApplicationsHandler))
	shelobItself.Handle("/api/applications", http.HandlerFunc(listApplicationsHandlerJson))

	go func() {
		for {
			select {
			case bs := <-backendChan:
				backends = bs
				rrbBackends = createRoundRobinBackends(backends)
			}
		}
	}()

	go backendManager(backendChan, updateChan)
	go trackUpdates(updateChan)

	redirect := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		t__start := time.Now().UnixNano()
		domain := stripPortFromDomain(req.Host)
		status := http.StatusOK

		if (domain == "localhost") || (domain == *masterDomain) {
			shelobItself.ServeHTTP(w, req)
		} else if backend := rrbBackends[domain]; backend != nil {
			backend.ServeHTTP(w, req)
		} else {
			w.WriteHeader(http.StatusNotFound)
			status = http.StatusNotFound
		}

		duration := float64(time.Now().UnixNano()-t__start) / 1000000
		logger.Info("",
			appField,
			eventField("request"),
			zap.Nest("request",
				zap.Nest("user",
					zap.String("addr", req.RemoteAddr),
					zap.String("agent", req.UserAgent()),
				),
				zap.String("domain", domain),
				zap.String("url", req.URL.String()),
				zap.String("method", req.Method),
				zap.String("protocol", req.Proto),
				zap.Int("status", status),
				zap.Float64("duration", duration),
			),
		)
	})

	logger.Info("shelob started",
		appField,
		eventField("started"),
		zap.Int("port", *httpPort),
	)

	s := &http.Server{
		Addr:    ":" + strconv.Itoa(*httpPort),
		Handler: redirect,
	}
	logger.Fatal(s.ListenAndServe().Error(),
		appField,
		eventField("shutdown"),
	)
}