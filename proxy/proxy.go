package proxy

import (
	"context"
	"crypto/tls"
	"github.com/dbcdk/shelob/logging"
	"github.com/dbcdk/shelob/mux"
	"github.com/dbcdk/shelob/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/viki-org/dnscache"
	"github.com/vulcand/oxy/forward"
	"github.com/vulcand/oxy/roundrobin"
	"github.com/vulcand/oxy/utils"
	"go.uber.org/zap"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func RedirectHandler(config *util.Config) http.Handler {
	webMux := mux.CreateWebMux(config)

	return http.HandlerFunc(func(plainwriter http.ResponseWriter, req *http.Request) {
		w := utils.NewProxyWriter(plainwriter)
		t__start := time.Now().UnixNano()
		domain := util.StripPortFromDomain(req.Host)
		status := http.StatusOK
		request_type := "unknown"

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
		} else if frontend := config.Frontends[domain]; frontend != nil { // select frontend
			request_type = dispatchRequest(frontend, w, req, config.Forwarder)
		} else {
			// TODO: make internal endpoint serving as explicit frontends -> get rid of this fallback
			// no matching frontends, try serving internally
			request_type = "internal"
			webMux.ServeHTTP(w, req)
		}
		status = w.StatusCode()

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
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	forwarder, err := forward.New(forward.PassHostHeader(true), forward.RoundTripper(transport))

	if err != nil {
		panic(err)
	}

	return forwarder
}

func dispatchRequest(frontend *util.Frontend, w http.ResponseWriter, req *http.Request, forwarder *forward.Forwarder) string {

	// http vs. https
	if req.TLS != nil {
		req.Header.Set("X-Forwarded-Proto", "https")
	} else {
		switch frontend.PlainHTTPPolicy {
		case util.PLAIN_HTTP_ALLOW:
			req.Header.Set("X-Forwarded-Proto", "http")
		case util.PLAIN_HTTP_REDIRECT:
			newUrl := util.UrlClone(req)
			newUrl.Scheme = "https"
			frontend.Redirect = &util.Redirect{
				Url:  newUrl,
				Code: 302,
			}
			frontend.Action = util.BACKEND_ACTION_REDIRECT
		case util.PLAIN_HTTP_REJECT:
			frontend.Action = util.BACKEND_ACTION_REJECT
		}
	}

	switch frontend.Action {
	case util.BACKEND_ACTION_REDIRECT:
		http.Redirect(w, req, frontend.Redirect.Url.String(), int(frontend.Redirect.Code))
	case util.BACKEND_ACTION_PROXY_RR:
		rr := createRRBackends(forwarder, frontend.Backends)
		if len(rr.Servers()) > 0 {
			rr.ServeHTTP(w, req)
		} else {
			status := http.StatusServiceUnavailable
			http.Error(w, http.StatusText(status), status)
		}
	case util.BACKEND_ACTION_REJECT:
		status := http.StatusForbidden
		http.Error(w, http.StatusText(status), status)
	}

	return actionToPrometheusRequestType(frontend.Action)
}

func actionToPrometheusRequestType(a uint16) (request_type string) {
	switch a {
	case util.BACKEND_ACTION_SERVE_INTERNAL:
		request_type = "internal"
	case util.BACKEND_ACTION_REDIRECT:
		request_type = "redirect"
	case util.BACKEND_ACTION_PROXY_RR:
		request_type = "proxy"
	}
	return
}

func createRRBackends(forwarder *forward.Forwarder, backends []util.Backend) *roundrobin.RoundRobin {
	rr, _ := roundrobin.New(forwarder)
	for _, backend := range backends {
		rr.UpsertServer(backend.Url)
	}

	return rr
}
