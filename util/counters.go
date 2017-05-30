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

	return Counters{
		Requests: *request_counter,
		Reloads:  reload_counter,
	}
}

func CreateAndRegisterCounters() Counters {
	counters := CreateCounters()
	prometheus.MustRegister(counters.Requests, counters.Reloads)

	return counters
}
