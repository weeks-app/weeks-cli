// Package output wraps github.com/basecamp/cli/output with the additions weeks
// needs on top of the shared 37signals envelope.
//
// The shared package owns the contract — {ok, data, summary, breadcrumbs},
// the format modes, and the exit-code mapping. Everything re-exported here is
// that contract verbatim, so a reader can follow one import to the source of
// truth. What weeks adds is below the re-exports: the confirmation gate.
package output

import (
	"fmt"

	"github.com/basecamp/cli/output"
)

// Core types, re-exported so command code imports one output package.
type (
	Response       = output.Response
	ErrorResponse  = output.ErrorResponse
	Breadcrumb     = output.Breadcrumb
	Format         = output.Format
	Options        = output.Options
	Writer         = output.Writer
	Error          = output.Error
	ResponseOption = output.ResponseOption
)

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
	New                = output.New
	DefaultOptions     = output.DefaultOptions
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
