package micro

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/log"
	"github.com/lesismal/arpc/util"
)

var (
	ErrServiceNotFound    = errors.New("service not found")
	ErrServiceUnreachable = errors.New("service unreachable")
)

type serviceNode struct {
	addr     string
	client   *arpc.Client
	shutdown bool
}

type serviceNodeList struct {
	mux   sync.RWMutex
	name  string
	index uint64
	nodes []*serviceNode
}

func (list *serviceNodeList) add(node *serviceNode) {
	list.mux.Lock()
	list.nodes = append(list.nodes, node)
	list.mux.Unlock()
}

func (list *serviceNodeList) update(node *serviceNode) {
	var found = false
	list.mux.Lock()
	defer list.mux.Unlock()

	for _, v := range list.nodes {
		if v == node {
			found = true
		}
	}
	if !found {
		node.client.Stop()
	}
}

func (list *serviceNodeList) delete(addr string) {
	list.mux.Lock()
	defer list.mux.Unlock()
	l := len(list.nodes)
	for i := l - 1; i >= 0; i-- {
		node := list.nodes[i]
		if addr == node.addr {
			node.shutdown = true
			if node.client != nil {
				node.client.Stop()
			}
			list.nodes = append(list.nodes[:i], list.nodes[i+1:]...)
		}
	}
}

func (list *serviceNodeList) next() (*arpc.Client, error) {
	list.mux.RLock()
	defer list.mux.RUnlock()
	l := len(list.nodes)
	if l == 0 {
		return nil, ErrServiceUnreachable
	}
	for i := 0; i < l; i++ {
		list.index++
		node := list.nodes[list.index%uint64(len(list.nodes))]
		if node.client != nil {
			return node.client, nil
		}
	}
	return nil, ErrServiceUnreachable
}

type serviceManager struct {
	mux         sync.RWMutex
	dialer      func(addr string) (net.Conn, error)
	serviceList map[string]*serviceNodeList
}

func (s *serviceManager) setServiceNode(name string, node *serviceNode) {
	s.mux.Lock()
	list, ok := s.serviceList[name]
	s.mux.Unlock()
	if !ok {
		list = &serviceNodeList{}
		s.serviceList[name] = list
	}
	list.add(node)
}

func (s *serviceManager) updateServiceNode(name string, node *serviceNode) {
	s.mux.Lock()
	list, ok := s.serviceList[name]
	s.mux.Unlock()
	if ok {
		list.update(node)
	} else {
		node.client.Stop()
	}
}

func (s *serviceManager) deleteServiceNode(name, addr string) {
	s.mux.Lock()
	list, ok := s.serviceList[name]
	s.mux.Unlock()
	if ok {
		list.delete(addr)
	}
}

func (s *serviceManager) AddServiceNode(name, addr string) {
	var node = &serviceNode{addr: addr}

	log.Info("AddServiceNode: [%v, %v]", name, addr)
	client, err := arpc.NewClient(func() (net.Conn, error) {
		return s.dialer(addr)
	})
	if err == nil {
		node.client = client
		s.setServiceNode(name, node)
		return
	}

	log.Info("AddServiceNode failed: [%v, %v], %v", name, addr, err)

	go util.Safe(func() {
		i := 0
		for !node.shutdown {
			i++
			time.Sleep(time.Second)
			log.Info("AddServiceNode: [%v, %v] retrying %v...", name, addr, i)
			client, err := arpc.NewClient(func() (net.Conn, error) {
				return s.dialer(addr)
			})
			if err == nil {
				node.client = client
				s.updateServiceNode(name, node)
				return
			}
			log.Info("AddServiceNode [%v, %v] retrying %v failed: %v", name, addr, i, err)
		}
	})
}

func (s *serviceManager) DeleteServiceNode(name, addr string) {
	s.mux.RLock()
	defer s.mux.RUnlock()
	list, ok := s.serviceList[name]
	if ok {
		list.delete(addr)
	}
}

func (s *serviceManager) GetClient(name string) (*arpc.Client, error) {
	s.mux.RLock()
	defer s.mux.RUnlock()
	list, ok := s.serviceList[name]
	if ok {
		return list.next()
	}
	return nil, ErrServiceNotFound
}

// func (s *Service) Discover(serviceName string, info interface{}) error {

// }

func NewServiceManager(dialer func(addr string) (net.Conn, error)) *serviceManager {
	return &serviceManager{
		dialer:      dialer,
		serviceList: map[string]*serviceNodeList{},
	}
}

type ServiceManager interface {
	AddServiceNode(name, addr string)
	DeleteServiceNode(name, addr string)
}
