package spec

import (
    "context"
    "errors"
    "os"
    "path/filepath"
    "strings"
    "testing"
    "time"
)

func TestLoad_BlocksFileURL(t *testing.T) {
    t.Parallel()
    ctx := context.Background()
    _, err := Load(ctx, "file:///etc/hosts")
    if err == nil {
        t.Fatalf("expected error for file:// URL")
    }
    var se *SpecError
    if !errors.As(err, &se) {
        t.Fatalf("expected SpecError, got %T", err)
    }
    if se.Code != InputError {
        t.Fatalf("expected InputError, got %v", se.Code)
    }
}

func TestLoad_UnsupportedScheme(t *testing.T) {
    t.Parallel()
    ctx := context.Background()
    _, err := Load(ctx, "ftp://example.com/spec.yaml")
    if err == nil {
        t.Fatalf("expected error for unsupported scheme")
    }
    var se *SpecError
    if !errors.As(err, &se) || se.Code != InputError {
        t.Fatalf("expected InputError, got %v (%T)", err, err)
    }
}

func TestLoad_NetworkError(t *testing.T) {
    t.Parallel()
    // Unused port to provoke a quick network failure.
    url := "http://127.0.0.1:1/spec.yaml"
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    _, err := Load(ctx, url, WithHTTPTimeout(200*time.Millisecond), WithMaxRetries(2))
    if err == nil {
        t.Fatalf("expected network error")
    }
    var se *SpecError
    if !errors.As(err, &se) || se.Code != NetworkError {
        t.Fatalf("expected NetworkError, got %v (%T)", err, err)
    }
}

func TestLoad_V3_InvalidSpec(t *testing.T) {
    t.Parallel()
    dir := t.TempDir()
    path := filepath.Join(dir, "bad.yaml")
    content := strings.TrimSpace(`openapi: 3.0.0
info:
  title: Bad
  version: "1.0.0"
paths:
  "/pet":
    get:
      responses: {}
`) + "\n"
    if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
        t.Fatalf("write: %v", err)
    }

    ctx := context.Background()
    _, err := Load(ctx, path)
    if err == nil {
        t.Fatalf("expected validation error for incomplete responses")
    }
    var se *SpecError
    if !errors.As(err, &se) {
        t.Fatalf("expected SpecError, got %T", err)
    }
    if se.Code != ValidationError && se.Code != ParseError { // parser version differences
        t.Fatalf("expected ValidationError/ParseError, got %v", se.Code)
    }
    if se.Location == "" {
        t.Fatalf("expected location to be set")
    }
}

func TestLoad_V2_Conversion_Success(t *testing.T) {
    t.Parallel()
    dir := t.TempDir()
    path := filepath.Join(dir, "swagger.yaml")
    content := strings.TrimSpace(`swagger: "2.0"
info:
  title: Sample
  version: "1.0.0"
paths:
  "/hello":
    get:
      responses:
        "200":
          description: ok
`) + "\n"
    if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
        t.Fatalf("write: %v", err)
    }

    ctx := context.Background()
    doc, err := Load(ctx, path)
    if err != nil {
        t.Fatalf("load: %v", err)
    }
    if doc == nil {
        t.Fatalf("expected doc")
    }
    if !strings.HasPrefix(doc.OpenAPI, "3.") {
        t.Fatalf("expected OpenAPI v3, got %q", doc.OpenAPI)
    }
}

func TestLoad_V2_Conversion_Failure(t *testing.T) {
    t.Parallel()
    dir := t.TempDir()
    path := filepath.Join(dir, "swagger-bad.yaml")
    content := strings.TrimSpace(`swagger: "2.0"
paths: {}
`) + "\n"
    if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
        t.Fatalf("write: %v", err)
    }

    ctx := context.Background()
    _, err := Load(ctx, path)
    if err == nil {
        t.Fatalf("expected conversion error")
    }
    var se *SpecError
    if !errors.As(err, &se) {
        t.Fatalf("expected SpecError, got %T", err)
    }
    if se.Code != ConversionError && se.Code != ValidationError && se.Code != ParseError {
        t.Fatalf("expected ConversionError/ValidationError/ParseError, got %v", se.Code)
    }
}

