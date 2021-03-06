package kubernetes

import "github.com/dbcdk/shelob/util"

type Object struct {
	Name      string
	Namespace string
}

type HostMatch struct {
	Object   Object
	HostName string
}

type PortMatch struct {
	Object Object
	Port   uint16
}

type Endpoint struct {
	Address string
	Port    uint16
}

type Ingress struct {
	Scheme          string
	Name            string
	Port            uint16
	Intercept       *util.Intercept
	PlainHTTPPolicy uint16
}

type Service struct {
	Port       uint16
	TargetPort uint16
}
