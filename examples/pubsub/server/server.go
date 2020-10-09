package main

import (
	"github.com/lesismal/arpc/pubsub"
)

var (
	address = "localhost:8888"

	password = "123qwe"
)

func main() {
	s := pubsub.NewServer()
	s.Password = password
	s.Run(address)
}
