# ARPC - Sync && Async Call supported

[![GoDoc][1]][2] [![MIT licensed][3]][4] [![Go Report Card][5]][6]

[1]: https://godoc.org/github.com/lesismal/arpc?status.svg
[2]: https://godoc.org/github.com/lesismal/arpc
[3]: https://img.shields.io/badge/license-MIT-blue.svg
[4]: LICENSE
[5]: https://goreportcard.com/badge/github.com/lesismal/arpc
[6]: https://goreportcard.com/report/github.com/lesismal/arpc


## Contents

- [ARPC - Sync && Async Call supported](#arpc---sync--async-call-supported)
	- [Contents](#contents)
	- [Features](#features)
	- [Header Layout](#header-layout)
	- [Installation](#installation)
	- [Quick start](#quick-start)
	- [API Examples](#api-examples)
		- [Register Routers](#register-routers)
		- [Async Response](#async-response)
		- [Client Call, CallAsync, Notify](#client-call-callasync-notify)
		- [Server Call, CallAsync, Notify](#server-call-callasync-notify)
		- [Broadcast && Ref Count Message](#broadcast--ref-count-message)
		- [Custom Net Protocol](#custom-net-protocol)
		- [Custom Codec](#custom-codec)
		- [Custom Logger](#custom-logger)
		- [Custom operations before conn's recv and send](#custom-operations-before-conns-recv-and-send)
		- [Custom arpc.Client's Reader from wrapping net.Conn](#custom-arpcclients-reader-from-wrapping-netconn)
		- [Custom arpc.Client's send queue capacity](#custom-arpcclients-send-queue-capacity)


## Features
- [x] Async Response
- [x] Client call Server Sync
- [x] Client call Server Async
- [x] Client notify Server
- [x] Server call Client Sync
- [x] Server call Client Async
- [x] Server notify Client
- [x] Mem Pools for Buffers and Objects
- [x] Broadcast Ref Count Message


## Header Layout

- LittleEndian

|  cmd   | async  | methodlen |  null   | bodylen | sequence |       method         | body |
| -----  |  ----  |   ----    |   ----  |  ----   |   ----   |        ----          | ---- |
| 1 byte | 1 byte |  1 bytes  | 1 bytes | 4 bytes |  8 bytes | 0 or methodlen bytes | ...  |



## Installation

1. Get and install arpc

```sh
$ go get -u github.com/lesismal/arpc
```

2. Import in your code:

```go
import "github.com/lesismal/arpc"
```


## Quick start
 
- start a [server](https://github.com/lesismal/arpc/blob/master/examples/rpc_sync/server/server.go)

```go
package main

import "github.com/lesismal/arpc"

func main() {
	server := arpc.NewServer()

	// register router
	server.Handler.Handle("/echo", func(ctx *arpc.Context) {
		str := ""
		ctx.Bind(&str)
		ctx.Write(str)
	})

	server.Run(":8888")
}
```

- start a [client](https://github.com/lesismal/arpc/blob/master/examples/rpc_sync/client/client.go)

```go
package main

import (
	"log"
	"net"
	"time"

	"github.com/lesismal/arpc"
)

func main() {
	client, err := arpc.NewClient(func() (net.Conn, error) {
		return net.DialTimeout("tcp", "localhost:8888", time.Second*3)
	})
	if err != nil {
		panic(err)
	}

	client.Run()
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
```



## API Examples

### Register Routers

```golang
var handler arpc.Handler

// package
handler = arpc.DefaultHandler
// server
handler = server.Handler
// client
handler = client.Handler

handler.Handle("/route", func(ctx *arpc.Context) { ... })
handler.Handle("/route2", func(ctx *arpc.Context) { ... })
handler.Handle("method", func(ctx *arpc.Context) { ... })
```

### Async Response

```golang
var handler arpc.Handler

// package
handler = arpc.DefaultHandler
// server
handler = server.Handler
// client
handler = client.Handler

func asyncResponse(ctx *arpc.Context, data interface{}) {
	defer ctx.Release()
	ctx.Write(data)
}

handler.Handle("/echo", func(ctx *arpc.Context) {
	req := ...
	ctx.Bind(req)
	clone := ctx.Clone()
	go asyncResponse(clone, req)
})
```

### Client Call, CallAsync, Notify

1. Call (Block, with timeout)

```golang
request := &Echo{...}
response := &Echo{}
timeout := time.Second*5
err := client.Call("/call/echo", request, response, timeout)
```

2. CallAsync (Nonblock, with callback and timeout)

```golang
request := &Echo{...}
timeout := time.Second*5
err := client.CallAsync("/call/echo", request, func(ctx *arpc.Context) {
	response := &Echo{}
	ctx.Bind(response)
	...	
}, timeout)
```

3. Notify (same as CallAsync without callback)

```golang
data := &Notify{...}
client.Notify("/notify", data, time.Second)
```

### Server Call, CallAsync, Notify

1. Get client and keep it in your application

```golang
var client *arpc.Client
server.Handler.Handle("/route", func(ctx *arpc.Context) {
	client = ctx.Client
	// release client
	client.OnDisconnected(func(c *arpc.Client){
		client = nil
	})
})

go func() {
	for {
		time.Sleep(time.Second)
		if client != nil {
			client.Call(...)
			client.CallAsync(...)
			client.Notify(...)
		}
	}
}()
```

2. Then Call/CallAsync/Notify

- [See Previous](#client-call-callasync-notify)

### Broadcast && Ref Count Message

- for more details:	[**server**](https://github.com/lesismal/arpc/blob/master/examples/broadcast/server/server.go) [**client**](https://github.com/lesismal/arpc/blob/master/examples/broadcast/client/client.go)

```golang
var mux = sync.RWMutex{}
var clientMap = make(map[*arpc.Client]struct{})

func broadcast() {
	msg := arpc.NewRefMessage(arpc.DefaultCodec, "/broadcast", fmt.Sprintf("broadcast msg %d", i))
	defer msg.Release()

	mux.RLock()
	for client := range clientMap {
		client.PushMsg(msg, arpc.TimeZero)
	}
	mux.RUnlock()
}
```

### Custom Net Protocol

```golang
// server
var ln net.Listener = ...
svr := arpc.NewServer()
svr.Serve(ln)

// client
dialer := func() (net.Conn, error) { 
	return ... 
}
client, err := arpc.NewClient(dialer)
```
 
### Custom Codec

```golang
// server
var codec arpc.Codec = ...

// package
arpc.DefaultCodec = codec

// server
svr := arpc.NewServer()
svr.Codec = codec

// client
client, err := arpc.NewClient(...)
client.Codec = codec
```

### Custom Logger

```golang
var logger arpc.Logger = ...
arpc.SetLogger(logger) // arpc.DefaultLogger = logger
``` 

### Custom operations before conn's recv and send

```golang
arpc.DefaultHandler.BeforeRecv(func(conn net.Conn) error) {
	// ...
})

arpc.DefaultHandler.BeforeSend(func(conn net.Conn) error) {
	// ...
})
```

### Custom arpc.Client's Reader from wrapping net.Conn 

```golang
arpc.DefaultHandler.SetReaderWrapper(func(conn net.Conn) io.Reader) {
	// ...
})
```

### Custom arpc.Client's send queue capacity 

```golang
arpc.DefaultHandler.SetSendQueueSize(4096)
```
