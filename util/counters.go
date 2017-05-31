package util

import "github.com/prometheus/client_golang/prometheus"

func CreateCounters() Counters {
	request_counter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_server_requests_total",
		Help: "Total number of http requests",
	}, []string{"domain", "code", "method", "type"})
	reload_counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "shelob_reloads_total",
		Help: "Number of times the service definitions have been reloaded",
	})
	last_update_gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "shelob_last_update_epoch",
		Help: "Unix time/epoch of last successful backend update",
	})

	return Counters{
		Requests:   *request_counter,
		Reloads:    reload_counter,
		LastUpdate: last_update_gauge,
	}
}

func CreateAndRegisterCounters() Counters {
	counters := CreateCounters()
	prometheus.MustRegister(counters.Requests, counters.Reloads, counters.LastUpdate)

	return counters
}
