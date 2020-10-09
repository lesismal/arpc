// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package pubsub

import (
	"net"
	"sync"
	"time"

	"github.com/lesismal/arpc"
)

// Client .
type Client struct {
	*arpc.Client

	Password string

	mux sync.RWMutex

	topicFactory func(name string, sharding int32, data []byte, timestamp int64) Topic

	topicHandlerMap map[string]TopicHandler

	onPublishHandler TopicHandler
}

// SetTopicFactory .
func (c *Client) SetTopicFactory(f func(name string, sharding int32, data []byte, timestamp int64) Topic) {
	if f == nil {
		panic("invalid Topic Factory: nil")
	}
	c.topicFactory = f
}

// NewTopic .
func (c *Client) NewTopic(name string, sharding int32, data []byte, timestamp int64) Topic {
	return c.topicFactory(name, sharding, data, timestamp)
}

// Authenticate .
func (c *Client) Authenticate() error {
	if c.Password == "" {
		return nil
	}
	err := c.Call(routeAuthenticate, c.Password, nil, time.Second*5)
	if err == nil {
		arpc.DefaultLogger.Info("%v [Authenticate] success to\t%v", c.Handler.LogTag(), c.Conn.RemoteAddr())
	} else {
		arpc.DefaultLogger.Info("%v [Authenticate] failed [%v] to\t%v", c.Handler.LogTag(), err, c.Conn.RemoteAddr())
	}
	return err
}

// Subscribe .
func (c *Client) Subscribe(topic string, h TopicHandler, timeout time.Duration) error {
	c.mux.Lock()
	// if _, ok := c.topicHandlerMap[topic]; ok {
	// 	panic(fmt.Errorf("handler exist for topic [%v]", topic))
	// }
	c.topicHandlerMap[topic] = h
	c.mux.Unlock()
	err := c.Call(routeSubscribe, c.NewTopic(topic, 0, nil, time.Now().Unix()), nil, timeout)
	if err == nil {
		arpc.DefaultLogger.Info("%v [Subscribe] [%v] success to\t%v", c.Handler.LogTag(), topic, c.Conn.RemoteAddr())
	} else {
		c.mux.Lock()
		delete(c.topicHandlerMap, topic)
		c.mux.Unlock()
		arpc.DefaultLogger.Error("%v [Subscribe] [%v] failed [%v] to\t%v", c.Handler.LogTag(), topic, err, c.Conn.RemoteAddr())
	}
	return err
}

// Unsubscribe .
func (c *Client) Unsubscribe(topic string, timeout time.Duration) error {
	err := c.Call(routeUnsubscribe, c.NewTopic(topic, 0, nil, time.Now().Unix()), nil, timeout)
	if err == nil {
		c.mux.Lock()
		delete(c.topicHandlerMap, topic)
		c.mux.Unlock()
		arpc.DefaultLogger.Info("%v[Unsubscribe] [%v] success to\t%v", c.Handler.LogTag(), topic, c.Conn.RemoteAddr())
	} else {
		arpc.DefaultLogger.Error("%v[Unsubscribe] [%v] failed [%v] to\t%v", c.Handler.LogTag(), topic, err, c.Conn.RemoteAddr())
	}
	return err
}

// Publish .
func (c *Client) Publish(topic string, v interface{}, timeout time.Duration) error {
	err := c.Call(routePublish, c.NewTopic(topic, 0, valueToBytes(c.Codec, v), time.Now().Unix()), nil, timeout)
	if err != nil {
		arpc.DefaultLogger.Error("%v [Publish] [%v] failed [%v] to\t%v", c.Handler.LogTag(), topic, err, c.Conn.RemoteAddr())
	}
	return err
}

// PublishToOne .
func (c *Client) PublishToOne(topic string, v interface{}, timeout time.Duration) error {
	err := c.Call(routePublishToOne, c.NewTopic(topic, 0, valueToBytes(c.Codec, v), time.Now().Unix()), nil, timeout)
	if err != nil {
		arpc.DefaultLogger.Error("%v [PublishToOne] [%v] failed [%v] to\t%v", c.Handler.LogTag(), topic, err, c.Conn.RemoteAddr())
	}
	return err
}

// OnPublish .
func (c *Client) OnPublish(h TopicHandler) {
	c.onPublishHandler = h
}

// onConfigItemChange .
func (c *Client) onConfigItemChange(tp Topic) {

}

func (c *Client) invalidTopic(topic string) bool {
	c.mux.RLock()
	_, ok := c.topicHandlerMap[topic]
	c.mux.RUnlock()

	return !ok
}

func (c *Client) initTopics() {
	c.mux.RLock()
	for topic := range c.topicHandlerMap {
		topicName := topic
		go safe(func() {
			for i := 0; i < 10; i++ {
				err := c.Call(routeSubscribe, c.NewTopic(topicName, 0, nil, time.Now().Unix()), nil, time.Second*10)
				if err == nil {
					arpc.DefaultLogger.Info("%v [Subscribe] [%v] success to\t%v", c.Handler.LogTag(), topicName, c.Conn.RemoteAddr())
					break
				} else {
					arpc.DefaultLogger.Info("%v [Subscribe] [%v] %v times failed [%v] to\t%v", c.Handler.LogTag(), topicName, i+1, err, c.Conn.RemoteAddr())
				}
				time.Sleep(time.Second)
			}
		})
	}
	c.mux.RUnlock()
}

func (c *Client) onPublish(ctx *arpc.Context) {
	topic := c.NewTopic("", 0, nil, 0)
	err := ctx.Bind(topic)
	if err != nil {
		arpc.DefaultLogger.Warn("%v [Publish IN] failed [%v], to\t%v", c.Handler.LogTag(), err, ctx.Client.Conn.RemoteAddr())
		return
	}

	if c.onPublishHandler == nil {
		c.mux.RLock()
		if h, ok := c.topicHandlerMap[topic.GetName()]; ok {
			h(topic)
			c.mux.RUnlock()
		} else {
			c.mux.RUnlock()
		}
	} else {
		c.onPublishHandler(topic)
	}
}

// NewClient .
func NewClient(dialer func() (net.Conn, error)) (*Client, error) {
	c, err := arpc.NewClient(dialer)
	if err != nil {
		return nil, err
	}
	c.Handler.SetLogTag("[APS CLI]")
	cli := &Client{
		Client:          c,
		topicFactory:    defaultTopicFactory,
		topicHandlerMap: map[string]TopicHandler{},
	}
	cli.Handler = cli.Handler.Clone()
	cli.Handler.Handle(routePublish, cli.onPublish)
	reconnected := false
	cli.Handler.HandleConnected(func(c *arpc.Client) {
		if reconnected == false {
			reconnected = true
		} else {
			if cli.Authenticate() == nil {
				cli.initTopics()
			}
		}
	})
	return cli, nil
}
