package main

import (
	"fmt"
	"github.com/vulcand/oxy/forward"
	"github.com/vulcand/oxy/roundrobin"
	"gopkg.in/alecthomas/kingpin.v2"
	"net/http"
	"strconv"
	"time"
)

var (
	app          = kingpin.New("timeattack", "Replays http requests").Version("1.0")
	httpPort     = kingpin.Flag("port", "Http port to listen on").Default("8080").Int()
	masterDomain = kingpin.Flag("domain", "All apps will by default be exposed as a subdomain to this domain").Default("localhost").String()
	marathons    = kingpin.Flag("marathon", "url to marathon (repeatable for multiple instances of marathon)").Required().Strings()
	updateInterval = kingpin.Flag("updateInterval", "Force updates this often [s]").Default("5").Int()
	updateTracker = make(chan int)
)

func configManager(config *ProxyConfiguration, backendChan chan map[string]*roundrobin.RoundRobin) error {
	for {
		backendChan <- updateBackends(config)

		select {
		case <-updateTracker:
			fmt.Printf("Update requested..\n")
		case <-time.After(time.Second * time.Duration(*updateInterval)):
			fmt.Printf("No changes for a while, forcing reload..\n")
		}
	}
}

func main() {
	kingpin.Parse()
	fwd, _ := forward.New() // Forwards incoming requests to whatever location URL points to, adds proper forwarding headers
	backends := make(map[string]*roundrobin.RoundRobin)
	backendChan := make(chan map[string]*roundrobin.RoundRobin)

	go func() {
		for {
			select {
			case bs := <-backendChan:
				backends = bs
			}
		}
	}()

	proxyConfiguration := ProxyConfiguration{*httpPort, fwd, backends}

	go configManager(&proxyConfiguration, backendChan)

	redirect := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		fmt.Printf("%v %v\n", req.RemoteAddr, req.Host)

		if backend := backends[req.Host]; backend != nil {
			backend.ServeHTTP(w, req)
		} else {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, "not found")
		}
	})

	s := &http.Server{
		Addr:    ":" + strconv.Itoa(proxyConfiguration.Port),
		Handler: redirect,
	}
	s.ListenAndServe()
}
