package redis

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/lesismal/arpc/extension/micro"
	"github.com/lesismal/arpc/log"
	"github.com/lesismal/arpc/util"
)

// Discovery .
type Discovery struct {
	client *redis.Client

	serviceNamespace string
	interval         time.Duration
	serviceManager   micro.ServiceManager

	serviceCache map[string]struct{}

	done chan struct{}
}

func (ds *Discovery) updateServices() error {
	ret := ds.client.HGetAll(context.Background(), ds.serviceNamespace)
	err := ret.Err()
	if err != nil {
		log.Error("Discovery updateServices failed: %v", err)
	}

	cache := map[string]struct{}{}
	services := ret.Val()
	if services != nil {
		for k, v := range services {
			deleteService := false
			strs := strings.Split(k, "/")
			if len(strs) == 3 {
				serviceName := strs[0]
				serviceAddr := strs[1]
				sweight := strs[2]
				weight, err := strconv.ParseInt(v, 10, 64)
				if err != nil || weight <= 0 {
					deleteService = true
				} else {
					expire, err := strconv.ParseInt(v, 10, 64)
					if err != nil {
						deleteService = true
					} else {
						if time.Now().Unix() > expire {
							deleteService = true
						} else {
							path := fmt.Sprintf("%v/%v/%v", ds.serviceNamespace, serviceName, serviceAddr)
							cache[path] = struct{}{}
							_, ok := ds.serviceCache[path]
							if !ok {
								ds.serviceManager.AddServiceNodes(path, sweight)
							}
							delete(ds.serviceCache, path)
						}
					}
				}
			} else {
				deleteService = true
			}
			if deleteService {
				ret := ds.client.HDel(context.Background(), ds.serviceNamespace, k)
				err = ret.Err()
				if err != nil {
					log.Error("Discovery delete expired service[%v: %v] failed: %v", ds.serviceNamespace, k, err)
				}
			}
		}
	}

	// for path := range cache {
	// 	delete(ds.serviceCache, path)
	// }
	for path := range ds.serviceCache {
		ds.serviceManager.DeleteServiceNodes(path)
	}

	ds.serviceCache = cache

	return nil
}

func (ds *Discovery) update() {
	ticker := time.NewTicker(ds.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ds.done:
			return
		case <-ticker.C:
			util.Safe(func() {
				ds.updateServices()
			})
		}
	}
}

// Stop .
func (ds *Discovery) Stop() error {
	close(ds.done)
	return nil
}

// NewDiscovery .
func NewDiscovery(addr string, serviceNamespace string, interval time.Duration, serviceManager micro.ServiceManager) (*Discovery, error) {
	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	ds := &Discovery{
		client:           client,
		serviceNamespace: serviceNamespace,
		interval:         interval,
		serviceManager:   serviceManager,
		serviceCache:     map[string]struct{}{},
		done:             make(chan struct{}),
	}

	err := ds.updateServices()
	if err != nil {
		return nil, err
	}
	go util.Safe(ds.update)

	log.Info("NewDiscovery [%v] success", serviceNamespace)

	return ds, nil
}
