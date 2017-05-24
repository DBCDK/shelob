package handlers

import (
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
)

func MetricsCountRequest(next http.Handler, counters []prometheus.Counter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, counter := range counters {
			counter.Inc()
		}
		next.ServeHTTP(w, r)
	})
}
