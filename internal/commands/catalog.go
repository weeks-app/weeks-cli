package commands

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Info describes one command completely enough for an agent to call it.
//
// The same type serves both discovery surfaces — one entry per command in
// `weeks commands --json`, and a single entry from `weeks <cmd> --help --agent`
// — so the catalog and the help can never disagree about what a command takes.
//
// The field names and JSON shape follow the 37signals toolkit, because the
// toolkit's surface snapshot and compatibility scripts read them. Diverging
// would leave weeks-cli nominally "on the toolkit" while none of its tooling
// worked on us.
type Info struct {
	// ID equals Path. It exists so that a catalog listing behaves like any
	// other collection under --ids-only.
	ID string `json:"id"`

	Command string `json:"command"`
	Path    string `json:"path"`
	Short   string `json:"short"`
	Long    string `json:"long,omitempty"`
	Usage   string `json:"usage"`

	Args           string       `json:"args,omitempty"`
	Aliases        []string     `json:"aliases,omitempty"`
	Subcommands    []Subcommand `json:"subcommands,omitempty"`
	Flags          []Flag       `json:"flags,omitempty"`
	InheritedFlags []Flag       `json:"inherited_flags,omitempty"`
}

// Subcommand is a child command, named so it can be invoked directly.
type Subcommand struct {
	Name  string `json:"name"`
	Short string `json:"short"`
	Path  string `json:"path"`
}

// Flag is one flag as an agent needs to see it: what to pass and what it means.
type Flag struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand,omitempty"`
	Type      string `json:"type"`
	Default   string `json:"default"`
	Usage     string `json:"usage"`
}

// Describe renders one command as an Info.
func Describe(cmd *cobra.Command) Info {
	// Cobra adds --help lazily, to the command it is about to run or help.
	// Without this, a command's entry in the catalog would be missing a flag
	// that its own --help --agent reports — so the two discovery surfaces
	// would disagree, and the surface snapshot would flap depending on which
	// command was invoked.
	cmd.InitDefaultHelpFlag()

	info := Info{
		ID:      cmd.CommandPath(),
		Command: cmd.Name(),
		Path:    cmd.CommandPath(),
		Short:   cmd.Short,
		Long:    cmd.Long,
		Usage:   cmd.UseLine(),
		Args:    argsSpec(cmd),
		Aliases: cmd.Aliases,
	}

	for _, sub := range cmd.Commands() {
		if !included(sub) {
			continue
		}
		info.Subcommands = append(info.Subcommands, Subcommand{
			Name:  sub.Name(),
			Short: sub.Short,
			Path:  sub.CommandPath(),
		})
		// Aliases are part of the surface: a script that calls `profile ls`
		// breaks just as hard when the alias goes away as when the command does.
		for _, alias := range sub.Aliases {
			info.Subcommands = append(info.Subcommands, Subcommand{
				Name:  alias,
				Short: sub.Short,
				Path:  strings.TrimSuffix(sub.CommandPath(), sub.Name()) + alias,
			})
		}
	}

	// Hidden flags are skipped to match what text help shows; pflag's VisitAll
	// visits them, so the filter has to be explicit.
	cmd.NonInheritedFlags().VisitAll(func(f *pflag.Flag) {
		if !f.Hidden {
			info.Flags = append(info.Flags, describeFlag(f))
		}
	})
	cmd.InheritedFlags().VisitAll(func(f *pflag.Flag) {
		if !f.Hidden {
			info.InheritedFlags = append(info.InheritedFlags, describeFlag(f))
		}
	})

	sort.Slice(info.Flags, func(i, j int) bool { return info.Flags[i].Name < info.Flags[j].Name })
	sort.Slice(info.InheritedFlags, func(i, j int) bool { return info.InheritedFlags[i].Name < info.InheritedFlags[j].Name })

	return info
}

// Catalog walks a command tree and describes every command in it, sorted by
// path. Cobra's own help and completion scaffolding is left out: it is not
// part of what weeks offers.
func Catalog(root *cobra.Command) []Info {
	var out []Info
	walk(root, &out)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func walk(cmd *cobra.Command, out *[]Info) {
	for _, sub := range cmd.Commands() {
		if !included(sub) {
			continue
		}
		*out = append(*out, Describe(sub))
		walk(sub, out)
	}
}

func included(cmd *cobra.Command) bool {
	return !cmd.Hidden && cmd.Name() != "help" && cmd.Name() != "completion"
}

func describeFlag(f *pflag.Flag) Flag {
	return Flag{
		Name:      f.Name,
		Shorthand: f.Shorthand,
		Type:      f.Value.Type(),
		Default:   f.DefValue,
		Usage:     f.Usage,
	}
}

// argsSpec reports the positional arguments from the Use line, the only place
// Cobra records them in a form worth publishing.
func argsSpec(cmd *cobra.Command) string {
	fields := strings.Fields(cmd.Use)
	if len(fields) <= 1 {
		return ""
	}
	return strings.Join(fields[1:], " ")
}
