package mux

import (
	"github.com/dbcdk/shelob/handlers"
	"github.com/dbcdk/shelob/util"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"net/http/pprof"
)

func CreateAdminMux(config *util.Config) *http.ServeMux {
	mux := http.NewServeMux()

	mux.Handle("/", http.HandlerFunc(handlers.CreateListApplicationsHandler(config)))
	mux.Handle("/api/applications", http.HandlerFunc(handlers.CreateListApplicationsHandlerJson(config)))
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)

	return mux
}
