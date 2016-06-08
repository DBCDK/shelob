package main

import (
	"encoding/json"
	"fmt"
	"github.com/vulcand/oxy/forward"
	"github.com/vulcand/oxy/roundrobin"
	"gopkg.in/alecthomas/kingpin.v2"
	"log"
	"net/http"
	"net/url"
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

//func requestHandler() *http.ServeMux {
//	mux := http.NewServeMux()
//}

func listApplicationsHandler(w http.ResponseWriter, r *http.Request) {
	json, err := json.Marshal(backends)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(json)
}

func statusServer(port int) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/", http.HandlerFunc(listApplicationsHandler))

	server := &http.Server{
		Addr:    ":" + strconv.Itoa(port),
		Handler: mux,
	}



	return server
}

func main() {
	kingpin.Parse()
	backendChan := make(chan map[string][]Backend)
	updateChan := make(chan time.Time)

	statusUrl, _ := url.Parse("http://localhost:8079")

	go func() {
		server := statusServer(8079)
		server.ListenAndServe()
	}()

	go func() {
		for {
			select {
			case bs := <-backendChan:
				backends = bs
				rrbBackends = createRoundRobinBackends(backends)
				rrbBackends["localhost:"+strconv.Itoa(*httpPort)], _ = roundrobin.New(forwarder)
				rrbBackends["localhost:"+strconv.Itoa(*httpPort)].UpsertServer(statusUrl)
			}
		}
	}()

	go backendManager(backendChan, updateChan)
	go trackUpdates(updateChan)

	redirect := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		//fmt.Printf("%v %v\n", req.RemoteAddr, req.Host)

		if backend := rrbBackends[req.Host]; backend != nil {
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
