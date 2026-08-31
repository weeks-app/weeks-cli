package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weeks-app/weeks-cli/internal/appctx"
	"github.com/weeks-app/weeks-cli/internal/config"
)

func TestCheckCredentialsFailsOnUnreadableStoredCredentials(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(config.EnvConfigDir, dir)
	t.Setenv(config.EnvNoKeyring, "1")

	body := `{"http://localhost:3000":"not a credential object"}`
	if err := os.WriteFile(filepath.Join(dir, "credentials.json"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}

	check, summary := checkCredentials(&appctx.App{BaseURL: "http://localhost:3000"})
	if summary != nil {
		t.Fatalf("summary = %#v, want nil", summary)
	}
	if check.Status != StatusFail {
		t.Fatalf("status = %q, want %q", check.Status, StatusFail)
	}
	if !strings.Contains(check.Message, "stored credentials are unreadable") {
		t.Fatalf("message = %q, want unreadable credential detail", check.Message)
	}
}
