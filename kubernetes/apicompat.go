package kubernetes

import (
	v1beta1 "k8s.io/api/extensions/v1beta1"
	networkingv1 "k8s.io/api/networking/v1"
)

type IngressCompat struct {
	v1beta1 *v1beta1.Ingress
	v1      *networkingv1.Ingress
}

type IngressRuleCompat struct {
	v1beta1 *v1beta1.IngressRule
	v1      *networkingv1.IngressRule
}

type HTTPIngressRuleValueCompat struct {
	v1beta1 *v1beta1.HTTPIngressRuleValue
	v1      *networkingv1.HTTPIngressRuleValue
}

type HTTPIngressPathCompat struct {
	v1beta1 *v1beta1.HTTPIngressPath
	v1      *networkingv1.HTTPIngressPath
}

type IngressBackendCompat struct {
	v1beta1 *v1beta1.IngressBackend
	v1      *networkingv1.IngressBackend
}

func (i IngressCompat) getRules() []IngressRuleCompat {
	var rules = make([]IngressRuleCompat, 0)
	if i.v1beta1 != nil {
		for _, r := range i.v1beta1.Spec.Rules {
			rules = append(rules, IngressRuleCompat{
				v1beta1: &r,
			})
		}
	}
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
	if i.v1beta1 != nil {
		return i.v1beta1.Name
	}
	if i.v1 != nil {
		return i.v1.Name
	}
	return ""
}

func (i IngressCompat) Namespace() string {
	if i.v1beta1 != nil {
		return i.v1beta1.Namespace
	}
	if i.v1 != nil {
		return i.v1.Namespace
	}
	return ""
}

func (r IngressRuleCompat) Host() string {
	if r.v1beta1 != nil {
		return r.v1beta1.Host
	}
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
	if i.v1beta1 != nil {
		value, present = i.v1beta1.Annotations[name]
	}
	if i.v1 != nil {
		value, present = i.v1.Annotations[name]
	}
	return
}

func (r IngressRuleCompat) Http() *HTTPIngressRuleValueCompat {
	if r.v1beta1 != nil && r.v1beta1.HTTP != nil {
		return &HTTPIngressRuleValueCompat{
			v1beta1: r.v1beta1.HTTP,
		}
	}
	if r.v1 != nil && r.v1.HTTP != nil {
		return &HTTPIngressRuleValueCompat{
			v1: r.v1.HTTP,
		}
	}
	return nil
}

func (r *HTTPIngressRuleValueCompat) Paths() []HTTPIngressPathCompat {
	var paths = make([]HTTPIngressPathCompat, 0)
	if r.v1beta1 != nil {
		for _, p := range r.v1beta1.Paths {
			paths = append(paths, HTTPIngressPathCompat{
				v1beta1: &p,
			})
		}
	}
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
	if p.v1beta1 != nil {
		return p.v1beta1.Path
	}
	if p.v1 != nil {
		return p.v1.Path
	}
	return ""
}

func (b IngressBackendCompat) ServiceName() string {
	if b.v1beta1 != nil {
		return b.v1beta1.ServiceName
	}
	if b.v1 != nil && b.v1.Service != nil {
		return b.v1.Service.Name
	}
	return ""
}

func (b IngressBackendCompat) ServicePort() int {
	if b.v1beta1 != nil {
		return b.v1beta1.ServicePort.IntValue()
	}
	if b.v1 != nil && b.v1.Service != nil {
		return int(b.v1.Service.Port.Number)
	}
	return 0
}

func (p HTTPIngressPathCompat) Backend() *IngressBackendCompat {
	if p.v1beta1 != nil {
		return &IngressBackendCompat{
			v1beta1: &p.v1beta1.Backend,
		}
	}
	if p.v1 != nil {
		return &IngressBackendCompat{
			v1: &p.v1.Backend,
		}
	}
	return nil
}
