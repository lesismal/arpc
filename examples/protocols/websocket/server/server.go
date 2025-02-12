package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gofiber/fiber/v3"
	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/extension/protocol/websocket"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

var handlerType = flag.String("h", "fiber", "use std/gin/fiber listener, fiber as default")

func main() {
	flag.Parse()

	var ln net.Listener

	switch *handlerType {
	case "gin":
		ln = ginListener()
	case "fiber":
		ln = fberListener()
	default:
		ln = stdListener()
	}

	svr := arpc.NewServer()
	// register router
	svr.Handler.Handle("/call/echo", func(ctx *arpc.Context) {
		str := ""
		err := ctx.Bind(&str)
		ctx.Write(str)
		log.Printf("/call/echo: \"%v\", error: %v", str, err)
	})

	svr.Handler.Handle("/notify", func(ctx *arpc.Context) {
		str := ""
		err := ctx.Bind(&str)
		log.Printf("/notify: \"%v\", error: %v", str, err)
	})

	svr.Handler.HandleConnected(func(c *arpc.Client) {
		// go c.Call("/server/call", "server call", 0)
		go c.Notify("/server/notify", time.Now().Format("Welcome! Now Is: 2006-01-02 15:04:05.000"), 0)
	})

	svr.Serve(ln)
}

func ginListener() net.Listener {
	router := gin.New()
	ln, _ := websocket.Listen("localhost:8888", nil)
	router.GET("/ws", func(c *gin.Context) {
		w := c.Writer
		r := c.Request
		ln.(*websocket.Listener).Handler(w, r)
	})
	go func() {
		err := router.Run("localhost:8888")
		if err != nil {
			log.Fatalf("router.Run failed: %v", err)
		}
	}()
	return ln
}

type arpcHTTPHandler struct {
	ln *websocket.Listener
}

func (ah *arpcHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ah.ln.Handler(w, r)
}

func fberListener() net.Listener {
	router := fiber.New()
	ln, _ := websocket.Listen("localhost:8888", nil)
	router.Get("/ws", func(c fiber.Ctx) error {
		handler := fasthttpadaptor.NewFastHTTPHandler(&arpcHTTPHandler{ln: ln.(*websocket.Listener)})
		handler(c.RequestCtx())
		return nil

	})
	go func() {
		err := router.Listen("localhost:8888")
		if err != nil {
			log.Fatalf("router.Run failed: %v", err)
		}
	}()
	return ln
}

func stdListener() net.Listener {
	ln, _ := websocket.Listen("localhost:8888", nil)
	http.HandleFunc("/ws", ln.(*websocket.Listener).Handler)
	go func() {
		err := http.ListenAndServe("localhost:8888", nil)
		if err != nil {
			log.Fatal("ListenAndServe: ", err)
		}
	}()
	return ln
}
