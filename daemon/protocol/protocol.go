// Package protocol holds what the daemon and the app say to each other: protocol_gen.go is generated from the
// zod schemas of shared/protocol, which the app parses with; this file adds what Go needs beside them.
package protocol

import (
	"encoding/json"
	"reflect"
	"time"
)

// timestampLayout is JavaScript's toISOString: UTC, milliseconds, a Z, which the schemas' datetimes want.
const timestampLayout = "2006-01-02T15:04:05.000Z"

// Timestamp writes a time as the protocol's datetimes are written.
func Timestamp(t time.Time) string {
	return t.UTC().Format(timestampLayout)
}

// ParseTimestamp reads one of the protocol's datetimes.
func ParseTimestamp(text string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, text)
}

// LocalHost is this computer, the host of a folder unless said otherwise.
func LocalHost() HostRef {
	return HostRef{Value: HostRefLocal{}}
}

// Dated is an event with the time the session dates it at: every event has an At field.
func Dated(event SessionEventVariant, at string) SessionEventVariant {
	value := reflect.New(reflect.TypeOf(event)).Elem()
	value.Set(reflect.ValueOf(event))
	value.FieldByName("At").SetString(at)
	return value.Interface().(SessionEventVariant)
}

// Kind is an event's kind, as its JSON says it.
func Kind(event SessionEvent) string {
	data, err := json.Marshal(event)
	if err != nil {
		return ""
	}
	var head struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal(data, &head)
	return head.Kind
}
