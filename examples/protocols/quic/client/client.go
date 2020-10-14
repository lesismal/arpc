package main

import (
	"crypto/tls"
	"log"
	"net"
	"time"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpcext/quic"
)

func main() {
	client, err := arpc.NewClient(func() (net.Conn, error) {
		tlsConf := &tls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{"quic-echo-example"},
		}
		return quic.Dial("localhost:8888", tlsConf, nil, 0)
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
