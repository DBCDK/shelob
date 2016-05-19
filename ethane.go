package main

import (
	"encoding/json"
	"fmt"
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
)

func backendManager(backendChan chan map[string]*roundrobin.RoundRobin, updateChan chan RawEvent) error {
	for {
		backends, err := updateBackends()

		if err != nil {
			println("Error:")
			println(err.Error())
		} else {
			backendChan <- backends
		}

		select {
		case rawEvent := <-updateChan:
			fmt.Printf("Update requested..\n")

			switch rawEvent.Event {
			case "status_update_event":
				var event EventStatusUpdate
				err := json.Unmarshal(rawEvent.Data, &event)
				if err != nil {
					println(err.Error())
					println(string(rawEvent.Data))
				}
			case "health_status_changed_event":
				//var event EventHealthStatusChanged
			default:
				println(rawEvent.Event)
				println(string(rawEvent.Data))
			}

			var event Event
			err := json.Unmarshal(rawEvent.Data, &event)
			if err != nil {
				println(err.Error())
				println(string(rawEvent.Data))
			}

		case <-time.After(time.Second * time.Duration(*updateInterval)):
			fmt.Printf("No changes for a while, forcing reload..\n")
		}
	}
}

func main() {
	kingpin.Parse()
	backends := make(map[string]*roundrobin.RoundRobin)
	backendChan := make(chan map[string]*roundrobin.RoundRobin)
	updateChan := make(chan RawEvent)

	go func() {
		for {
			select {
			case bs := <-backendChan:
				backends = bs
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
