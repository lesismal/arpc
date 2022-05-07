package redis

import (
	"context"
	"fmt"
	"time"

	redis "github.com/go-redis/redis/v8"
	"github.com/lesismal/arpc/log"
	"github.com/lesismal/arpc/util"
)

//Register .
type Register struct {
	serviceNamespace string
	serviceName      string
	serviceAddr      string
	weight           int
	field            string
	done             chan struct{}
	interval         time.Duration
	expire           time.Duration
	client           *redis.Client
}

// Stop .
func (s *Register) keepalive() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.client.HSet(context.Background(), s.serviceNamespace, s.field, time.Now().Add(s.expire).Unix())
		}
	}
}

// Stop .
func (s *Register) Stop() error {
	ret := s.client.HDel(context.Background(), s.serviceNamespace, s.field)
	err := ret.Err()
	if err != nil {
		log.Error("Register Stop failed: %v", err)
		return err
	}
	return nil
}

// NewRegister .
func NewRegister(addr, serviceNamespace, serviceName, serviceAddr string, weight int, interval, expire time.Duration) (*Register, error) {
	if weight <= 0 {
		weight = 1
	}
	if interval <= 0 {
		interval = time.Second * 5
	}
	if expire <= interval {
		expire = interval + time.Second*5
	}

	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	field := fmt.Sprintf("%v/%v/%v", serviceName, serviceAddr, weight)
	ret := client.HSetNX(context.Background(), serviceNamespace, field, time.Now().Add(expire).Unix())
	err := ret.Err()
	if err != nil {
		log.Error("NewRegister [%v, %v] client.HSetNX failed: %v", serviceNamespace, fmt.Sprintf("%v/%v", serviceName, serviceAddr), err)
		return nil, err
	}

	register := &Register{
		serviceNamespace: serviceNamespace,
		serviceName:      serviceName,
		serviceAddr:      serviceAddr,
		weight:           weight,
		field:            field,
		interval:         interval,
		expire:           expire,
		done:             make(chan struct{}),
		client:           client,
	}

	log.Info("NewRegister [%v: %v/%v] success", serviceNamespace, serviceName, serviceAddr)

	go util.Safe(register.keepalive)

	return register, nil
}
