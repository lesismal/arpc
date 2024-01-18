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
	defer stream.CloseRecv()
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
			log.Printf("[server] [stream id: %v] stream_server_to_client closed", stream.Id())
			break
		}
		if err != nil {
			panic(err)
		}
		log.Printf("[server] [stream id: %v] stream_server_to_client: %v", stream.Id(), str)
	}
}

func OnStream(stream *arpc.Stream) {
	defer stream.CloseRecv()
	for {
		str := ""
		err := stream.Recv(&str)
		if err == io.EOF {
			stream.CloseSend()
			log.Printf("[server] [stream id: %v] stream_client_to_server closed", stream.Id())
			break
		}
		if err != nil {
			panic(err)
		}
		log.Printf("[server] [stream id: %v] stream_client_to_server: [%v]", stream.Id(), str)
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
