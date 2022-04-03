/*
 * Copyright 2021 CloudWeGo Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"time"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/codec"
	"github.com/lesismal/arpc/extension/pubsub"
	"github.com/lesismal/nbio/mempool"
	"github.com/lesismal/nbio/nbhttp"
	"github.com/lesismal/nbio/nbhttp/websocket"
	"github.com/lesismal/nbio/taskpool"
)

var (
	psServer *pubsub.Server

	executePool = taskpool.NewMixedPool(1024*4, 1, 1024*runtime.NumCPU())

	keepaliveTime = 60 * time.Second
)

func onWebsocket(w http.ResponseWriter, r *http.Request) {
	// if err := authenticate(...); err != nil {
	// 	some log
	// 	return
	// }

	upgrader := newUpgrader()
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		conn.Close()
		return
	}
	wsConn := conn.(*websocket.Conn)
	wsConn.SetReadDeadline(time.Now().Add(keepaliveTime))
}

func main() {
	// disable frameworks' log
	{
		// alog.SetLevel(alog.LevelNone)
		// nlog.SetLevel(nlog.LevelNone)
	}

	// init pubsub server handler for nbhttp
	{
		// arpc.DefaultAllocator = mempool.DefaultMemPool
		// psServer.Handler.EnablePool(true)
		arpc.SetAsyncResponse(true)
		psServer = pubsub.NewServer()
		// must set async write for nbio
		psServer.Handler.SetAsyncWrite(false)
	}

	mux := &http.ServeMux{}
	mux.HandleFunc("/ws", onWebsocket)

	svr := nbhttp.NewEngine(nbhttp.Config{
		Network:                 "tcp",
		Addrs:                   []string{"localhost:8888"},
		Handler:                 mux,
		ReleaseWebsocketPayload: true,
	})

	err := svr.Start()
	if err != nil {
		fmt.Printf("nbio.Start failed: %v\n", err)
		return
	}

	go func() {
		topicName := "Broadcast"
		for i := 0; true; i++ {
			time.Sleep(time.Second)
			psServer.Publish(topicName, fmt.Sprintf("message from server %v", i))
		}
	}()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	<-interrupt
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	svr.Shutdown(ctx)
}

type WebsocketConn struct {
	*websocket.Conn
}

func (c *WebsocketConn) Write(data []byte) (int, error) {
	err := c.Conn.WriteMessage(websocket.BinaryMessage, data)
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

// Session .
type Session struct {
	*arpc.Client
	cache []byte
}

func newUpgrader() *websocket.Upgrader {
	u := websocket.NewUpgrader()
	u.OnOpen(onOpen)
	u.OnMessage(onMessage)
	u.OnClose(onClose)
	return u
}

func onOpen(c *websocket.Conn) {
	client := &arpc.Client{Conn: &WebsocketConn{c}, Codec: codec.DefaultCodec, Handler: psServer.Handler}
	client.SetState(true)
	session := &Session{
		Client: client,
	}
	c.SetSession(session)
}

func onClose(c *websocket.Conn, err error) {
	iSession := c.Session()
	if iSession == nil {
		c.Close()
		return
	}
	session, ok := iSession.(*Session)
	if ok {
		if session.cache != nil {
			mempool.Free(session.cache)
			session.cache = nil
		}
		psServer.Handler.OnDisconnected(session.Client)
	}
}

func onMessage(c *websocket.Conn, mt websocket.MessageType, data []byte) {
	iSession := c.Session()
	if iSession == nil {
		c.Close()
		return
	}

	session := iSession.(*Session)
	start := 0
	if session.cache != nil {
		session.cache = append(session.cache, data...)
		data = session.cache
	}

	recieved := false
	for {
		if len(data)-start < arpc.HeadLen {
			goto Exit
		}
		header := arpc.Header(data[start : start+4])
		total := arpc.HeadLen + header.BodyLen()
		if len(data)-start < total {
			goto Exit
		}

		buffer := mempool.Malloc(total)
		copy(buffer, data[start:start+total])
		start += total
		msg := psServer.Handler.NewMessageWithBuffer(buffer)
		psServer.Handler.OnMessage(session.Client, msg)
		recieved = true
	}

Exit:
	if recieved {
		c.SetReadDeadline(time.Now().Add(keepaliveTime))
	}
	if session.cache != nil {
		if start == len(data) {
			mempool.Free(data)
			session.cache = nil
		} else {
			left := mempool.Malloc(len(data) - start)
			copy(left, data[start:])
			mempool.Free(data)
			session.cache = left
		}
	} else if start < len(data) {
		left := mempool.Malloc(len(data) - start)
		copy(left, data[start:])
		mempool.Free(data)
		session.cache = left
	}
}
