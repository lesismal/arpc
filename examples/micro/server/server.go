package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/log"
	"github.com/lesismal/arpc/micro/etcd"
)

func main() {
	var (
		appPrefix       = "app"
		service         = "echo"
		addr            = "localhost:8888"
		weight          = 2
		ttl       int64 = 10

		endpoints = []string{"localhost:2379", "localhost:22379", "localhost:32379"}
	)

	svr := arpc.NewServer()
	// register router
	svr.Handler.Handle("/echo", func(ctx *arpc.Context) {
		str := ""
		err := ctx.Bind(&str)
		ret := fmt.Sprintf("%v_from_%v", str, addr)
		ctx.Write(ret)
		log.Info("/echo: \"%v\", error: %v", ret, err)
	})
	go svr.Run(addr)

	key := fmt.Sprintf("%v/%v/%v", appPrefix, service, addr)
	value := fmt.Sprintf("%v", weight)
	register, err := etcd.NewRegister(endpoints, key, value, ttl)
	if err != nil {
		log.Error("NewRegister failed: %v", err)
		panic(err)
	}
	defer register.Stop()

	quit := make(chan os.Signal)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	svr.Stop()
}
