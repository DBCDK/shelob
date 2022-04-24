package kubernetes

import (
	networkingv1 "k8s.io/api/networking/v1"
)

type IngressCompat struct {
	v1      *networkingv1.Ingress
}

type IngressRuleCompat struct {
	v1      *networkingv1.IngressRule
}

type HTTPIngressRuleValueCompat struct {
	v1      *networkingv1.HTTPIngressRuleValue
}

type HTTPIngressPathCompat struct {
	v1      *networkingv1.HTTPIngressPath
}

type IngressBackendCompat struct {
	v1      *networkingv1.IngressBackend
}

func (i IngressCompat) getRules() []IngressRuleCompat {
	var rules = make([]IngressRuleCompat, 0)
	if i.v1 != nil {
		for _, r := range i.v1.Spec.Rules {
			rules = append(rules, IngressRuleCompat{
				v1: &r,
			})
		}
	}
	return rules
}

func (i IngressCompat) Name() string {
	if i.v1 != nil {
		return i.v1.Name
	}
	return ""
}

func (i IngressCompat) Namespace() string {
	if i.v1 != nil {
		return i.v1.Namespace
	}
	return ""
}

func (r IngressRuleCompat) Host() string {
	if r.v1 != nil {
		return r.v1.Host
	}
	return ""
}

func (i IngressCompat) getAnnotation(name string) (value string) {
	value, _ = i.getOptionalAnnotation(name)
	return
}

func (i IngressCompat) getOptionalAnnotation(name string) (value string, present bool) {
	if i.v1 != nil {
		value, present = i.v1.Annotations[name]
	}
	return
}

func (r IngressRuleCompat) Http() *HTTPIngressRuleValueCompat {
	if r.v1 != nil && r.v1.HTTP != nil {
		return &HTTPIngressRuleValueCompat{
			v1: r.v1.HTTP,
		}
	}
	return nil
}

func (r *HTTPIngressRuleValueCompat) Paths() []HTTPIngressPathCompat {
	var paths = make([]HTTPIngressPathCompat, 0)
	if r.v1 != nil {
		for _, p := range r.v1.Paths {
			paths = append(paths, HTTPIngressPathCompat{
				v1: &p,
			})
		}
	}
	return paths
}

func (p HTTPIngressPathCompat) Path() string {
	if p.v1 != nil {
		return p.v1.Path
	}
	return ""
}

func (b IngressBackendCompat) ServiceName() string {
	if b.v1 != nil && b.v1.Service != nil {
		return b.v1.Service.Name
	}
	return ""
}

func (b IngressBackendCompat) ServicePort() int {
	if b.v1 != nil && b.v1.Service != nil {
		return int(b.v1.Service.Port.Number)
	}
	return 0
}

func (p HTTPIngressPathCompat) Backend() *IngressBackendCompat {
	if p.v1 != nil {
		return &IngressBackendCompat{
			v1: &p.v1.Backend,
		}
	}
	return nil
}
