package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func trackUpdates(updateChan chan time.Time) {
	marathonEventChan := make(chan RawEvent)
	go func() {
		select {
		case rawEvent := <-marathonEventChan:
			updateChan <- time.Now()
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

	}()

	for {
		err := doTrackUpdates(marathonEventChan)
		if err != nil {
			println("Error:")
			println(err.Error())
		}
		time.Sleep(time.Second)
	}
}

func doTrackUpdates(marathonEventChan chan RawEvent) error {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: *insecureSSL},
	}
	client := &http.Client{Transport: tr}

	req, err := http.NewRequest("GET", (*marathons)[0]+"/v2/events", nil)
	if err != nil {
		return err
	}

	// todo: validate the presence of ':' to avoid segfaults
	if len(*marathonAuth) > 0 {
		auth := strings.SplitN(*marathonAuth, ":", 2)
		req.SetBasicAuth(auth[0], auth[1])
	}

	req.Header.Set("Accept", "text/event-stream")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	event := RawEvent{}
	eventReader := bufio.NewReader(resp.Body)

	eventId := 0

	for {
		line, err := eventReader.ReadBytes('\n')
		if err != nil {
			return err
		}

		fmt.Printf("Event %v (%v bytes): %v\n", eventId, len(line), string(line))

		switch {
		case bytes.HasPrefix(line, []byte("event:")):
			event.Event = strings.TrimSpace(string(line[7:]))
		case bytes.HasPrefix(line, []byte("data:")):
			event.Data = line[6:]
		case bytes.Equal(line, []byte("\r\n")):
			if len(event.Event) > 0 && len(event.Data) > 0 {
				marathonEventChan <- event
			} else {
			}
			event = RawEvent{}
		default:
			println("Ignored event: " + string(line))
		}

		eventId += 1
	}
}

func updateBackends() (map[string][]Backend, error) {
	backends := make(map[string][]Backend)

	apps, appsErr := getApps()
	tasks, tasksErr := getTasks()
	if appsErr != nil {
		return nil, appsErr
	}
	if tasksErr != nil {
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
				appDomain := strconv.Itoa(portIndex) + "." + domain
				if _, ok := backends[appDomain]; !ok {
					backends[appDomain] = make([]Backend, 0)
				}

				url, err := url.Parse(fmt.Sprintf("http://%s:%v", task.Host, actualPort))
				if err != nil {
					continue
				}

				backends[appDomain] = append(backends[appDomain], Backend{Url: url})
				//fmt.Printf("%v -> %v\n", domainWithPort, url)
			}
		}

		// create custom domain mappings
		for label, exposedDomain := range app.Labels {
			if strings.HasPrefix(label, labelPrefix) {
				port, err := strconv.Atoi(label[len(labelPrefix):len(label)])
				if err != nil {
					continue
				}
				for _, task := range indexedTasks[appId] {
					if (port+1 > len(task.Ports)) {
						fmt.Printf("Ignoring illegal port-index %v for app %s\n", port, task.AppId)
						continue
					}

					url, err := url.Parse(fmt.Sprintf("http://%s:%v", task.Host, task.Ports[port]))
					if err != nil {
						continue
					}

					if _, ok := backends[exposedDomain]; !ok {
						backends[exposedDomain] = make([]Backend, 0)
					}

					backends[exposedDomain] = append(backends[exposedDomain], Backend{Url: url})
					//fmt.Printf("%v -> %v\n", exposedDomain, url)
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
	if len(*marathonAuth) > 0 {
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
	if len(*marathonAuth) > 0 {
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
