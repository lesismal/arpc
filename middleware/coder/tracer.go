package coder

import (
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/util"
)

const (
	TraceIdKey = "traceid"
	SpanIdKey  = "spanid"
)

type Tracer struct {
	traceId string
	spanId  uint64
}

func (tracer *Tracer) Encode(client *arpc.Client, msg *arpc.Message) *arpc.Message {
	traceId := ""
	spanId := ""
	iTraceId, _ := msg.Get(TraceIdKey)
	if iTraceId != nil {
		traceId = iTraceId.(string)
	} else if tracer.traceId != "" {
		traceId = tracer.traceId
	}
	iSpanId, _ := msg.Get(SpanIdKey)
	if iSpanId != nil {
		spanId = iSpanId.(string)
	} else {
		spanId = strconv.FormatUint(atomic.AddUint64(&tracer.spanId, 1), 10)
	}
	tracerInfo := (traceId + ":" + spanId)
	msg.Buffer = append(msg.Buffer, tracerInfo...)
	traceInfoLen := uint16(len(tracerInfo))
	msg.Buffer = append(msg.Buffer, byte(traceInfoLen>>8), byte(traceInfoLen&0xFF))
	msg.SetFlagBit(FlagBitTracer, true)
	msg.SetBodyLen(len(msg.Buffer) - 16)
	return msg
}

func (tracer *Tracer) Decode(client *arpc.Client, msg *arpc.Message) *arpc.Message {
	if msg.IsFlagBitSet(FlagBitTracer) {
		traceId := ""
		spanId := ""
		bufLen := len(msg.Buffer)
		if bufLen > 2 {
			traceInfoLen := int((msg.Buffer[bufLen-2] << 8) | msg.Buffer[bufLen-1])
			if bufLen >= 2+traceInfoLen {
				traceInfo := util.BytesToStr(msg.Buffer[bufLen-2-traceInfoLen : bufLen-2])
				traceInfoArr := strings.Split(traceInfo, ":")
				if len(traceInfoArr) >= 2 {
					traceId, spanId = traceInfoArr[0], traceInfoArr[1]
				}

			}
			msg.Set(TraceIdKey, traceId)
			msg.Set(SpanIdKey, spanId)
			msg.Buffer = msg.Buffer[:len(msg.Buffer)-2-traceInfoLen]
			msg.SetFlagBit(FlagBitTracer, false)
			msg.SetBodyLen(len(msg.Buffer) - 16)
		}
	}
	return msg
}

func NewTracer(traceId string, initSpanId uint64) *Tracer {
	return &Tracer{
		traceId: traceId,
		spanId:  initSpanId,
	}
}
