package main

import (
	"context"
	"log"
	"net"
	"sync"

	"github.com/PeronGH/datagram-forwarder/forwarder"
)

func main() {
	clientConn1, remoteConn1, close1 := forwarder.DatagramConnPipe()
	clientConn2, remoteConn2, close2 := forwarder.DatagramConnPipe()
	defer close1()
	defer close2()

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		// client 1
		client("client 1", clientConn1)
	}()

	go func() {
		defer wg.Done()
		// client 2
		client("client 2", clientConn2)
	}()

	go func() {
		defer wg.Done()
		// server
		server := forwarder.NewServer(context.Background(), &net.UDPAddr{IP: net.IPv4(1, 1, 1, 1), Port: 53})
		defer server.Close()

		go server.Handle(remoteConn1)
		go server.Handle(remoteConn2)

		server.Wait()
	}()

	wg.Wait()
}

func client(name string, replayConn forwarder.DatagramConn) {
	ln, err := net.ListenUDP("udp", nil)
	if err != nil {
		log.Printf("%s: error: %v", name, err)
		return
	}
	defer ln.Close()
	log.Printf("%s: listen on %s", name, ln.LocalAddr())

	err = forwarder.RunClient(forwarder.ClientConfig{
		Ctx:       context.Background(),
		RelayConn: replayConn,
		Listener:  ln,
	})
	if err != nil {
		log.Printf("%s: error: %v", name, err)
		return
	}
}