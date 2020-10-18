// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package pubsub

import (
	"encoding/binary"
	"sync"
	"time"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/log"
	"github.com/lesismal/arpc/util"
)

const (
	// MaxTopicNameLen .
	MaxTopicNameLen = 1024
)

// TopicHandler .
type TopicHandler func(tp *Topic)

// Topic .
type Topic struct {
	Name      string
	Data      []byte
	Timestamp int64
	raw       []byte
}

func (tp *Topic) toBytes() ([]byte, error) {
	nameLen := uint16(len(tp.Name))
	tail := make([]byte, len(tp.Name)+10)
	copy(tail, tp.Name)
	binary.LittleEndian.PutUint16(tail[nameLen:], nameLen)
	binary.LittleEndian.PutUint64(tail[nameLen+2:], uint64(tp.Timestamp))
	dataLen := len(tp.Data)
	tp.Data = append(tp.Data, tail...)
	tp.raw = tp.Data
	tp.Data = tp.raw[:dataLen]
	return tp.raw, nil
}

func (tp *Topic) fromBytes(data []byte) error {
	if len(data) < 10 {
		return ErrInvalidTopicBytes
	}
	nameLen := int(binary.LittleEndian.Uint16(data[len(data)-10:]))
	if nameLen == 0 || nameLen > MaxTopicNameLen {
		return ErrInvalidTopicNameLength
	}
	tp.Timestamp = int64(binary.LittleEndian.Uint64(data[len(data)-8:]))
	tp.Name = string(data[len(data)-nameLen-10 : len(data)-10])
	tp.Data = data[:len(data)-nameLen-10]
	tp.raw = data
	return nil
}

func newTopic(topicName string, data []byte) (*Topic, error) {
	if topicName == "" {
		return nil, ErrInvalidTopicEmpty
	}
	if len(topicName) > MaxTopicNameLen {
		return nil, ErrInvalidTopicNameLength
	}
	return &Topic{Name: topicName, Data: data, Timestamp: time.Now().UnixNano()}, nil
}

// TopicAgent .
type TopicAgent struct {
	Name string

	mux sync.RWMutex

	clients map[*arpc.Client]util.Empty
}

// Add .
func (t *TopicAgent) Add(c *arpc.Client) {
	t.mux.Lock()
	t.clients[c] = util.Empty{}
	t.mux.Unlock()
}

// Delete .
func (t *TopicAgent) Delete(c *arpc.Client) {
	t.mux.Lock()
	delete(t.clients, c)
	t.mux.Unlock()
}

// Publish .
func (t *TopicAgent) Publish(s *Server, from *arpc.Client, topic *Topic) {
	msg := s.NewMessage(arpc.CmdNotify, routePublish, topic.raw)
	t.mux.RLock()
	for to := range t.clients {
		err := to.PushMsg(msg, arpc.TimeZero)
		if err != nil {
			if from != nil {
				log.Error("[Publish] [topic: '%v'] failed %v, from\t%v\tto\t%v", topic.Name, err, from.Conn.RemoteAddr(), to.Conn.RemoteAddr())
			} else {
				log.Error("[Publish] [topic: '%v'] failed %v, from Server to\t%v", topic.Name, err, to.Conn.RemoteAddr())
			}
		}
	}
	t.mux.RUnlock()
	if from != nil {
		log.Debug("%v [Publish] [topic: '%v'] from\t%v", s.Handler.LogTag(), topic.Name, from.Conn.RemoteAddr())
	} else {
		log.Debug("%v [Publish] [topic: '%v'] from Server", s.Handler.LogTag(), topic.Name)
	}
}

// PublishToOne .
func (t *TopicAgent) PublishToOne(s *Server, from *arpc.Client, topic *Topic) {
	msg := s.NewMessage(arpc.CmdNotify, routePublish, topic.raw)
	t.mux.RLock()
	for to := range t.clients {
		err := to.PushMsg(msg, arpc.TimeZero)
		if err != nil {
			if from != nil {
				log.Error("[PublishToOne] [topic: '%v'] failed %v, from\t%v\tto\t%v", topic.Name, err, from.Conn.RemoteAddr(), to.Conn.RemoteAddr())
			} else {
				log.Error("[PublishToOne] [topic: '%v'] failed %v, from Server to\t%v", topic.Name, err, to.Conn.RemoteAddr())
			}
		} else {
			if from != nil {
				log.Debug("%v [PublishToOne] [topic: '%v'] from\t%v", s.Handler.LogTag(), topic.Name, from.Conn.RemoteAddr())
			} else {
				log.Debug("%v [PublishToOne] [topic: '%v'] from Server", s.Handler.LogTag(), topic.Name)
			}
			break
		}
	}
	t.mux.RUnlock()
}

func newTopicAgent(topic string) *TopicAgent {
	return &TopicAgent{
		Name:    topic,
		clients: map[*arpc.Client]util.Empty{},
	}
}
