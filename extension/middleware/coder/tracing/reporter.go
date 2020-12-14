package tracing

import (
	"fmt"

	"github.com/opentracing/basictracer-go"
)

type defaultReporter struct{}

// RecordSpan complies with the basictracer.Recorder interface.
func (r *defaultReporter) RecordSpan(span basictracer.RawSpan) {
	fmt.Printf(
		"RecordSpan: %v[%v, %v us] --> %v logs. context: %v; baggage: %v\n",
		span.Operation, span.Start, span.Duration, len(span.Logs),
		span.Context, span.Context.Baggage)
	for i, l := range span.Logs {
		fmt.Printf(
			"    log %v @ %v: %v\n", i, l.Timestamp, l.Fields)
	}
}
