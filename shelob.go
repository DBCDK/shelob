package main

import (
	"encoding/json"
	"fmt"
	"github.com/vulcand/oxy/forward"
	"github.com/vulcand/oxy/roundrobin"
	"gopkg.in/alecthomas/kingpin.v2"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"
)

var (
	app            = kingpin.New("timeattack", "Replays http requests").Version("1.0")
	httpPort       = kingpin.Flag("port", "Http port to listen on").Default("8080").Int()
	masterDomain   = kingpin.Flag("domain", "All apps will by default be exposed as a subdomain to this domain").Default("localhost").String()
	marathons      = kingpin.Flag("marathon", "url to marathon (repeatable for multiple instances of marathon)").Required().Strings()
	marathonAuth   = kingpin.Flag("marathon-auth", "username:password for marathon").String()
	updateInterval = kingpin.Flag("update-interval", "Force updates this often [s]").Default("5").Int()
	insecureSSL    = kingpin.Flag("insecureSSL", "Ignore SSL errors").Default("false").Bool()
	shelobItself   = http.NewServeMux()
	forwarder, _   = forward.New()
	backends       = make(map[string][]Backend)
	rrbBackends    = make(map[string]*roundrobin.RoundRobin)
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
			fmt.Printf("Update requested %v ago\n", delay)
		case <-time.After(time.Second * time.Duration(*updateInterval)):
			fmt.Printf("No changes for a while, forcing reload..\n")
		}
	}
}

func listApplicationsHandler(w http.ResponseWriter, r *http.Request) {
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

	t.Execute(w, backends)
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
	strPort := strconv.Itoa(*httpPort)
	return (req.Host == "localhost:"+strPort) || (req.Host == *masterDomain+":"+strPort)
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
		//fmt.Printf("%v %v\n", req.RemoteAddr, req.Host)

		if routeToSelf(req) {
			shelobItself.ServeHTTP(w, req)
		} else if backend := rrbBackends[req.Host]; backend != nil {
			backend.ServeHTTP(w, req)
		} else {
			w.WriteHeader(http.StatusNotFound)
			//fmt.Fprint(w, "not found")
		}
	})

	s := &http.Server{
		Addr:    ":" + strconv.Itoa(*httpPort),
		Handler: redirect,
	}
	log.Fatal(s.ListenAndServe())
}
