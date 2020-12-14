package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/extension/middleware/coder/tracing"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
)

func main() {
	client, err := arpc.NewClient(func() (net.Conn, error) {
		return net.DialTimeout("tcp", "localhost:8888", time.Second*3)
	})
	if err != nil {
		panic(err)
	}
	defer client.Stop()

	tracer := tracing.NewTracer(nil)
	opentracing.InitGlobalTracer(tracer)

	client.Handler.UseCoder(tracer)

	reader := bufio.NewReader(os.Stdin)
	for {
		span := opentracing.StartSpan("getInput")
		span.SetTag("component", "client")
		ctx := opentracing.ContextWithSpan(context.Background(), span)
		// Make sure that global baggage propagation works.
		span.SetBaggageItem("user", "echo")
		span.LogFields(log.Object("ctx", ctx))
		fmt.Print("\n\nEnter text (empty string to exit): ")
		text, _ := reader.ReadString('\n')
		text = strings.TrimSpace(text)
		if len(text) == 0 {
			fmt.Println("Exiting.")
			os.Exit(0)
		}

		span.LogFields(log.String("user text", text))

		req := text
		rsp := ""
		err = client.Call("/echo", &req, &rsp, time.Second*5, tracing.Values(span.Context()))
		if err != nil {
			span.LogFields(log.Error(err))
		} else {
			span.LogFields(log.String("rsp", rsp))
		}

		span.Finish()
	}
}
