package tracing

import (
	"github.com/gogo/protobuf/proto"
	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/extension/middleware/coder"
	"github.com/opentracing/basictracer-go"
	"github.com/opentracing/basictracer-go/wire"
	opentracing "github.com/opentracing/opentracing-go"
)

const (
	appenderName = "arpc-opentracing"
)

// Tracer .
type Tracer struct {
	opentracing.Tracer
	*coder.Appender
}

// Inject .
func (t *Tracer) Inject(sc opentracing.SpanContext, format interface{}, carrier interface{}) error {
	return t.Tracer.Inject(sc, format, carrier)
}

// Extract .
func (t *Tracer) Extract(format interface{}, opaqueCarrier interface{}) (opentracing.SpanContext, error) {
	if ac, ok := opaqueCarrier.(*arpc.Context); ok {
		ispanCtx, ok := ac.Get(appenderName)
		if ok {
			spanCtx, ok := ispanCtx.(opentracing.SpanContext)
			if ok {
				return spanCtx, nil
			}
		}
		return nil, opentracing.ErrInvalidSpanContext
	}
	return t.Tracer.Extract(format, opaqueCarrier)
}

// ValueToBytes .
func (t *Tracer) ValueToBytes(value interface{}) ([]byte, error) {
	sc, ok := value.(basictracer.SpanContext)
	if !ok {
		return nil, opentracing.ErrInvalidSpanContext
	}

	state := wire.TracerState{
		TraceId:      sc.TraceID,
		SpanId:       sc.SpanID,
		Sampled:      sc.Sampled,
		BaggageItems: sc.Baggage,
	}
	bytes, err := proto.Marshal(&state)
	if err != nil {
		return nil, err
	}

	return bytes, nil
}

// BytesToValue .
func (t *Tracer) BytesToValue(bytes []byte) (interface{}, error) {
	ctx := wire.TracerState{}
	if err := proto.Unmarshal(bytes, &ctx); err != nil {
		return nil, opentracing.ErrSpanContextCorrupted
	}
	sc := basictracer.SpanContext{
		TraceID: ctx.TraceId,
		SpanID:  ctx.SpanId,
		Sampled: ctx.Sampled,
		Baggage: ctx.BaggageItems,
	}
	return sc, nil
}

// NewTracer .
func NewTracer(recorder basictracer.SpanRecorder) *Tracer {
	if recorder == nil {
		recorder = &defaultReporter{}
	}
	t := &Tracer{}
	t.Tracer = basictracer.New(recorder)
	t.Appender = coder.NewAppender(appenderName, coder.FlagBitOpenTracing, t.ValueToBytes, t.BytesToValue)
	return t
}
