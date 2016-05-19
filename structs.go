package main

import (
	"net/url"
	"time"
)

type Frontend struct {
	Backends []Backend
}

type Backend struct {
	url url.URL
}

type Apps struct {
	Apps []App
}

type App struct {
	Id     string
	Labels map[string]string
}

type Tasks struct {
	Tasks []Task
}

type Task struct {
	AppId              string
	Id                 string
	SlaveId            string
	Host               string
	Ports              []int
	IpAddress          string
	HealthCheckResults []HealthCheckResult
	ServicePorts       []int
	StagedAt           *time.Time
	StartedAt          *time.Time
	Version            string
}

type HealthCheckResult struct {
	TaskId              string
	Alive               bool
	FirstSuccess        *time.Time
	LastSuccess         *time.Time
	LastFailure         *time.Time
	ConsecutiveFailures int
}

type IpAddress struct {
	IpAddress string
	Protocol  string
}

type RawEvent struct {
	Event string
	Data  []byte
}

type Event struct {
	EventType string
	Timestamp time.Time
}

type EventStreamAttached struct {
	Event
	RemoteAddress string
}

type EventStreamDetached struct {
	Event
	RemoteAddress string
}

type EventStatusUpdate struct {
	Event
	AppId       string
	Host        string
	IpAddresses []IpAddress
	Message     string
	Ports       []int
	SlaveId     string
	TaskId      string
	TaskStatus  string
	Timestamp   time.Time
	Version     string // could also be time.Time
}

type EventHealthStatusChanged struct {
	Alive   bool
	AppId   string
	TaskId  string
	Version string
}
