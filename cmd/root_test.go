package cmd

import (
	"strings"
	"testing"
)

// TestRootCmd_LLMFlagUsage_MentionsInstanceSyntax verifies that the --llm
// flag help text documents the named-instance syntax, so users learn they
// can target a specific openai-compat instance (and that bare openai-compat
// means the "default" instance) without reading the source.
func TestRootCmd_LLMFlagUsage_MentionsInstanceSyntax(t *testing.T) {
	flag := rootCmd.PersistentFlags().Lookup("llm")
	if flag == nil {
		t.Fatal("expected --llm flag to be registered")
	}
	if !strings.Contains(flag.Usage, "openai-compat:<name>") {
		t.Errorf("--llm usage should mention named-instance syntax %q; got: %q", "openai-compat:<name>", flag.Usage)
	}
}
