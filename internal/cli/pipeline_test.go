package cli

import (
    "bytes"
    "io"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

const minimalSpecYAML = "" +
    "openapi: 3.0.0\n" +
    "info:\n" +
    "  title: Test API\n" +
    "  version: '1.0.0'\n" +
    "paths:\n" +
    "  /hello:\n" +
    "    get:\n" +
    "      summary: Hello\n" +
    "      responses:\n" +
    "        '200':\n" +
    "          description: ok\n"

func captureStdout(fn func()) string {
    old := os.Stdout
    r, w, _ := os.Pipe()
    os.Stdout = w
    defer func() { os.Stdout = old }()
    fn()
    _ = w.Close()
    var buf bytes.Buffer
    _, _ = io.Copy(&buf, r)
    return buf.String()
}

func TestGeneratePipeline_DryRun_Go(t *testing.T) {
    dir := t.TempDir()
    specPath := filepath.Join(dir, "spec.yaml")
    if err := os.WriteFile(specPath, []byte(minimalSpecYAML), 0o600); err != nil {
        t.Fatalf("write spec: %v", err)
    }
    outDir := filepath.Join(dir, "out-go")

    root := NewRootCmd()
    root.SetOut(io.Discard)
    root.SetErr(io.Discard)
    root.SetArgs([]string{"generate", "--input", specPath, "--lang", "go", "--out", outDir, "--dry-run"})

    out := captureStdout(func() {
        if err := root.Execute(); err != nil {
            t.Fatalf("execute: %v", err)
        }
    })
    if !strings.Contains(out, "Planned writes to") {
        t.Fatalf("expected dry-run plan output, got: %s", out)
    }
    // Dry-run should not create the directory
    if _, err := os.Stat(outDir); err == nil {
        t.Fatalf("expected no writes on dry-run")
    }
}

func TestGeneratePipeline_DryRun_Npm(t *testing.T) {
    dir := t.TempDir()
    specPath := filepath.Join(dir, "spec.yaml")
    if err := os.WriteFile(specPath, []byte(minimalSpecYAML), 0o600); err != nil {
        t.Fatalf("write spec: %v", err)
    }
    outDir := filepath.Join(dir, "out-npm")

    root := NewRootCmd()
    root.SetOut(io.Discard)
    root.SetErr(io.Discard)
    root.SetArgs([]string{"generate", "--input", specPath, "--lang", "npm", "--out", outDir, "--dry-run"})

    out := captureStdout(func() {
        if err := root.Execute(); err != nil {
            t.Fatalf("execute: %v", err)
        }
    })
    if !strings.Contains(out, "Planned writes to") {
        t.Fatalf("expected dry-run plan output, got: %s", out)
    }
    if _, err := os.Stat(outDir); err == nil {
        t.Fatalf("expected no writes on dry-run")
    }
}
