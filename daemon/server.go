package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"file-transfer/p2p"
	"file-transfer/protocols/ipc"
	"fmt"
	"log"
	"net"
	"os"
	"reflect"
	"time"
)

type DaemonServer struct {
	listener   *net.UnixListener
	clientConn *net.UnixConn
	network    Network
}

type Network interface {
	Connect(ctx context.Context, addrRaw string) (*net.UDPAddr, error)
	Close() error
}

func InitializeDaemon(path string, network Network) (*DaemonServer, error) {

	if _, err := os.Stat(path); err == nil {
		fmt.Println(os.Remove(path))
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	listener, err := net.ListenUnix("unixpacket", &net.UnixAddr{
		Name: path, Net: "unixpacket",
	})
	if err != nil {
		return nil, err
	}

	conn, err := listener.Accept()
	if err != nil {
		fmt.Printf("failed to accept unix conn %v\n", err)
		return nil, err
	}
	fmt.Println("connected with client through unix")
	return &DaemonServer{
		listener: listener, clientConn: conn.(*net.UnixConn),
		network: network,
	}, nil
}

func (daemon *DaemonServer) Close() error {
	return errors.Join(daemon.listener.Close(), daemon.clientConn.Close())

}

func (daemon *DaemonServer) Response(replyTo uint32, eventType ipc.EventType, data any) error {
	return daemon.write(ipc.Event{ReplyTo: replyTo, Type: eventType, Data: data})
}

func (daemon *DaemonServer) Send(eventType ipc.EventType, data any) error {
	return daemon.write(ipc.Event{Type: eventType, Data: data})
}

func (daemon *DaemonServer) write(responseMsg ipc.Event) error {
	result, err := json.Marshal(responseMsg)
	if err != nil {
		return err
	}

	_, err = daemon.clientConn.Write(result)
	if err != nil {
		return fmt.Errorf("error writing through unix connection %v\n", err)
	}
	fmt.Println(reflect.TypeOf(responseMsg.Data))
	return err
}

func (daemon *DaemonServer) DispatchClientMessages(messageQueue chan ipc.Command) {
	ctx := context.Background()
	for cmd := range messageQueue {
		var evType ipc.EventType
		var data any
		var err error
		fmt.Println(cmd)
		switch cmd.Type {
		case ipc.CmdAcknowledgePublicIP:
			continue
		case ipc.CmdDiscover:
			evType = ipc.EvDiscoverResult
			data, err = discovery()
		case ipc.CmdRequestHandshake:
			var reqData ipc.RequestHandshakeData
			if err := json.Unmarshal(cmd.Data, &reqData); err != nil {
				panic(err)
			}
			var endpoint *net.UDPAddr
			endpoint, err = daemon.network.Connect(ctx, reqData.Endpoint)
			fmt.Println(endpoint)
			evType = ipc.EvHandshakeResult
			data = ipc.HandshakeResult{Success: err == nil}
		default:
			err = fmt.Errorf("unknown command type %v", cmd.Type)
		}
		if err != nil {
			log.Printf("failed to dispatch command %v (id %d), err:%v\n", cmd.Type, cmd.ID, err)
			daemon.Response(cmd.ID, ipc.EvError, err.Error())
			continue
		}

		daemon.Response(cmd.ID, evType, data)
	}
}

func (daemon *DaemonServer) DispatchP2PMessages(messages chan p2p.Message) {
	for message := range messages {
		switch message.Type {
		case p2p.ACKNL:
			if err := daemon.Send(ipc.EvIncomingHandshake, nil); err != nil {
				fmt.Println("error dispatching p2p message in daemon", err)
			}
		}
	}
}

func discovery() (ipc.ResponseDiscovery, error) {
	fmt.Println("launch discovery task")
	time.Sleep(time.Second * 2)
	fmt.Println("discovery task done")
	return ipc.ResponseDiscovery{Discovered: []string{"192.165.1.5"}}, nil
}

func (daemon *DaemonServer) ListenMessages(messageBuffer chan ipc.Command) {
	buffer := make([]byte, 1024)
	for {
		r, err := daemon.clientConn.Read(buffer)
		if r <= 0 {
			continue
		}
		if err != nil {
			fmt.Println("failed to read from client connection", err)
		}
		fmt.Println("received from client", string(buffer[:r]))
		message := ipc.Command{}
		if err := json.Unmarshal(buffer[:r], &message); err != nil {
			fmt.Printf("failed to unmarshal client message %v\n", err)
			continue
		}
		messageBuffer <- message
	}
}
