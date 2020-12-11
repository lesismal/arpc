package tracer

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/extension/middleware/coder"
	"github.com/lesismal/arpc/internal/util"
)

const (
	// TraceIDKey .
	TraceIDKey = "traceid"
	// SpanIDKey .
	SpanIDKey = "spanid"

	// TraceInfoSep .
	TraceInfoSep = ":|:"
	// TraceOrSpanIDSep .
	TraceOrSpanIDSep = "_"
)

// Span represents a span.
type Span map[string]interface{}

// Values returns span values.
func (sp Span) Values() map[string]interface{} {
	return sp
}

// TraceID returns trace id.
func (sp Span) TraceID() string {
	traceID := ""
	iTraceID, ok := sp[TraceIDKey]
	if ok {
		traceID = iTraceID.(string)
	}
	return traceID
}

// SpanID returns span id.
func (sp Span) SpanID() string {
	spanID := ""
	iSpanID, ok := sp[SpanIDKey]
	if ok {
		spanID = iSpanID.(string)
	}
	return spanID
}

func (sp Span) pack(msg *arpc.Message) {
	traceID := ""
	spanID := ""
	iTraceID := sp[TraceIDKey]
	if iTraceID != nil {
		traceID = iTraceID.(string)
	}
	if traceID == "" {
		return
	}
	iSpanID := sp[SpanIDKey]
	if iSpanID != nil {
		spanID = iSpanID.(string)
	}
	if spanID == "" {
		return
	}

	tracerInfo := (traceID + TraceInfoSep + spanID)
	msg.Buffer = append(msg.Buffer, tracerInfo...)
	traceInfoLen := uint16(len(tracerInfo))
	msg.Buffer = append(msg.Buffer, byte(traceInfoLen>>8), byte(traceInfoLen&0xFF))
	msg.SetFlagBit(coder.FlagBitTracer, true)
	msg.SetBodyLen(len(msg.Buffer) - 16)
}

func (sp Span) unpack(msg *arpc.Message) {
	if msg.IsFlagBitSet(coder.FlagBitTracer) {
		bufLen := len(msg.Buffer)
		if bufLen > 2 {
			traceID := ""
			spanID := ""
			traceInfoLen := (int(msg.Buffer[bufLen-2]) << 8) | int(msg.Buffer[bufLen-1])
			if bufLen >= 2+traceInfoLen {
				traceInfo := util.BytesToStr(msg.Buffer[bufLen-2-traceInfoLen : bufLen-2])
				traceInfoArr := strings.Split(traceInfo, TraceInfoSep)
				if len(traceInfoArr) >= 2 {
					traceID, spanID = traceInfoArr[0], traceInfoArr[1]
				}
			}
			if traceID != "" && spanID != "" {
				msg.Set(TraceIDKey, traceID)
				msg.Set(SpanIDKey, spanID)
			}
			msg.Buffer = msg.Buffer[:len(msg.Buffer)-2-traceInfoLen]
			msg.SetFlagBit(coder.FlagBitTracer, false)
			msg.SetBodyLen(len(msg.Buffer) - 16)
		}
	}
}

// Tracer represents a trace coding middleware.
type Tracer struct {
	tracePrefix string
	spanPrefix  string
	traceCount  uint64
	spanCount   uint64
}

// Encode implements arpc MessageCoder.
func (tracer *Tracer) Encode(client *arpc.Client, msg *arpc.Message) *arpc.Message {
	sp := Span(msg.Values())
	sp.pack(msg)

	return msg
}

// Decode implements arpc MessageCoder.
func (tracer *Tracer) Decode(client *arpc.Client, msg *arpc.Message) *arpc.Message {
	sp := Span(msg.Values())
	sp.unpack(msg)
	return msg
}

// NextSpan inherit pre span's trace id and creates a new span.
func (tracer *Tracer) NextSpan(values map[string]interface{}) Span {
	if len(values) == 0 {
		return tracer.NewSpan()
	}

	traceID := ""
	spanID := ""
	iTraceID, ok := values[TraceIDKey]
	if ok {
		traceID, ok = iTraceID.(string)
		if !ok {
			return tracer.NewSpan()
		}
	}
	iSpanID, ok := values[SpanIDKey]
	if ok {
		spanID, ok = iSpanID.(string)
		if ok {
			pos := strings.LastIndex(spanID, TraceOrSpanIDSep)
			if pos > 0 {
				spanCount, err := strconv.ParseUint(spanID[pos+1:], 10, 64)
				if err != nil {
					return tracer.NewSpan()
				}
				spanID = spanID[:pos] + TraceOrSpanIDSep + strconv.FormatUint(spanCount+1, 10)
			}
		} else {
			spanID = fmt.Sprintf("%v%v%v%v%v", tracer.spanPrefix, TraceOrSpanIDSep, time.Now().UnixNano(), TraceOrSpanIDSep, atomic.AddUint64(&tracer.spanCount, 1))
		}
	}

	return Span{
		TraceIDKey: traceID,
		SpanIDKey:  spanID,
	}
}

// NewSpan creates a new span.
func (tracer *Tracer) NewSpan() Span {
	now := time.Now().UnixNano()
	return Span{
		TraceIDKey: fmt.Sprintf("%v%v%v%v%v", tracer.tracePrefix, TraceOrSpanIDSep, now, TraceOrSpanIDSep, atomic.AddUint64(&tracer.traceCount, 1)),
		SpanIDKey:  fmt.Sprintf("%v%v%v%v%v", tracer.spanPrefix, TraceOrSpanIDSep, now, TraceOrSpanIDSep, atomic.AddUint64(&tracer.spanCount, 1)),
	}
}

// New returns the trace coding middleware.
func New(tracePrefix string, spanPrefix string) *Tracer {
	return &Tracer{
		tracePrefix: tracePrefix,
		spanPrefix:  spanPrefix,
		traceCount:  0,
		spanCount:   0,
	}
}
