package main

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/vulcand/oxy/forward"
	"github.com/vulcand/oxy/roundrobin"
	"gopkg.in/alecthomas/kingpin.v2"
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
	forwarder, _   = forward.New()
	backends       = make(map[string]*roundrobin.RoundRobin)
)

func backendManager(backendChan chan map[string]*roundrobin.RoundRobin, updateChan chan time.Time) error {
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
	spew.Fprint(w, backends)
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
	backendChan := make(chan map[string]*roundrobin.RoundRobin)
	updateChan := make(chan time.Time)

	go func() {
		server := statusServer(8079)
		server.ListenAndServe()
	}()

	go func() {
		for {
			select {
			case bs := <-backendChan:
				backends = bs
				//backends["localhost"] =
			}
		}
	}()

	go backendManager(backendChan, updateChan)
	go trackUpdates(updateChan)

	redirect := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		//fmt.Printf("%v %v\n", req.RemoteAddr, req.Host)

		if backend := backends[req.Host]; backend != nil {
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
	s.ListenAndServe()
}
