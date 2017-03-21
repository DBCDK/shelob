package handlers

import (
	"encoding/json"
	"github.com/dbcdk/shelob/util"
	"net/http"
)

func CreateStatusHandler(config *util.Config, shutdownInProgress *bool) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if *shutdownInProgress {
			b, _ := json.Marshal(util.ShelobStatus{Name: config.InstanceName, Up: false})
			http.Error(w, string(b), http.StatusServiceUnavailable)
		} else {
			b, _ := json.Marshal(util.ShelobStatus{Name: config.InstanceName, Up: true})
			w.Write(b)
		}
	}
}
