package tui

import (
	"os"
	"path/filepath"
	"testing"
)

// W2-01 A1 positive: explicit YTSS_CONFIG_DIR wins as the start directory.
func TestFilePickerStartDirEnvWins(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("YTSS_CONFIG_DIR", dir)
	fp := NewFilePickerModel()
	if fp.fp.CurrentDirectory != dir {
		t.Errorf("CurrentDirectory = %q, want %q (env wins)", fp.fp.CurrentDirectory, dir)
	}
}

// W2-01 A1 boundary: with no env, the candidate chain picks the first
// existing directory (on this machine: ~/kouko-obsidian-vault/_config;
// anywhere: home as the final fallback).
func TestFilePickerStartDirFallsBackThroughChain(t *testing.T) {
	t.Setenv("YTSS_CONFIG_DIR", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	fp := NewFilePickerModel()
	got := fp.fp.CurrentDirectory
	if got == "" {
		t.Fatal("CurrentDirectory empty, want a candidate or home")
	}
	candidates := []string{
		filepath.Join(home, "kouko-obsidian-vault", "_config"),
		filepath.Join(home, ".config", "ytss"),
		home,
	}
	ok := false
	for _, c := range candidates {
		if got == c {
			ok = true
		}
	}
	if !ok {
		t.Errorf("CurrentDirectory = %q, want one of the candidate chain %v", got, candidates)
	}
	if _, err := os.Stat(got); err != nil {
		t.Errorf("CurrentDirectory %q does not exist", got)
	}
}

// W2-01 A2 positive: only .yaml/.yml are selectable.
func TestFilePickerAllowedTypes(t *testing.T) {
	fp := NewFilePickerModel()
	if len(fp.fp.AllowedTypes) != 2 {
		t.Fatalf("AllowedTypes = %v, want 2 entries", fp.fp.AllowedTypes)
	}
	for _, ext := range fp.fp.AllowedTypes {
		if ext != ".yaml" && ext != ".yml" {
			t.Errorf("AllowedTypes contains %q, want only .yaml/.yml", ext)
		}
	}
}

// W2-01 A2 negative: ChosenPath starts empty (no accidental selection).
func TestFilePickerChosenPathStartsEmpty(t *testing.T) {
	fp := NewFilePickerModel()
	if got := fp.ChosenPath(); got != "" {
		t.Errorf("ChosenPath = %q, want empty before any selection", got)
	}
}
