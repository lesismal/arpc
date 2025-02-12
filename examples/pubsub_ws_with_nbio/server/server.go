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
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
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

	executePool = taskpool.New(1024*4, 1024)

	keepaliveTime = 60 * time.Second

	upgrader = newUpgrader()
)

func onWebsocket(w http.ResponseWriter, r *http.Request) {
	// if err := authenticate(...); err != nil {
	// 	some log
	// 	return
	// }

	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		wsConn.Close()
		return
	}
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

func (c *WebsocketConn) Read(buf []byte) (int, error) {
	return -1, errors.New("unsupported")
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
	pCache *[]byte
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
		if session.pCache != nil {
			mempool.Free(session.pCache)
			session.pCache = nil
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
	if session.pCache != nil {
		session.pCache = mempool.Append(session.pCache, data...)
		data = *session.pCache
	}

	received := false
	for {
		if len(data)-start < arpc.HeadLen {
			goto Exit
		}
		header := arpc.Header(data[start : start+4])
		total := arpc.HeadLen + header.BodyLen()
		if len(data)-start < total {
			goto Exit
		}

		pBuffer := mempool.Malloc(total)
		copy(*pBuffer, data[start:start+total])
		start += total
		msg := psServer.Handler.NewMessageWithBuffer(pBuffer)
		psServer.Handler.OnMessage(session.Client, msg)
		received = true
	}

Exit:
	if received {
		c.SetReadDeadline(time.Now().Add(keepaliveTime))
	}
	if session.pCache != nil {
		if start == len(data) {
			mempool.Free(session.pCache)
			session.pCache = nil
		} else {
			pLeft := mempool.Malloc(len(data) - start)
			copy(*pLeft, data[start:])
			mempool.Free(session.pCache)
			session.pCache = pLeft
		}
	} else if start < len(data) {
		pLeft := mempool.Malloc(len(data) - start)
		copy(*pLeft, data[start:])
		mempool.Free(session.pCache)
		session.pCache = pLeft
	}
}
