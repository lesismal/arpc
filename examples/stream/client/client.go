package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/lesismal/arpc"
)

var (
	addr = "localhost:8888"

	method = "Hello"
)

// HelloReq .
type HelloReq struct {
	Msg string
}

// HelloRsp .
type HelloRsp struct {
	Msg string
}

func dialer() (net.Conn, error) {
	return net.DialTimeout("tcp", addr, time.Second*3)
}

func main() {
	arpc.EnablePool(true)

	client, err := arpc.NewClient(dialer)
	if err != nil {
		log.Println("NewClient failed:", err)
		return
	}

	wg := &sync.WaitGroup{}
	wg.Add(1)
	client.Handler.HandleStream("/stream_server_to_client", func(stream *arpc.Stream) {
		defer wg.Done()
		defer stream.Close()
		str := ""
		for {
			err := stream.Recv(&str)
			if err == io.EOF {
				err = stream.Close()
				if err != nil {
					panic(err)
				}
				log.Println("[client] stream_server_to_client closed with:", str)
				break
			}
			if err != nil {
				panic(err)
			}
			log.Println("[client] stream_server_to_client:", str)
			str := ""
			err = stream.Send(&str)
			if err != nil {
				panic(err)
			}
		}
	})

	stream := client.NewStream("/stream_client_to_server")
	defer stream.Close()
	for i := 0; i < 3; i++ {
		err := stream.Send(fmt.Sprintf("stream data %v", i))
		if err != nil {
			panic(err)
		}
		str := ""
		err = stream.Recv(&str)
		if err != nil {
			panic(err)
		}
	}
	err = stream.SendAndClose(fmt.Sprintf("stream data %v", 3))
	if err != nil {
		panic(err)
	}

	defer client.Stop()

	data := make([]byte, 10)
	rand.Read(data)
	req := &HelloReq{Msg: base64.RawStdEncoding.EncodeToString(data)}
	rsp := &HelloRsp{}
	err = client.Call(method, req, rsp, time.Second*5)
	if err != nil {
		log.Printf("Call failed: %v", err)
	} else if rsp.Msg != req.Msg {
		log.Fatal("Call failed: not equal")
	}

	wg.Wait()
}
