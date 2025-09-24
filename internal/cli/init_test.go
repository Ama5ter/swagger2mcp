package cli

import (
    "io"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestInit_WritesSampleConfig(t *testing.T) {
    t.Parallel()
    dir := t.TempDir()
    path := filepath.Join(dir, "config.yaml")

    root := NewRootCmd()
    root.SetOut(io.Discard)
    root.SetErr(io.Discard)
    root.SetArgs([]string{"init", "--out", path})

    if err := root.Execute(); err != nil {
        t.Fatalf("init execute: %v", err)
    }

    data, err := os.ReadFile(path)
    if err != nil {
        t.Fatalf("read config: %v", err)
    }
    s := string(data)
    if !strings.Contains(s, "swagger2mcp configuration") {
        t.Fatalf("unexpected config contents: %s", s)
    }
}

func TestInit_ExistingWithoutForce(t *testing.T) {
    t.Parallel()
    dir := t.TempDir()
    path := filepath.Join(dir, "config.yaml")
    if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
        t.Fatalf("prewrite: %v", err)
    }

    root := NewRootCmd()
    root.SetOut(io.Discard)
    root.SetErr(io.Discard)
    root.SetArgs([]string{"init", "--out", path})

    err := root.Execute()
    if err == nil {
        t.Fatalf("expected error for existing file without --force")
    }
    if _, ok := err.(usageError); !ok {
        t.Fatalf("expected usage error, got %T: %v", err, err)
    }
}

