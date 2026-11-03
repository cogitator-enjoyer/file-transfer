package client

import "encoding/json"

type RequestMessage interface {
	request()
}

type EventType uint8

const (
	Discover EventType = iota
	RequestHandshake
	AcknowledgePublicIP
	ResponseRequestHandshake
)

type Message struct {
	Type EventType `json:"type"`
	Data any       `json:"data,omitempty"`
}

type Response struct {
	Type EventType       `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

type RequestHandshakeData struct {
	Endpoint string `json:"endpoint"`
}

type ResponseMessage interface {
	response()
}

type ResponseDiscovery struct {
	Discovered []string `json:"discovered"`
}

type ClientPublicIP struct {
	IP string `json:"ip"`
}

type HandshakeResult struct {
	Success bool `json:"success"`
}

func (r ResponseDiscovery) response() {}
