// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package pubsub

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/lesismal/arpc/log"
)

func TestPubSub(t *testing.T) {
	var (
		address   = "localhost:8888"
		password  = "123qwe"
		topicName = "test"
		chDone    = make(chan int)
	)

	go func() {
		s := NewServer()
		s.Password = password
		s.Run(address)
	}()

	go func() {
		time.Sleep(time.Second / 2)
		client := newClient(t, address, password)
		consumer(client, topicName, chDone)
	}()

	go func() {
		time.Sleep(time.Second / 2)
		client := newClient(t, address, password)
		producer(client, topicName, chDone)
	}()

	<-chDone
}

func newClient(t *testing.T, address, password string) *Client {
	client, err := NewClient(func() (net.Conn, error) {
		return net.DialTimeout("tcp", address, time.Second*3)
	})
	if err != nil {
		t.Fatal(err)
	}
	client.Password = password

	// authentication
	err = client.Authenticate()
	if err != nil {
		t.Fatal(err)
	}

	return client
}

func consumer(c *Client, topicName string, chDone chan int) {
	cnt := 0
	err := c.Subscribe(topicName, func(topic *Topic) {
		cnt++
		log.Info("[OnTopic] [%v] \"%v\" %v", topic.Name, string(topic.Data), time.Unix(topic.Timestamp, 0).Format("15:04:05"))
		if cnt >= 3 {
			close(chDone)
		}
	}, time.Second)

	if err != nil {
		panic(err)
	}
}

func producer(c *Client, topicName string, chDone chan int) {
	ticker := time.NewTicker(time.Second)
	for i := 0; true; i++ {
		select {
		case <-ticker.C:
			c.Publish(topicName, fmt.Sprintf("message %d", i), time.Second)
		case <-chDone:
			return
		}
	}
}
