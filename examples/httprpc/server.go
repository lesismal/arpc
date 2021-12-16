package main

import (
	"net/http"
	"time"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/codec"
	"github.com/lesismal/arpc/extension/arpchttp"
	"github.com/lesismal/arpc/extension/protocol/websocket"
	"github.com/lesismal/arpc/log"
)

func main() {
	svr := arpc.NewServer()
	wsHandler := svr.Handler
	wsHandler.Handle("/ws/echo", func(ctx *arpc.Context) {
		log.Info("/ws/echo: %v", string(ctx.Body()))
		ctx.Write(ctx.Body())
	})
	wsHandler.Handle("/ws/notify", func(ctx *arpc.Context) {
		log.Info("/ws/notify: %v", string(ctx.Body()))
	})

	httpHandler := arpc.DefaultHandler
	httpHandler.SetAsyncWrite(false)
	httpHandler.Handle("/http/echo", func(ctx *arpc.Context) {
		log.Info("/http/echo: %v", string(ctx.Body()))
		ctx.Write(ctx.Body())
	})
	httpHandler.Handle("/http/notify", func(ctx *arpc.Context) {
		log.Info("/http/notify: %v", string(ctx.Body()))
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Info("url: %v", r.URL.String())
		if r.URL.Path == "/" {
			http.ServeFile(w, r, "chat.html")
		} else if r.URL.Path == "/arpc.js" {
			http.ServeFile(w, r, "arpc.js")
		} else {
			http.NotFound(w, r)
		}
	})

	ln, _ := websocket.Listen("localhost:8888", nil)
	go func() {
		err := http.ListenAndServe("localhost:8888", nil)
		if err != nil {
			log.Error("ListenAndServe: %v", err)
		}
	}()
	time.Sleep(time.Second / 100)

	http.HandleFunc("/ws/rpc", ln.(*websocket.Listener).Handler)
	http.HandleFunc("/http/rpc", arpchttp.Handler(httpHandler, codec.DefaultCodec))

	svr.Serve(ln)
}
