package proxy

import (
	"strings"
	"strconv"
	"net/http"
	httputil "github.com/dbcdk/shelob/http"
	"time"
	"github.com/dbcdk/shelob/util"
	"github.com/dbcdk/shelob/logging"
	"go.uber.org/zap"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vulcand/oxy/forward"
	"github.com/viki-org/dnscache"
	"net"
	"context"
	"github.com/vulcand/oxy/roundrobin"
)

func routeToSelf(req *http.Request, config *util.Config) bool {
	return (req.Host == "localhost") || (req.Host == config.Domain)
}

func RedirectHandler(config *util.Config) http.Handler {
	webMux := httputil.CreateWebMux(config)

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		t__start := time.Now().UnixNano()
		domain := util.StripPortFromDomain(req.Host)
		status := http.StatusOK
		request_type := "internal"

		tooManyXForwardedHostHeaders := false

		if xForwardedHostHeader, ok := req.Header["X-Forwarded-Host"]; ok {
			// The XFH-header must not be repeated
			if len(xForwardedHostHeader) == 1 {
				xForwardedHost := xForwardedHostHeader[0]
				// .. but it can contain a list of hosts. Pick the first one in the list, if that's the case
				if strings.Contains(xForwardedHost, ",") {
					parts := strings.Split(xForwardedHost, ",")
					xForwardedHost = strings.TrimSpace(parts[0])
				}

				req.Host = xForwardedHost
			} else {
				tooManyXForwardedHostHeaders = true
			}
			delete(req.Header, "X-Forwarded-Host")
		}

		if tooManyXForwardedHostHeaders {
			status = http.StatusBadRequest
			http.Error(w, "X-Forwarded-Host must not be repeated", status)
		} else if routeToSelf(req, config) {
			webMux.ServeHTTP(w, req)
		} else if backend := config.RrbBackends[domain]; backend != nil {
			request_type = "proxy"
			backend.ServeHTTP(w, req)
		} else {
			status = http.StatusNotFound
			http.Error(w, http.StatusText(status), status)
		}

		duration := float64(time.Now().UnixNano()-t__start) / 1000000

		promLabels := prometheus.Labels{
			"domain": domain,
			"code":   strconv.Itoa(status),
			"method": req.Method,
			"type":   request_type,
		}
		config.Counters.Requests.With(promLabels).Inc()

		if config.Logging.AccessLog {
			logging.GetInstance().Info("request",
				zap.String("event", "request"),
				zap.Any("request", map[string]interface{}{
					"duration": duration,
					"user": map[string]interface{}{
						"addr":  req.RemoteAddr,
						"agent": req.UserAgent(),
					},
					"domain":   domain,
					"method":   req.Method,
					"protocol": req.Proto,
					"status":   status,
					"url":      req.URL.String(),
				}),
			)

		}
	})

}

func CreateForwarder() *forward.Forwarder {
	resolver := dnscache.New(time.Minute * 1)

	dialContextFn := func(ctx context.Context, network string, address string) (net.Conn, error) {
		separator := strings.LastIndex(address, ":")
		ip, _ := resolver.FetchOneString(address[:separator])
		dialer := &net.Dialer{
			Timeout: 1 * time.Second,
		}

		return dialer.DialContext(ctx, network, ip+address[separator:])
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConnsPerHost:   10,
		MaxIdleConns:          100,
		IdleConnTimeout:       5 * time.Second,
		TLSHandshakeTimeout:   2 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DialContext:           dialContextFn,
	}

	forwarder, err := forward.New(forward.PassHostHeader(true), forward.RoundTripper(transport))

	if err != nil {
		panic(err)
	}

	return forwarder
}

func CreateRoundRobinBackends(forwarder *forward.Forwarder, backends map[string][]util.Backend) map[string]*roundrobin.RoundRobin {
	rrbBackends := make(map[string]*roundrobin.RoundRobin)

	for domain, backendList := range backends {
		rrbBackends[domain], _ = roundrobin.New(forwarder)

		for _, backend := range backendList {
			rrbBackends[domain].UpsertServer(backend.Url)
		}
	}

	return rrbBackends
}
