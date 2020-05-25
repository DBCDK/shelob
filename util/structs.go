package util

import (
	"encoding/json"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vulcand/oxy/forward"
	"github.com/vulcand/oxy/roundrobin"
	"k8s.io/client-go/rest"
	"net/url"
	"time"
)

type Config struct {
	HttpPort            int
	HttpsPort           int
	MetricsPort         int
	ReuseHttpPort       bool
	IgnoreSSLErrors     bool
	InstanceName        string
	Domain              string
	ShutdownDelay       int
	ReloadEvery         int
	ReloadRollup        int
	AcceptableUpdateLag int
	Frontends           map[string]*Frontend
	Forwarder           *forward.Forwarder
	Logging             Logging
	State               State
	Counters            Counters
	LastUpdate          time.Time
	HasBeenUpdated      bool
	Kubeconfig          *rest.Config
	DisableWatch        bool
	IgnoreNamespaces    map[string]bool
	CertNamespace       string
	WildcardCertPrefix  string
}

type Logging struct {
	AccessLog bool
}

type State struct {
	ShutdownInProgress bool
	ShutdownChan       chan bool
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

const (
	BACKEND_ACTION_SERVE_INTERNAL = iota
	BACKEND_ACTION_PROXY_RR
	BACKEND_ACTION_REDIRECT
	BACKEND_ACTION_RESPOND
)

const (
	PLAIN_HTTP_ALLOW = iota
	PLAIN_HTTP_REDIRECT
	PLAIN_HTTP_REJECT
)

type Frontend struct {
	Action          uint16
	PlainHTTPPolicy uint16
	Intercept       *Intercept
	Backends        []Backend
	RR              *roundrobin.RoundRobin
}

type Backend struct {
	Url *url.URL
}

type Intercept struct {
	Url          *url.URL
	Code         uint16
	ResponseText string
	Action       uint16
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

// convert url to string when serializing
func (backend Backend) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Url string `json:"url"`
	}{
		Url: backend.Url.String(),
	})
}
