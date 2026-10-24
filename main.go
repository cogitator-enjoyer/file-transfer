package main

import (
	"encoding/json"
	"errors"
	"file-transfer/client"
	"file-transfer/p2p"
	"file-transfer/tasks"
	"fmt"
	"log"
	"net"
	"os"
)

type DaemonServer struct {
	listener   *net.UnixListener
	clientConn *net.UnixConn

	p2pServer *p2p.Server
}

// 2type DispatchMessage struct {
//
// 2}

func initializeDaemon() (*DaemonServer, error) {
	path := "/tmp/unix.sock"
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

func (daemon *DaemonServer) response(eventType client.EventType, data any) {
	responseMsg := client.Message{
		Type: eventType,
		Data: data,
	}
	result, err := json.Marshal(responseMsg)
	if err != nil {
		panic(err)
	}

	_, err = daemon.clientConn.Write(result)
	if err != nil {
		fmt.Println("error writing to client", err)
	}
	fmt.Println("responded to client with", data)
}

func (daemon *DaemonServer) dispatchMessages(messageQueue chan client.Message) {
	for clientMessage := range messageQueue {
		if tasks.IsTask(clientMessage.Type) {
			err := tasks.Launch(clientMessage, daemon.response)
			if err != nil {
				fmt.Println("task failed to launch", err)
			}
		}

		if clientMessage.Type == client.RequestHandshake {
			// reqData, _ := clientMessage.Data.(client.RequestHandshakeData)
			// daemon.p2pServer.RequestHandshake(reqData.Addr)
		}
	}
}

func listenMessages(daemon *DaemonServer, messageBuffer chan client.Message) {
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
		message := client.Message{}
		if err := json.Unmarshal(buffer[:r], &message); err != nil {
			fmt.Printf("failed to unmarshal client message %v\n", err)
			continue
		}
		messageBuffer <- message
		// fmt.Println(message)

	}
}

func main() {
	fmt.Println("hhhhhhhhhhhhhhhhhhh")
	exitSignal := make(chan os.Signal, 1)

	server, err := initializeDaemon()
	if err != nil {
		log.Fatalf("failed to establish unix connection with client %v\n", err)
	}
	p2pServer, err := p2p.InitServer()
	if err != nil {
		log.Fatalf("failed to initialize p2p server %v\n", err)
	}

	server.p2pServer = p2pServer
	defer server.listener.Close()
	defer server.clientConn.Close()
	messageChan := make(chan client.Message)
	go listenMessages(server, messageChan)
	go server.dispatchMessages(messageChan)
	<-exitSignal
}
