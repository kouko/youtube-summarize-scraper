package output

import (
	"testing"
	"time"
)

func TestConvertUploadDate_TimestampShiftsForward(t *testing.T) {
	// UTC 2024-03-14 23:00:00 → Asia/Taipei (UTC+8) = 2024-03-15 07:00:00
	loc, _ := time.LoadLocation("Asia/Taipei")
	ts := time.Date(2024, 3, 14, 23, 0, 0, 0, time.UTC).Unix()
	got := ConvertUploadDate("20240314", ts, loc)
	if got != "2024-03-15" {
		t.Errorf("expected 2024-03-15, got %s", got)
	}
}

func TestConvertUploadDate_TimestampShiftsBackward(t *testing.T) {
	// UTC 2024-03-15 01:00:00 → US/Pacific (UTC-7 DST) = 2024-03-14 18:00:00
	loc, _ := time.LoadLocation("US/Pacific")
	ts := time.Date(2024, 3, 15, 1, 0, 0, 0, time.UTC).Unix()
	got := ConvertUploadDate("20240315", ts, loc)
	if got != "2024-03-14" {
		t.Errorf("expected 2024-03-14, got %s", got)
	}
}

func TestConvertUploadDate_TimestampZeroFallback(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Taipei")
	got := ConvertUploadDate("20240315", 0, loc)
	if got != "2024-03-15" {
		t.Errorf("expected 2024-03-15, got %s", got)
	}
}

func TestConvertUploadDate_EmptyString(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Taipei")
	got := ConvertUploadDate("", 0, loc)
	if got != "" {
		t.Errorf("expected empty string, got %s", got)
	}
}

func TestConvertUploadDate_NilLocation(t *testing.T) {
	ts := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC).Unix()
	got := ConvertUploadDate("20240315", ts, nil)
	if got != "2024-03-15" {
		t.Errorf("expected 2024-03-15, got %s", got)
	}
}

func TestConvertUploadDate_UTC(t *testing.T) {
	ts := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC).Unix()
	got := ConvertUploadDate("20240315", ts, time.UTC)
	if got != "2024-03-15" {
		t.Errorf("expected 2024-03-15, got %s", got)
	}
}

func TestFormatUploadTime_WithTimestamp(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Taipei")
	ts := time.Date(2024, 3, 15, 23, 30, 0, 0, time.UTC).Unix()
	got := FormatUploadTime(ts, loc)
	expected := "2024-03-16T07:30:00+08:00"
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestFormatUploadTime_Zero(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Taipei")
	got := FormatUploadTime(0, loc)
	if got != "" {
		t.Errorf("expected empty string, got %s", got)
	}
}

func TestFormatUploadTime_NilLocation(t *testing.T) {
	ts := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC).Unix()
	got := FormatUploadTime(ts, nil)
	expected := "2024-03-15T12:00:00Z"
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}
