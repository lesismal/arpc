package main

import (
	"fmt"
	"io"
	"log"
	"net"

	"github.com/lesismal/arpc"
)

const (
	addr = "localhost:8888"
)

// HelloReq .
type HelloReq struct {
	Msg string
}

// HelloRsp .
type HelloRsp struct {
	Msg string
}

// OnHello .
func OnHello(ctx *arpc.Context) {
	req := &HelloReq{}
	ctx.Bind(req)
	ctx.Write(&HelloRsp{Msg: req.Msg})

	stream := ctx.Client.NewStream("/stream_server_to_client")
	go func() {
		for i := 0; i < 3; i++ {
			err := stream.Send(fmt.Sprintf("stream data %v", i))
			if err != nil {
				panic(err)
			}
		}
		err := stream.SendAndClose(fmt.Sprintf("stream data %v", 3))
		if err != nil {
			panic(err)
		}
	}()

	for {
		str := ""
		err := stream.Recv(&str)
		if err == io.EOF {
			log.Println("[server] stream_server_to_client closed with:", stream.Id(), str)
			break
		}
		if err != nil {
			panic(err)
		}
		log.Println("[server] stream_server_to_client:", stream.Id(), str)
	}
}

func OnStream(stream *arpc.Stream) {
	defer stream.Close()
	for {
		str := ""
		err := stream.Recv(&str)
		if err == io.EOF {
			err = stream.Close()
			if err != nil {
				panic(err)
			}
			log.Println("[server] stream_client_to_server closed with:", str)
			break
		}
		if err != nil {
			panic(err)
		}
		log.Println("[server] stream_client_to_server:", str)
		err = stream.Send(&str)
		if err != nil {
			panic(err)
		}
	}
}

func main() {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	svr := arpc.NewServer()
	svr.Handler.EnablePool(true)
	svr.Handler.SetAsyncResponse(true)
	svr.Handler.Handle("Hello", OnHello)
	svr.Handler.HandleStream("/stream_client_to_server", OnStream)
	svr.Serve(ln)
}
