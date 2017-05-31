package handlers

import (
	"encoding/json"
	"github.com/dbcdk/shelob/util"
	"net/http"
	"time"
)

func CreateShelobStatus(config *util.Config) util.ShelobStatus {
	timeSinceUpdate := time.Now().Sub(config.LastUpdate)
	upperLimit := time.Duration(config.AcceptableUpdateLag) * time.Second
	stale := !config.HasBeenUpdated || (config.AcceptableUpdateLag != 0 && timeSinceUpdate > upperLimit)

	ok := true
	ok = ok && !config.State.ShutdownInProgress
	ok = ok && !stale

	return util.ShelobStatus{
		Name:       config.InstanceName,
		Ok:         ok,
		Up:         !config.State.ShutdownInProgress,
		Stale:      stale,
		LastUpdate: config.LastUpdate,
		UpdateLag:  timeSinceUpdate.Seconds(),
	}
}

func CreateStatusHandler(config *util.Config) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		status := CreateShelobStatus(config)
		b, _ := json.Marshal(status)

		if status.Ok {
			w.Write(b)
		} else {
			http.Error(w, string(b), http.StatusServiceUnavailable)
		}
	}
}
