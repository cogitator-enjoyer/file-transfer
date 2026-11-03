package main

import (
	"context"
	"encoding/json"
	"errors"
	"file-transfer/client"
	"file-transfer/p2p"
	"fmt"
	"log"
	"net"
	"os"
	"path"
	"reflect"
	"time"
)

type DaemonServer struct {
	listener   *net.UnixListener
	clientConn *net.UnixConn

	p2pServer *p2p.Server
}

// 2type DispatchMessage struct {
//
// 2}

func initializeDaemon(path string) (*DaemonServer, error) {

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
	}, nil
}

func (daemon *DaemonServer) Response(eventType client.EventType, data any) error {
	return daemon.send(eventType, data)
}

func (daemon *DaemonServer) Send(eventType client.EventType, data any) error {
	return daemon.send(eventType, data)
}

func (daemon *DaemonServer) send(eventType client.EventType, data any) error {
	responseMsg := client.Message{
		Type: eventType,
		Data: data,
	}

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

func (daemon *DaemonServer) dispatchClientMessages(messageQueue chan client.Response) {
	ctx := context.Background()
	for clientMessage := range messageQueue {
		var data any
		var err error
		switch clientMessage.Type {
		case client.AcknowledgePublicIP:
			continue
		case client.Discover:
			data, err = discovery()
		case client.RequestHandshake:
			var reqData client.RequestHandshakeData
			if err := json.Unmarshal(clientMessage.Data, &reqData); err != nil {
				panic(err)
			}
			var endpoint *net.UDPAddr
			endpoint, err = daemon.p2pServer.Connect(ctx, reqData.Endpoint)
			fmt.Println(endpoint)
		}
		if err != nil {
			log.Printf("failed to dispatch message %v, err:%v\n", clientMessage.Type, err)
			continue
		}

		daemon.Response(clientMessage.Type, data)
	}
}

func (daemon *DaemonServer) DispatchP2PMessages(messages chan p2p.Message) {
	for message := range messages {
		switch message.Type {
		case p2p.ACKNL:
			if err := daemon.Send(client.ResponseRequestHandshake, nil); err != nil {
				fmt.Println("error dispatching p2p message in daemon", err)
			}
		}
	}
}

func discovery() (client.ResponseDiscovery, error) {
	fmt.Println("launch discovery task")
	time.Sleep(time.Second * 2)
	fmt.Println("discovery task done")
	return client.ResponseDiscovery{Discovered: []string{"192.165.1.5"}}, nil
}

func listenMessages(daemon *DaemonServer, messageBuffer chan client.Response) {
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
		message := client.Response{}
		if err := json.Unmarshal(buffer[:r], &message); err != nil {
			fmt.Printf("failed to unmarshal client message %v\n", err)
			continue
		}
		messageBuffer <- message
		// fmt.Println(message)

	}
}

func main() {
	sockPath := "/tmp/unix.sock"
	if len(os.Args)-1 > 0 {
		sockPath = path.Join("/tmp/", fmt.Sprintf("%s.sock", os.Args[1]))
	}

	fmt.Println(sockPath)
	fmt.Println("starting...")
	exitSignal := make(chan os.Signal, 1)
	daemon, err := initializeDaemon(sockPath)
	if err != nil {
		log.Fatalf("failed to establish unix connection with client %v\n", err)
	}
	p2pServer, err := p2p.InitServer()
	if err != nil {
		log.Fatalf("failed to initialize p2p server %v\n", err)
	}

	daemon.p2pServer = p2pServer
	defer daemon.listener.Close()
	defer daemon.clientConn.Close()
	clientMessageChan := make(chan client.Response)
	p2pMessageChan := make(chan p2p.Message)
	if err := daemon.Send(client.AcknowledgePublicIP, client.ClientPublicIP{
		IP: p2pServer.PublicIP,
	}); err != nil {
		panic(err)
	}

	go p2pServer.Dispatch(p2pMessageChan)
	go listenMessages(daemon, clientMessageChan)
	go daemon.dispatchClientMessages(clientMessageChan)
	go daemon.DispatchP2PMessages(p2pMessageChan)
	<-exitSignal
}
