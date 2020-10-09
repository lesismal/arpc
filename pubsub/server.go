// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package pubsub

import (
	"sync"

	"github.com/lesismal/arpc"
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

	Sharding int32

	Password string

	mux sync.RWMutex

	topics map[string]*TopicAgent

	topicFactory func(name string, sharding int32, data []byte, timestamp int64) Topic

	clients map[*arpc.Client]map[string]*TopicAgent
}

// SetTopicFactory .
func (s *Server) SetTopicFactory(f func(name string, sharding int32, data []byte, timestamp int64) Topic) {
	if f == nil {
		panic("invalid Topic Factory: nil")
	}
	s.topicFactory = f
}

// NewTopic .
func (s *Server) NewTopic(name string, sharding int32, data []byte, timestamp int64) Topic {
	return s.topicFactory(name, sharding, data, timestamp)
}

// Publish topic
func (s *Server) Publish(topic Topic) {
	defer handlePanic()
	topic.SetSharding(s.Sharding)
	s.getOrMakeTopic(topic.GetName()).Publish(s, nil, topic)
}

// PublishToOne topic
func (s *Server) PublishToOne(topic Topic) {
	defer handlePanic()
	topic.SetSharding(s.Sharding)
	s.getOrMakeTopic(topic.GetName()).PublishToOne(s, nil, topic)
}

func (s *Server) invalid(ctx *arpc.Context) bool {
	return ctx.Client.UserData == nil
}

func (s *Server) onAuthenticate(ctx *arpc.Context) {
	defer handlePanic()

	passwd := ""
	err := ctx.Bind(&passwd)
	if err != nil {
		ctx.Error(err)
		arpc.DefaultLogger.Warn("%v [Authenticate] failed [%v], from\t%v", s.Handler.LogTag(), err, ctx.Client.Conn.RemoteAddr())
		return
	}

	if passwd == s.Password {
		s.addClient(ctx.Client)
		ctx.Write(nil)
		arpc.DefaultLogger.Warn("%v [Authenticate] success from\t%v", s.Handler.LogTag(), ctx.Client.Conn.RemoteAddr())
	} else {
		ctx.Error(ErrInvalidPassword)
		arpc.DefaultLogger.Warn("%v [Authenticate] failed [%v], from\t%v", s.Handler.LogTag(), ErrInvalidPassword, ctx.Client.Conn.RemoteAddr())
	}
}

func (s *Server) onSubscribe(ctx *arpc.Context) {
	defer handlePanic()

	if s.invalid(ctx) {
		arpc.DefaultLogger.Warn("%v [Subscribe] invalid ctx from\t%v", s.Handler.LogTag(), ctx.Client.Conn.RemoteAddr())
		return
	}

	topic := s.NewTopic("", 0, nil, 0)
	err := ctx.Bind(topic)
	if err != nil {
		ctx.Error(err)
		arpc.DefaultLogger.Warn("%v [Subscribe] failed [%v], from\t%v", s.Handler.LogTag(), err, ctx.Client.Conn.RemoteAddr())
		return
	}
	topicName := topic.GetName()
	if topicName != "" {
		cts := ctx.Client.UserData.(*clientTopics)
		cts.mux.Lock()
		tp, ok := cts.topicAgents[topicName]
		if !ok {
			tp = s.getOrMakeTopic(topicName)
			cts.topicAgents[topic.GetName()] = tp
			cts.mux.Unlock()
			tp.Add(ctx.Client)
			ctx.Write(nil)
			arpc.DefaultLogger.Info("%v [Subscribe] [%v] from\t%v", s.Handler.LogTag(), topicName, ctx.Client.Conn.RemoteAddr())
		} else {
			cts.mux.Unlock()
			ctx.Write(nil)
		}
	} else {
		ctx.Error(ErrInvalidTopicEmpty)
		arpc.DefaultLogger.Error("%v [Subscribe] failed [%v], from\t%v", s.Handler.LogTag(), ErrInvalidTopicEmpty, ctx.Client.Conn.RemoteAddr())
	}
}

func (s *Server) onUnsubscribe(ctx *arpc.Context) {
	defer handlePanic()

	if s.invalid(ctx) {
		arpc.DefaultLogger.Warn("%v [Unsubscribe] invalid ctx from\t%v", s.Handler.LogTag(), ctx.Client.Conn.RemoteAddr())
		return
	}

	topic := s.NewTopic("", 0, nil, 0)
	err := ctx.Bind(topic)
	if err != nil {
		ctx.Error(err)
		arpc.DefaultLogger.Warn("%v [Unsubscribe] failed [%v], from\t%v", s.Handler.LogTag(), err, ctx.Client.Conn.RemoteAddr())
		return
	}
	topicName := topic.GetName()
	if topicName != "" {
		cts := ctx.Client.UserData.(*clientTopics)
		cts.mux.Lock()
		if ta, ok := cts.topicAgents[topicName]; ok {
			delete(cts.topicAgents, topicName)
			cts.mux.Unlock()
			ta.Delete(ctx.Client)
			ctx.Write(nil)
			arpc.DefaultLogger.Info("%v [Unsubscribe] [%v] from\t%v", s.Handler.LogTag(), ta.Name, ctx.Client.Conn.RemoteAddr())
		} else {
			cts.mux.Unlock()
			ctx.Write(nil)
		}
	} else {
		ctx.Error(ErrInvalidTopicEmpty)
		arpc.DefaultLogger.Error("%v [Unsubscribe] failed [%v], from\t%v", s.Handler.LogTag(), ErrInvalidTopicEmpty, ctx.Client.Conn.RemoteAddr())
	}
}

func (s *Server) onPublish(ctx *arpc.Context) {
	defer handlePanic()

	if s.invalid(ctx) {
		arpc.DefaultLogger.Warn("%v [Publish] invalid ctx from\t%v", s.Handler.LogTag(), ctx.Client.Conn.RemoteAddr())
		return
	}
	topic := s.NewTopic("", 0, nil, 0)
	err := ctx.Bind(topic)
	if err != nil {
		ctx.Error(err)
		arpc.DefaultLogger.Warn("%v [Publish] failed [%v], from\t%v", s.Handler.LogTag(), err, ctx.Client.Conn.RemoteAddr())
		return
	}

	topicName := topic.GetName()
	if topicName != "" {
		ctx.Write(nil)
		s.Publish(topic)
		// arpc.DefaultLogger.Debug("%v [Publish] [%v], %v from\t%v", s.Handler.LogTag(), topicName, ctx.Client.Conn.RemoteAddr())
	} else {
		ctx.Error(ErrInvalidTopicEmpty)
		arpc.DefaultLogger.Error("%v [Publish] failed [%v], from\t%v", s.Handler.LogTag(), ErrInvalidTopicEmpty, ctx.Client.Conn.RemoteAddr())
	}
}

func (s *Server) onPublishToOne(ctx *arpc.Context) {
	defer handlePanic()

	if s.invalid(ctx) {
		arpc.DefaultLogger.Warn("%v [PublishToOne] invalid ctx from\t%v", s.Handler.LogTag(), ctx.Client.Conn.RemoteAddr())
		return
	}
	topic := s.NewTopic("", 0, nil, 0)
	err := ctx.Bind(topic)
	if err != nil {
		ctx.Error(err)
		arpc.DefaultLogger.Warn("%v [PublishToOne] failed [%v], from\t%v", s.Handler.LogTag(), err, ctx.Client.Conn.RemoteAddr())
		return
	}

	topicName := topic.GetName()
	if topicName != "" {
		ctx.Write(nil)
		s.PublishToOne(topic)
		// arpc.DefaultLogger.Debug("%v [Publish] [%v], %v from\t%v", s.Handler.LogTag(), topicName, ctx.Client.Conn.RemoteAddr())
	} else {
		ctx.Error(ErrInvalidTopicEmpty)
		arpc.DefaultLogger.Error("%v [PublishToOne] failed [%v], from\t%v", s.Handler.LogTag(), ErrInvalidTopicEmpty, ctx.Client.Conn.RemoteAddr())
	}
}

func (s *Server) getTopic(topic string) (*TopicAgent, bool) {
	s.mux.RLock()
	tp, ok := s.topics[topic]
	s.mux.RUnlock()
	return tp, ok
}

func (s *Server) getOrMakeTopic(topic string) *TopicAgent {
	s.mux.RLock()
	tp, ok := s.topics[topic]
	s.mux.RUnlock()
	if !ok {
		s.mux.Lock()
		tp, ok = s.topics[topic]
		if !ok {
			tp = newTopicAgent(topic)
			s.topics[topic] = tp
		}
		s.mux.Unlock()
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

	defer handlePanic()

	cts := c.UserData.(*clientTopics)
	cts.mux.RLock()
	defer cts.mux.RUnlock()
	for _, tp := range cts.topicAgents {
		tp.Delete(c)
		arpc.DefaultLogger.Info("%v [Disconnected Unsubscribe] [%v] from\t%v", s.Handler.LogTag(), tp.Name, c.Conn.RemoteAddr())
	}
}

// NewServer .
func NewServer() *Server {
	s := arpc.NewServer()
	svr := &Server{
		Server:       s,
		topics:       map[string]*TopicAgent{},
		topicFactory: defaultTopicFactory,
		clients:      map[*arpc.Client]map[string]*TopicAgent{},
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
