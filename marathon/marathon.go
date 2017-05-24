package marathon

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/dbcdk/shelob/logging"
	"github.com/dbcdk/shelob/util"
	"go.uber.org/zap"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	log = logging.GetInstance()
)

func trackUpdates(config *util.Config, updateChan chan time.Time) {
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

		case <-time.After(time.Second * time.Duration(config.UpdateInterval)):
			log.Info("No changes for a while, forcing reload",
				zap.String("event", "reload"),
			)
		}

	}()

	for {
		err := doTrackUpdates(config, marathonEventChan)
		if err != nil {
			println("Error:")
			println(err.Error())
		}
		time.Sleep(time.Second)
	}
}

func doTrackUpdates(config *util.Config, marathonEventChan chan RawEvent) error {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: config.IgnoreSSLErrors},
	}
	client := &http.Client{Transport: tr}

	req, err := http.NewRequest("GET", (config.Marathon.Urls)[0]+"/v2/events", nil)
	if err != nil {
		return err
	}

	// todo: validate the presence of ':' to avoid segfaults
	if len(config.Marathon.Auth) > 0 {
		auth := strings.SplitN(config.Marathon.Auth, ":", 2)
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

		log.Info("marathon event",
			zap.String("event", "marathonEvent"),
			zap.Any("marathon", map[string]interface{}{
				"eventId": eventId,
				"event":   string(line),
			}),
		)

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
			log.Error("ignored marathon event",
				zap.String("event", "marathonEvent"),
				zap.Any("marathon", map[string]interface{}{
					"ignored": eventId,
				}),
			)

		}

		eventId += 1
	}
}

func UpdateBackends(config *util.Config) (map[string][]util.Backend, error) {
	backends := make(map[string][]util.Backend)

	apps, appsErr := getApps(config)
	tasks, tasksErr := getTasks(config)
	if appsErr != nil {
		return nil, appsErr
	}
	if tasksErr != nil {
		return nil, tasksErr
	}

	var indexedApps = indexApps(apps)
	var indexedTasks = indexTasks(tasks)
	var labelPrefix = config.Marathon.LabelPrefix + ".port."

	for appId, app := range indexedApps {
		if config.Domain != "" {
			// create default domain mappings
			// appId /foo/bar -> $portId.bar.foo.$defaultDomain
			appIdParts := strings.Split(appId[1:], "/")
			reversedAppIdParts := util.ReverseStringArray(appIdParts)
			reversedAppIdParts = append(reversedAppIdParts, config.Domain)
			domain := strings.Join(reversedAppIdParts, ".")
			for _, task := range indexedTasks[appId] {
				for portIndex, actualPort := range task.Ports {
					appDomain := strconv.Itoa(portIndex) + "." + domain
					if _, ok := backends[appDomain]; !ok {
						backends[appDomain] = make([]util.Backend, 0)
					}

					url, err := url.Parse(fmt.Sprintf("http://%s:%v", task.Host, actualPort))
					if err != nil {
						continue
					}

					backends[appDomain] = append(backends[appDomain], util.Backend{Url: url})
					//fmt.Printf("%v -> %v\n", domainWithPort, url)
				}
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
					if port+1 > len(task.Ports) {
						log.Info("illegal port-index",
							zap.String("event", "invalidMarathonApp"),
							zap.Any("marathon", map[string]interface{}{
								"appId":         appId,
								"lastPort":      len(task.Ports) - 1,
								"requestedPort": port,
							}),
						)

						continue
					}

					url, err := url.Parse(fmt.Sprintf("http://%s:%v", task.Host, task.Ports[port]))
					if err != nil {
						continue
					}

					if _, ok := backends[exposedDomain]; !ok {
						backends[exposedDomain] = make([]util.Backend, 0)
					}

					backends[exposedDomain] = append(backends[exposedDomain], util.Backend{Url: url})
					//fmt.Printf("%v -> %v\n", exposedDomain, url)
				}
			}
		}
	}

	return backends, nil
}

func getApps(config *util.Config) (apps Apps, err error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: config.IgnoreSSLErrors},
	}
	client := &http.Client{Transport: tr}

	req, err := http.NewRequest("GET", (config.Marathon.Urls)[0]+"/v2/apps", nil)
	if err != nil {
		return apps, err
	}

	// todo: validate the presence of ':' to avoid segfaults
	if len(config.Marathon.Auth) > 0 {
		auth := strings.SplitN(config.Marathon.Auth, ":", 2)
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

func getTasks(config *util.Config) (tasks Tasks, err error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	req, err := http.NewRequest("GET", (config.Marathon.Urls)[0]+"/v2/tasks?status=running", nil)
	if err != nil {
		return tasks, err
	}

	// todo: validate the presence of ':' to avoid segfaults
	if len(config.Marathon.Auth) > 0 {
		auth := strings.SplitN(config.Marathon.Auth, ":", 2)
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
