package output_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/weeks-app/weeks-cli/internal/output"
)

// The envelope is the contract the agent skill teaches. These tests exist so
// that a change to it has to be deliberate.

func TestOKEnvelopeShape(t *testing.T) {
	var buf bytes.Buffer
	w := output.New(output.Options{Format: output.FormatJSON, Writer: &buf})
	w.SetDefaultContext(map[string]any{
		"profile":      "default",
		"config_scope": "local",
		"config_dir":   "/work/.weeks",
		"base_url":     "https://weeks.app",
	})

	err := w.OK(map[string]any{"id": 1},
		output.WithSummary("One thing."),
		output.WithBreadcrumbs(output.Breadcrumb{Action: "next", Cmd: "weeks doctor", Description: "Check health"}),
	)
	if err != nil {
		t.Fatalf("OK: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output was not JSON: %v", err)
	}

	if got["ok"] != true {
		t.Errorf("ok = %v, want true", got["ok"])
	}
	if got["summary"] != "One thing." {
		t.Errorf("summary = %v", got["summary"])
	}
	context, ok := got["context"].(map[string]any)
	if !ok {
		t.Fatalf("context = %v, want object", got["context"])
	}
	if context["profile"] != "default" || context["config_scope"] != "local" {
		t.Errorf("context = %v", context)
	}
	crumbs, ok := got["breadcrumbs"].([]any)
	if !ok || len(crumbs) != 1 {
		t.Fatalf("breadcrumbs = %v, want one entry", got["breadcrumbs"])
	}
	crumb := crumbs[0].(map[string]any)
	for _, field := range []string{"action", "cmd", "description"} {
		if crumb[field] == nil {
			t.Errorf("breadcrumb is missing %q", field)
		}
	}
}

func TestQuietStripsTheEnvelope(t *testing.T) {
	var buf bytes.Buffer
	w := output.New(output.Options{Format: output.FormatQuiet, Writer: &buf})

	if err := w.OK(map[string]any{"id": 7}, output.WithSummary("ignored")); err != nil {
		t.Fatalf("OK: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output was not JSON: %v", err)
	}
	if _, present := got["ok"]; present {
		t.Errorf("--quiet emitted the envelope: %s", buf.String())
	}
	if got["id"] != float64(7) {
		t.Errorf("id = %v, want the data itself", got["id"])
	}
}

func TestErrorEnvelopeNeverClaimsSuccess(t *testing.T) {
	var buf bytes.Buffer
	w := output.New(output.Options{Format: output.FormatJSON, Writer: &buf})

	if err := w.Err(output.ErrConfirmationRequired("Dana already works that slot")); err != nil {
		t.Fatalf("Err: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output was not JSON: %v", err)
	}
	if got["ok"] != false {
		t.Errorf("ok = %v, want false", got["ok"])
	}
	if got["code"] != output.CodeConfirmationRequired {
		t.Errorf("code = %v, want %q", got["code"], output.CodeConfirmationRequired)
	}
	if got["hint"] == nil {
		t.Error("a gate with no hint leaves the caller nowhere to go")
	}
}

func TestErrorEnvelopeCanCarryBreadcrumbs(t *testing.T) {
	var buf bytes.Buffer
	w := output.New(output.Options{Format: output.FormatJSON, Writer: &buf})
	w.SetDefaultContext(map[string]any{"profile": "acme", "config_scope": "global"})

	err := output.WithErrorNext(output.ErrUsage("--space is required"), output.Breadcrumb{
		Action: "defaults", Cmd: "weeks defaults set", Description: "Choose defaults",
	})
	if writeErr := w.Err(err); writeErr != nil {
		t.Fatalf("Err: %v", writeErr)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output was not JSON: %v", err)
	}
	crumbs, ok := got["breadcrumbs"].([]any)
	if !ok || len(crumbs) != 1 {
		t.Fatalf("breadcrumbs = %v, want one entry", got["breadcrumbs"])
	}
	context, ok := got["context"].(map[string]any)
	if !ok || context["profile"] != "acme" || context["config_scope"] != "global" {
		t.Fatalf("context = %v", got["context"])
	}
}

func TestMarkdownErrorIsNotJSON(t *testing.T) {
	var buf bytes.Buffer
	w := output.New(output.Options{Format: output.FormatMarkdown, Writer: &buf})

	err := output.WithErrorNext(output.ErrUsage("--space is required"), output.Breadcrumb{
		Action: "spaces", Cmd: "weeks spaces list", Description: "List spaces",
	})
	if writeErr := w.Err(err); writeErr != nil {
		t.Fatalf("Err: %v", writeErr)
	}

	got := buf.String()
	if strings.Contains(got, `"ok"`) {
		t.Errorf("markdown error leaked JSON:\n%s", got)
	}
	for _, want := range []string{"**Error:** --space is required", "`usage`", "**Next**", "`weeks spaces list`"} {
		if !strings.Contains(got, want) {
			t.Errorf("markdown error is missing %q:\n%s", want, got)
		}
	}
}

func TestConfirmationRequiredHasItsOwnExitCode(t *testing.T) {
	err := output.ErrConfirmationRequired("gated")

	if got := output.ExitCode(err); got != output.ExitConfirmationRequired {
		t.Errorf("exit code = %d, want %d", got, output.ExitConfirmationRequired)
	}
	// The whole point of a separate code is that a caller can distinguish a
	// gate from a server error without parsing the body.
	if output.ExitConfirmationRequired == output.ExitAPI {
		t.Error("the gate collides with api_error, so no caller can tell them apart")
	}
	if !output.IsConfirmationRequired(err) {
		t.Error("IsConfirmationRequired did not recognize its own error")
	}
	if output.IsConfirmationRequired(output.ErrAuth("nope")) {
		t.Error("IsConfirmationRequired matched an unrelated error")
	}
}

func TestSharedExitCodesAreUnchanged(t *testing.T) {
	// These are the numbers the skill documents. Changing one silently would
	// break every script that branches on them.
	cases := map[string]int{
		output.CodeUsage:                1,
		output.CodeNotFound:             2,
		output.CodeAuth:                 3,
		output.CodeForbidden:            4,
		output.CodeRateLimit:            5,
		output.CodeNetwork:              6,
		output.CodeAPI:                  7,
		output.CodeAmbiguous:            8,
		output.CodeConfirmationRequired: 9,
	}
	for code, want := range cases {
		if got := output.ExitCodeFor(code); got != want {
			t.Errorf("ExitCodeFor(%q) = %d, want %d", code, got, want)
		}
	}
}

func TestExitCodeOfNilIsSuccess(t *testing.T) {
	if got := output.ExitCode(nil); got != output.ExitOK {
		t.Errorf("ExitCode(nil) = %d, want %d", got, output.ExitOK)
	}
}

// Styled output is what a person sees. It is a projection of the same envelope
// an agent receives, so these tests are as much about the two not diverging as
// about how it looks.

func TestStyledOutputIsNotJSON(t *testing.T) {
	var buf bytes.Buffer
	w := output.New(output.Options{Format: output.FormatStyled, Writer: &buf})
	w.SetDefaultContext(map[string]any{
		"profile":      "default",
		"config_scope": "local",
		"config_dir":   "/work/.weeks",
		"base_url":     "https://weeks.app",
	})

	if err := w.OK(map[string]any{"grant": "device_code"},
		output.WithSummary("Signed in to https://weeks.app."),
		output.WithBreadcrumbs(output.Breadcrumb{
			Action: "verify", Cmd: "weeks auth status", Description: "Confirm the credential",
		}),
	); err != nil {
		t.Fatalf("OK: %v", err)
	}

	got := buf.String()
	if strings.Contains(got, `"ok"`) {
		t.Errorf("styled output leaked the envelope:\n%s", got)
	}
	for _, want := range []string{"Signed in to https://weeks.app.", "Using", "profile", "default", "config store", "local (/work/.weeks)", "grant", "device_code", "weeks auth status"} {
		if !strings.Contains(got, want) {
			t.Errorf("styled output is missing %q:\n%s", want, got)
		}
	}
}

func TestSuccessAndFailureBothCarryAMarker(t *testing.T) {
	// The two outcomes of one command have to look like outcomes of one
	// command. A bold line against a red ✗ did not.
	var ok, bad bytes.Buffer
	if err := output.New(output.Options{Format: output.FormatStyled, Writer: &ok}).
		OK(nil, output.WithSummary("Signed in.")); err != nil {
		t.Fatalf("OK: %v", err)
	}
	if err := output.New(output.Options{Format: output.FormatStyled, Writer: &bad}).
		Err(output.ErrAuth("not signed in")); err != nil {
		t.Fatalf("Err: %v", err)
	}

	if !strings.Contains(ok.String(), "✓") {
		t.Errorf("a success carries no marker:\n%s", ok.String())
	}
	if !strings.Contains(bad.String(), "✗") {
		t.Errorf("a failure carries no marker:\n%s", bad.String())
	}
}

func TestStyledErrorShowsCodeAndHint(t *testing.T) {
	var buf bytes.Buffer
	w := output.New(output.Options{Format: output.FormatStyled, Writer: &buf})

	if err := w.Err(output.ErrConfirmationRequired("Dana already works that slot")); err != nil {
		t.Fatalf("Err: %v", err)
	}

	got := buf.String()
	// The code is what someone quotes when they ask about it, and the hint is
	// what they do about it; neither may be dropped just because it is pretty.
	for _, want := range []string{"Dana already works that slot", output.CodeConfirmationRequired, "--confirm"} {
		if !strings.Contains(got, want) {
			t.Errorf("styled error is missing %q:\n%s", want, got)
		}
	}
}

func TestStyledErrorShowsBreadcrumbs(t *testing.T) {
	var buf bytes.Buffer
	w := output.New(output.Options{Format: output.FormatStyled, Writer: &buf})

	err := output.WithErrorNext(output.ErrUsage("--team is required"), output.Breadcrumb{
		Action: "defaults", Cmd: "weeks defaults set", Description: "Choose defaults",
	})
	if writeErr := w.Err(err); writeErr != nil {
		t.Fatalf("Err: %v", writeErr)
	}

	got := buf.String()
	for _, want := range []string{"Next", "weeks defaults set", "Choose defaults"} {
		if !strings.Contains(got, want) {
			t.Errorf("styled error is missing %q:\n%s", want, got)
		}
	}
}

func TestStyledOutputHasNoEscapesWhenNotATerminal(t *testing.T) {
	// A buffer is not a terminal, so a redirected styled run must stay plain:
	// escape codes in a file someone is reading later are noise.
	var buf bytes.Buffer
	w := output.New(output.Options{Format: output.FormatStyled, Writer: &buf})

	if err := w.OK(map[string]any{"id": 1}, output.WithSummary("One thing.")); err != nil {
		t.Fatalf("OK: %v", err)
	}
	if strings.Contains(buf.String(), "\x1b[") {
		t.Errorf("styled output emitted ANSI escapes to a non-terminal:\n%q", buf.String())
	}
}

func TestJSONIsUnchangedByStyledSupport(t *testing.T) {
	// The whole point of rendering styled output as a projection is that the
	// agent-facing envelope is untouched.
	var buf bytes.Buffer
	w := output.New(output.Options{Format: output.FormatJSON, Writer: &buf})

	if err := w.OK(map[string]any{"id": 1}, output.WithSummary("One thing.")); err != nil {
		t.Fatalf("OK: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("JSON mode stopped emitting JSON: %v", err)
	}
	if got["ok"] != true || got["summary"] != "One thing." {
		t.Errorf("envelope changed shape: %v", got)
	}
}
