package main

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/log"
)

type Message struct {
	User      uint64 `json:"user"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

// NewMessage .
func NewMessage(user uint64, msg string) *Message {
	return &Message{
		User:      user,
		Message:   msg,
		Timestamp: time.Now().UnixNano(),
	}
}

func main() {
	hostPort := "localhost:8888"

	router := gin.New()
	wssGroup := router.Group("/ws")
	wss := NewWebSocketServer(hostPort, wssGroup)
	defer wss.Stop()

	router.GET("/", func(c *gin.Context) {
		c.File("../webchat/chat.html")
	})

	router.GET("arpc.js", func(c *gin.Context) {
		c.File("../webchat/arpc.js")
	})

	room := NewRoom().Run()

	wss.svr.Handler.Handle("/chat/user/say", func(ctx *arpc.Context) {
		if user, ok := ctx.Client.Get("user"); ok {
			if userid, ok := user.(uint64); ok {
				msg := &Message{User: userid}
				err := ctx.Bind(&msg.Message)
				if err == nil {
					room.Broadcast(msg)
				}
			}
		}
	})

	wss.svr.Handler.HandleConnected(func(c *arpc.Client) {
		room.Enter(c)
	})

	wss.svr.Handler.HandleDisconnected(func(c *arpc.Client) {
		room.Leave(c)
	})

	//GET /room returns an array of user ids connected to the server
	router.GET("/room", func(c *gin.Context) {
		users := make([]int64, len(room.users))
		for _, user := range room.users {
			users = append(users, int64(user))
		}

		c.JSON(200, gin.H{
			"users": users,
		})
	})

	log.Info("Starting server at %v", hostPort)
	wss.Start()
	router.Run(hostPort)
	log.Debug("Server stopped")
}
