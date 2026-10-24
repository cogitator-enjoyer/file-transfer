package p2p

import (
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"sync"

	"github.com/google/uuid"
	"github.com/pion/stun"
)

type Server struct {
	socket   *net.UDPConn
	response responseMap
}

type responseMap struct {
	mappings map[uuid.UUID]chan Response
	mu       *sync.Mutex
}

func (rmap *responseMap) get(id uuid.UUID) (chan Response, bool) {
	ok, ch := rmap.mappings[id]
	return ok, ch
}

func (rmap *responseMap) add(id uuid.UUID, ch chan Response) {
	rmap.mu.Lock()
	rmap.mappings[id] = ch
	rmap.mu.Unlock()
}

func (rmap *responseMap) delete(id uuid.UUID, ch chan Response) {
	rmap.mu.Lock()
	delete(rmap.mappings, id)
	rmap.mu.Unlock()
}

type MessageType uint8

const (
	ACKNL MessageType = iota
)

type Response struct {
	result  error
	message Message
}

type Message struct {
	Type MessageType `json:"type"`
	ID   uuid.UUID   `json:"id"`
}

func getPublicIp(socket *net.UDPConn) (*netip.AddrPort, error) {
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
	go demultiplex(socket, stunR)

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

	return &publicAddr, nil
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

func demultiplex(conn *net.UDPConn, stunR net.Conn) {
	var buffer []byte = make([]byte, 1024)
	for {
		n, err := conn.Read(buffer)
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
		if _, err = conn.Write(buffer[:n]); err != nil {
			fmt.Printf("socket write fail: %v\n", err)
			continue
		}
	}

}

func InitServer() (*Server, error) {
	var public *netip.AddrPort
	socket, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, err
	}
	rMap := responseMap{
		mu:       &sync.Mutex{},
		mappings: make(map[uuid.UUID]chan Response),
	}
	if public, err = getPublicIp(socket); err != nil {
		return nil, err
	}
	fmt.Println(public)
	fmt.Println("p2p server is listening on", socket.LocalAddr().String())
	return &Server{
		response: rMap,
		socket:   socket,
	}, nil

}

func (server *Server) process() {
	var buffer []byte = make([]byte, 1024)
	for {
		var msg Message
		r, source, err := server.socket.ReadFromUDPAddrPort(buffer)
		if err != nil {
			fmt.Printf("error reading udp socket %v\n", err)
			continue
		}

		fmt.Println("got message from", source.Addr().String())

		if err := json.Unmarshal(buffer[:r], &msg); err != nil {
			fmt.Printf("unable to unmarshal json from %s, error: %v\n", source.Addr().String(), err)
			continue
		}

		responseChan, ok := server.response.get(msg.ID)
		if ok {
			responseChan <- Response{
				result: nil, message: msg,
			}
		}

	}
}

func (server *Server) write(msg Message, addr string) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("couldn't marshal message, error: %v\n", err)
	}

	addrPort, err := netip.ParseAddrPort(addr)
	if err != nil {
		return err
	}

	n, err := server.socket.WriteToUDP(payload, net.UDPAddrFromAddrPort(addrPort))
	if err != nil {
		return fmt.Errorf("coudlb't write to %s, err: %v\n", addr, err)
	}

	fmt.Printf("sent %d bytes to %s\n", n, addr)
	return nil
}

func createIndexKey(first string, messageType MessageType) string {
	return fmt.Sprintf("%s:%s", first, strconv.Itoa(int(messageType)))
}

func newMessage(msgType MessageType) Message {
	return Message{
		Type: msgType,
		ID:   uuid.New(),
	}
}

func (server *Server) SendWithResponse(msg Message, addr string) chan Response {
	responseCh := make(chan Response, 1)
	err := server.write(msg, addr)
	if err != nil {

	}
	server.response.add(msg.ID, responseCh)
	return responseCh
}

func (server *Server) Acknowledge(addr string) (net.Addr, bool, error) {
	fmt.Println("acknowledge", addr)

	response := <-server.SendWithResponse(newMessage(ACKNL), addr)
	if response.result != nil {
		return nil, false, response.result
	}

	return nil, false, nil
}
