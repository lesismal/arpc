package router

import (
	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/internal/util"
)

// Recover returns the recovery middleware handler.
func Recover() arpc.HandlerFunc {
	return func(ctx *arpc.Context) {
		defer util.Recover()
		ctx.Next()
	}
}
