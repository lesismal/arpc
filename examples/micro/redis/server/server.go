package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lesismal/arpc/extension/micro/redis"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/log"
)

func main() {
	var (
		namespace = "app"
		service   = "echo"
		addr      = "localhost:8888"
		weight    = 3
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
	go func() {
		err := svr.Run(addr)
		log.Error("server exit: %v", err)
		os.Exit(0)
	}()

	time.Sleep(time.Second / 2)
	register, err := redis.NewRegister("localhost:6379", namespace, service, addr, weight, time.Second*5, time.Second*8)
	if err != nil {
		log.Error("NewRegister failed: %v", err)
		panic(err)
	}
	defer register.Stop()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	svr.Stop()
}
