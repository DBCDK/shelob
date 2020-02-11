package handlers

import (
	"encoding/json"
	"github.com/dbcdk/shelob/util"
	"html/template"
	"net/http"
	"strings"
)

func CreateListApplicationsHandler(config *util.Config) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		data := make(map[string][]util.Backend)
		port := "80"

		if strings.Contains(r.Host, ":") {
			port = strings.SplitN(r.Host, ":", 2)[1]
		}

		for domain, frontend := range config.Frontends {
			if port != "80" {
				domain = domain + ":" + port
			}

			data[domain] = frontend.Backends
		}

		var page = `
<!DOCTYPE html>
<html>
	<head>
		<meta charset="UTF-8">
		<title>{{.Domain}}</title>
	</head>
	<body>
		<h1>Available applications:</h1>
		<ul>
			{{range $domain, $backends := . }}<li><a href="http://{{ $domain }}">{{ $domain }}</a></li>
			{{ end }}
		</ul>
	</body>
</html>`

		t, err := template.New("t").Parse(page)
		if err != nil {
			panic(err)
		}

		err = t.Execute(w, data)

		if err != nil {
			panic(err)
		}
	}
}

func CreateListApplicationsHandlerJson(config *util.Config) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		json, err := json.Marshal(config.Frontends)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(json)
	}
}
