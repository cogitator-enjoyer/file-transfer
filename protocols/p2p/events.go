package events

type EventType uint8

type Event interface {
}

const (
	RequestHandshake EventType = iota
	ResponseRequestHandshake
)

type NetworkEvent struct {
	Type EventType
}
