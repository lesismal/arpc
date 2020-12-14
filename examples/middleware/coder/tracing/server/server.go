package main

import (
	"bytes"
	"io/ioutil"
	golog "log"
	"net/http"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/extension/middleware/coder/tracing"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
)

func httpServer() {
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		textCarrier := opentracing.HTTPHeadersCarrier(req.Header)
		wireSpanContext, err := opentracing.GlobalTracer().Extract(
			opentracing.TextMap, textCarrier)
		if err != nil {
			panic(err)
		}
		serverSpan := opentracing.GlobalTracer().StartSpan(
			"http-server-span",
			ext.RPCServerOption(wireSpanContext))
		serverSpan.SetTag("component", "http-server")
		defer serverSpan.Finish()

		fullBody, err := ioutil.ReadAll(req.Body)
		if err != nil {
			serverSpan.LogFields(log.Error(err))
		}
		serverSpan.LogFields(log.String("request body", string(fullBody)))
	})

	golog.Fatal(http.ListenAndServe("localhost:8889", nil))
}

func main() {
	tracer := tracing.NewTracer(nil)
	opentracing.InitGlobalTracer(tracer)

	go httpServer()

	svr := arpc.NewServer()
	svr.Handler.UseCoder(tracer)
	svr.Handler.Handle("/echo", func(ctx *arpc.Context) {
		wireSpanContext, err := opentracing.GlobalTracer().Extract(nil, ctx)
		if err != nil {
			panic(err)
		}
		serverSpan := opentracing.GlobalTracer().StartSpan(
			"arpc-server-span",
			ext.RPCServerOption(wireSpanContext))
		serverSpan.SetTag("component", "arpc-server")
		defer serverSpan.Finish()

		payload := ""
		err = ctx.Bind(&payload)
		if err != nil {
			serverSpan.LogFields(log.Error(err))
		}
		serverSpan.LogFields(log.String("payload", payload))
		ctx.Write(payload)

		httpClient := &http.Client{}
		httpReq, _ := http.NewRequest("POST", "http://localhost:8889/", bytes.NewReader([]byte(payload)))
		textCarrier := opentracing.HTTPHeadersCarrier(httpReq.Header)
		err = serverSpan.Tracer().Inject(serverSpan.Context(), opentracing.TextMap, textCarrier)
		if err != nil {
			panic(err)
		}
		resp, err := httpClient.Do(httpReq)
		if err != nil {
			serverSpan.LogFields(log.Error(err))
		} else {
			serverSpan.LogFields(log.Object("response", resp))
		}
	})

	svr.Run("localhost:8888")
}
