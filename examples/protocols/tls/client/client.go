package main

import (
	"crypto/tls"
	"log"
	"net"
	"time"

	"github.com/lesismal/arpc"
)

func main() {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}
	client, err := arpc.NewClient(func() (net.Conn, error) {
		return tls.Dial("tcp", "localhost:8888", tlsConfig)
	})
	if err != nil {
		panic(err)
	}
	defer client.Stop()

	req := "hello"
	rsp := ""
	err = client.Call("/echo", &req, &rsp, time.Second*5)
	if err != nil {
		log.Fatalf("Call failed: %v", err)
	} else {
		log.Printf("Call Response: \"%v\"", rsp)
	}
}
