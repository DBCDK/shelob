package http

import (
	"net/http"
	"github.com/dbcdk/shelob/handlers"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/dbcdk/shelob/util"
)

func CreateWebMux(config *util.Config, shutdownInProgress *bool) *http.ServeMux {
	mux := http.NewServeMux()

	mux.Handle("/", http.HandlerFunc(handlers.CreateListApplicationsHandler(config)))
	mux.Handle("/api/applications", http.HandlerFunc(handlers.CreateListApplicationsHandlerJson(config)))
	mux.Handle("/status", http.HandlerFunc(handlers.CreateStatusHandler(config, shutdownInProgress)))
	mux.Handle("/metrics", promhttp.Handler())

	return mux
}