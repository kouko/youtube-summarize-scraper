package pipeline

import (
	"fmt"
	"testing"
)

func TestClassifyResult(t *testing.T) {
	cause := fmt.Errorf("all providers failed: 429 quota")
	tests := []struct {
		name string
		err  error
		want resultBucket
	}{
		{"nil is success", nil, bucketSuccess},
		{"skipped sentinel", errSkipped, bucketSkipped},
		{"partial sentinel", errPartial, bucketPartial},
		{"wrapped partial (summarization failed under transcription)", fmt.Errorf("%w: %v", errPartial, cause), bucketPartial},
		{"other error is failed", fmt.Errorf("disk full"), bucketFailed},
	}
	for _, tt := range tests {
		if got := classifyResult(tt.err); got != tt.want {
			t.Errorf("%s: classifyResult = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestStats_PartialKeptSeparateFromSuccessAndFailed(t *testing.T) {
	// One of each outcome must land in its own bucket — in particular a
	// partial (transcription produced, summary failed) must NOT inflate
	// Success nor mix into Failed.
	s := &Stats{}
	for _, err := range []error{
		nil,                                    // success
		errSkipped,                             // skipped
		fmt.Errorf("%w: quota", errPartial),    // partial
		fmt.Errorf("both subtitle+whisper KO"), // failed
	} {
		switch classifyResult(err) {
		case bucketSuccess:
			s.Success++
		case bucketSkipped:
			s.Skipped++
		case bucketPartial:
			s.Partial++
		case bucketFailed:
			s.Failed++
		}
	}
	if s.Success != 1 || s.Skipped != 1 || s.Partial != 1 || s.Failed != 1 {
		t.Errorf("counts = %+v, want Success=1 Skipped=1 Partial=1 Failed=1", s)
	}
}
