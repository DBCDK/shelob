package main

import (
	"encoding/json"
	log "github.com/Sirupsen/logrus"
	"github.com/vulcand/oxy/forward"
	"github.com/vulcand/oxy/roundrobin"
	"gopkg.in/alecthomas/kingpin.v2"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"
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
	backends            = make(map[string][]Backend)
	rrbBackends         = make(map[string]*roundrobin.RoundRobin)
	shutdownInProgress  = false
)

func init() {
	log.SetFormatter(&log.JSONFormatter{
		FieldMap: log.FieldMap{
			log.FieldKeyTime: "timestamp",
		},
	})
	log.SetLevel(log.DebugLevel)
}

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

func statusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if shutdownInProgress {
		b, _ := json.Marshal(ShelobStatus{Name: *instanceName, Up: false})
		http.Error(w, string(b), http.StatusServiceUnavailable)
	} else {
		b, _ := json.Marshal(ShelobStatus{Name: *instanceName, Up: true})
		w.Write(b)
	}
}

func listApplicationsHandler(w http.ResponseWriter, r *http.Request) {
	data := make(map[string][]Backend)
	port := "80"

	if strings.Contains(r.Host, ":") {
		port = strings.SplitN(r.Host, ":", 2)[1]
	}

	for domain, backends := range backends {
		if port != "80" {
			domain = domain + ":" + port
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

	registerSignals()

	backendChan := make(chan map[string][]Backend)
	updateChan := make(chan time.Time)

	shelobItself.Handle("/", http.HandlerFunc(listApplicationsHandler))
	shelobItself.Handle("/status", http.HandlerFunc(statusHandler))
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
	// go trackUpdates(updateChan)

	redirect := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		t__start := time.Now().UnixNano()
		domain := stripPortFromDomain(req.Host)
		status := http.StatusOK

		tooManyXForwardedHostHeaders := false

		if xForwardedHost, ok := req.Header["X-Forwarded-Host"]; ok {
			if len(xForwardedHost) == 1 {
				req.Host = xForwardedHost[0]
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
		} else if backend := rrbBackends[domain]; backend != nil {
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
		Handler: redirect,
	}
	log.WithFields(log.Fields{
		"app":   "shelob",
		"event": "shutdown",
	}).Fatal(s.ListenAndServe())
}
