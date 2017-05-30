package mux

import (
	"github.com/dbcdk/shelob/handlers"
	"github.com/dbcdk/shelob/util"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
)

func CreateWebMux(config *util.Config) *http.ServeMux {
	mux := http.NewServeMux()

	mux.Handle("/", http.HandlerFunc(handlers.CreateListApplicationsHandler(config)))
	mux.Handle("/api/applications", http.HandlerFunc(handlers.CreateListApplicationsHandlerJson(config)))
	mux.Handle("/status", http.HandlerFunc(handlers.CreateStatusHandler(config)))
	mux.Handle("/metrics", promhttp.Handler())

	return mux
}
