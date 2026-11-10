package main

import (
	"file-transfer/daemon"
	"file-transfer/p2p"
	"file-transfer/protocols/ipc"
	"fmt"
	"log"
	"os"
	"path"
)

func main() {
	sockPath := "/tmp/unix.sock"
	if len(os.Args)-1 > 0 {
		sockPath = path.Join("/tmp/", fmt.Sprintf("%s.sock", os.Args[1]))
	}

	fmt.Println(sockPath)
	fmt.Println("starting...")
	exitSignal := make(chan os.Signal, 1)

	p2pServer, err := p2p.InitServer()
	if err != nil {
		log.Fatalf("failed to initialize p2p server %v\n", err)
	}

	daemon, err := daemon.InitializeDaemon(sockPath, p2pServer)
	if err != nil {
		log.Fatalf("failed to establish unix connection with client %v\n", err)
	}

	defer daemon.Close()
	defer p2pServer.Close()

	clientMessageChan := make(chan ipc.Command)
	p2pMessageChan := make(chan p2p.Message)
	if err := daemon.Send(ipc.EvPublicIP, ipc.ClientPublicIP{
		IP: p2pServer.PublicIP,
	}); err != nil {
		panic(err)
	}
	go daemon.ListenMessages(clientMessageChan)
	go p2pServer.Dispatch(p2pMessageChan)
	go daemon.DispatchClientMessages(clientMessageChan)
	go daemon.DispatchP2PMessages(p2pMessageChan)
	<-exitSignal
}
