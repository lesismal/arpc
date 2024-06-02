package router

import (
	"time"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/log"
)

var (
	cmdName = map[byte]string{
		arpc.CmdRequest: "request",
		arpc.CmdNotify:  "notify",
	}
)

// Logger returns the logger middleware.
func Logger() arpc.HandlerFunc {
	return func(ctx *arpc.Context) {
		t := time.Now()

		ctx.Next()

		cmd := ctx.Message.Cmd()
		method := ctx.Message.Method()
		addr := ctx.Client.Conn.RemoteAddr()
		cost := time.Since(t).Milliseconds()

		switch cmd {
		case arpc.CmdRequest, arpc.CmdNotify:
			err := ctx.ResponseError()
			if err == nil {
				log.Info("[%v | %v] from %v success, %v ms cost", cmdName[cmd], method, addr, cost)
			} else {
				log.Info("[%v | %v], from %v error: [%v], %v ms cost", cmdName[cmd], method, addr, err, cost)
			}
		default:
			log.Error("invalid cmd: %d,\tdropped", cmd)
			ctx.Done()
		}
	}
}
