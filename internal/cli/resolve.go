package cli

import (
	"os"

	bcprofile "github.com/basecamp/cli/profile"

	"github.com/weeks-app/weeks-cli/internal/config"
	"github.com/weeks-app/weeks-cli/internal/output"
)

// resolveFormat maps the output flags to a format. The flags are ordered by
// how specific the answer they ask for is: a caller who wants only the ids
// means it more than one who wants JSON, and both mean it more than the
// default, which decides by whether anyone is watching.
func resolveFormat(flags *rootFlags) output.Format {
	switch {
	case flags.count:
		return output.FormatCount
	case flags.idsOnly:
		return output.FormatIDs
	case flags.quiet:
		return output.FormatQuiet
	case flags.markdown:
		return output.FormatMarkdown
	case flags.json || flags.agent:
		return output.FormatJSON
	case flags.styled:
		// Below --json on purpose: asking for both is a contradiction, and the
		// machine-readable answer is the safer one to honor.
		return output.FormatStyled
	default:
		return output.FormatAuto
	}
}

// resolveBaseURL picks the installation to talk to: --base-url, then
// WEEKS_BASE_URL, then the profile's own base URL, then the hosted default.
//
// The flag beats the environment so one command can be pointed elsewhere
// inside an already-configured shell, and the environment beats the profile so
// a worktree collection's .env.collection can retarget every profile at the
// Rails server on its own port without editing anyone's stored config.
func resolveBaseURL(flagValue string, prof *bcprofile.Profile) string {
	if flagValue != "" {
		return flagValue
	}
	if env := config.BaseURLFromEnv(); env != "" {
		return env
	}
	if prof != nil && prof.BaseURL != "" {
		return prof.BaseURL
	}
	return config.DefaultBaseURL
}

// interactive reports whether a human is watching this invocation: stdout is a
// terminal and no machine format was asked for.
func interactive(flags *rootFlags) bool {
	if flags.json || flags.agent || flags.quiet || flags.idsOnly || flags.count || flags.markdown {
		return false
	}
	// --styled says a person is reading, even where TTY detection cannot tell.
	if flags.styled {
		return true
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
