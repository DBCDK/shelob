package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	_ "github.com/davecgh/go-spew/spew"
	"github.com/vulcand/oxy/forward"
	"github.com/vulcand/oxy/roundrobin"
	"io/ioutil"
	"net/http"
	"net/url"
	_ "net/url"
	"strconv"
	"strings"
	"time"
)

func configManager(config *ProxyConfiguration, backendChan chan map[string]*roundrobin.RoundRobin) error {
	for {
		select {
		case <-time.After(time.Second * 2):
			fmt.Printf("No changes for a while, forcing reload..\n")
		}

		backendChan <- updateBackends(config)

		//for domain, frontend := range config.Domains {
		//	println(domain)
		//	rrb, _ := roundrobin.New(config.Forwarder)
		//	for _, backend := range frontend.Backends {
		//		rrb.UpsertServer(&backend.url)
		//	}
		//
		//	config.Backends[domain] = rrb
		//}
	}
}

func reverseStringArray(array []string) []string {
	for i, j := 0, len(array)-1; i < j; i, j = i+1, j-1 {
		array[i], array[j] = array[j], array[i]
	}

	return array
}

func updateBackends(config *ProxyConfiguration) map[string]*roundrobin.RoundRobin {
	//resp, err := http.PostForm(
	//	"https://mesos-master-t02:8080/v2/eventSubscriptions",
	//	url.Values{"callbackUrl": {"foo"}})

	backends := make(map[string]*roundrobin.RoundRobin)

	defaultDomain := "localhost"

	apps, appsErr := getApps()
	tasks, tasksErr := getTasks()
	if appsErr != nil {
		println("Error:")
		println(appsErr.Error())
		return nil
	}
	if tasksErr != nil {
		println("Error:")
		println(tasksErr.Error())
		return nil
	}

	var indexedApps = indexApps(apps)
	var indexedTasks = indexTasks(tasks)
	var labelPrefix = "expose.port."

	for appId, app := range indexedApps {
		// create default domain mappings
		// appId /foo/bar -> $portId.bar.foo.$defaultDomain
		appIdParts := strings.Split(appId[1:], "/")
		reversedAppIdParts := reverseStringArray(appIdParts)
		reversedAppIdParts = append(reversedAppIdParts, defaultDomain)
		domain := strings.Join(reversedAppIdParts, ".")

		for _, task := range indexedTasks[appId] {
			for portIndex, actualPort := range task.Ports {
				domainWithPort := strconv.Itoa(portIndex) + "." + domain + ":" + strconv.Itoa(config.Port)
				if _, ok := backends[domainWithPort]; !ok {
					backends[domainWithPort], _ = roundrobin.New(config.Forwarder)
				}

				url, err := url.Parse(fmt.Sprintf("http://%s:%v", task.Host, actualPort))
				if err != nil {
					continue
				}

				backends[domainWithPort].UpsertServer(url)
				fmt.Printf("%v -> %v\n", domainWithPort, url)

			}
		}

		// create custom domain mappings
		for label, exposedDomain := range app.Labels {
			if strings.HasPrefix(label, labelPrefix) {
				domainWithPort := exposedDomain + ".localhost:" + strconv.Itoa(config.Port)
				frontend := Frontend{}
				port, err := strconv.Atoi(label[len(labelPrefix):len(label)])
				if err != nil {
					continue
				}
				for _, task := range indexedTasks[appId] {
					url, err := url.Parse(fmt.Sprintf("http://%s:%v", task.Host, task.Ports[port]))
					if err != nil {
						continue
					}

					if _, ok := backends[domainWithPort]; !ok {
						backends[domainWithPort], _ = roundrobin.New(config.Forwarder)
					}

					frontend.Backends = append(frontend.Backends, Backend{*url})

					backends[domainWithPort].UpsertServer(url)
					fmt.Printf("%v -> %v\n", exposedDomain, url)
				}
			}
		}
	}

	return backends
}

func getApps() (apps Apps, err error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	req, err := http.NewRequest("GET", "https://mesos-master-t01:8080/v2/apps", nil)
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

	req, err := http.NewRequest("GET", "https://mesos-master-t01:8080/v2/tasks", nil)
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

func indexApps(apps Apps) (indexedApps map[string]App) {
	indexedApps = make(map[string]App)

	for _, app := range apps.Apps {
		indexedApps[app.Id] = app
	}
	return indexedApps
}

func indexTasks(tasks Tasks) (indexedTasks map[string][]Task) {
	indexedTasks = make(map[string][]Task)

	for _, task := range tasks.Tasks {
		if indexedTasks[task.Id] == nil {
			indexedTasks[task.Id] = []Task{}
		}
		indexedTasks[task.AppId] = append(indexedTasks[task.AppId], task)
	}
	return indexedTasks
}

func main() {
	port := 8080
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

	proxyConfiguration := ProxyConfiguration{port, fwd, backends}

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
