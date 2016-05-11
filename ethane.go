package main

import (
	"errors"
	"fmt"
	"github.com/samuel/go-zookeeper/zk"
	"github.com/vulcand/oxy/forward"
	"github.com/vulcand/oxy/roundrobin"
	"github.com/vulcand/oxy/testutils"
	"net/http"
	"time"
)

func print_backends(backends map[string]*roundrobin.RoundRobin) {
	for hostname, rrb := range backends {
		fmt.Printf("backends for %v\n", hostname)
		for _, backend := range rrb.Servers() {
			println(backend.Host)
		}
	}
}

func zk_connect() (*zk.Conn, <-chan zk.Event, error) {
	//return zk.Connect([]string{"mesos-master-p01"}, time.Second)
	return zk.Connect([]string{"mesos-master-t01"}, time.Second)
}

func bar() {
	for {
		err := foo()
		if err != nil {
			println(err.Error())
		}
		time.Sleep(time.Second)
	}
}

func foo() error {
	println("connecting to zk")
	zkConn, _, err := zk_connect() //*10)
	defer zkConn.Close()
	if err != nil {
		return err
	}

	//_, _, marathonEventChannel, err := zkConn.ChildrenW("/marathon/state")
	_, _, marathonEventChannel, err := zkConn.ChildrenW("/")
	if err != nil {
		return err
	}

	eventCounter := 0

	for {
		switch zkConn.State() {
		case zk.StateUnknown:
			return errors.New("zk state unknown")
		case zk.StateDisconnected:
			return errors.New("zk state disconnected")
		case zk.StateAuthFailed:
			return errors.New("zk state auth failed")
		case zk.StateConnected:
			return errors.New("zk state connected")
		}

		println(zkConn.State())

		select {
		case event := <-marathonEventChannel:
			eventCounter++
			fmt.Printf("#%v: %+v\n", eventCounter, event)
			if eventCounter == 2 {
				println("ding")
				return nil
			}
			if event.Err != nil {
				println(event.Err)
			} else {
				updateBackends()
			}
		case <-time.After(time.Second * 5):
			fmt.Printf("No changes for a while, forcing reload..\n")
		}

		updateBackends()
	}
}

func updateBackends() {

}

func main() {
	// Forwards incoming requests to whatever location URL points to, adds proper forwarding headers
	fwd, _ := forward.New()

	backends := make(map[string]*roundrobin.RoundRobin)
	backends["localhost:8080"], _ = roundrobin.New(fwd)
	backends["mesos.localhost:8080"], _ = roundrobin.New(fwd)
	backends["marathon.localhost:8080"], _ = roundrobin.New(fwd)

	backends["localhost:8080"].UpsertServer(testutils.ParseURI("http://mesos-agent-p01.dbc.dk:31915/"))
	backends["localhost:8080"].UpsertServer(testutils.ParseURI("http://mesos-agent-p06.dbc.dk:31236"))
	backends["mesos.localhost:8080"].UpsertServer(testutils.ParseURI("http://mesos-master-p01:5050"))
	backends["mesos.localhost:8080"].UpsertServer(testutils.ParseURI("http://mesos-master-p02:5050"))
	backends["mesos.localhost:8080"].UpsertServer(testutils.ParseURI("http://mesos-master-p03:5050"))
	backends["marathon.localhost:8080"].UpsertServer(testutils.ParseURI("http://mesos-master-p01:8080"))
	backends["marathon.localhost:8080"].UpsertServer(testutils.ParseURI("http://mesos-master-p02:8080"))
	backends["marathon.localhost:8080"].UpsertServer(testutils.ParseURI("http://mesos-master-p03:8080"))

	//print_backends(backends)

	go bar()

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
		Addr:    ":8080",
		Handler: redirect,
	}
	s.ListenAndServe()
}
