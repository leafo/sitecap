package main

import (
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
)

type Metrics struct {
	TotalRequests   atomic.Int64  `metric:"sitecap_requests_total"`
	SuccessRequests atomic.Int64  `metric:"sitecap_requests_success_total"`
	FailedRequests  atomic.Int64  `metric:"sitecap_requests_failed_total"`
	TotalDuration   atomic.Uint64 `metric:"sitecap_duration_seconds_total"`
}

var metrics Metrics

func (m *Metrics) String() string {
	var sb strings.Builder

	v := reflect.ValueOf(m).Elem()
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		metricName := fieldType.Tag.Get("metric")
		if metricName == "" {
			continue
		}

		var value string
		switch field.Type().String() {
		case "atomic.Int64":
			atomicInt := field.Interface().(atomic.Int64)
			value = strconv.FormatInt(atomicInt.Load(), 10)
		case "atomic.Uint64":
			atomicUint := field.Interface().(atomic.Uint64)
			nanoseconds := atomicUint.Load()
			seconds := float64(nanoseconds) / 1e9
			value = strconv.FormatFloat(seconds, 'f', 6, 64)
		}

		sb.WriteString(metricName + " " + value + "\n")
	}

	return sb.String()
}

func (m *Metrics) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, m.String())
}
