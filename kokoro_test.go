package main

import (
	"math"
	"net/http"
	"testing"
)

func TestParseWordTimestampsAcceptsBothShapes(t *testing.T) {
	startTime, endTime := 0.1, 0.4
	start, end := 0.5, 0.9

	got := parseWordTimestamps([]rawTimestamp{
		{Word: "one", StartTime: &startTime, EndTime: &endTime},
		{Word: "two", Start: &start, End: &end},
	})

	want := []WordTS{
		{Word: "one", StartTime: 0.1, EndTime: 0.4},
		{Word: "two", StartTime: 0.5, EndTime: 0.9},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d timestamps, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("timestamp %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestNormalizeWordTimestampsRepairsMissingAndInvalidTimes(t *testing.T) {
	got := normalizeWordTimestamps([]WordTS{
		{Word: "one", StartTime: 0.2},
		{Word: "two", StartTime: 0.8, EndTime: 2},
		{Word: "three", StartTime: 0.7, EndTime: math.NaN()},
	})

	want := [][2]float64{{0.2, 0.8}, {0.8, 2}, {0.8, 1.05}}
	for i := range want {
		if got[i].StartTime != want[i][0] || got[i].EndTime != want[i][1] {
			t.Errorf("timestamp %d: got [%v,%v], want [%v,%v]",
				i, got[i].StartTime, got[i].EndTime, want[i][0], want[i][1])
		}
	}
}

func TestNormalizeWordTimestampsReturnsNonNilEmptySlice(t *testing.T) {
	if got := normalizeWordTimestamps(nil); got == nil || len(got) != 0 {
		t.Fatalf("got %#v, want non-nil empty slice", got)
	}
}

func TestRetryableCaptionError(t *testing.T) {
	if !retryableCaptionError(&upstreamHTTPError{status: http.StatusTooManyRequests}) {
		t.Error("429 should be retryable")
	}
	if !retryableCaptionError(&upstreamHTTPError{status: http.StatusBadGateway}) {
		t.Error("502 should be retryable")
	}
	if retryableCaptionError(&upstreamHTTPError{status: http.StatusBadRequest}) {
		t.Error("400 should not be retryable")
	}
}
