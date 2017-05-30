package util

import (
	"encoding/json"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vulcand/oxy/roundrobin"
	"net/url"
	"time"
)

type Config struct {
	HttpPort        int
	MetricsPort     int
	ReuseHttpPort   bool
	IgnoreSSLErrors bool
	InstanceName    string
	Marathon        MarathonConfig
	Domain          string
	ShutdownDelay   int
	UpdateInterval  int
	Backends        map[string][]Backend
	RrbBackends     map[string]*roundrobin.RoundRobin
	Logging         Logging
	State           State
	Counters        Counters
	LastUpdate      time.Time
}

type Logging struct {
	AccessLog bool
}

type State struct {
	ShutdownInProgress bool
}

type Counters struct {
	Requests   prometheus.CounterVec
	Reloads    prometheus.Counter
	LastUpdate prometheus.Gauge
}

type MarathonConfig struct {
	Auth        string
	LabelPrefix string
	Urls        []string
}

type ShelobStatus struct {
	Name       string    `json:"name"`
	Up         bool      `json:"up"`
	LastUpdate time.Time `json:"lastUpdate"`
	UpdateLag  float64   `json:"updateLag"`
}

type Frontend struct {
	Backends []Backend
}

type Backend struct {
	Url *url.URL
}

// convert url to string when serializing
func (backend Backend) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Url string `json:"url"`
	}{
		Url: backend.Url.String(),
	})
}
