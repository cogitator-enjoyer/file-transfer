package p2p

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/pion/stun"
)

type Server struct {
	socket             *net.UDPConn
	operationResponses operationResponseMap
	PublicIP           string
	messages           chan connMessage
}

type peerState uint8

const (
	Connecting peerState = iota
	Connected
	Failed
)

type operationResponseMap struct {
	mappings map[MessageType]responseMap
	mu       *sync.Mutex
}

func (rmap *operationResponseMap) get(operation MessageType) (responseMap, bool) {
	chMap, ok := rmap.mappings[operation]
	return chMap, ok
}

func (rmap *operationResponseMap) add(operation MessageType, chMap responseMap) {
	rmap.mu.Lock()
	rmap.mappings[operation] = chMap
	rmap.mu.Unlock()
}

func (rmap *operationResponseMap) delete(operation MessageType) {
	rmap.mu.Lock()
	delete(rmap.mappings, operation)
	rmap.mu.Unlock()
}

type connMessage struct {
	addr   netip.AddrPort
	buffer []byte
}

type responseMap struct {
	mappings map[netip.AddrPort]chan Response
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

func (rmap *responseMap) get(id netip.AddrPort) (chan Response, bool) {
	rmap.mu.Lock()
	ch, ok := rmap.mappings[id]
	rmap.mu.Unlock()
	return ch, ok
}

func (rmap *responseMap) add(id netip.AddrPort, ch chan Response) {
	rmap.mu.Lock()
	rmap.mappings[id] = ch
	rmap.mu.Unlock()
}

func (rmap *responseMap) delete(id netip.AddrPort) {
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

var operations []MessageType = []MessageType{
	ACKNL, RES_ACKNL,
}

type Response struct {
	err     error
	message Message
}

type Message struct {
	Type MessageType `json:"type"`
	Data any         `json:"data"`
}

func getPublicIp(socket *net.UDPConn, messages chan connMessage) (*netip.AddrPort, error) {
	var stunErr error
	rawUri := "stun.l.google.com:19302"
	var publicAddr netip.AddrPort
	// uri, err := stun.ParseURI(rawUri)
	// if err != nil {
	// 	return err
	// }

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
	opMappings := operationResponseMap{
		mappings: make(map[MessageType]responseMap),
		mu:       &sync.Mutex{},
	}
	for _, operation := range operations {
		rMap := responseMap{
			mu:       &sync.Mutex{},
			mappings: make(map[netip.AddrPort]chan Response),
		}
		opMappings.mappings[operation] = rMap
	}

	messages := make(chan connMessage)
	if public, err = getPublicIp(socket, messages); err != nil {
		return nil, err
	}
	fmt.Println(public)
	fmt.Println("p2p server is listening on", socket.LocalAddr().String())
	return &Server{
		socket:             socket,
		PublicIP:           public.String(),
		messages:           messages,
		operationResponses: opMappings,
	}, nil

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

func createIndexKey(first string, messageType MessageType) string {
	return fmt.Sprintf("%s:%s", first, strconv.Itoa(int(messageType)))
}

// func newMessage(msgType MessageType) Message {
// 	return Message{
// 		Type: msgType,
// 		ID:   uuid.New(),
// 	}
// }

// func (server *Server) SendWithResponse(msg Message, addr string) chan Response {
// 	responseCh := make(chan Response, 1)
// 	err := server.writeToStringAddr(msg, addr)
// 	if err != nil {

// 	}
// 	server.response.add(msg.ID, responseCh)
// 	return responseCh
// }

// func (server *Server) Acknowledge(addr string) (net.Addr, bool, error) {
// 	fmt.Println("acknowledge", addr)

// 	response := <-server.SendWithResponse(newMessage(ACKNL), addr)
// 	if response.result != nil {
// 		return nil, false, response.result
// 	}

// 	return nil, false, nil
// }

func (server *Server) postResponseChan(msgType MessageType, addr netip.AddrPort) error {
	responseMap, exist := server.operationResponses.get(msgType)
	if !exist {
		return fmt.Errorf("response map of operation type %v does not exist", msgType)
	}
	responseMap.add(addr, make(chan Response))
	return nil
}

func (server *Server) getResponseChan(msgType MessageType, addr netip.AddrPort) (chan Response, error) {
	responseMap, exist := server.operationResponses.get(msgType)
	if !exist {
		return nil, fmt.Errorf("response map of operation type %v does not exist", msgType)
	}
	responseCh, ok := responseMap.get(addr)
	if !ok {
		return nil, fmt.Errorf("response channel of operation type %v, addr %v does not exist", msgType, addr)
	}

	return responseCh, nil

}

func (server *Server) Connect(ctx context.Context, addrRaw string) (*net.UDPAddr, error) {
	var endpoint *net.UDPAddr
	addrPort, err := netip.ParseAddrPort(addrRaw)
	if err != nil {
		return endpoint, err
	}
	addrPort = normalizeLocalAddrPort(addrPort)

	// post response that i expect from other peer
	if err := server.postResponseChan(RES_ACKNL, addrPort); err != nil {
		return nil, err
	}

	responseCh, err := server.getResponseChan(RES_ACKNL, addrPort)
	if err != nil {
		return nil, err
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
				server.write(Message{
					Type: ACKNL,
				}, addrPort)
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
			case msg := <-responseCh:

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
			fmt.Printf("%v requested handshake\n", msg.addr)
			server.writeToStringAddr(Message{
				Type: RES_ACKNL,
			}, msg.addr.String())
			continue
		}
		fmt.Println(server.operationResponses.mappings)
		chMap, ok := server.operationResponses.get(payload.Type)
		if !ok {
			fmt.Printf("failed to dispatch p2p server message, couldn't find response map for given operation %v\n", payload.Type)
			continue
		}

		responseCh, ok := chMap.get(msg.addr)
		if !ok {
			fmt.Printf("failed to dispatch p2p server message, couldn't find response map for %v address of operation type %v\n", msg.addr, payload.Type)
			continue
		}

		responseCh <- Response{
			message: payload,
		}

	}
}
