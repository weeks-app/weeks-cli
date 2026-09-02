package output

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// Styled rendering: what a person at a terminal sees.
//
// The shared toolkit deliberately has no renderer — its FormatStyled case
// falls through to JSON with a note that apps provide their own. Until this
// existed, weeks took it up on that and printed a JSON envelope at anyone who
// ran a command in their own shell, which is a fine contract for an agent and
// a poor one for a human.
//
// The envelope is still the source of truth. Everything here is a projection
// of the same Response an agent receives, so the two can never disagree about
// what happened — only about how much punctuation it takes to say it.

// StyledRenderer lets a command render its own payload when the generic
// key-and-value treatment would bury the point. `doctor` is the case that
// motivated it: a list of checks wants marks down the left margin, not an
// object dump.
type StyledRenderer interface {
	RenderStyled(w io.Writer, style *Style) error
}

// Style carries the escape codes, or empty strings when color is off.
type Style struct {
	Bold, Dim, Green, Red, Yellow, Blue, Reset string
}

// NewStyle returns a palette for w. Color is dropped when the destination is
// not a terminal, when NO_COLOR is set (no-color.org), or when TERM says the
// terminal cannot do it — three different ways of being asked not to.
func NewStyle(w io.Writer) *Style {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" || !isTerminal(w) {
		return &Style{}
	}
	return &Style{
		Bold:   "\x1b[1m",
		Dim:    "\x1b[2m",
		Green:  "\x1b[32m",
		Red:    "\x1b[31m",
		Yellow: "\x1b[33m",
		Blue:   "\x1b[34m",
		Reset:  "\x1b[0m",
	}
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// renderStyled writes a success response for a human.
//
// Order is deliberate and matches how the answer is read: what happened, then
// the detail, then what to do next.
func renderStyled(w io.Writer, resp *Response, style *Style) error {
	// A failure prints a red ✗ and its code; a success printed a bold line and
	// nothing else, so the two outcomes of the same command did not look like
	// outcomes of the same command. The tick is the envelope's `ok` made
	// visible, which is the one thing a person scanning the output is after.
	if resp.Summary != "" {
		if _, err := fmt.Fprintf(w, "%s✓%s %s%s%s\n",
			style.Green, style.Reset, style.Bold, resp.Summary, style.Reset); err != nil {
			return err
		}
	}

	if resp.Notice != "" {
		if _, err := fmt.Fprintf(w, "%s! %s%s\n", style.Yellow, resp.Notice, style.Reset); err != nil {
			return err
		}
	}

	if resp.Data != nil {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if custom, ok := resp.Data.(StyledRenderer); ok {
			if err := custom.RenderStyled(w, style); err != nil {
				return err
			}
		} else if err := renderValue(w, style, NormalizeData(resp.Data), 0); err != nil {
			return err
		}
	}

	if len(resp.Breadcrumbs) > 0 {
		return renderBreadcrumbs(w, resp.Breadcrumbs, style)
	}

	return nil
}

func renderBreadcrumbs(w io.Writer, crumbs []Breadcrumb, style *Style) error {
	if len(crumbs) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "\n%sNext%s\n", style.Dim, style.Reset); err != nil {
		return err
	}
	// Commands differ in length, so the descriptions are padded into a
	// column. Ragged right of a varying-width command is hard to scan,
	// which defeats the point of listing them.
	width := 0
	for _, crumb := range crumbs {
		if len(crumb.Cmd) > width {
			width = len(crumb.Cmd)
		}
	}
	for _, crumb := range crumbs {
		padding := strings.Repeat(" ", width-len(crumb.Cmd))
		if _, err := fmt.Fprintf(w, "  %s%s%s%s   %s%s%s\n",
			style.Blue, crumb.Cmd, style.Reset, padding,
			style.Dim, crumb.Description, style.Reset); err != nil {
			return err
		}
	}
	return nil
}

// renderErrorStyled writes a failure for a human. The code is shown because it
// is the thing worth quoting when asking someone else about it.
func renderErrorStyled(w io.Writer, resp *ErrorResponse, style *Style) error {
	if _, err := fmt.Fprintf(w, "%s✗ %s%s %s(%s)%s\n",
		style.Red, resp.Error, style.Reset,
		style.Dim, resp.Code, style.Reset); err != nil {
		return err
	}
	if resp.Hint != "" {
		if _, err := fmt.Fprintf(w, "  %s\n", resp.Hint); err != nil {
			return err
		}
	}
	return renderBreadcrumbs(w, resp.Breadcrumbs, style)
}

// renderValue prints normalized JSON-ish data as aligned lines.
//
// It is deliberately shallow: one level of keys, lists of objects as indented
// blocks. Anything deeper than a CLI answer should be is a sign the command
// should be returning less, not that this should grow a tree view.
func renderValue(w io.Writer, style *Style, value any, indent int) error {
	pad := strings.Repeat("  ", indent)

	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		width := 0
		for key := range typed {
			keys = append(keys, key)
			if len(key) > width {
				width = len(key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			label := strings.ReplaceAll(key, "_", " ")
			switch nested := typed[key].(type) {
			case map[string]any, []map[string]any, []any:
				if _, err := fmt.Fprintf(w, "%s%s%s%s\n", pad, style.Dim, label, style.Reset); err != nil {
					return err
				}
				if err := renderValue(w, style, nested, indent+1); err != nil {
					return err
				}
			default:
				if _, err := fmt.Fprintf(w, "%s%s%-*s%s  %s\n",
					pad, style.Dim, width, label, style.Reset, scalar(typed[key])); err != nil {
					return err
				}
			}
		}
	case []map[string]any:
		for _, item := range typed {
			if err := renderValue(w, style, item, indent); err != nil {
				return err
			}
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range typed {
			if err := renderValue(w, style, item, indent); err != nil {
				return err
			}
		}
	default:
		if _, err := fmt.Fprintf(w, "%s%s\n", pad, scalar(value)); err != nil {
			return err
		}
	}
	return nil
}

// scalar renders a leaf. A null becomes an em dash rather than "<nil>": the
// CLI uses null to mean "this does not apply", such as a token that never
// expires, and printing Go's zero value at someone would be noise.
func scalar(value any) string {
	switch typed := value.(type) {
	case nil:
		return "—"
	case string:
		if typed == "" {
			return "—"
		}
		return typed
	case bool:
		if typed {
			return "yes"
		}
		return "no"
	case float64:
		if typed == float64(int64(typed)) {
			return fmt.Sprintf("%d", int64(typed))
		}
		return fmt.Sprintf("%g", typed)
	default:
		return fmt.Sprintf("%v", typed)
	}
}
