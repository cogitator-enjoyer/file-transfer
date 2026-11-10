package ipc

import "encoding/json"

type CommandType uint8

const (
	CmdDiscover CommandType = iota + 1
	CmdRequestHandshake
	CmdAcknowledgePublicIP
	CmdAnswerHandshake
)

type EventType uint8

const (
	EvDiscoverResult EventType = iota + 1
	EvHandshakeResult
	EvIncomingHandshake
	EvError
	EvPublicIP
)

// what client sends to daemon
type Command struct {
	ID   uint32          `json:"id"`
	Type CommandType     `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// what daemon responds to client
type Event struct {
	ReplyTo uint32    `json:"reply_to,omitempty"`
	Type    EventType `json:"type"`
	Data    any       `json:"data,omitempty"`
}

type RequestHandshakeData struct {
	Endpoint string `json:"endpoint"`
}

type ResponseDiscovery struct {
	Discovered []string `json:"discovered"`
}

type HandshakeResult struct {
	Success bool `json:"success"`
}

type ClientPublicIP struct {
	IP string `json:"ip"`
}
