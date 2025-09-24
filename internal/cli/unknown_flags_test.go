package cli

import (
    "io"
    "strings"
    "testing"
)

func TestUnknownFlag_ShowsHelpAndUsageError(t *testing.T) {
    t.Parallel()
    root := NewRootCmd()
    root.SetOut(io.Discard)
    root.SetErr(io.Discard)
    root.SetArgs([]string{"generate", "--unknown-flag"})

    err := root.Execute()
    if err == nil {
        t.Fatalf("expected error for unknown flag")
    }
    if _, ok := err.(usageError); !ok {
        t.Fatalf("expected usage error, got %T: %v", err, err)
    }
    if !strings.Contains(err.Error(), "unknown flag") || !strings.Contains(err.Error(), "Usage:") {
        t.Fatalf("unexpected error text: %v", err)
    }
}

