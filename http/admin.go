package http

import (
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/dbcdk/shelob/util"
	"net/http"
	"net/http/pprof"
)

func CreateAdminMux(config *util.Config) *http.ServeMux {
	mux := http.NewServeMux()

	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)

	return mux
}