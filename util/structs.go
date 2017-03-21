package util

import (
	"encoding/json"
	"github.com/vulcand/oxy/roundrobin"
	"net/url"
)

type Config struct {
	HttpPort        int
	IgnoreSSLErrors bool
	InstanceName    string
	Marathon        MarathonConfig
	Domain          string
	ShutdownDelay   int
	UpdateInterval  int
	Backends        map[string][]Backend
	RrbBackends     map[string]*roundrobin.RoundRobin
}

type MarathonConfig struct {
	Auth        string
	LabelPrefix string
	Urls        []string
}

type ShelobStatus struct {
	Name string `json:"name"`
	Up   bool   `json:"up"`
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
