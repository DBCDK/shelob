package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/vulcand/oxy/roundrobin"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func updateBackends() (map[string]*roundrobin.RoundRobin, error) {
	//resp, err := http.PostForm(
	//	"https://mesos-master-t02:8080/v2/eventSubscriptions",
	//	url.Values{"callbackUrl": {"foo"}})

	backends := make(map[string]*roundrobin.RoundRobin)

	println("get apps")
	apps, appsErr := getApps()
	println("get tasks")
	tasks, tasksErr := getTasks()
	if appsErr != nil {
		println("get apps failed")
		return nil, appsErr
	}
	if tasksErr != nil {
		println("get tasks failed")
		return nil, tasksErr
	}

	var indexedApps = indexApps(apps)
	var indexedTasks = indexTasks(tasks)
	var labelPrefix = "expose.port."

	for appId, app := range indexedApps {
		// create default domain mappings
		// appId /foo/bar -> $portId.bar.foo.$defaultDomain
		appIdParts := strings.Split(appId[1:], "/")
		reversedAppIdParts := reverseStringArray(appIdParts)
		reversedAppIdParts = append(reversedAppIdParts, *masterDomain)
		domain := strings.Join(reversedAppIdParts, ".")

		for _, task := range indexedTasks[appId] {
			for portIndex, actualPort := range task.Ports {
				domainWithPort := strconv.Itoa(portIndex) + "." + domain + ":" + strconv.Itoa(*httpPort)
				if _, ok := backends[domainWithPort]; !ok {
					backends[domainWithPort], _ = roundrobin.New(forwarder)
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
				domainWithPort := exposedDomain + ":" + strconv.Itoa(*httpPort)
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
						backends[domainWithPort], _ = roundrobin.New(forwarder)
					}

					frontend.Backends = append(frontend.Backends, Backend{*url})

					backends[domainWithPort].UpsertServer(url)
					fmt.Printf("%v -> %v\n", exposedDomain, url)
				}
			}
		}
	}

	return backends, nil
}

func getApps() (apps Apps, err error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: *insecureSSL},
	}
	client := &http.Client{Transport: tr}

	req, err := http.NewRequest("GET", (*marathons)[0]+"/v2/apps", nil)
	if err != nil {
		return apps, err
	}

	// todo: validate the presence of ':' to avoid segfaults
	if (len(*marathonAuth) > 0) {
		auth := strings.SplitN(*marathonAuth, ":", 2)
		req.SetBasicAuth(auth[0], auth[1])
	}

	req.Header.Set("Accept", "application/json")

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

	req, err := http.NewRequest("GET", (*marathons)[0]+"/v2/tasks", nil)
	if err != nil {
		return tasks, err
	}

	// todo: validate the presence of ':' to avoid segfaults
	if (len(*marathonAuth) > 0) {
		auth := strings.SplitN(*marathonAuth, ":", 2)
		req.SetBasicAuth(auth[0], auth[1])
	}

	req.Header.Set("Accept", "application/json")

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
