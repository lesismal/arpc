# ARPC - More Effective Network Communication 

[![GoDoc][1]][2] [![MIT licensed][3]][4] [![Build Status][5]][6] [![Go Report Card][7]][8] [![Coverage Statusd][9]][10]

[1]: https://godoc.org/github.com/lesismal/arpc?status.svg
[2]: https://godoc.org/github.com/lesismal/arpc
[3]: https://img.shields.io/badge/license-MIT-blue.svg
[4]: LICENSE
[5]: https://travis-ci.org/lesismal/arpc.svg?branch=master
[6]: https://travis-ci.org/lesismal/arpc
[7]: https://goreportcard.com/badge/github.com/lesismal/arpc
[8]: https://goreportcard.com/report/github.com/lesismal/arpc
[9]: https://codecov.io/gh/lesismal/arpc/branch/master/graph/badge.svg
[10]: https://codecov.io/gh/lesismal/arpc


| pattern | directions | description |
| ------ | ----- | ---- |
|  call  | c -> s<br>s -> c | request and response |
| notify | c -> s<br>s -> c | request without response |


## Contents

- [ARPC - More Effective Network Communication](#arpc---more--effective-network-communication)
	- [Contents](#contents)
	- [Features](#features)
	- [Header Layout](#header-layout)
	- [Installation](#installation)
	- [Quick start](#quick-start)
	- [API Examples](#api-examples)
		- [Register Routers](#register-routers)
		- [Client Call, CallAsync, Notify](#client-call-callasync-notify)
		- [Server Call, CallAsync, Notify](#server-call-callasync-notify)
		- [Broadcast - Notify](#broadcast---notify)
		- [Async Response](#async-response)
		- [Handle New Connection](#handle-new-connection)
		- [Handle Disconnected](#handle-disconnected)
		- [Handle Client's send queue overstock](#handle-clients-send-queue-overstock)
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
- [x] Batch Write | Writev | net.Buffers 
- [x] Broadcast

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
		if err := ctx.Bind(&str); err == nil {
			ctx.Write(str)
		}
	})

	server.Run(":8888")
}
```

- start a [client](https://github.com/lesismal/arpc/blob/master/examples/rpc/client/client.go)

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



### Client Call, CallAsync, Notify

1. Call (Block, with timeout/context)

```golang
request := &Echo{...}
response := &Echo{}
timeout := time.Second*5
err := client.Call("/call/echo", request, response, timeout)
// ctx, cancel := context.WithTimeout(context.Background(), time.Second)
// defer cancel()
// err := client.CallWith(ctx, "/call/echo", request, response)
```

2. CallAsync (Nonblock, with callback and timeout/context)

```golang
request := &Echo{...}

timeout := time.Second*5
err := client.CallAsync("/call/echo", request, func(ctx *arpc.Context) {
	response := &Echo{}
	ctx.Bind(response)
	...	
}, timeout)

// ctx, cancel := context.WithTimeout(context.Background(), time.Second)
// defer cancel()
// err := client.CallAsyncWith(ctx, "/call/echo", request, func(ctx *arpc.Context) {
// 	response := &Echo{}
// 	ctx.Bind(response)
// 	...	
// })
```

3. Notify (same as CallAsync with timeout/context, without callback)

```golang
data := &Notify{...}
client.Notify("/notify", data, time.Second)
// ctx, cancel := context.WithTimeout(context.Background(), time.Second)
// defer cancel()
// client.NotifyWith(ctx, "/notify", data)
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

### Broadcast - Notify

- for more details:	[**server**](https://github.com/lesismal/arpc/blob/master/examples/broadcast/server/server.go) [**client**](https://github.com/lesismal/arpc/blob/master/examples/broadcast/client/client.go)

```golang
var mux = sync.RWMutex{}
var clientMap = make(map[*arpc.Client]struct{})

func broadcast() {
	msg := arpc.NewMessage(arpc.CmdNotify, "/broadcast", fmt.Sprintf("broadcast msg %d", i), nil)
	mux.RLock()
	for client := range clientMap {
		client.PushMsg(msg, arpc.TimeZero)
	}
	mux.RUnlock()
}
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
	ctx.Write(data)
}

handler.Handle("/echo", func(ctx *arpc.Context) {
	req := ...
	err := ctx.Bind(req)
	if err == nil {
		go asyncResponse(ctx, req)
	}
})
```


### Handle New Connection

```golang
// package
arpc.DefaultHandler.HandleConnected(func(c *arpc.Client) {
	...
})

// server
svr := arpc.NewServer()
svr.Handler.HandleConnected(func(c *arpc.Client) {
	...
})

// client
client, err := arpc.NewClient(...)
client.Handler.HandleConnected(func(c *arpc.Client) {
	...
})
```

### Handle Disconnected

```golang
// package
arpc.DefaultHandler.HandleDisconnected(func(c *arpc.Client) {
	...
})

// server
svr := arpc.NewServer()
svr.Handler.HandleDisconnected(func(c *arpc.Client) {
	...
})

// client
client, err := arpc.NewClient(...)
client.Handler.HandleDisconnected(func(c *arpc.Client) {
	...
})
```

### Handle Client's send queue overstock

```golang
// package
arpc.DefaultHandler.HandleOverstock(func(c *arpc.Client) {
	...
})

// server
svr := arpc.NewServer()
svr.Handler.HandleOverstock(func(c *arpc.Client) {
	...
})

// client
client, err := arpc.NewClient(...)
client.Handler.HandleOverstock(func(c *arpc.Client) {
	...
})
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
