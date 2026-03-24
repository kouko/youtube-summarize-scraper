package pipeline

import (
	"context"
	"testing"
)

func newTestPipeline() *Pipeline {
	ctx, cancel := context.WithCancel(context.Background())
	return &Pipeline{
		ctx:    ctx,
		cancel: cancel,
	}
}

func TestStopped_InitiallyFalse(t *testing.T) {
	p := newTestPipeline()
	if p.stopped() {
		t.Error("stopped: expected false initially")
	}
}

func TestStopped_TrueAfterShutdown(t *testing.T) {
	p := newTestPipeline()
	p.Shutdown()
	if !p.stopped() {
		t.Error("stopped: expected true after Shutdown")
	}
}

func TestResetContext_ClearsStopped(t *testing.T) {
	p := newTestPipeline()
	p.Shutdown()
	if !p.stopped() {
		t.Fatal("stopped: expected true after Shutdown")
	}

	p.ResetContext()
	if p.stopped() {
		t.Error("stopped: expected false after ResetContext")
	}
}

func TestShutdown_Idempotent(t *testing.T) {
	p := newTestPipeline()
	p.Shutdown()
	p.Shutdown() // Should not panic.
	if !p.stopped() {
		t.Error("stopped: expected true after double Shutdown")
	}
}
