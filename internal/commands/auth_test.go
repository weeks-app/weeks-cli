package commands

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/weeks-app/weeks-cli/internal/appctx"
	"github.com/weeks-app/weeks-cli/internal/config"
	"github.com/weeks-app/weeks-cli/internal/output"
)

func TestHostedDefaultClientRejectsBrowserLogin(t *testing.T) {
	var out bytes.Buffer
	app := &appctx.App{
		Out:      output.New(output.Options{Format: output.FormatJSON, Writer: &out}),
		BaseURL:  config.DefaultBaseURL,
		ClientID: config.DefaultHostedClientID,
	}

	cmd := NewAuthCmd()
	cmd.SetContext(appctx.With(context.Background(), app))
	cmd.SetArgs([]string{"login", "--browser"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("browser login succeeded for the hosted default client")
	}

	var structured *output.Error
	if !errors.As(err, &structured) {
		t.Fatalf("error = %T %[1]v, want structured output error", err)
	}
	if structured.Code != output.CodeUsage {
		t.Fatalf("code = %q, want %q", structured.Code, output.CodeUsage)
	}
	if !strings.Contains(structured.Message, "device login only") {
		t.Fatalf("message = %q, want device-only explanation", structured.Message)
	}
}
