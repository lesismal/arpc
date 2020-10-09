// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package pubsub

import (
	"sync"

	"github.com/lesismal/arpc"
)

var (
	defaultTopicFactory = func(name string, sharding int32, data []byte, timestamp int64) Topic {
		return &topic{Name: name, Data: data, Sharding: sharding, Timestamp: timestamp}
	}
)

// TopicHandler .
type TopicHandler func(tp Topic)

// Topic .
type Topic interface {
	GetName() string
	SetName(string)
	GetData() []byte
	SetData([]byte)
	GetSharding() int32
	SetSharding(int32)
	GetTimestamp() int64
	SetTimestamp(int64)
}

// SetTopicFactory .
func SetTopicFactory(f func(name string, sharding int32, data []byte, timestamp int64) Topic) {
	if f == nil {
		panic("invalid Topic Factory: nil")
	}
	defaultTopicFactory = f
}

// // NewTopic .
// func NewTopic(name string, sharding int32, data []byte, timestamp int64) Topic {
// 	if defaultTopicFactory != nil {
// 		return defaultTopicFactory(name, sharding, data, timestamp)
// 	}
// 	return &topic{Name: name, Sharding: sharding, Data: data, Timestamp: timestamp}
// }

type topic struct {
	Name      string
	Data      []byte
	Sharding  int32
	Timestamp int64
}

func (tp *topic) GetName() string {
	if tp != nil {
		return tp.Name
	}
	return ""
}

func (tp *topic) SetName(name string) {
	if tp != nil {
		tp.Name = name
	}
}

func (tp *topic) GetData() []byte {
	if tp != nil {
		return tp.Data
	}
	return nil
}

func (tp *topic) SetData(data []byte) {
	if tp != nil {
		tp.Data = data
	}
}

func (tp *topic) GetSharding() int32 {
	if tp != nil {
		return tp.Sharding
	}
	return 0
}

func (tp *topic) SetSharding(sharding int32) {
	if tp != nil {
		tp.Sharding = sharding
	}
}

func (tp *topic) GetTimestamp() int64 {
	if tp != nil {
		return tp.Timestamp
	}
	return 0
}

func (tp *topic) SetTimestamp(timestamp int64) {
	if tp != nil {
		tp.Timestamp = timestamp
	}
}

// TopicAgent .
type TopicAgent struct {
	Name string

	mux sync.RWMutex

	clients map[*arpc.Client]empty
}

// Add .
func (t *TopicAgent) Add(c *arpc.Client) {
	t.mux.Lock()
	t.clients[c] = empty{}
	t.mux.Unlock()
}

// Delete .
func (t *TopicAgent) Delete(c *arpc.Client) {
	t.mux.Lock()
	delete(t.clients, c)
	t.mux.Unlock()
}

// Publish .
func (t *TopicAgent) Publish(s *Server, from *arpc.Client, topic Topic) {
	msg := arpc.NewMessage(arpc.CmdNotify, routePublish, topic, s.Codec)
	t.mux.RLock()
	for to := range t.clients {
		err := to.PushMsg(msg, arpc.TimeZero)
		if err != nil {
			if from != nil {
				arpc.DefaultLogger.Debug("[Publish] [%v] failed %v, from\t%v\tto\t%v", topic.GetName(), err, from.Conn.RemoteAddr(), to.Conn.RemoteAddr())
			} else {
				arpc.DefaultLogger.Debug("[Publish] [%v] failed %v, from Server to\t%v", topic.GetName(), err, to.Conn.RemoteAddr())
			}
		}
	}
	t.mux.RUnlock()
	if from != nil {
		arpc.DefaultLogger.Debug("%v [Publish] [%v] from\t%v", s.Handler.LogTag(), topic.GetName(), from.Conn.RemoteAddr())
	} else {
		arpc.DefaultLogger.Debug("%v [Publish] [%v] from Server", s.Handler.LogTag(), topic.GetName())
	}
}

// PublishToOne .
func (t *TopicAgent) PublishToOne(s *Server, from *arpc.Client, topic Topic) {
	msg := arpc.NewMessage(arpc.CmdNotify, routePublish, topic, s.Codec)
	t.mux.RLock()
	for to := range t.clients {
		err := to.PushMsg(msg, arpc.TimeZero)
		if err != nil {
			if from != nil {
				arpc.DefaultLogger.Debug("[PublishToOne] [%v] failed %v, from\t%v\tto\t%v", topic.GetName(), err, from.Conn.RemoteAddr(), to.Conn.RemoteAddr())
			} else {
				arpc.DefaultLogger.Debug("[PublishToOne] [%v] failed %v, from Server to\t%v", topic.GetName(), err, to.Conn.RemoteAddr())
			}
		} else {
			if from != nil {
				arpc.DefaultLogger.Debug("%v [PublishToOne] [%v] from\t%v", s.Handler.LogTag(), topic.GetName(), from.Conn.RemoteAddr())
			} else {
				arpc.DefaultLogger.Debug("%v [PublishToOne] [%v] from Server", s.Handler.LogTag(), topic.GetName())
			}
			break
		}
	}
	t.mux.RUnlock()
}

func newTopicAgent(topic string) *TopicAgent {
	return &TopicAgent{
		Name:    topic,
		clients: map[*arpc.Client]empty{},
	}
}
