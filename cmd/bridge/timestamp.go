package main

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// timestamppbOrZero converts a time.Time to a *timestamppb.Timestamp,
// returning nil for the zero value so optional timestamps stay absent on
// the wire.
func timestamppbOrZero(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}
