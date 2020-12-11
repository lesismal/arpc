package etcd

import (
	"context"
	"time"

	"github.com/lesismal/arpc/extension/micro"
	"github.com/lesismal/arpc/internal/log"
	"github.com/lesismal/arpc/internal/util"
	"go.etcd.io/etcd/client/v3"
)

// Discovery .
type Discovery struct {
	client         *clientv3.Client
	prefix         string
	serviceManager micro.ServiceManager
}

func (ds *Discovery) init() {
	go util.Safe(ds.lazyInit)
	ds.watch()
}

func (ds *Discovery) lazyInit() {
	time.Sleep(time.Second / 100)
	resp, err := ds.client.Get(context.Background(), ds.prefix, clientv3.WithPrefix())
	if err != nil {
		return
	}

	for _, ev := range resp.Kvs {
		if ds.serviceManager != nil {
			ds.serviceManager.AddServiceNodes(string(ev.Key), string(ev.Value))
		}
	}
}

func (ds *Discovery) watch() {
	rch := ds.client.Watch(context.Background(), ds.prefix, clientv3.WithPrefix())
	log.Info("Discovery watching: %s", ds.prefix)
	for wresp := range rch {
		for _, ev := range wresp.Events {
			switch ev.Type {
			case clientv3.EventTypePut:
				if ds.serviceManager != nil {
					ds.serviceManager.AddServiceNodes(string(ev.Kv.Key), string(ev.Kv.Value))
				}
			case clientv3.EventTypeDelete:
				if ds.serviceManager != nil {
					ds.serviceManager.DeleteServiceNodes(string(ev.Kv.Key))
				}
			}
		}
	}
}

// Stop .
func (ds *Discovery) Stop() error {
	return ds.client.Close()
}

// NewDiscovery .
func NewDiscovery(endpoints []string, prefix string, serviceManager micro.ServiceManager) (*Discovery, error) {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		log.Error("NewDiscovery [%v] clientv3.New failed: %v", prefix, err)
		return nil, err
	}

	ds := &Discovery{
		client:         client,
		prefix:         prefix,
		serviceManager: serviceManager,
	}

	go util.Safe(ds.init)

	log.Info("NewDiscovery [%v] success", prefix)

	return ds, nil
}
