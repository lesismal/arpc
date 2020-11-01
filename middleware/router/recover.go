package router

import (
	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/util"
)

func Recover() arpc.HandlerFunc {
	return func(ctx *arpc.Context) {
		defer util.Recover()
		ctx.Next()
	}
}
