package main

import (
	"net"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/extension/protocol/websocket"
	"github.com/lesismal/arpc/log"
)

type WebSocketServer struct {
	HostPort string
	svr      *arpc.Server
	ln       net.Listener
}

func (wss *WebSocketServer) Start() {
	go wss.svr.Serve(wss.ln)
}

func (wss *WebSocketServer) Stop() error {
	err := wss.svr.Stop()

	if err != nil {
		log.Error("websocket server stop error", err)
	}

	return err
}

func NewWebSocketServer(hostPort string, r *gin.RouterGroup) *WebSocketServer {

	ln, err := websocket.Listen(hostPort, nil)
	if err != nil {
		log.Error("websocket could not listen on given port %s", hostPort)
		os.Exit(1)
	}

	//register web socket route with arpc listener - note the  router group group needs to be registered to  `/ws`
	r.GET("", netHttpToGinAdapter(ln.(*websocket.Listener).Handler))

	log.Debug("websocket server starting on %s", hostPort)
	svr := arpc.NewServer()

	wss := WebSocketServer{
		HostPort: hostPort,
		svr:      svr,
		ln:       ln,
	}

	return &wss
}

func netHttpToGinAdapter(handler http.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		handler(c.Writer, c.Request)
	}
}
