package kubernetes

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"time"

	"github.com/dbcdk/shelob/util"
	"github.com/vulcand/oxy/forward"
	"go.uber.org/zap"
	apicorev1 "k8s.io/api/core/v1"
	machinerymetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	clientnetworkingv1 "k8s.io/client-go/kubernetes/typed/networking/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

const (
	REDIRECT_URL_ANNOTATION      = "shelob.redirect.url"
	REDIRECT_CODE_ANNOTATION     = "shelob.redirect.code"
	RESPONSE_CODE_ANNOTATION     = "shelob.response.code"
	RESPONSE_TEXT_ANNOTATION     = "shelob.response.text"
	PLAIN_HTTP_POLICY_ANNOTATION = "shelob.plain.http.policy"
)

func UpdateFrontends(config *util.Config) (map[string]*util.Frontend, error) {

	clients, err := GetKubeClient(config.Kubeconfig)
	if err != nil {
		return nil, err
	}

	v1ingresses, err := getIngressesv1(clients.NetworkingV1().Ingresses(""))
	if err != nil {
		return nil, err
	}

	var ingresses = make(map[HostMatch]Ingress)
	for k, v := range v1ingresses {
		ingresses[k] = v
	}

	services, err := getServices(clients.CoreV1().Services(""))
	if err != nil {
		return nil, err
	}

	endpoints, err := getEndpoints(clients.CoreV1().Endpoints(""))
	if err != nil {
		return nil, err
	}

	return mergeFrontends(config.Forwarder, ingresses, services, endpoints), nil
}

func mergeFrontends(forwarder *forward.Forwarder, ingresses map[HostMatch]Ingress, services map[PortMatch]Service, endpoints map[Object][]Endpoint) map[string]*util.Frontend {
	frontends := make(map[string]*util.Frontend)
	for n, i := range ingresses {
		if i.Intercept != nil {
			frontends[n.HostName] = &util.Frontend{
				Action:          i.Intercept.Action,
				PlainHTTPPolicy: i.PlainHTTPPolicy,
				Intercept:       i.Intercept,
				Backends:        []util.Backend{},
				RR:              nil,
			}
		} else {
			backends := toBackendList(i.Scheme, services[PortMatch{Object: n.Object, Port: i.Port}], endpoints[n.Object])
			frontends[n.HostName] = &util.Frontend{
				Action:          util.BACKEND_ACTION_PROXY_RR,
				PlainHTTPPolicy: i.PlainHTTPPolicy,
				Intercept:       nil,
				Backends:        backends,
				RR:              util.CreateRR(forwarder, backends),
			}
		}
	}

	return frontends
}

func toBackendList(scheme string, service Service, endpoints []Endpoint) []util.Backend {
	backends := make([]util.Backend, 0)
	for _, e := range endpoints {
		if e.Port == service.TargetPort {
			backends = append(backends, util.Backend{
				Url: &url.URL{
					Scheme: scheme,
					Host:   e.Address + ":" + strconv.FormatInt(int64(e.Port), 10),
				},
			})
		}
	}

	return backends
}

func getServices(client clientcorev1.ServiceInterface) (map[PortMatch]Service, error) {
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	services, err := client.List(ctx, machinerymetav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	out := make(map[PortMatch]Service)
	for _, s := range services.Items {
		for n, ss := range mapService(s) {
			out[n] = ss
		}
	}

	return out, nil
}

func mapService(service apicorev1.Service) map[PortMatch]Service {
	out := make(map[PortMatch]Service)
	for _, s := range service.Spec.Ports {
		var sourcePort, targetPort uint16
		sourcePort, _ = i32toPort(s.Port)
		if _, err := i32toPort(s.TargetPort.IntVal); err == nil {
			targetPort, _ = i32toPort(s.TargetPort.IntVal)
		} else {
			targetPort = sourcePort
		}

		if sourcePort > 0 && targetPort > 0 {
			out[PortMatch{
				Object: Object{
					Name:      service.Name,
					Namespace: service.Namespace,
				},
				Port: sourcePort,
			}] = Service{
				Port:       uint16(s.Port),
				TargetPort: targetPort,
			}
		}
	}

	return out
}

func getEndpoints(client clientcorev1.EndpointsInterface) (map[Object][]Endpoint, error) {
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	endpoints, err := client.List(ctx, machinerymetav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	out := make(map[Object][]Endpoint)
	for _, e := range endpoints.Items {
		out[Object{Name: e.Name, Namespace: e.Namespace}] = mapEndpoint(e)
	}

	return out, nil
}

func getIngressesv1(client clientnetworkingv1.IngressInterface) (map[HostMatch]Ingress, error) {
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	ingresses, err := client.List(ctx, machinerymetav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	out := make(map[HostMatch]Ingress)
	for _, i := range ingresses.Items {
		in := IngressCompat{
			v1:      &i,
		}
		for host, backend := range mapIngress(in) {
			out[HostMatch{
				Object:   Object{Name: backend.Name, Namespace: i.Namespace},
				HostName: host,
			}] = backend
		}
	}

	return out, nil
}

func mapEndpoint(in apicorev1.Endpoints) []Endpoint {
	out := make([]Endpoint, 0)
	for _, s := range in.Subsets {
		for _, p := range s.Ports {
			if p.Protocol == apicorev1.ProtocolTCP {
				if _, err := i32toPort(p.Port); err == nil {
					for _, a := range s.Addresses {
						out = append(out, Endpoint{
							Address: a.IP,
							Port:    uint16(p.Port),
						})
					}
				}
			}
		}
	}

	return out
}

func mapIngress(in IngressCompat) map[string]Ingress {
	out := make(map[string]Ingress)

	for _, r := range in.getRules() {
		//find suitable path target (we only support / for now)
		var backend *Ingress
		if r.Http() != nil {
			for _, p := range r.Http().Paths() {
				if p.Backend() != nil && (p.Path() == "" || p.Path() == "/") {
					backend = mapBackend(in, *p.Backend())
				}
			}
		}

		intercept := mapIntercept(in)
		if r.Host() != "" && intercept != nil {
			out[r.Host()] = Ingress{
				Scheme:          "http",
				Name:            r.Host(),
				Port:            80,
				Intercept:       intercept,
				PlainHTTPPolicy: mapPlainHTTPPolicy(in),
			}
		} else if r.Host() != "" && backend != nil {
			out[r.Host()] = *backend
		} else {
			log.Debug("Ignoring ingress rule with no hostname, suitable backend, catch-all path or rule for /",
				zap.String("name", in.Name()),
				zap.String("namespace", in.Namespace()),
				zap.String("host", r.Host()))
		}
	}

	return out
}

func mapPlainHTTPPolicy(in IngressCompat) uint16 {
	_policy := in.getAnnotation(PLAIN_HTTP_POLICY_ANNOTATION)
	switch _policy {
	case "allow":
		return util.PLAIN_HTTP_ALLOW
	case "redirect":
		return util.PLAIN_HTTP_REDIRECT
	case "reject":
		return util.PLAIN_HTTP_REJECT
	default:
		return util.PLAIN_HTTP_REDIRECT
	}
}

func mapIntercept(in IngressCompat) (data *util.Intercept) {
	data = nil

	if _redirectUrl, redirect := in.getOptionalAnnotation(REDIRECT_URL_ANNOTATION); redirect {
		url, err := url.Parse(_redirectUrl)
		if err == nil {
			_code, err := strconv.ParseInt(in.getAnnotation(REDIRECT_CODE_ANNOTATION), 10, 16)
			var code uint16
			if err == nil && (_code == 301 || _code == 302 || _code == 307) {
				code = uint16(_code)
			} else {
				code = 307 // "307 Temporary Redirect" is the default
			}
			data = &util.Intercept{
				Url:    url,
				Code:   code,
				Action: util.BACKEND_ACTION_REDIRECT,
			}
		}
	} else if _responseCode, response := in.getOptionalAnnotation(RESPONSE_CODE_ANNOTATION); response {
		_code, err := strconv.ParseInt(_responseCode, 10, 16)
		var code uint16
		if err == nil && (_code == 400 || _code == 403 || _code == 404 || _code == 410) {
			code = uint16(_code)
		} else {
			code = 400 // "400 Bad Request" is the default
		}
		data = &util.Intercept{
			Code:         code,
			ResponseText: in.getAnnotation(RESPONSE_TEXT_ANNOTATION),
			Action:       util.BACKEND_ACTION_RESPOND,
		}
	}

	return
}

func mapBackend(in IngressCompat, backend IngressBackendCompat) *Ingress {

	namespace := in.Namespace()

	port := backend.ServicePort()
	if _, err := toPort(port); err != nil {
		log.Warn("Dropping ingress backend with invalid port (hint: port names not supported)",
			zap.String("name", backend.ServiceName()),
			zap.String("namespace", namespace))

		return nil
	}
	return &Ingress{
		Name:            backend.ServiceName(),
		Port:            uint16(port),
		Scheme:          "http",
		PlainHTTPPolicy: mapPlainHTTPPolicy(in),
	}
}

func toPort(port int) (uint16, error) {
	if port > 0 && port < math.MaxUint16 {
		return uint16(port), nil
	} else {
		return 0, fmt.Errorf("int out of range: %d", port)
	}
}

func i32toPort(port int32) (uint16, error) {
	if port > 0 && port < math.MaxUint16 {
		return uint16(port), nil
	} else {
		return 0, fmt.Errorf("int out of range: %d", port)
	}
}
