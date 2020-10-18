// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package pubsub

import (
	"sync"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/log"
	"github.com/lesismal/arpc/util"
)

var (
	addClient interface{} = true
)

type clientTopics struct {
	mux         sync.RWMutex
	topicAgents map[string]*TopicAgent
}

// Server .
type Server struct {
	*arpc.Server

	Password string

	psmux sync.RWMutex

	topics map[string]*TopicAgent

	clients map[*arpc.Client]map[string]*TopicAgent
}

// Publish topic
func (s *Server) Publish(topicName string, v interface{}) error {
	topic, err := newTopic(topicName, util.ValueToBytes(s.Codec, v))
	if err != nil {
		return err
	}
	_, err = topic.toBytes()
	if err != nil {
		return err
	}
	s.getOrMakeTopic(topic.Name).Publish(s, nil, topic)
	return nil
}

// PublishToOne topic
func (s *Server) PublishToOne(topicName string, v interface{}) error {
	topic, err := newTopic(topicName, util.ValueToBytes(s.Codec, v))
	if err != nil {
		return err
	}
	_, err = topic.toBytes()
	if err != nil {
		return err
	}
	s.getOrMakeTopic(topic.Name).PublishToOne(s, nil, topic)
	return nil
}

func (s *Server) invalid(ctx *arpc.Context) bool {
	return ctx.Client.UserData == nil
}

func (s *Server) onAuthenticate(ctx *arpc.Context) {
	defer util.Recover()

	passwd := ""
	err := ctx.Bind(&passwd)
	if err != nil {
		ctx.Error(ErrInvalidPassword)
		log.Error("%v [Authenticate] failed: %v, from\t%v", s.Handler.LogTag(), err, ctx.Client.Conn.RemoteAddr())
		return
	}

	if passwd == s.Password {
		s.addClient(ctx.Client)
		ctx.Write(nil)
		log.Info("%v [Authenticate] success from\t%v", s.Handler.LogTag(), ctx.Client.Conn.RemoteAddr())
	} else {
		ctx.Error(ErrInvalidPassword)
		log.Error("%v [Authenticate] failed: %v, from\t%v", s.Handler.LogTag(), ErrInvalidPassword, ctx.Client.Conn.RemoteAddr())
	}
}

func (s *Server) onSubscribe(ctx *arpc.Context) {
	defer util.Recover()

	if s.invalid(ctx) {
		log.Error("%v [Subscribe] invalid ctx from\t%v", s.Handler.LogTag(), ctx.Client.Conn.RemoteAddr())
		return
	}

	topic := &Topic{}
	err := topic.fromBytes(ctx.Body())
	if err != nil {
		ctx.Error(err)
		log.Error("%v [Subscribe] failed: %v, from\t%v", s.Handler.LogTag(), err, ctx.Client.Conn.RemoteAddr())
		return
	}
	topicName := topic.Name
	if topicName != "" {
		cts := ctx.Client.UserData.(*clientTopics)
		cts.mux.Lock()
		tp, ok := cts.topicAgents[topicName]
		if !ok {
			tp = s.getOrMakeTopic(topicName)
			cts.topicAgents[topicName] = tp
			cts.mux.Unlock()
			tp.Add(ctx.Client)
			ctx.Write(nil)
			log.Info("%v [Subscribe] [topic: '%v'] success from\t%v", s.Handler.LogTag(), topicName, ctx.Client.Conn.RemoteAddr())
		} else {
			cts.mux.Unlock()
			ctx.Write(nil)
		}
	} else {
		ctx.Error(ErrInvalidTopicEmpty)
		log.Error("%v [Subscribe] failed: %v, from\t%v", s.Handler.LogTag(), ErrInvalidTopicEmpty, ctx.Client.Conn.RemoteAddr())
	}
}

func (s *Server) onUnsubscribe(ctx *arpc.Context) {
	defer util.Recover()

	if s.invalid(ctx) {
		log.Error("%v [Unsubscribe] invalid ctx from\t%v", s.Handler.LogTag(), ctx.Client.Conn.RemoteAddr())
		return
	}

	topic := &Topic{}
	err := topic.fromBytes(ctx.Body())
	if err != nil {
		ctx.Error(err)
		log.Error("%v [Unsubscribe] failed: %v, from\t%v", s.Handler.LogTag(), err, ctx.Client.Conn.RemoteAddr())
		return
	}
	topicName := topic.Name
	if topicName != "" {
		cts := ctx.Client.UserData.(*clientTopics)
		cts.mux.Lock()
		if ta, ok := cts.topicAgents[topicName]; ok {
			delete(cts.topicAgents, topicName)
			cts.mux.Unlock()
			ta.Delete(ctx.Client)
			ctx.Write(nil)
			log.Info("%v [Unsubscribe] [topic: '%v'] success from\t%v", s.Handler.LogTag(), ta.Name, ctx.Client.Conn.RemoteAddr())
		} else {
			cts.mux.Unlock()
			ctx.Write(nil)
		}
	} else {
		ctx.Error(ErrInvalidTopicEmpty)
		log.Error("%v [Unsubscribe] failed: %v, from\t%v", s.Handler.LogTag(), ErrInvalidTopicEmpty, ctx.Client.Conn.RemoteAddr())
	}
}

func (s *Server) onPublish(ctx *arpc.Context) {
	defer util.Recover()

	if s.invalid(ctx) {
		log.Error("%v [Publish] invalid ctx from\t%v", s.Handler.LogTag(), ctx.Client.Conn.RemoteAddr())
		return
	}

	topic := &Topic{}
	err := topic.fromBytes(ctx.Body())
	if err != nil {
		ctx.Error(err)
		log.Error("%v [Publish] failed: %v, from\t%v", s.Handler.LogTag(), err, ctx.Client.Conn.RemoteAddr())
		return
	}

	topicName := topic.Name
	if topicName != "" {
		ctx.Write(nil)
		s.getOrMakeTopic(topic.Name).Publish(s, ctx.Client, topic)
		// log.Debug("%v [Publish] [%v], %v from\t%v", s.Handler.LogTag(), topicName, ctx.Client.Conn.RemoteAddr())
	} else {
		ctx.Error(ErrInvalidTopicEmpty)
		log.Error("%v [Publish] failed: %v, from\t%v", s.Handler.LogTag(), ErrInvalidTopicEmpty, ctx.Client.Conn.RemoteAddr())
	}
}

func (s *Server) onPublishToOne(ctx *arpc.Context) {
	defer util.Recover()

	if s.invalid(ctx) {
		log.Error("%v [PublishToOne] invalid ctx from\t%v", s.Handler.LogTag(), ctx.Client.Conn.RemoteAddr())
		return
	}
	topic := &Topic{}
	err := topic.fromBytes(ctx.Body())
	if err != nil {
		ctx.Error(err)
		log.Error("%v [PublishToOne] failed: %v, from\t%v", s.Handler.LogTag(), err, ctx.Client.Conn.RemoteAddr())
		return
	}

	topicName := topic.Name
	if topicName != "" {
		ctx.Write(nil)
		s.getOrMakeTopic(topic.Name).PublishToOne(s, ctx.Client, topic)
		// log.Debug("%v [Publish] [%v], %v from\t%v", s.Handler.LogTag(), topicName, ctx.Client.Conn.RemoteAddr())
	} else {
		ctx.Error(ErrInvalidTopicEmpty)
		log.Error("%v [PublishToOne] failed: %v, from\t%v", s.Handler.LogTag(), ErrInvalidTopicEmpty, ctx.Client.Conn.RemoteAddr())
	}
}

func (s *Server) getTopic(topic string) (*TopicAgent, bool) {
	s.psmux.RLock()
	tp, ok := s.topics[topic]
	s.psmux.RUnlock()
	return tp, ok
}

func (s *Server) getOrMakeTopic(topic string) *TopicAgent {
	s.psmux.RLock()
	tp, ok := s.topics[topic]
	s.psmux.RUnlock()
	if !ok {
		s.psmux.Lock()
		tp, ok = s.topics[topic]
		if !ok {
			tp = newTopicAgent(topic)
			s.topics[topic] = tp
		}
		s.psmux.Unlock()
	}
	return tp
}

// addClient .
func (s *Server) addClient(c *arpc.Client) {
	c.UserData = &clientTopics{
		topicAgents: map[string]*TopicAgent{},
	}
}

func (s *Server) deleteClient(c *arpc.Client) {
	if c.UserData == nil {
		return
	}

	defer util.Recover()

	cts := c.UserData.(*clientTopics)
	cts.mux.RLock()
	defer cts.mux.RUnlock()
	for _, tp := range cts.topicAgents {
		tp.Delete(c)
		log.Info("%v [Disconnected Unsubscribe] [topic: '%v'] from\t%v", s.Handler.LogTag(), tp.Name, c.Conn.RemoteAddr())
	}
}

// NewServer .
func NewServer() *Server {
	s := arpc.NewServer()
	svr := &Server{
		Server:  s,
		topics:  map[string]*TopicAgent{},
		clients: map[*arpc.Client]map[string]*TopicAgent{},
	}
	s.Handler.SetLogTag("[APS SVR]")
	svr.Handler.Handle(routeAuthenticate, svr.onAuthenticate)
	svr.Handler.Handle(routeSubscribe, svr.onSubscribe)
	svr.Handler.Handle(routeUnsubscribe, svr.onUnsubscribe)
	svr.Handler.Handle(routePublish, svr.onPublish)
	svr.Handler.Handle(routePublishToOne, svr.onPublishToOne)

	svr.Handler.HandleDisconnected(svr.deleteClient)
	return svr
}
