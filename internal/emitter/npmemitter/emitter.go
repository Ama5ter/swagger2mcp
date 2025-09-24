package npmemitter

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

// Options controls how the npm/TypeScript emitter renders a project.
type Options struct {
	OutDir      string // required; target directory to write the project
	ToolName    string // CLI/tool name; used in README and semantics
	PackageName string // npm package name; defaults to derived tool name when empty
	Force       bool   // overwrite existing files
	DryRun      bool   // don't write, only plan
	Verbose     bool
}

// PlannedFile describes a file the emitter intends to write.
type PlannedFile struct {
	RelPath string
	Size    int
	Mode    os.FileMode
}

// Result returns the planned files and final resolved names.
type Result struct {
	ToolName    string
	PackageName string
	Planned     []PlannedFile
}

// Emit renders a Node/TypeScript MCP tool project using the provided ServiceModel (IM).
func Emit(ctx context.Context, sm *genspec.ServiceModel, opts Options) (*Result, error) {
	_ = ctx
	if sm == nil {
		return nil, fmt.Errorf("npmemitter: nil ServiceModel")
	}
	if strings.TrimSpace(opts.OutDir) == "" {
		return nil, fmt.Errorf("npmemitter: OutDir is required")
	}
	toolName := sanitizeToolName(opts.ToolName)
	if toolName == "" {
		toolName = deriveToolName(sm.Title)
		if toolName == "" {
			toolName = "mcp-tool"
		}
	}
	pkgName := sanitizePackageName(strings.TrimSpace(opts.PackageName))
	if pkgName == "" {
		pkgName = toolName
	}

	tmplData := newTemplateData(toolName, pkgName, sm)

	// Build file map
	files := map[string][]byte{}
	// editorconfig + formatting configs
	files[".editorconfig"] = []byte(renderEditorConfig())
	files[".prettierrc.json"] = []byte(renderPrettierRC())
	files[".eslintrc.json"] = []byte(renderESLintRC())
	// package.json
	files["package.json"] = []byte(renderPackageJSON(tmplData))
	// .mcpbignore to reduce bundle size
	files[".mcpbignore"] = []byte(renderMCPBIgnore())
	// tsconfig.json
	files["tsconfig.json"] = []byte(renderTSConfig())
	// Makefile
	files["Makefile"] = []byte(renderMakefileNpm())
	// README
	files["README.md"] = []byte(renderReadme(tmplData))
	// src/index.ts bootstrap (minimal stdio MCP server)
	files[filepath.Join("src", "index.ts")] = []byte(renderIndexTs())
	// spec model + loader + data
	files[filepath.Join("src", "spec", "model.ts")] = []byte(renderSpecModelTs())
	modelJSON, err := json.MarshalIndent(sm, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal model.json: %w", err)
	}
	files[filepath.Join("src", "spec", "model.json")] = append(modelJSON, '\n')
	files[filepath.Join("src", "spec", "loader.ts")] = []byte(renderSpecLoaderTs())
	// methods
	files[filepath.Join("src", "mcp", "methods", "listEndpoints.ts")] = []byte(renderListEndpointsTs())
	files[filepath.Join("src", "mcp", "methods", "searchEndpoints.ts")] = []byte(renderSearchEndpointsTs())
	files[filepath.Join("src", "mcp", "methods", "getEndpointDetails.ts")] = []byte(renderGetEndpointDetailsTs())
	files[filepath.Join("src", "mcp", "methods", "listSchemas.ts")] = []byte(renderListSchemasTs())
	files[filepath.Join("src", "mcp", "methods", "getSchemaDetails.ts")] = []byte(renderGetSchemaDetailsTs())
	files[filepath.Join("src", "mcp", "methods", "index.ts")] = []byte(renderMethodsIndexTs())
	// mcpb manifest
	files["manifest.json"] = []byte(renderMCPBManifest(tmplData))
	// tests
	files[filepath.Join("__tests__", "mcp-methods.test.ts")] = []byte(renderGeneratedTestsTs())
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

	return &Result{ToolName: toolName, PackageName: pkgName, Planned: planned}, nil
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
			return fmt.Errorf("npmemitter: output directory %q is not empty (use --force to overwrite)", abs)
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

func sanitizePackageName(name string) string {
	// Simplified npm name sanitizer (no scope handling here); keep lowercase, dot, dash
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	// replace spaces and slashes
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "/", "-")
	// remove invalid chars
	b := strings.Builder{}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	out = strings.Trim(out, "-.")
	return out
}

func deriveToolName(title string) string {
	t := strings.TrimSpace(title)
	if t == "" {
		return ""
	}
	t = strings.ToLower(t)
	repl := strings.NewReplacer("/", " ", "_", " ", ".", " ", ",", " ", ":", " ")
	t = repl.Replace(t)
	parts := strings.Fields(t)
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "-")
}
