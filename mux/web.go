package mux

import (
	"net/http"
	"github.com/dbcdk/shelob/handlers"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/dbcdk/shelob/util"
)

func CreateWebMux(config *util.Config) *http.ServeMux {
	mux := http.NewServeMux()

	mux.Handle("/", http.HandlerFunc(handlers.CreateListApplicationsHandler(config)))
	mux.Handle("/api/applications", http.HandlerFunc(handlers.CreateListApplicationsHandlerJson(config)))
	mux.Handle("/status", http.HandlerFunc(handlers.CreateStatusHandler(config)))
	mux.Handle("/metrics", promhttp.Handler())

	return mux
}