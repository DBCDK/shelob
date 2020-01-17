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
	Backends            map[string][]BackendInterface
	RrbBackends         map[string]*roundrobin.RoundRobin
	RedirectBackends    map[string]*Redirect
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

type Redirect struct {
	Url  *url.URL
	Code uint16
}

type BackendInterface interface {
	Proxy() *Backend
	Redirect() *Redirect
}

func (b Backend) Proxy() *Backend {
	return &b
}

func (r Redirect) Proxy() *Backend {
	return nil
}

func (b Backend) Redirect() *Redirect {
	return nil
}

func (r Redirect) Redirect() *Redirect {
	return &r
}

// convert url to string when serializing
func (backend Backend) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Url string `json:"url"`
	}{
		Url: backend.Url.String(),
	})
}
