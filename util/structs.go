package util

import (
	"encoding/json"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vulcand/oxy/roundrobin"
	"k8s.io/client-go/rest"
	"net/url"
	"time"
)

type Config struct {
	HttpPort            int
	MetricsPort         int
	ReuseHttpPort       bool
	IgnoreSSLErrors     bool
	InstanceName        string
	Domain              string
	ShutdownDelay       int
	ReloadEvery         int
	ReloadRollup        int
	AcceptableUpdateLag int
	Backends            map[string][]Backend
	RrbBackends         map[string]*roundrobin.RoundRobin
	Logging             Logging
	State               State
	Counters            Counters
	LastUpdate          time.Time
	HasBeenUpdated      bool
	Kubeconfig          *rest.Config
	DisableWatch        bool
	IgnoreNamespaces	map[string]bool
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

type ShelobStatus struct {
	Name       string    `json:"name"`
	Ok         bool      `json:"ok"`
	Up         bool      `json:"up"`
	Stale      bool      `json:"stale"`
	LastUpdate time.Time `json:"lastUpdate"`
	UpdateLag  float64   `json:"updateLag"`
}

type Frontend struct {
	Backends []Backend
}

type Reload struct {
	Time   time.Time
	Reason string
}

func NewReload(reason string) Reload {
	return Reload{
		Time:   time.Now(),
		Reason: reason,
	}
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
