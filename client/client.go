package client

type RequestMessage interface {
	request()
}

type EventType uint8

const (
	Discover EventType = iota
	RequestHandshake
)

type Message struct {
	Type EventType `json:"type"`
	Data any       `json:"data,omitempty"`
}

type RequestHandshakeData struct {
	Addr string
}

type ResponseMessage interface {
	response()
}

type ResponseDiscovery struct {
	Discovered []string `json:"discovered"`
}

func (r ResponseDiscovery) response() {}
