// Package output wraps github.com/basecamp/cli/output with the additions weeks
// needs on top of the shared 37signals envelope.
//
// The shared package owns the contract — {ok, data, summary, breadcrumbs},
// the format modes, and the exit-code mapping. Everything re-exported here is
// that contract verbatim, so a reader can follow one import to the source of
// truth. What weeks adds is below the re-exports: the confirmation gate.
package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/basecamp/cli/output"
)

// Core types, re-exported so command code imports one output package.
type (
	Response       = output.Response
	Breadcrumb     = output.Breadcrumb
	Format         = output.Format
	Options        = output.Options
	Error          = output.Error
	ResponseOption = output.ResponseOption
)

type ErrorResponse struct {
	OK          bool           `json:"ok"`
	Error       string         `json:"error"`
	Code        string         `json:"code"`
	Hint        string         `json:"hint,omitempty"`
	Breadcrumbs []Breadcrumb   `json:"breadcrumbs,omitempty"`
	Meta        map[string]any `json:"meta,omitempty"`
}

type BreadcrumbError struct {
	Err         error
	Breadcrumbs []Breadcrumb
}

func (e *BreadcrumbError) Error() string { return e.Err.Error() }

func (e *BreadcrumbError) Unwrap() error { return e.Err }

func WithErrorNext(err error, crumbs ...Breadcrumb) error {
	return &BreadcrumbError{Err: err, Breadcrumbs: crumbs}
}

// Writer answers a command.
//
// It wraps the toolkit's writer rather than aliasing it because weeks has one
// format the toolkit does not implement: FormatStyled, the human one. The
// toolkit's own styled case falls through to JSON, on the stated understanding
// that each app supplies its rendering. This is that.
//
// Everything else is delegated unchanged, so the envelope an agent reads comes
// from the toolkit in every mode where an agent is reading.
type Writer struct {
	inner  *output.Writer
	opts   Options
	target io.Writer
}

// New creates a writer.
func New(opts Options) *Writer {
	if opts.Writer == nil {
		opts.Writer = os.Stdout
	}
	return &Writer{inner: output.New(opts), opts: opts, target: opts.Writer}
}

// DefaultOptions returns options for standard output.
func DefaultOptions() Options { return output.DefaultOptions() }

// styled reports whether this invocation should be rendered for a person:
// either asked for outright, or auto-detected because a terminal is attached.
func (w *Writer) styled() bool {
	if w.opts.Format == FormatStyled {
		return true
	}
	return w.opts.Format == FormatAuto && isTerminal(w.target)
}

// OK writes a success response.
func (w *Writer) OK(data any, opts ...ResponseOption) error {
	if !w.styled() {
		return w.inner.OK(data, opts...)
	}
	resp := &Response{OK: true, Data: data}
	for _, opt := range opts {
		opt(resp)
	}
	return renderStyled(w.target, resp, NewStyle(w.target))
}

// Err writes a failure.
func (w *Writer) Err(err error, opts ...ErrorResponseOption) error {
	structured := AsError(err)
	resp := &ErrorResponse{OK: false, Error: structured.Message, Code: structured.Code, Hint: structured.Hint}
	var withCrumbs *BreadcrumbError
	if errors.As(err, &withCrumbs) {
		resp.Breadcrumbs = append(resp.Breadcrumbs, withCrumbs.Breadcrumbs...)
	}
	for _, opt := range opts {
		opt(resp)
	}
	if w.opts.Format == FormatMarkdown {
		return renderErrorMarkdown(w.target, resp)
	}
	if !w.styled() {
		enc := json.NewEncoder(w.target)
		enc.SetIndent("", "  ")
		return enc.Encode(resp)
	}
	return renderErrorStyled(w.target, resp, NewStyle(w.target))
}

// ErrorResponseOption modifies an error response.
type ErrorResponseOption func(*ErrorResponse)

func WithErrorBreadcrumbs(b ...Breadcrumb) ErrorResponseOption {
	return func(r *ErrorResponse) { r.Breadcrumbs = append(r.Breadcrumbs, b...) }
}

func renderErrorMarkdown(w io.Writer, resp *ErrorResponse) error {
	if _, err := fmt.Fprintf(w, "**Error:** %s\n\n`%s`\n", resp.Error, resp.Code); err != nil {
		return err
	}
	if resp.Hint != "" {
		if _, err := fmt.Fprintf(w, "\n%s\n", resp.Hint); err != nil {
			return err
		}
	}
	if len(resp.Breadcrumbs) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "\n**Next**"); err != nil {
		return err
	}
	for _, crumb := range resp.Breadcrumbs {
		if _, err := fmt.Fprintf(w, "- `%s` - %s\n", crumb.Cmd, crumb.Description); err != nil {
			return err
		}
	}
	return nil
}

// Format modes.
const (
	FormatAuto     = output.FormatAuto
	FormatJSON     = output.FormatJSON
	FormatMarkdown = output.FormatMarkdown
	FormatStyled   = output.FormatStyled
	FormatQuiet    = output.FormatQuiet
	FormatIDs      = output.FormatIDs
	FormatCount    = output.FormatCount
)

// Shared error codes and their exit codes.
const (
	CodeUsage     = output.CodeUsage
	CodeNotFound  = output.CodeNotFound
	CodeAuth      = output.CodeAuth
	CodeForbidden = output.CodeForbidden
	CodeRateLimit = output.CodeRateLimit
	CodeNetwork   = output.CodeNetwork
	CodeAPI       = output.CodeAPI
	CodeAmbiguous = output.CodeAmbiguous

	ExitOK        = output.ExitOK
	ExitUsage     = output.ExitUsage
	ExitNotFound  = output.ExitNotFound
	ExitAuth      = output.ExitAuth
	ExitForbidden = output.ExitForbidden
	ExitRateLimit = output.ExitRateLimit
	ExitNetwork   = output.ExitNetwork
	ExitAPI       = output.ExitAPI
	ExitAmbiguous = output.ExitAmbiguous
)

// Constructors and response options.
var (
	// NormalizeData turns arbitrary Go values into the JSON-ish shapes the
	// renderers walk, so styled output and JSON output describe the same tree.
	NormalizeData = output.NormalizeData

	WithSummary        = output.WithSummary
	WithNotice         = output.WithNotice
	WithBreadcrumbs    = output.WithBreadcrumbs
	WithoutBreadcrumbs = output.WithoutBreadcrumbs
	WithContext        = output.WithContext
	WithMeta           = output.WithMeta

	AsError     = output.AsError
	ErrUsage    = output.ErrUsage
	ErrNotFound = output.ErrNotFound
	ErrAuth     = output.ErrAuth
	ErrNetwork  = output.ErrNetwork
	ErrAPI      = output.ErrAPI
)

// CodeConfirmationRequired is the weeks-specific error code for a gated
// operation: the command was understood and is legal, but it crosses a gate
// the product also enforces in the UI — a staffing overlap, a negative
// planning context — so it needs an explicit go-ahead.
//
// It is not in the shared toolkit because no other 37signals CLI has a
// domain gate of this shape. The contract, which the agent skill teaches:
// the caller re-runs the identical command with --confirm.
const CodeConfirmationRequired = "confirmation_required"

// ExitConfirmationRequired is the exit code for CodeConfirmationRequired.
//
// It sits at 9, one past the shared toolkit's highest code (ExitAmbiguous = 8),
// so weeks never collides with a code the toolkit may add meaning to. A
// script can branch on it without parsing JSON.
const ExitConfirmationRequired = 9

// ErrConfirmationRequired reports that an operation is gated. Reason states
// what the gate found — it is shown to a human and read by an agent, so it
// names the conflict rather than the rule.
func ErrConfirmationRequired(reason string) *Error {
	return &Error{
		Code:    CodeConfirmationRequired,
		Message: reason,
		Hint:    "Re-run the same command with --confirm to proceed.",
	}
}

// IsConfirmationRequired reports whether err is the confirmation gate.
func IsConfirmationRequired(err error) bool {
	return err != nil && AsError(err).Code == CodeConfirmationRequired
}

// ExitCodeFor maps an error code to its exit code, covering the weeks-specific
// codes as well as the shared ones. Command dispatch uses this rather than the
// toolkit's version, which would fold confirmation_required into ExitAPI.
func ExitCodeFor(code string) int {
	if code == CodeConfirmationRequired {
		return ExitConfirmationRequired
	}
	return output.ExitCodeFor(code)
}

// ExitCode returns the exit code for err, whatever its shape.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	return ExitCodeFor(AsError(err).Code)
}

// SilentExit ends the process with a specific code after the command has
// already written its own answer.
//
// It exists for commands whose failure is data rather than an error: `weeks
// doctor` succeeds at diagnosing an unhealthy installation, so its envelope is
// a success envelope with healthy:false — but the exit code still has to be
// non-zero for the CI step or the agent that reads only the status. Without
// this, such a command would have to print two envelopes and leave the caller
// to work out which one counts.
type SilentExit struct{ Code int }

func (e *SilentExit) Error() string { return fmt.Sprintf("exit %d", e.Code) }
