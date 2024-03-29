package util

import (
	"crypto/tls"
	"github.com/vulcand/oxy/forward"
	"github.com/vulcand/oxy/roundrobin"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func ReverseStringArray(array []string) []string {
	for i, j := 0, len(array)-1; i < j; i, j = i+1, j-1 {
		array[i], array[j] = array[j], array[i]
	}

	return array
}

func StripPortFromDomain(domainWithPort string) string {
	if strings.Contains(domainWithPort, ":") {
		domainWithoutPort := strings.Split(domainWithPort, ":")[0]
		return domainWithoutPort
	}

	return domainWithPort
}

func UrlClone(req *http.Request) *url.URL {
	return &url.URL{
		Scheme:     req.URL.Scheme,
		Opaque:     req.URL.Opaque,
		User:       req.URL.User,
		Host:       StripPortFromDomain(req.Host),
		Path:       req.URL.Path,
		RawPath:    req.URL.RawPath,
		ForceQuery: req.URL.ForceQuery,
		RawQuery:   req.URL.RawQuery,
		Fragment:   req.URL.Fragment,
	}
}

func CreateRR(forwarder *forward.Forwarder, backends []Backend) *roundrobin.RoundRobin {
	// randomize the list of backends to try to circumvent slightly biased load towards the beginning of the backend list (at high backend reconcile rates)
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(backends), func(i, j int) { backends[i], backends[j] = backends[j], backends[i] })

	rr, _ := roundrobin.New(forwarder)
	for _, backend := range backends {
		rr.UpsertServer(backend.Url)
	}

	return rr
}

func ParseX509(cert []byte, key []byte) (*tls.Certificate, error) {
	out, err := tls.X509KeyPair(cert, key)
	return &out, err
}
