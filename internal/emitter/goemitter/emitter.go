package goemitter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	genspec "github.com/mark3labs/swagger2mcp/internal/spec"
)

// Options controls how the Go emitter renders a project.
type Options struct {
	OutDir     string // required; target directory to write the project
	ToolName   string // tool binary name; used under cmd/<tool>/
	ModuleName string // go module name; defaults to ToolName when empty
	Force      bool   // overwrite existing files
	DryRun     bool   // don't write, only plan
	Verbose    bool
}

// PlannedFile describes a file the emitter intends to write.
type PlannedFile struct {
	RelPath string
	Size    int
	Mode    os.FileMode
}

// Result returns the planned files and final resolved names.
type Result struct {
	ToolName   string
	ModuleName string
	Planned    []PlannedFile
}

// Emit renders a Go MCP tool project using the provided ServiceModel (IM).
func Emit(ctx context.Context, sm *genspec.ServiceModel, opts Options) (*Result, error) {
	_ = ctx
	if sm == nil {
		return nil, fmt.Errorf("goemitter: nil ServiceModel")
	}
	if strings.TrimSpace(opts.OutDir) == "" {
		return nil, fmt.Errorf("goemitter: OutDir is required")
	}
	toolName := sanitizeToolName(opts.ToolName)
	if toolName == "" {
		// derive from service title as a fallback
		toolName = deriveToolName(sm.Title)
		if toolName == "" {
			toolName = "mcp-tool"
		}
	}
	moduleName := strings.TrimSpace(opts.ModuleName)
	if moduleName == "" {
		moduleName = toolName
	}

	tmplData := newTemplateData(toolName, moduleName, sm)

	// Build file map
	files := map[string][]byte{}
	// editorconfig for consistent formatting
	files[".editorconfig"] = []byte(renderEditorConfig())
	// go.mod
	gomod := renderGoMod(tmplData)
	files["go.mod"] = []byte(gomod)
	// Makefile
	files["Makefile"] = []byte(renderMakefileGo())
	// README
	files["README.md"] = []byte(renderReadme(tmplData))
	// main.go
	mainPath := filepath.Join("cmd", toolName, "main.go")
	files[mainPath] = []byte(renderMainGo(tmplData))
	// internal/spec model + loader + data
	files[filepath.Join("internal", "spec", "model.go")] = []byte(renderSpecModelGo())
	// model.json
	modelJSON, err := json.MarshalIndent(sm, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal model.json: %w", err)
	}
	files[filepath.Join("internal", "spec", "model.json")] = append(modelJSON, '\n')
	files[filepath.Join("internal", "spec", "loader.go")] = []byte(renderSpecLoaderGo())
	// mcp server bootstrap wiring
	files[filepath.Join("internal", "mcp", "server.go")] = []byte(renderMCPBootstrapGo(tmplData))
	// methods (inject module import path)
	files[filepath.Join("internal", "mcp", "methods", "list_endpoints.go")] = []byte(renderListEndpointsGo(tmplData))
	files[filepath.Join("internal", "mcp", "methods", "search_endpoints.go")] = []byte(renderSearchEndpointsGo(tmplData))
	files[filepath.Join("internal", "mcp", "methods", "utils.go")] = []byte(renderUtilsGo(tmplData))
	files[filepath.Join("internal", "mcp", "methods", "get_endpoint_details.go")] = []byte(renderGetEndpointDetailsGo(tmplData))
	files[filepath.Join("internal", "mcp", "methods", "list_schemas.go")] = []byte(renderListSchemasGo(tmplData))
	files[filepath.Join("internal", "mcp", "methods", "get_schema_details.go")] = []byte(renderGetSchemaDetailsGo(tmplData))
	// tests
	files[filepath.Join("tests", "mcp_methods_test.go")] = []byte(renderGeneratedTests(tmplData))
	// testdata sample spec (informational)
	files[filepath.Join("testdata", "sample.yaml")] = []byte(sampleSpecYAML)

	// Plan in deterministic order
	rels := make([]string, 0, len(files))
	for p := range files {
		rels = append(rels, filepath.ToSlash(p))
	}
	sort.Strings(rels)

	planned := make([]PlannedFile, 0, len(rels))
	for _, rel := range rels {
		planned = append(planned, PlannedFile{RelPath: rel, Size: len(files[rel]), Mode: 0o644})
	}

	// Write if not dry-run
	if !opts.DryRun {
		if err := writeFiles(opts.OutDir, files, opts.Force); err != nil {
			return nil, err
		}
	}

	return &Result{ToolName: toolName, ModuleName: moduleName, Planned: planned}, nil
}

func writeFiles(outDir string, files map[string][]byte, force bool) error {
	abs, err := filepath.Abs(outDir)
	if err != nil {
		return fmt.Errorf("resolve out dir: %w", err)
	}
	// Pre-flight: if directory exists and not empty and not force, error.
	if st, err := os.Stat(abs); err == nil && st.IsDir() && !force {
		// check emptiness
		entries, rerr := os.ReadDir(abs)
		if rerr == nil && len(entries) > 0 {
			return fmt.Errorf("goemitter: output directory %q is not empty (use --force to overwrite)", abs)
		}
	}
	for rel, content := range files {
		p := filepath.Join(abs, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return fmt.Errorf("mkdir: %w", err)
		}
		// atomic write via temp file + rename
		tmp := p + ".tmp-" + time.Now().Format("20060102150405")
		if err := os.WriteFile(tmp, content, 0o644); err != nil {
			return fmt.Errorf("write temp %s: %w", rel, err)
		}
		if err := os.Rename(tmp, p); err != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("rename %s: %w", rel, err)
		}
	}
	return nil
}

func sanitizeToolName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	// replace spaces and slashes
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ToLower(name)
	// keep alnum, dash, underscore only
	b := strings.Builder{}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	out = strings.Trim(out, "-")
	return out
}

func deriveToolName(title string) string {
	t := strings.TrimSpace(title)
	if t == "" {
		return ""
	}
	// split on spaces and punctuation, join with dash
	t = strings.ToLower(t)
	repl := strings.NewReplacer("/", " ", "_", " ", ".", " ", ",", " ", ":", " ")
	t = repl.Replace(t)
	parts := strings.Fields(t)
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "-")
}
