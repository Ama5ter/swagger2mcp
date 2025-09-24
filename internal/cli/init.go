package cli

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/spf13/cobra"
)

// InitConfig captures the options for the init command.
type InitConfig struct {
	OutputPath string
	Force      bool
	Verbose    bool
}

var initRunner = runInit

func newInitCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "init",
        Short: "Scaffold a sample swagger2mcp configuration file",
        Long:  "Scaffold a commented swagger2mcp configuration file that documents available options.",
        RunE: func(cmd *cobra.Command, args []string) error {
            out, err := cmd.Flags().GetString("out")
            if err != nil {
                return err
            }
            force, err := cmd.Flags().GetBool("force")
            if err != nil {
                return err
            }
            verbose, err := cmd.Flags().GetBool("verbose")
            if err != nil {
                return err
            }
            cfg := &InitConfig{
                OutputPath: out,
                Force:      force,
                Verbose:    verbose,
            }
            return initRunner(cmd.Context(), cfg)
        },
    }

    cmd.Flags().String("out", "swagger2mcp.yaml", "Where to write the sample config file")
    cmd.Flags().Bool("force", false, "Overwrite the target file if it already exists")

    return cmd
}

func runInit(ctx context.Context, cfg *InitConfig) error {
    _ = ctx

    out := strings.TrimSpace(cfg.OutputPath)
    if out == "" {
        out = "swagger2mcp.yaml"
    }
    absPath, err := filepath.Abs(out)
    if err != nil {
        return fmt.Errorf("init: resolve output path: %w", err)
    }

    if st, err := os.Stat(absPath); err == nil && !cfg.Force {
        if st.Mode().IsRegular() {
            return newUsageError(fmt.Sprintf("init: %q already exists (use --force to overwrite)", absPath))
        }
    }

    if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
        return newUsageError(fmt.Sprintf("init: cannot create parent directory: %v", err))
    }

    content := strings.TrimSpace(sampleConfigYAML) + "\n"

    // Atomic write via temp + rename
    tmp := absPath + ".tmp"
    if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
        return newUsageError(fmt.Sprintf("init: cannot write temp file: %v\nHint: choose a different --out or check directory permissions.", err))
    }
    if err := os.Rename(tmp, absPath); err != nil {
        _ = os.Remove(tmp)
        return newUsageError(fmt.Sprintf("init: cannot place file at %s: %v", absPath, err))
    }
    fmt.Fprintf(os.Stdout, "Wrote sample config to %s\n", absPath)
    return nil
}

// sampleConfigYAML is a commented example config documenting available options.
const sampleConfigYAML = `# swagger2mcp configuration (YAML)
# All fields are optional. Command-line flags override config values.

# Path or URL to the Swagger/OpenAPI document (http/https or local file).
# input: ./openapi.yaml

# Target language to emit (go|npm). Defaults to go when omitted.
# lang: go

# Output directory. When omitted, derived from toolName or spec title.
# out: ./out

# Only include operations with these tags (comma-separated or list).
# includeTags: [public,read]

# Exclude operations with these tags (comma-separated or list).
# excludeTags: [internal]

# Override tool binary/package name. Sanitized to lowercase/dash.
# toolName: api-docs

# Go: module name (e.g., example.com/mytool). npm: package name.
# packageName: example.com/mytool

# Preview planned outputs without writing files.
# dryRun: false

# Overwrite non-empty output directory.
# force: false

# Enable verbose logging.
# verbose: false
`
