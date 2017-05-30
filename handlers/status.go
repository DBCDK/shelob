package handlers

import (
	"encoding/json"
	"github.com/dbcdk/shelob/util"
	"net/http"
	"time"
)

func CreateShelobStatus(config *util.Config) util.ShelobStatus {
	timeSinceUpdate := time.Now().Sub(config.LastUpdate)
	upperLimit := time.Duration(2*config.UpdateInterval) * time.Second
	updateOk := timeSinceUpdate < upperLimit
	up := !config.State.ShutdownInProgress && updateOk

	return util.ShelobStatus{
		Name:       config.InstanceName,
		Up:         up,
		LastUpdate: config.LastUpdate,
		UpdateLag:  timeSinceUpdate.Seconds(),
	}
}

func CreateStatusHandler(config *util.Config) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		status := CreateShelobStatus(config)
		b, _ := json.Marshal(status)

		if status.Up {
			w.Write(b)
		} else {
			http.Error(w, string(b), http.StatusServiceUnavailable)
		}
	}
}
