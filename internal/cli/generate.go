package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	goemitter "github.com/mark3labs/swagger2mcp/internal/emitter/goemitter"
	npmemitter "github.com/mark3labs/swagger2mcp/internal/emitter/npmemitter"
	pyemitter "github.com/mark3labs/swagger2mcp/internal/emitter/pyemitter"
	genspec "github.com/mark3labs/swagger2mcp/internal/spec"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

// GenerateConfig captures all inputs that influence the generate command after
// merging defaults, config file values, and CLI overrides.
type GenerateConfig struct {
	Input       string
	Lang        string
	Out         string
	IncludeTags []string
	ExcludeTags []string
	ToolName    string
	PackageName string
	ConfigPath  string
	DryRun      bool
	Force       bool
	Verbose     bool
}

func defaultGenerateConfig() GenerateConfig {
	return GenerateConfig{Lang: "go"}
}

var generateRunner = runGenerate

func newGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate an MCP tool project from an OpenAPI/Swagger document",
		Long: "Generate an MCP tool project from an OpenAPI/Swagger document. " +
			"Options can be provided via flags, config files, or defaults.",
		Example: strings.TrimSpace(`  swagger2mcp generate --input spec.yaml --lang go --out ./out
  swagger2mcp --config config.yaml generate --force --dry-run`),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := resolveGenerateConfig(cmd)
			if err != nil {
				return err
			}
			return generateRunner(cmd.Context(), cfg)
		},
	}

	flags := cmd.Flags()
	flags.String("input", "", "Path or URL to the Swagger/OpenAPI document")
	flags.String("lang", "", "Target language to emit (go|npm|python); defaults to go")
	flags.String("out", "", "Output directory (derived from spec when omitted)")
	flags.StringSlice("include-tags", nil, "Only include operations with these tags")
	flags.StringSlice("exclude-tags", nil, "Exclude operations with these tags")
	flags.String("tool-name", "", "Override the generated MCP tool name")
	flags.String("package-name", "", "Override the generated package/module name")
	flags.Bool("dry-run", false, "Preview planned outputs without writing files")
	flags.Bool("force", false, "Overwrite existing output when set")

	return cmd
}

func resolveGenerateConfig(cmd *cobra.Command) (*GenerateConfig, error) {
	cfg := defaultGenerateConfig()

	configPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return nil, err
	}
	configPath = strings.TrimSpace(configPath)
	if configPath != "" {
		cfg.ConfigPath = configPath
		if err := applyGenerateConfigFromFile(&cfg, configPath); err != nil {
			return nil, err
		}
	}

	if err := applyGenerateFlagOverrides(cmd.Flags(), &cfg); err != nil {
		return nil, err
	}

	cfg.normalize()
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func applyGenerateFlagOverrides(flags *pflag.FlagSet, cfg *GenerateConfig) error {
	if flags.Changed("input") {
		value, err := flags.GetString("input")
		if err != nil {
			return err
		}
		cfg.Input = strings.TrimSpace(value)
	}
	if flags.Changed("lang") {
		value, err := flags.GetString("lang")
		if err != nil {
			return err
		}
		cfg.Lang = strings.TrimSpace(value)
	}
	if flags.Changed("out") {
		value, err := flags.GetString("out")
		if err != nil {
			return err
		}
		cfg.Out = strings.TrimSpace(value)
	}
	if flags.Changed("include-tags") {
		value, err := flags.GetStringSlice("include-tags")
		if err != nil {
			return err
		}
		cfg.IncludeTags = sanitizeTags(value)
	}
	if flags.Changed("exclude-tags") {
		value, err := flags.GetStringSlice("exclude-tags")
		if err != nil {
			return err
		}
		cfg.ExcludeTags = sanitizeTags(value)
	}
	if flags.Changed("tool-name") {
		value, err := flags.GetString("tool-name")
		if err != nil {
			return err
		}
		cfg.ToolName = strings.TrimSpace(value)
	}
	if flags.Changed("package-name") {
		value, err := flags.GetString("package-name")
		if err != nil {
			return err
		}
		cfg.PackageName = strings.TrimSpace(value)
	}
	if flags.Changed("dry-run") {
		value, err := flags.GetBool("dry-run")
		if err != nil {
			return err
		}
		cfg.DryRun = value
	}
	if flags.Changed("force") {
		value, err := flags.GetBool("force")
		if err != nil {
			return err
		}
		cfg.Force = value
	}
	if flags.Changed("verbose") {
		value, err := flags.GetBool("verbose")
		if err != nil {
			return err
		}
		cfg.Verbose = value
	}

	return nil
}

func (c *GenerateConfig) normalize() {
	c.Input = strings.TrimSpace(c.Input)
	c.Lang = strings.ToLower(strings.TrimSpace(c.Lang))
	c.Out = strings.TrimSpace(c.Out)
	c.ToolName = strings.TrimSpace(c.ToolName)
	c.PackageName = strings.TrimSpace(c.PackageName)
	c.IncludeTags = sanitizeTags(c.IncludeTags)
	c.ExcludeTags = sanitizeTags(c.ExcludeTags)
}

func (c *GenerateConfig) validate() error {
	if c.Input == "" {
		return newUsageError("generate: --input is required (set via flag or config file)")
	}

	switch c.Lang {
	case "", "go", "npm", "python":
		if c.Lang == "" {
			c.Lang = "go"
		}
	default:
		return newUsageError(fmt.Sprintf("generate: unsupported --lang %q (allowed: go, npm, python)", c.Lang))
	}

	overlap := intersect(c.IncludeTags, c.ExcludeTags)
	if len(overlap) > 0 {
		return newUsageError(fmt.Sprintf("generate: include/exclude tags overlap: %s", strings.Join(overlap, ", ")))
	}

	return nil
}

func runGenerate(ctx context.Context, cfg *GenerateConfig) error {
	// 1) Load the spec (file or http/https URL) with validation and conversion
	doc, err := genspec.Load(ctx, cfg.Input)
	if err != nil {
		// Map structured spec errors into friendly messages
		var se *genspec.SpecError
		if errors.As(err, &se) {
			msg := fmt.Sprintf("spec: %s", se.Message)
			if se.Location != "" {
				msg = fmt.Sprintf("%s\nLocation: %s", msg, se.Location)
			}
			if se.JSONPointer != "" {
				msg = fmt.Sprintf("%s\nPointer: %s", msg, se.JSONPointer)
			}
			return newUsageError(msg)
		}
		return err
	}

	// 2) Build the internal model (IM) with tag filters
	sm, err := genspec.BuildServiceModel(
		ctx,
		doc,
		nil, // v2Raw - we'll add this later when we detect v2 conversion
		genspec.WithIncludeTags(cfg.IncludeTags),
		genspec.WithExcludeTags(cfg.ExcludeTags),
	)
	if err != nil {
		return fmt.Errorf("build model: %w", err)
	}

	// 3) Derive sensible defaults for names and out dir when omitted
	outDir := strings.TrimSpace(cfg.Out)
	resolvedToolName := strings.TrimSpace(cfg.ToolName)
	if resolvedToolName == "" {
		resolvedToolName = deriveToolName(sm.Title)
		if resolvedToolName == "" {
			resolvedToolName = "mcp-tool"
		}
	} else {
		resolvedToolName = sanitizeToolName(resolvedToolName)
		if resolvedToolName == "" { // after sanitization
			resolvedToolName = "mcp-tool"
		}
	}
	if outDir == "" {
		outDir = resolvedToolName
	}

	// Ensure outDir is absolute only for display; emitters handle actual creation/writes
	absOut := outDir
	if ap, err := filepath.Abs(outDir); err == nil {
		absOut = ap
	}

	// 4) Emit for the chosen language
	switch cfg.Lang {
	case "go":
		res, err := goemitter.Emit(ctx, sm, goemitter.Options{
			OutDir:     outDir,
			ToolName:   resolvedToolName,
			ModuleName: strings.TrimSpace(cfg.PackageName),
			Force:      cfg.Force,
			DryRun:     cfg.DryRun,
			Verbose:    cfg.Verbose,
		})
		if err != nil {
			return wrapOutputError(err, absOut)
		}
		if cfg.DryRun {
			printPlan(absOut, len(res.Planned), func() []string {
				paths := make([]string, 0, len(res.Planned))
				for _, p := range res.Planned {
					paths = append(paths, p.RelPath)
				}
				return paths
			}())
		}
	case "npm":
		res, err := npmemitter.Emit(ctx, sm, npmemitter.Options{
			OutDir:      outDir,
			ToolName:    resolvedToolName,
			PackageName: strings.TrimSpace(cfg.PackageName),
			Force:       cfg.Force,
			DryRun:      cfg.DryRun,
			Verbose:     cfg.Verbose,
		})
		if err != nil {
			return wrapOutputError(err, absOut)
		}
		if cfg.DryRun {
			printPlan(absOut, len(res.Planned), func() []string {
				paths := make([]string, 0, len(res.Planned))
				for _, p := range res.Planned {
					paths = append(paths, p.RelPath)
				}
				return paths
			}())
		}
	case "python":
		res, err := pyemitter.Emit(ctx, sm, pyemitter.Options{
			OutDir:      outDir,
			ToolName:    resolvedToolName,
			PackageName: strings.TrimSpace(cfg.PackageName),
			Force:       cfg.Force,
			DryRun:      cfg.DryRun,
			Verbose:     cfg.Verbose,
		})
		if err != nil {
			return wrapOutputError(err, absOut)
		}
		if cfg.DryRun {
			printPlan(absOut, len(res.Planned), func() []string {
				paths := make([]string, 0, len(res.Planned))
				for _, p := range res.Planned {
					paths = append(paths, p.RelPath)
				}
				return paths
			}())
		}
	default:
		// Should not happen due to earlier validation, but keep defensive.
		return newUsageError(fmt.Sprintf("generate: unsupported --lang %q (allowed: go, npm, python)", cfg.Lang))
	}

	return nil
}

func printPlan(outDir string, count int, relPaths []string) {
	fmt.Fprintf(os.Stdout, "Planned writes to %s (%d files):\n", outDir, count)
	for _, p := range relPaths {
		fmt.Fprintf(os.Stdout, "- %s\n", p)
	}
}

func wrapOutputError(err error, outDir string) error {
	// Provide clearer guidance for common FS failures.
	msg := err.Error()
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "permission") || strings.Contains(lower, "read-only") || strings.Contains(lower, "mkdir") || strings.Contains(lower, "rename") || strings.Contains(lower, "output directory") {
		return newUsageError(fmt.Sprintf("output error for %s: %s\nHint: choose a different --out or use --force when appropriate.", outDir, msg))
	}
	return err
}

func sanitizeToolName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ToLower(name)
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	out := strings.Trim(b.String(), "-")
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

func sanitizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(tags))
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func intersect(a, b []string) []string {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(a))
	for _, item := range a {
		set[item] = struct{}{}
	}
	var result []string
	for _, item := range b {
		if _, ok := set[item]; ok {
			result = append(result, item)
		}
	}
	return result
}

func applyGenerateConfigFromFile(cfg *GenerateConfig, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return newUsageError(fmt.Sprintf("read config file %q: %v", path, err))
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return newUsageError(fmt.Sprintf("parse config file %q: %v", path, err))
	}

	for key, value := range raw {
		normalized := normalizeKey(key)
		switch normalized {
		case "input":
			str, err := valueAsString(value)
			if err != nil {
				return newUsageError(fmt.Sprintf("config field %q: %v", key, err))
			}
			cfg.Input = str
		case "lang":
			str, err := valueAsString(value)
			if err != nil {
				return newUsageError(fmt.Sprintf("config field %q: %v", key, err))
			}
			cfg.Lang = str
		case "out":
			str, err := valueAsString(value)
			if err != nil {
				return newUsageError(fmt.Sprintf("config field %q: %v", key, err))
			}
			cfg.Out = str
		case "includetags":
			list, err := valueAsStringSlice(value)
			if err != nil {
				return newUsageError(fmt.Sprintf("config field %q: %v", key, err))
			}
			cfg.IncludeTags = sanitizeTags(list)
		case "excludetags":
			list, err := valueAsStringSlice(value)
			if err != nil {
				return newUsageError(fmt.Sprintf("config field %q: %v", key, err))
			}
			cfg.ExcludeTags = sanitizeTags(list)
		case "toolname":
			str, err := valueAsString(value)
			if err != nil {
				return newUsageError(fmt.Sprintf("config field %q: %v", key, err))
			}
			cfg.ToolName = str
		case "packagename":
			str, err := valueAsString(value)
			if err != nil {
				return newUsageError(fmt.Sprintf("config field %q: %v", key, err))
			}
			cfg.PackageName = str
		case "dryrun":
			val, err := valueAsBool(value)
			if err != nil {
				return newUsageError(fmt.Sprintf("config field %q: %v", key, err))
			}
			cfg.DryRun = val
		case "force":
			val, err := valueAsBool(value)
			if err != nil {
				return newUsageError(fmt.Sprintf("config field %q: %v", key, err))
			}
			cfg.Force = val
		case "verbose":
			val, err := valueAsBool(value)
			if err != nil {
				return newUsageError(fmt.Sprintf("config field %q: %v", key, err))
			}
			cfg.Verbose = val
		default:
			return newUsageError(fmt.Sprintf("config file %q: unknown field %q", path, key))
		}
	}

	return nil
}

func normalizeKey(raw string) string {
	lowered := strings.ToLower(strings.TrimSpace(raw))
	lowered = strings.ReplaceAll(lowered, "-", "")
	lowered = strings.ReplaceAll(lowered, "_", "")
	return lowered
}

func valueAsString(v any) (string, error) {
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val), nil
	case nil:
		return "", nil
	default:
		return "", fmt.Errorf("expected string, got %T", v)
	}
}

func valueAsStringSlice(v any) ([]string, error) {
	switch val := v.(type) {
	case nil:
		return nil, nil
	case string:
		if strings.TrimSpace(val) == "" {
			return nil, nil
		}
		return splitAndTrim(val), nil
	case []any:
		items := make([]string, 0, len(val))
		for idx, elem := range val {
			str, err := valueAsString(elem)
			if err != nil {
				return nil, fmt.Errorf("element %d: %w", idx, err)
			}
			if str != "" {
				items = append(items, str)
			}
		}
		return items, nil
	default:
		return nil, fmt.Errorf("expected string or list, got %T", v)
	}
}

func valueAsBool(v any) (bool, error) {
	switch val := v.(type) {
	case bool:
		return val, nil
	case string:
		trimmed := strings.ToLower(strings.TrimSpace(val))
		switch trimmed {
		case "true", "t", "1", "yes", "y":
			return true, nil
		case "false", "f", "0", "no", "n":
			return false, nil
		case "":
			return false, nil
		default:
			return false, fmt.Errorf("invalid boolean value %q", val)
		}
	case nil:
		return false, nil
	default:
		return false, fmt.Errorf("expected boolean, got %T", v)
	}
}

func splitAndTrim(csv string) []string {
	parts := strings.Split(csv, ",")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}
