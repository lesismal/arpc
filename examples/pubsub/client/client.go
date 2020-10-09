package main

import (
	"fmt"
	"net"
	"time"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/pubsub"
)

var (
	address = "localhost:8888"

	password = "123qwe"

	topicName = "Broadcast"
)

func onTopic(topic pubsub.Topic) {
	arpc.DefaultLogger.Info("[OnTopic] [%v] \"%v\" %v", topic.GetName(), string(topic.GetData()), time.Unix(topic.GetTimestamp(), 0).Format("15:04:05"))
}

func consumer(c *pubsub.Client) {
	if err := c.Subscribe(topicName, onTopic, time.Second); err != nil {
		panic(err)
	}
}

func producer(c *pubsub.Client) {
	ticker := time.NewTicker(time.Second)
	for i := 0; true; i++ {
		_, ok := <-ticker.C
		if !ok {
			break
		}
		if i%5 == 0 {
			c.Publish(topicName, fmt.Sprintf("message %d", i), time.Second)
		} else {
			c.PublishToOne(topicName, fmt.Sprintf("message %d", i), time.Second)
		}
	}
}

func dialer() (net.Conn, error) {
	return net.DialTimeout("tcp", address, time.Second*3)
}

func newClient() *pubsub.Client {
	client, err := pubsub.NewClient(dialer)
	if err != nil {
		panic(err)
	}
	client.Password = password
	client.Run()

	// authentication
	err = client.Authenticate()
	if err != nil {
		panic(err)
	}

	return client
}

func main() {
	{
		for i := 0; i < 5; i++ {
			client := newClient()
			defer client.Stop()
			consumer(client)
		}
	}

	{
		client := newClient()
		defer client.Stop()
		go producer(client)
	}

	<-make(chan int)
}
