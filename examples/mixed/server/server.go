package main

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/lesismal/arpc"
)

const (
	addr = ":8888"
)

// HelloReq .
type HelloReq struct {
	Msg string
}

// HelloRsp .
type HelloRsp struct {
	Msg string
}

// OnClientHello .
func OnClientHello(ctx *arpc.Context) {
	req := &HelloReq{}
	rsp := &HelloRsp{}

	ctx.Bind(req)
	log.Printf("OnClientHello: \"%v\"", req.Msg)

	rsp.Msg = req.Msg
	go ctx.Clone().Write(rsp)
}

// OnClientNotify .
func OnClientNotify(ctx *arpc.Context) {
	str := ""
	ctx.Bind(&str)
	log.Printf("OnClientNotify: \"%v\"", str)
}

// OnClientCallAsync .
func OnClientCallAsync(ctx *arpc.Context) {
	str := ""
	ctx.Bind(&str)
	log.Printf("OnClientCallAsync: \"%v\"", str)
	ctx.Write(str)

	client := ctx.Client

	go func() {
		req := &HelloReq{Msg: "ServerHello"}
		rsp := &HelloRsp{}
		err := client.Call("ServerHello", req, rsp, time.Second*5)
		if err != nil {
			log.Printf("ServerHello Call failed: %v", err)
		} else {
			log.Printf("ServerHello Call Rsp: \"%v\"", rsp.Msg)
		}
	}()

	go func() {
		for i := 0; true; i++ {
			time.Sleep(time.Second * 2)
			client.Notify("ServerNotify", fmt.Sprintf("ServerNotify %v", i), time.Second)
		}
	}()
	go func() {
		for i := 0; true; i++ {
			time.Sleep(time.Second * 2)
			msg := arpc.NewRefMessage(client.Codec, "ServerNotifyRefMessage", fmt.Sprintf("ServerNotifyRefMessage %v", i))
			client.PushMsg(msg, arpc.TimeZero)
			client.PushMsg(msg, arpc.TimeForever)
			client.PushMsg(msg, time.Second)
			msg.Release()
		}
	}()
	go func() {
		time.Sleep(time.Second)
		for i := 0; true; i++ {
			time.Sleep(time.Second * 2)
			client.CallAsync("ServerCallAsync", fmt.Sprintf("ServerCallAsync %v", i), OnServerCallAsyncRsp, time.Second)
		}
	}()
}

// OnServerCallAsyncRsp .
func OnServerCallAsyncRsp(ctx *arpc.Context) {
	str := ""
	ctx.Bind(&str)
	if len(str) < 10 {
		log.Printf("OnServerCallAsyncRsp: \"%v\"", str)
	}
	log.Printf("OnServerCallAsyncRsp: \"%v\"", str)
}

func main() {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	svr := arpc.NewServer()
	svr.Handler.Handle("ClientHello", OnClientHello)
	svr.Handler.Handle("ClientNotify", OnClientNotify)
	svr.Handler.Handle("ClientCallAsync", OnClientCallAsync)

	svr.Serve(ln)
}
