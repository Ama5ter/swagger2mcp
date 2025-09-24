package cli

import (
    "fmt"

    "github.com/spf13/cobra"
)

// Execute runs the swagger2mcp CLI.
func Execute() error {
	return NewRootCmd().Execute()
}

// NewRootCmd constructs the root command so tests can exercise the CLI easily.
func NewRootCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:           "swagger2mcp",
        Short:         "Generate MCP tool projects from Swagger/OpenAPI specs",
        Long:          "swagger2mcp scaffolds Go or Node MCP tools from Swagger/OpenAPI documents with helpful defaults and validation.",
        SilenceErrors: true,
        SilenceUsage:  true,
        RunE: func(cmd *cobra.Command, args []string) error {
            return cmd.Help()
        },
    }

    // Convert Cobra flag errors (like unknown flags) into friendly usage errors
    // that also show the command's help text.
    cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
        return newUsageError(fmt.Sprintf("%v\n\n%s", err, c.UsageString()))
    })

    cmd.PersistentFlags().StringP("config", "c", "", "Config file path (YAML or JSON)")
    cmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose logging output")

    g := newGenerateCmd()
    g.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
        return newUsageError(fmt.Sprintf("%v\n\n%s", err, c.UsageString()))
    })
    cmd.AddCommand(g)

    i := newInitCmd()
    i.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
        return newUsageError(fmt.Sprintf("%v\n\n%s", err, c.UsageString()))
    })
    cmd.AddCommand(i)

    return cmd
}
