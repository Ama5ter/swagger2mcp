package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateConfigFromFlags(t *testing.T) {
	t.Parallel()

	root := NewRootCmd()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	var captured *GenerateConfig
	generateRunner = func(ctx context.Context, cfg *GenerateConfig) error {
		captured = cfg
		return nil
	}
	t.Cleanup(func() { generateRunner = runGenerate })

	root.SetArgs([]string{
		"--verbose",
		"generate",
		"--input", "spec.yaml",
		"--lang", "npm",
		"--out", "./build",
		"--include-tags", "foo,bar",
		"--exclude-tags", "baz",
		"--tool-name", "my-tool",
		"--package-name", "pkg",
		"--dry-run",
		"--force",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if captured == nil {
		t.Fatalf("expected config to be captured")
	}

	if captured.Input != "spec.yaml" {
		t.Errorf("input mismatch: got %q", captured.Input)
	}
	if captured.Lang != "npm" {
		t.Errorf("lang mismatch: got %q", captured.Lang)
	}
	if captured.Out != "./build" {
		t.Errorf("out mismatch: got %q", captured.Out)
	}
	if want := []string{"foo", "bar"}; !equalStringSlices(captured.IncludeTags, want) {
		t.Errorf("include tags mismatch: got %v", captured.IncludeTags)
	}
	if want := []string{"baz"}; !equalStringSlices(captured.ExcludeTags, want) {
		t.Errorf("exclude tags mismatch: got %v", captured.ExcludeTags)
	}
	if captured.ToolName != "my-tool" {
		t.Errorf("tool name mismatch: got %q", captured.ToolName)
	}
	if captured.PackageName != "pkg" {
		t.Errorf("package name mismatch: got %q", captured.PackageName)
	}
	if !captured.DryRun {
		t.Errorf("expected dry-run true")
	}
	if !captured.Force {
		t.Errorf("expected force true")
	}
	if !captured.Verbose {
		t.Errorf("expected verbose true")
	}
}

func TestGenerateConfigPrecedence(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := strings.TrimSpace(`input: config-spec.yaml
lang: go
out: from-config
includeTags:
  - cfgFoo
excludeTags: cfgBar
toolName: cfg-tool
packageName: cfgpkg
dryRun: true
force: false
verbose: true
`) + "\n"

	if err := os.WriteFile(configPath, []byte(configContent), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	root := NewRootCmd()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	var captured *GenerateConfig
	generateRunner = func(ctx context.Context, cfg *GenerateConfig) error {
		captured = cfg
		return nil
	}
	t.Cleanup(func() { generateRunner = runGenerate })

	root.SetArgs([]string{
		"--config", configPath,
		"generate",
		"--input", "flag-spec.yaml",
		"--include-tags", "flagTag",
		"--dry-run=false",
		"--force",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if captured == nil {
		t.Fatalf("expected config to be captured")
	}

	if captured.Input != "flag-spec.yaml" {
		t.Errorf("input: want %q got %q", "flag-spec.yaml", captured.Input)
	}
	if captured.Lang != "go" {
		t.Errorf("lang: want go got %q", captured.Lang)
	}
	if captured.Out != "from-config" {
		t.Errorf("out: want from-config got %q", captured.Out)
	}
	if want := []string{"flagTag"}; !equalStringSlices(captured.IncludeTags, want) {
		t.Errorf("include tags: want %v got %v", want, captured.IncludeTags)
	}
	if want := []string{"cfgBar"}; !equalStringSlices(captured.ExcludeTags, want) {
		t.Errorf("exclude tags: want %v got %v", want, captured.ExcludeTags)
	}
	if captured.ToolName != "cfg-tool" {
		t.Errorf("tool name mismatch: got %q", captured.ToolName)
	}
	if captured.PackageName != "cfgpkg" {
		t.Errorf("package name mismatch: got %q", captured.PackageName)
	}
	if captured.DryRun {
		t.Errorf("expected dry-run false after flag override")
	}
	if !captured.Force {
		t.Errorf("expected force true after flag override")
	}
	if !captured.Verbose {
		t.Errorf("expected verbose true from config file")
	}
	if captured.ConfigPath != configPath {
		t.Errorf("config path mismatch: got %q", captured.ConfigPath)
	}
}

func TestGenerateConfigUnknownKey(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "bad.yaml")
	if err := os.WriteFile(configPath, []byte("unknown: value\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	root := NewRootCmd()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	root.SetArgs([]string{
		"--config", configPath,
		"generate",
		"--input", "spec.yaml",
	})

	err := root.Execute()
	if err == nil {
		t.Fatalf("expected an error")
	}
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
