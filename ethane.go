package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	_ "github.com/davecgh/go-spew/spew"
	"github.com/samuel/go-zookeeper/zk"
	"github.com/vulcand/oxy/forward"
	"github.com/vulcand/oxy/roundrobin"
	"github.com/vulcand/oxy/testutils"
	"io/ioutil"
	"net/http"
	"net/url"
	_ "net/url"
	"strconv"
	"strings"
	"time"
	"github.com/davecgh/go-spew/spew"
)

type ProxyConfiguration struct {
	Domains map[string]Frontend
}

type Frontend struct {
	Backends []Backend
}

type Backend struct {
	url url.URL
}

type Apps struct {
	Apps []App
}

type App struct {
	Id     string
	Labels map[string]string
}

type Tasks struct {
	Tasks []Task
}

type Task struct {
	AppId              string
	Id                 string
	SlaveId            string
	Host               string
	Ports              []int
	IpAddresses        []string
	HealthCheckResults []HealthCheckResult
	ServicePorts       []int
	StagedAt           *time.Time
	StartedAt          *time.Time
	Version            string
}

type HealthCheckResult struct {
	TaskId              string
	Alive               bool
	FirstSuccess        *time.Time
	LastSuccess         *time.Time
	LastFailure         *time.Time
	ConsecutiveFailures int
}

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
	proxyConfiguration := ProxyConfiguration{make(map[string]Frontend)}

	println("connecting to zk")
	zkConn, _, err := zk_connect()
	defer zkConn.Close()
	if err != nil {
		return err
	}

	_, _, marathonEventChannel, err := zkConn.ChildrenW("/marathon/state")
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

		//fmt.Printf("zk conn: %+v\n", zkConn)

		select {
		case event := <-marathonEventChannel:
			eventCounter++
			fmt.Printf("#%v: %+v\n", eventCounter, event)
			println('!')
			println(eventCounter)
			println(event.Path)
			if eventCounter == 2 {
				println("ding")
				return nil
			}
			if event.Err != nil {
				println(event.Err)
			} else {
				updateBackends(proxyConfiguration)
			}
		case <-time.After(time.Second * 1):
			fmt.Printf("No changes for a while, forcing reload..\n")
		}

		updateBackends(proxyConfiguration)
		spew.Dump(proxyConfiguration)
	}
}

func updateBackends(config ProxyConfiguration) {
	//resp, err := http.PostForm(
	//	"https://mesos-master-t02:8080/v2/eventSubscriptions",
	//	url.Values{"callbackUrl": {"foo"}})

	apps, appsErr := getApps()
	tasks, tasksErr := getTasks()
	if appsErr != nil {
		println("Error:")
		println(appsErr.Error())
		return
	}
	if tasksErr != nil {
		println("Error:")
		println(tasksErr.Error())
		return
	}

	var groupedApps = groupApps(apps)
	var groupedTasks = groupTasks(tasks)

	var labelPrefix = "expose.port."

	for appId, app := range groupedApps {
		for label, exposedDomain := range app.Labels {
			if strings.HasPrefix(label, labelPrefix) {
				frontend := Frontend{}
				port, err := strconv.Atoi(label[len(labelPrefix):len(label)])
				if err != nil {
					continue
				}
				for _, task := range groupedTasks[appId] {
					url, err := url.Parse(fmt.Sprintf("http://%s:%v", task.Host, task.Ports[port]))
					if err != nil {
						continue
					}


					frontend.Backends = append(frontend.Backends, Backend{*url})

					fmt.Printf("%v -> %v\n", exposedDomain, url)
				}

				config.Domains[exposedDomain] = frontend
			}
		}
	}
}

func getApps() (apps Apps, err error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	req, err := http.NewRequest("GET", "https://mesos-master-t02:8080/v2/apps", nil)
	if err != nil {
		return apps, err
	}

	req.SetBasicAuth("admin", "admin")

	resp, err := client.Do(req)
	if err != nil {
		return apps, err
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return apps, err
	}

	err = json.Unmarshal(body, &apps)
	if err != nil {
		return apps, err
	}

	return apps, nil
}

func getTasks() (tasks Tasks, err error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	req, err := http.NewRequest("GET", "https://mesos-master-t02:8080/v2/tasks", nil)
	if err != nil {
		return tasks, err
	}

	req.SetBasicAuth("admin", "admin")

	resp, err := client.Do(req)
	if err != nil {
		return tasks, err
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return tasks, err
	}

	err = json.Unmarshal(body, &tasks)
	if err != nil {
		return tasks, err
	}

	return tasks, nil
}

func groupApps(apps Apps) (groupedApps map[string]App) {
	groupedApps = make(map[string]App)

	for _, app := range apps.Apps {
		groupedApps[app.Id] = app
	}
	return groupedApps
}

func groupTasks(tasks Tasks) (groupedTasks map[string][]Task) {
	groupedTasks = make(map[string][]Task)

	for _, task := range tasks.Tasks {
		if groupedTasks[task.Id] == nil {
			groupedTasks[task.Id] = []Task{}
		}
		groupedTasks[task.AppId] = append(groupedTasks[task.AppId], task)
		//println()
		//println(task.AppId)
		//spew.Dump(groupedTasks[task.AppId])
	}

	return groupedTasks
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

	print_backends(backends)

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
