package marathon

import (
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
)

var (
	log = logging.GetInstance()
)

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
	var indexedTasks = indexHealthyTasks(tasks)
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

func indexHealthyTasks(tasks Tasks) (indexedTasks map[string][]Task) {
	indexedTasks = make(map[string][]Task)

	for _, task := range tasks.Tasks {
		if indexedTasks[task.Id] == nil {
			indexedTasks[task.Id] = []Task{}
		}
		// Skip non healthy Tasks
		if len(task.HealthCheckResults) > 0 && task.HealthCheckResults[0].Alive {
			indexedTasks[task.AppId] = append(indexedTasks[task.AppId], task)
		}
	}
	return indexedTasks
}
