package handlers

import (
	"net/http"
	"github.com/prometheus/client_golang/prometheus"
)

func MetricsCountRequest(next http.Handler, counters []prometheus.Counter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, counter := range counters {
			counter.Inc()
		}
		next.ServeHTTP(w, r)
	})
}
