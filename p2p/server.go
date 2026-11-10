package p2p

import (
	"context"
	"encoding/json"
	events "file-transfer/protocols/p2p"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"
	"uuid"

	"github.com/pion/stun"
)

type Server struct {
	socket   *net.UDPConn
	peers    peerHub
	PublicIP string
	messages chan connMessage
	events   chan events.Event
}

type peerState uint8

const (
	Connecting peerState = iota
	Connected
	Failed
	NewPeer
)

type sessionPeer struct {
	state     peerState
	inbox     chan Message
	addr      netip.AddrPort
	responses responseMap
}

func (p *sessionPeer) run() {
	for msg := range p.inbox {
		response, ok := p.responses.get(msg.EventID)
		if !ok {
			fmt.Printf("can't find response channel for peer %v, event type %v, event id %v\n", p.addr, msg.Type, msg.EventID)
		}
		if msg.Type == RES_ACKNL {
			p.state = Connected
			fmt.Printf("peer with addr %v is connected!\n", p.addr.Addr().String())
		}
		response <- Response{
			message: msg,
		}
	}
}

type peerHub struct {
	mu   *sync.Mutex
	pmap map[netip.AddrPort]*sessionPeer
}

func (h peerHub) getOrCreate(addr netip.AddrPort) *sessionPeer {
	h.mu.Lock()
	peer, ok := h.get(addr)
	if ok {
		return peer
	}

	peer = new(sessionPeer)
	peer.state = NewPeer
	peer.inbox = make(chan Message)
	peer.responses = responseMap{mappings: make(map[uuid.UUID]chan Response), mu: &sync.Mutex{}}
	h.pmap[addr] = peer

	go peer.run()
	h.mu.Unlock()
	return peer
}

func (p peerHub) get(addr netip.AddrPort) (*sessionPeer, bool) {
	peer, ok := p.pmap[addr]
	return peer, ok
}

type connMessage struct {
	addr   netip.AddrPort
	buffer []byte
}

type responseMap struct {
	mappings map[uuid.UUID]chan Response
	mu       *sync.Mutex
}

// ::ffff:1.2.3.4 -> 1.2.3.4, 0.0.0.0 -> 127.0.0.1
func normalizeLocalAddrPort(addr netip.AddrPort) netip.AddrPort {
	ip := addr.Addr().Unmap()
	if ip.IsUnspecified() {
		ip = netip.AddrFrom4([4]byte{127, 0, 0, 1})
	}
	return netip.AddrPortFrom(ip, addr.Port())
}

func (rmap *responseMap) get(id uuid.UUID) (chan Response, bool) {
	rmap.mu.Lock()
	ch, ok := rmap.mappings[id]
	rmap.mu.Unlock()

	return ch, ok
}

func (rmap *responseMap) add(id uuid.UUID, ch chan Response) {
	rmap.mu.Lock()
	fmt.Println("adding response channel with id", id)
	rmap.mappings[id] = ch
	rmap.mu.Unlock()
}

func (rmap *responseMap) delete(id uuid.UUID) {
	rmap.mu.Lock()
	delete(rmap.mappings, id)
	rmap.mu.Unlock()
}

type MessageType uint8

const keepaliveInterval int = 15

const (
	ACKNL MessageType = iota
	RES_ACKNL
)

type Response struct {
	err     error
	message Message
}

type Message struct {
	EventID uuid.UUID   `json:"event_id"`
	Type    MessageType `json:"type"`
	Data    any         `json:"data"`
}

var operations []MessageType = []MessageType{
	ACKNL, RES_ACKNL,
}

func getPublicIp(socket *net.UDPConn, messages chan connMessage) (*netip.AddrPort, error) {
	var stunErr error
	rawUri := os.Getenv("STUN_ADDR")
	if rawUri == "" {
		rawUri = "stun.l.google.com:19302"
	}
	var publicAddr netip.AddrPort

	stunAddr, err := net.ResolveUDPAddr("udp", rawUri)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve stun addr - %s\n", stunAddr)
	}
	fmt.Println(stunAddr)
	stunR, stunW := net.Pipe()

	go multiplex(socket, *stunAddr, stunR)
	go demultiplex(socket, stunR, messages)

	client, err := stun.NewClient(stunW, stun.WithNoConnClose())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize stun client, %v\n", err)
	}
	fmt.Println("start stun do")
	if err = client.Do(stun.MustBuild(stun.TransactionID, stun.BindingRequest), func(res stun.Event) {
		fmt.Println("inside")
		if res.Error != nil {
			stunErr = fmt.Errorf("Failed STUN transaction: %s", res.Error)
			return
		}

		var xorAddr stun.XORMappedAddress
		if getErr := xorAddr.GetFrom(res.Message); getErr != nil {
			stunErr = fmt.Errorf("Failed to get XOR-MAPPED-ADDRESS: %s", getErr)
			return
		}

		publicAddr, stunErr = netip.ParseAddrPort(fmt.Sprintf("%s:%d", xorAddr.IP.String(), xorAddr.Port))

	}); err != nil {
		return nil, fmt.Errorf("Do: %v, %v", err, stunErr)
	}

	if stunErr != nil {
		return nil, stunErr
	}

	go keepAlive(client, keepaliveInterval)
	return &publicAddr, nil
}

func keepAlive(c *stun.Client, interval int) {
	t := time.NewTicker(time.Second * time.Duration(interval))
	for range t.C {
		if err := c.Do(stun.MustBuild(stun.TransactionID, stun.BindingRequest), func(res stun.Event) {
			if res.Error != nil {
				log.Printf("Failed STUN transaction: %s\n", res.Error)
			}
		}); err != nil {
			log.Printf("Failed STUN transaction: %s\n", err)
		}
	}
}

func multiplex(conn *net.UDPConn, stunAddr net.UDPAddr, stunReader net.Conn) {
	var buffer []byte = make([]byte, 1024)
	for {
		n, err := stunReader.Read(buffer)
		if err != nil {
			fmt.Printf("failed to read stun reader %v\n", err)
		}
		if n == 0 {
			continue
		}
		fmt.Println("from stun client", string(buffer))

		if _, err = conn.WriteTo(buffer[:n], &stunAddr); err != nil {
			fmt.Printf("socket write fail: %v\n", err)
			continue
		}
	}
}

func demultiplex(conn *net.UDPConn, stunR net.Conn, messages chan connMessage) {
	var buffer []byte = make([]byte, 1024)
	for {
		n, addr, err := conn.ReadFromUDPAddrPort(buffer)
		if err != nil {
			fmt.Printf("failed to read socket %v\n", err)
		}
		if n == 0 {
			continue
		}
		if stun.IsMessage(buffer[:n]) {
			fmt.Println("got something from server stun", string(buffer[:n]))
			stunR.Write(buffer[:n])
			continue
		}
		fmt.Println("demultiplexing outbound", addr, string(buffer[:n]))
		messages <- connMessage{
			addr:   normalizeLocalAddrPort(addr),
			buffer: buffer[:n],
		}
	}

}

func InitServer() (*Server, error) {
	var public *netip.AddrPort
	socket, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, err
	}

	messages := make(chan connMessage)
	if public, err = getPublicIp(socket, messages); err != nil {
		return nil, err
	}

	fmt.Println(public)
	fmt.Println("p2p server is listening on", socket.LocalAddr().String())
	return &Server{
		socket:   socket,
		PublicIP: public.String(),
		messages: messages,
		peers: peerHub{
			pmap: make(map[netip.AddrPort]*sessionPeer),
			mu:   &sync.Mutex{},
		},
	}, nil

}

func (server *Server) Close() error {
	return server.socket.Close()
}

func (server *Server) writeToStringAddr(msg Message, addr string) error {
	addrPort, err := netip.ParseAddrPort(addr)
	if err != nil {
		return err
	}
	return server.write(msg, addrPort)
}
func (server *Server) write(msg Message, addr netip.AddrPort) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("couldn't marshal message, error: %v\n", err)
	}

	n, err := server.socket.WriteToUDP(payload, net.UDPAddrFromAddrPort(addr))
	if err != nil {
		return fmt.Errorf("coudlb't write to %s, err: %v\n", addr, err)
	}

	fmt.Printf("sent %d bytes to %s\n", n, addr)
	return nil
}

func (server *Server) Connect(ctx context.Context, addrRaw string) (*net.UDPAddr, error) {
	addrPort, err := netip.ParseAddrPort(addrRaw)
	if err != nil {
		return nil, err
	}
	addrPort = normalizeLocalAddrPort(addrPort)

	ip := addrPort.Addr()
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return server.connectLocal(ctx, addrPort)
	}

	panic("not implemented")

}

func (server *Server) connectLocal(ctx context.Context, addrPort netip.AddrPort) (*net.UDPAddr, error) {
	var err error
	var endpoint *net.UDPAddr

	eventUUID := uuid.New()
	peer := server.peers.getOrCreate(addrPort)
	resp := make(chan Response)
	peer.responses.add(eventUUID, resp)

	outMessage := Message{
		Type:    ACKNL,
		EventID: eventUUID,
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		defer cancel()

		ticker := time.NewTicker(time.Second * 1)
		defer ticker.Stop()

		timer := time.NewTimer(time.Second * 10)
		defer timer.Stop()

		for {
			select {
			case <-ticker.C:
				server.write(outMessage, addrPort)
			case <-timer.C:
				cancel()
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {

		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-resp:

				fmt.Println("got it", msg)
				ip := addrPort.Addr().As16()
				endpoint = &net.UDPAddr{IP: ip[:], Port: int(addrPort.Port())}
				cancel()
			}
		}

	}()

	wg.Wait()
	if endpoint == nil {
		err = fmt.Errorf("couldn't establish connection with %v\n", addrPort)
	}

	return endpoint, err
}

func (server *Server) Dispatch(daemonDispatcher chan Message) {

	for msg := range server.messages {

		fmt.Println("got message from", msg.addr.String())
		var payload Message
		if err := json.Unmarshal(msg.buffer, &payload); err != nil {
			fmt.Printf("unable to unmarshal json from %s, error: %v\n", msg.buffer, err)
			continue
		}
		if payload.Type == ACKNL {
			handleRequestHandshake(payload, server, msg.addr)
			continue
		}
		peer := server.peers.getOrCreate(msg.addr)
		peer.inbox <- payload

	}
}

func handleRequestHandshake(msg Message, server *Server, from netip.AddrPort) {
	ip := from.Addr()
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		fmt.Printf("%v requested handshake from local network\n", from)
		server.writeToStringAddr(Message{
			Type:    RES_ACKNL,
			EventID: msg.EventID,
		}, from.String())
	} else {
		//
	}

}
