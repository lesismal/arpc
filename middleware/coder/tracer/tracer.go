package tracer

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/middleware/coder"
	"github.com/lesismal/arpc/util"
)

const (
	TraceIdKey = "traceid"
	SpanIdKey  = "spanid"

	TraceInfoSep     = ":|:"
	TraceOrSpanIDSep = "_"
)

type Span map[string]interface{}

func (sp Span) Values() map[string]interface{} {
	return sp
}

func (sp Span) TraceID() string {
	traceID := ""
	iTraceID, ok := sp[TraceIdKey]
	if ok {
		traceID = iTraceID.(string)
	}
	return traceID
}

func (sp Span) SpanID() string {
	spanID := ""
	iSpanID, ok := sp[SpanIdKey]
	if ok {
		spanID = iSpanID.(string)
	}
	return spanID
}

func (sp Span) pack(msg *arpc.Message) {
	traceID := ""
	spanID := ""
	iTraceId, _ := sp[TraceIdKey]
	if iTraceId != nil {
		traceID = iTraceId.(string)
	}
	if traceID == "" {
		return
	}
	iSpanId, _ := sp[SpanIdKey]
	if iSpanId != nil {
		spanID = iSpanId.(string)
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
			traceInfoLen := int((msg.Buffer[bufLen-2] << 8) | msg.Buffer[bufLen-1])
			if bufLen >= 2+traceInfoLen {
				traceInfo := util.BytesToStr(msg.Buffer[bufLen-2-traceInfoLen : bufLen-2])
				traceInfoArr := strings.Split(traceInfo, TraceInfoSep)
				if len(traceInfoArr) >= 2 {
					traceID, spanID = traceInfoArr[0], traceInfoArr[1]
				}
			}
			if traceID != "" && spanID != "" {
				msg.Set(TraceIdKey, traceID)
				msg.Set(SpanIdKey, spanID)
			}
			msg.Buffer = msg.Buffer[:len(msg.Buffer)-2-traceInfoLen]
			msg.SetFlagBit(coder.FlagBitTracer, false)
			msg.SetBodyLen(len(msg.Buffer) - 16)
		}
	}
}

type Tracer struct {
	tracePrefix string
	spanPrefix  string
	traceCount  uint64
	spanCount   uint64
}

func (tracer *Tracer) Encode(client *arpc.Client, msg *arpc.Message) *arpc.Message {
	sp := Span(msg.Values())
	sp.pack(msg)

	return msg
}

func (tracer *Tracer) Decode(client *arpc.Client, msg *arpc.Message) *arpc.Message {
	sp := Span(msg.Values())
	sp.unpack(msg)
	return msg
}

func (tracer *Tracer) NextSpan(values map[string]interface{}) Span {
	if len(values) == 0 {
		return tracer.NewSpan()
	}

	traceID := ""
	spanID := ""
	iTraceID, ok := values[TraceIdKey]
	if ok {
		traceID, ok = iTraceID.(string)
		if !ok {
			return tracer.NewSpan()
		}
	}
	iSpanID, ok := values[SpanIdKey]
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
		TraceIdKey: traceID,
		SpanIdKey:  spanID,
	}
}

func (tracer *Tracer) NewSpan() Span {
	now := time.Now().UnixNano()
	return Span{
		TraceIdKey: fmt.Sprintf("%v%v%v%v%v", tracer.tracePrefix, TraceOrSpanIDSep, now, TraceOrSpanIDSep, atomic.AddUint64(&tracer.traceCount, 1)),
		SpanIdKey:  fmt.Sprintf("%v%v%v%v%v", tracer.spanPrefix, TraceOrSpanIDSep, now, TraceOrSpanIDSep, atomic.AddUint64(&tracer.spanCount, 1)),
	}
}

func New(tracePrefix string, spanPrefix string) *Tracer {
	return &Tracer{
		tracePrefix: tracePrefix,
		spanPrefix:  spanPrefix,
		traceCount:  0,
		spanCount:   0,
	}
}
