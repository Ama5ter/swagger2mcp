package npmemitter

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
        OutDir:      dir,
        ToolName:    "mytool",
        PackageName: "example-mytool",
        DryRun:      true,
    })
    if err != nil {
        t.Fatalf("emit: %v", err)
    }
    if res.ToolName != "mytool" || res.PackageName != "example-mytool" {
        t.Fatalf("names mismatch: %+v", res)
    }
    // Expect a handful of key files in the plan
    want := []string{
        "manifest.json",
        "Makefile",
        "package.json",
        "tsconfig.json",
        filepath.ToSlash(filepath.Join("src", "index.ts")),
        filepath.ToSlash(filepath.Join("src", "spec", "model.ts")),
        filepath.ToSlash(filepath.Join("src", "spec", "loader.ts")),
        filepath.ToSlash(filepath.Join("src", "spec", "model.json")),
        filepath.ToSlash(filepath.Join("src", "mcp", "methods", "listEndpoints.ts")),
        filepath.ToSlash(filepath.Join("__tests__", "mcp-methods.test.ts")),
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
        OutDir:      dir,
        ToolName:    "mytool",
        PackageName: "example-mytool",
        Force:       true,
        DryRun:      false,
    })
    if err != nil {
        t.Fatalf("emit: %v", err)
    }

    // package.json
    pkgPath := filepath.Join(dir, "package.json")
    data, err := os.ReadFile(pkgPath)
    if err != nil { t.Fatalf("read package.json: %v", err) }
    if !strings.Contains(string(data), "\"name\": \"example-mytool\"") {
        t.Fatalf("package.json missing package name: %s", string(data))
    }
    if !strings.Contains(string(data), "\"bundle\"") {
        t.Fatalf("package.json missing bundle script: %s", string(data))
    }

    // listEndpoints file exists
    listPath := filepath.Join(dir, "src", "mcp", "methods", "listEndpoints.ts")
    if _, err := os.Stat(listPath); err != nil {
        t.Fatalf("missing methods file: %v", err)
    }

    // model.json is valid JSON
    modelJSONPath := filepath.Join(dir, "src", "spec", "model.json")
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
    _, err := Emit(ctx, minimalModel(), Options{OutDir: dir, ToolName: "tool", PackageName: "pkg"})
    if err == nil {
        t.Fatalf("expected error on non-empty dir without force")
    }
}
