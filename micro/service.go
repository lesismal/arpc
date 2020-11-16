package micro

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/log"
	"github.com/lesismal/arpc/util"
)

var (
	// ErrServiceNotFound .
	ErrServiceNotFound = errors.New("service not found")
	// ErrServiceUnreachable .
	ErrServiceUnreachable = errors.New("service unreachable")
)

// ServiceManager .
type ServiceManager interface {
	AddServiceNodes(path string, value string)
	DeleteServiceNodes(path string)
	ClientBy(serviceName string) (*arpc.Client, error)
}

type ServiceNode struct {
	name     string
	addr     string
	client   *arpc.Client
	shutdown bool
}

type serviceNodeList struct {
	mux   sync.RWMutex
	name  string
	index uint64
	nodes []*ServiceNode
}

func (list *serviceNodeList) addByAddr(addr string, nodes []*ServiceNode) {
	list.mux.Lock()
	defer list.mux.Unlock()
	for _, v := range list.nodes {
		// addr already added
		if v.addr == addr {
			return
		}
	}
	list.nodes = append(list.nodes, nodes...)
}

func (list *serviceNodeList) update(node *ServiceNode) {
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
		if node.client != nil && node.client.CheckState() == nil {
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

func (s *serviceManager) setServiceNode(name string, addr string, nodes []*ServiceNode) {
	s.mux.Lock()
	list, ok := s.serviceList[name]
	s.mux.Unlock()
	if !ok {
		list = &serviceNodeList{}
		s.serviceList[name] = list
	}
	list.addByAddr(addr, nodes)
}

func (s *serviceManager) updateServiceNode(name string, node *ServiceNode) {
	s.mux.Lock()
	list, ok := s.serviceList[name]
	s.mux.Unlock()
	if ok {
		list.update(node)
	} else {
		node.client.Stop()
	}
}

// AddServiceNodes add nodes by path's addr and weight/value, would be called by a Discovery when service was setted
func (s *serviceManager) AddServiceNodes(path string, value string) {
	arr := strings.Split(path, "/")
	if len(arr) < 3 {
		return
	}

	app, name, addr := arr[0], arr[1], arr[2]
	weight := 1
	n, err := strconv.Atoi(value)
	if err == nil && n > 0 {
		weight = n
	}

	var nodes = make([]*ServiceNode, weight)
	for i := 0; i < weight; i++ {
		nodes[i] = &ServiceNode{name: name, addr: addr}
	}

	client, err := arpc.NewClient(func() (net.Conn, error) {
		return s.dialer(addr)
	})
	for i := 0; i < weight; i++ {
		nodes[i].client = client
		s.setServiceNode(name, addr, nodes)
	}
	if err == nil {
		log.Info("AddServiceNodes: [%v, %v, %v, %v]", app, name, addr, weight)
	} else {
		log.Info("AddServiceNodes failed, retrying later: [%v, %v, %v, %v], %v", app, name, addr, weight, err)

		go util.Safe(func() {
			i := 0
			for !nodes[0].shutdown {
				i++
				time.Sleep(time.Second)
				log.Info("AddServiceNodes: [%v, %v, %v, %v] retrying %v...", app, name, addr, weight, i)
				client, err := arpc.NewClient(func() (net.Conn, error) {
					return s.dialer(addr)
				})
				if err == nil {
					time.Sleep(time.Second / 100)
					for i := 0; i < weight; i++ {
						nodes[i].client = client
						s.updateServiceNode(name, nodes[i])
					}

					return
				}
				log.Info("AddServiceNodes [%v, %v, %v, %v] retrying %v failed: %v", app, name, addr, weight, i, err)
			}
		})
	}
}

// DeleteServiceNodes deletes all nods for path's addr, would be called by a Discovery when service was setted
func (s *serviceManager) DeleteServiceNodes(path string) {
	arr := strings.Split(path, "/")
	if len(arr) < 3 {
		return
	}
	app, name, addr := arr[0], arr[1], arr[2]
	s.mux.RLock()
	defer s.mux.RUnlock()
	list, ok := s.serviceList[name]
	if ok {
		list.delete(addr)
		log.Info("DeleteServiceNodes: [%v, %v, %v]", app, name, addr)
	}
}

// Client returns a reachable client by service's name
func (s *serviceManager) ClientBy(serviceName string) (*arpc.Client, error) {
	s.mux.RLock()
	defer s.mux.RUnlock()
	list, ok := s.serviceList[serviceName]
	if ok {
		return list.next()
	}
	return nil, ErrServiceNotFound
}

// NewServiceManager .
func NewServiceManager(dialer func(addr string) (net.Conn, error)) ServiceManager {
	return &serviceManager{
		dialer:      dialer,
		serviceList: map[string]*serviceNodeList{},
	}
}
