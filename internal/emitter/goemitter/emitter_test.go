package goemitter

import (
    "context"
    "encoding/json"
    "os"
    "path/filepath"
    "strings"
    "testing"

    genspec "github.com/mark3labs/swagger2mcp/internal/spec"
)

func minimalModel() *genspec.ServiceModel {
    return &genspec.ServiceModel{
        Title:   "Sample API",
        Version: "1.0.0",
        Endpoints: []genspec.EndpointModel{
            {ID: "get /hello", Method: genspec.GET, Path: "/hello", Summary: "Say hello", Tags: []string{"read"}},
        },
        Schemas: map[string]genspec.Schema{
            "Hello": {Name: "Hello", Type: "object", Description: "Greeting"},
        },
    }
}

func TestEmit_DryRun_Plan(t *testing.T) {
    t.Parallel()
    ctx := context.Background()
    dir := t.TempDir()

    sm := minimalModel()
    res, err := Emit(ctx, sm, Options{
        OutDir:     dir,
        ToolName:   "mytool",
        ModuleName: "example.com/mytool",
        DryRun:     true,
    })
    if err != nil {
        t.Fatalf("emit: %v", err)
    }
    if res.ToolName != "mytool" || res.ModuleName != "example.com/mytool" {
        t.Fatalf("names mismatch: %+v", res)
    }
    // Expect a handful of key files in the plan
    want := []string{
        "go.mod",
        "Makefile",
        "README.md",
        filepath.ToSlash(filepath.Join("cmd", "mytool", "main.go")),
        filepath.ToSlash(filepath.Join("internal", "mcp", "server.go")),
        filepath.ToSlash(filepath.Join("internal", "spec", "model.go")),
        filepath.ToSlash(filepath.Join("internal", "spec", "loader.go")),
        filepath.ToSlash(filepath.Join("internal", "spec", "model.json")),
        filepath.ToSlash(filepath.Join("internal", "mcp", "methods", "list_endpoints.go")),
        filepath.ToSlash(filepath.Join("tests", "mcp_methods_test.go")),
    }
    have := make(map[string]bool, len(res.Planned))
    for _, pf := range res.Planned { have[pf.RelPath] = true }
    for _, p := range want {
        if !have[p] {
            t.Fatalf("planned missing %s", p)
        }
    }
    // Dry-run should not have written files
    if entries, _ := os.ReadDir(dir); len(entries) != 0 {
        t.Fatalf("expected no files written on dry-run")
    }
}

func TestEmit_WriteAndContents(t *testing.T) {
    t.Parallel()
    ctx := context.Background()
    dir := t.TempDir()
    sm := minimalModel()
    _, err := Emit(ctx, sm, Options{
        OutDir:     dir,
        ToolName:   "mytool",
        ModuleName: "example.com/mytool",
        Force:      true,
        DryRun:     false,
    })
    if err != nil {
        t.Fatalf("emit: %v", err)
    }

    // go.mod
    gomodPath := filepath.Join(dir, "go.mod")
    data, err := os.ReadFile(gomodPath)
    if err != nil { t.Fatalf("read go.mod: %v", err) }
    if !strings.Contains(string(data), "module example.com/mytool") {
        t.Fatalf("go.mod missing module name: %s", string(data))
    }

    // methods import path
    listPath := filepath.Join(dir, "internal", "mcp", "methods", "list_endpoints.go")
    lst, err := os.ReadFile(listPath)
    if err != nil { t.Fatalf("read methods: %v", err) }
    if !strings.Contains(string(lst), "example.com/mytool/internal/spec") {
        t.Fatalf("methods file missing import rewrite: %s", string(lst))
    }

    // model.json is valid JSON
    modelJSONPath := filepath.Join(dir, "internal", "spec", "model.json")
    j, err := os.ReadFile(modelJSONPath)
    if err != nil { t.Fatalf("read model.json: %v", err) }
    var v any
    if err := json.Unmarshal(j, &v); err != nil {
        t.Fatalf("model.json invalid: %v", err)
    }
}

func TestEmit_NoForce_NonEmptyDir(t *testing.T) {
    t.Parallel()
    ctx := context.Background()
    dir := t.TempDir()
    // create a file to make directory non-empty
    if err := os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("x"), 0o600); err != nil {
        t.Fatalf("prewrite: %v", err)
    }
    _, err := Emit(ctx, minimalModel(), Options{OutDir: dir, ToolName: "tool", ModuleName: "mod"})
    if err == nil {
        t.Fatalf("expected error on non-empty dir without force")
    }
}
