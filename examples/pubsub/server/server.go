package main

import (
	"fmt"
	"time"

	"github.com/lesismal/arpc/pubsub"
)

var (
	address = "localhost:8888"

	password = "123qwe"
)

func main() {
	s := pubsub.NewServer()
	s.Password = password
	go func() {
		for i := 0; true; i++ {
			time.Sleep(time.Second)
			s.Publish("Broadcast", fmt.Sprintf("message from server %v", i))
		}
	}()
	s.Run(address)
}
