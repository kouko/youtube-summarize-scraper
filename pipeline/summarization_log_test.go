package pipeline

import (
	"strings"
	"testing"
)

func TestEmptyResponseError_NamesProviderAndModel(t *testing.T) {
	err := emptyResponseError("stage 1", "qwen-code", "coder-model")
	if err == nil {
		t.Fatal("expected a non-nil error")
	}
	msg := err.Error()
	for _, want := range []string{"stage 1", "empty response", "qwen-code", "coder-model"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message %q is missing %q", msg, want)
		}
	}
}

func TestEmptyResponseError_StillReadsWithUnknownProvider(t *testing.T) {
	// Defensive: even if provider/model are empty, the message must still
	// identify it as an empty-response failure rather than crash or read blank.
	err := emptyResponseError("stage 1", "", "")
	if err == nil || !strings.Contains(err.Error(), "empty response") {
		t.Errorf("expected an empty-response error, got: %v", err)
	}
}
