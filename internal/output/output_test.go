package output_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/weeks-app/weeks-cli/internal/output"
)

// The envelope is the contract the agent skill teaches. These tests exist so
// that a change to it has to be deliberate.

func TestOKEnvelopeShape(t *testing.T) {
	var buf bytes.Buffer
	w := output.New(output.Options{Format: output.FormatJSON, Writer: &buf})

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
