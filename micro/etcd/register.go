package etcd

import (
	"context"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/lesismal/arpc/log"
	"github.com/lesismal/arpc/util"
)

//Register
type Register struct {
	key         string
	value       string
	client      *clientv3.Client
	leaseID     clientv3.LeaseID
	chKeepalive <-chan *clientv3.LeaseKeepAliveResponse
}

// listenTTL .
func (s *Register) listenTTL() {
	for resp := range s.chKeepalive {
		log.Debug("Register listenTTL: %v", resp)
	}
	log.Info("Register listenTTL exit")
}

// Stop .
func (s *Register) Stop() error {
	// cancel ttl
	if _, err := s.client.Revoke(context.Background(), s.leaseID); err != nil {
		log.Error("Register Stop failed: %v", err)
		return err
	}
	return s.client.Close()
}

// NewRegister .
func NewRegister(endpoints []string, key string, value string, ttl int64) (*Register, error) {
	// step 1: new client
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		log.Error("NewRegister [%v, %v] clientv3.New failed: %v", key, value, err)
		return nil, err
	}

	// step 2: generate ttl
	resp, err := client.Grant(context.Background(), ttl)
	if err != nil {
		log.Error("NewRegister [%v, %v] client.Grant failed: %v", key, value, err)
		return nil, err
	}

	// step 3: set kv
	_, err = client.Put(context.Background(), key, value, clientv3.WithLease(resp.ID))
	if err != nil {
		log.Error("NewRegister [%v, %v] client.Put failed: %v", key, value, err)
		return nil, err
	}

	// step 4: set ttl and keepalive
	chKeepalive, err := client.KeepAlive(context.Background(), resp.ID)
	if err != nil {
		log.Error("NewRegister [%v, %v] client.KeepAlive failed: %v", key, value, err)
		return nil, err
	}

	register := &Register{
		key:         key,
		value:       value,
		client:      client,
		leaseID:     resp.ID,
		chKeepalive: chKeepalive,
	}

	log.Info("NewRegister [%v, %v] success", key, value)

	go util.Safe(register.listenTTL)

	return register, nil
}
