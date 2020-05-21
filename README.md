# ARPC - Sync && Async Call supported

[![GoDoc][1]][2] [![MIT licensed][3]][4] [![Go Report Card][5]][6]

[1]: https://godoc.org/github.com/lesismal/arpc?status.svg
[2]: https://godoc.org/github.com/lesismal/arpc
[3]: https://img.shields.io/badge/license-MIT-blue.svg
[4]: LICENSE
[5]: https://goreportcard.com/badge/github.com/lesismal/arpc
[6]: https://goreportcard.com/report/github.com/lesismal/arpc



## Features
- [x] Client call Server Sync
- [x] Client call Server Async
- [x] Client notify Server
- [x] Server call Client Sync
- [x] Server call Client Async
- [x] Server notify Client
- [x] Mem Pools for Buffers and Objects
- [x] Broadcast Ref Count Message



## Protocol

- Header: LittleEndian

|  cmd   | async  | methodlen |  null   | bodylen | sequence |       method         | body |
| -----  |  ----  |   ----    |   ----  |  ----   |   ----   |        ----          | ---- |
| 1 byte | 1 byte |  1 bytes  | 1 bytes | 4 bytes |  8 bytes | 0 or methodlen bytes | ...  |



## Examples

### 一、[Rpc Sync](https://github.com/lesismal/arpc/tree/master/examples/rpc_sync)

- [server](https://github.com/lesismal/arpc/blob/master/examples/rpc_sync/server/server.go)
- [client](https://github.com/lesismal/arpc/blob/master/examples/rpc_sync/client/client.go)

```sh
go run github.com/lesismal/arpc/examples/rpc_sync/server
go run github.com/lesismal/arpc/examples/rpc_sync/client
```


### 二、[Rpc Async](https://github.com/lesismal/arpc/tree/master/examples/rpc_async)

- [server](https://github.com/lesismal/arpc/blob/master/examples/rpc_async/server/server.go)
- [client](https://github.com/lesismal/arpc/blob/master/examples/rpc_async/client/client.go)

```sh
go run github.com/lesismal/arpc/examples/rpc_async/server
go run github.com/lesismal/arpc/examples/rpc_async/client
```


### 三、[Notify](https://github.com/lesismal/arpc/tree/master/examples/notify)

- [server](https://github.com/lesismal/arpc/blob/master/examples/notify/server/server.go)
- [client](https://github.com/lesismal/arpc/blob/master/examples/notify/client/client.go)

```sh
go run github.com/lesismal/arpc/examples/notify/server
go run github.com/lesismal/arpc/examples/notify/client
```


### 四、[Benchmark](https://github.com/lesismal/arpc/tree/master/examples/bench)

- [server](https://github.com/lesismal/arpc/blob/master/examples/bench/server/server.go)
- [client](https://github.com/lesismal/arpc/blob/master/examples/bench/client/client.go)

```sh
go run github.com/lesismal/arpc/examples/bench/server
go run github.com/lesismal/arpc/examples/bench/client
```


### 五、[Mixed](https://github.com/lesismal/arpc/tree/master/examples/mixed)

- [server](https://github.com/lesismal/arpc/blob/master/examples/mixed/server/server.go)
- [client](https://github.com/lesismal/arpc/blob/master/examples/mixed/client/client.go)

```sh
go run github.com/lesismal/arpc/examples/mixed/server
go run github.com/lesismal/arpc/examples/mixed/client
```


### 六、Echo
- server

```golang
package main

import (
	"log"
	"net"

	"github.com/lesismal/arpc"
)

const (
	addr = ":8888"
)

type HelloReq struct {
	Msg string
}

type HelloRsp struct {
	Msg string
}

func OnHello(ctx *arpc.Context) {
	req := &HelloReq{}
	rsp := &HelloRsp{}

	ctx.Bind(req)
	log.Printf("OnHello: \"%v\"", req.Msg)

	rsp.Msg = req.Msg
	ctx.Write(rsp)
}

func main() {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	svr := arpc.NewServer()
	svr.Handler.Handle("Hello", OnHello)
	svr.Serve(ln)
}
```

- client

```golang
package main

import (
	"log"
	"net"
	"time"

	"github.com/lesismal/arpc"
)

const (
	addr = "localhost:8888"
)

type HelloReq struct {
	Msg string
}

type HelloRsp struct {
	Msg string
}

func dialer() (net.Conn, error) {
	return net.DialTimeout("tcp", addr, time.Second*3)
}

func main() {
	client, err := arpc.NewClient(dialer)
	if err != nil {
		log.Println("NewClient failed:", err)
		return
	}

	client.Run()
	defer client.Stop()

	req := &HelloReq{Msg: "Hello"}
	rsp := &HelloRsp{}
	err = client.Call("Hello", req, rsp, time.Second*5)
	if err != nil {
		log.Println("Call Hello failed: %v", err)
	} else {
		log.Printf("HelloRsp: \"%v\"", rsp.Msg)
	}
}
```