package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bcprofile "github.com/basecamp/cli/profile"

	"github.com/weeks-app/weeks-cli/internal/appctx"
	"github.com/weeks-app/weeks-cli/internal/output"
)

func TestProfileDefaultReportsConfigReadErrors(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := NewProfileCmd()
	cmd.SetContext(appctx.With(context.Background(), &appctx.App{
		Out:      output.New(output.Options{Format: output.FormatJSON, Writer: &out}),
		Profiles: bcprofile.NewStore(configPath),
	}))
	cmd.SetArgs([]string{"default", "local"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("profile default succeeded with malformed config")
	}
	if !strings.Contains(err.Error(), "could not read profiles") {
		t.Fatalf("error = %q, want config read error", err)
	}
	if strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %q, should not report not_found for malformed config", err)
	}
}
