# ARPC - Sync && Async Call supported

[![GoDoc][1]][2] [![MIT licensed][3]][4] [![Go Report Card][5]][6]

[1]: https://godoc.org/github.com/lesismal/arpc?status.svg
[2]: https://godoc.org/github.com/lesismal/arpc
[3]: https://img.shields.io/badge/license-MIT-blue.svg
[4]: LICENSE
[5]: https://goreportcard.com/badge/github.com/lesismal/arpc
[6]: https://goreportcard.com/report/github.com/lesismal/arpc

## Protocol

- Header: LittleEndian

|  cmd   | async  | methodlen |  null   | bodylen | sequence |       method         | body |
| -----  |  ----  |   ----    |   ----  |  ----   |   ----   |        ----          | ---- |
| 1 byte | 1 byte |  1 bytes  | 1 bytes | 4 bytes |  8 bytes | 0 or methodlen bytes | ...  |






## Examples


### 一、Rpc Sync

- server

```sh
go run github.com/lesismal/arpc/examples/bench/server
```

- client

```sh
go run github.com/lesismal/arpc/examples/bench/client
```


### 二、Rpc Async

- server

```sh
go run github.com/lesismal/arpc/examples/bench/server
```

- client

```sh
go run github.com/lesismal/arpc/examples/bench/client
```


### 三、Notify

- server

```sh
go run github.com/lesismal/arpc/examples/notify/server
```

- client

```sh
go run github.com/lesismal/arpc/examples/notify/client
```


### 四、Benchmark

- server

```sh
go run github.com/lesismal/arpc/examples/bench/server
```

- client

```sh
go run github.com/lesismal/arpc/examples/bench/client
```


### 五、Echo
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