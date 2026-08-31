// Package cli assembles the weeks command tree and runs it.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	bcprofile "github.com/basecamp/cli/profile"
	"github.com/spf13/cobra"

	"github.com/weeks-app/weeks-cli/internal/appctx"
	"github.com/weeks-app/weeks-cli/internal/commands"
	"github.com/weeks-app/weeks-cli/internal/config"
	"github.com/weeks-app/weeks-cli/internal/output"
	"github.com/weeks-app/weeks-cli/internal/version"
)

// DefaultClientID is the OAuth client id weeks-cli authenticates as against
// the hosted installation. A self-hosted or development weeks issues its own
// Platform::Application, so WEEKS_CLIENT_ID or the profile overrides it.
//
// It is deliberately empty until the hosted application exists: an empty
// client id fails at login with a message that says what to set, which is
// better than a plausible-looking id that fails inside Doorkeeper.
const DefaultClientID = ""

type rootFlags struct {
	json     bool
	styled   bool
	quiet    bool
	agent    bool
	markdown bool
	idsOnly  bool
	count    bool
	verbose  bool
	confirm  bool
	profile  string
	baseURL  string
}

// Execute builds the command tree, runs it, and exits with the code the
// error's structured code maps to.
func Execute() {
	root, flags := NewRootCmd()

	err := root.ExecuteContext(context.Background())
	if err == nil {
		return
	}

	// A command that already printed its own answer only needs the status.
	var silent *output.SilentExit
	if errors.As(err, &silent) {
		os.Exit(silent.Code)
	}

	// Cobra reports its own usage errors as plain errors. Give them the
	// structured shape every other failure has, so an agent parsing stdout
	// never has to special-case "the CLI did not understand me".
	var structured *output.Error
	if !errors.As(err, &structured) {
		structured = output.ErrUsage(err.Error())
	}

	w := output.New(output.Options{
		Format:  resolveFormat(flags),
		Writer:  os.Stderr,
		Verbose: flags.verbose,
	})
	_ = w.Err(structured)
	os.Exit(output.ExitCodeFor(structured.Code))
}

// NewRootCmd builds the root command and returns it together with the flag
// struct it writes into, which Execute needs to format a failure.
func NewRootCmd() (*cobra.Command, *rootFlags) {
	flags := &rootFlags{}

	root := &cobra.Command{
		Use:   "weeks",
		Short: "Plan and staff work in weeks, from the terminal",
		Long: "weeks is the command-line and agent interface to weeks, the staff-scheduling app.\n\n" +
			"Every command answers with the same JSON envelope — {ok, data, summary, breadcrumbs} —\n" +
			"when its output is not a terminal, so a script or an agent never has to parse prose.\n" +
			"Breadcrumbs name the commands that usually come next.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.Version,

		// Resolving the profile, base URL, and credential store once here is
		// what lets every leaf command be about its own subject.
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			app, err := buildApp(flags)
			if err != nil {
				return err
			}
			cmd.SetContext(appctx.With(cmd.Context(), app))
			return nil
		},
	}

	pf := root.PersistentFlags()
	pf.BoolVar(&flags.json, "json", false, "Emit the JSON envelope even on a terminal")
	pf.BoolVar(&flags.styled, "styled", false, "Render for a person even when output is not a terminal")
	pf.BoolVar(&flags.quiet, "quiet", false, "Emit only the data, without the envelope, for piping")
	pf.BoolVar(&flags.agent, "agent", false, "Answer in the shape an agent reads: JSON, and structured help")
	pf.BoolVar(&flags.markdown, "markdown", false, "Render output as Markdown")
	pf.BoolVar(&flags.idsOnly, "ids-only", false, "Emit one id per line and nothing else")
	pf.BoolVar(&flags.count, "count", false, "Emit the number of results and nothing else")
	pf.BoolVarP(&flags.verbose, "verbose", "v", false, "Include the detail a normal run omits")
	pf.BoolVar(&flags.confirm, "confirm", false, "Proceed past a confirmation_required gate")
	pf.StringVar(&flags.profile, "profile", "", "Named profile to act as (default: the configured one)")
	pf.StringVar(&flags.baseURL, "base-url", "", "weeks installation to talk to")

	// --help --agent has to be answered before Cobra prints its own help,
	// which is why help is a function on the command rather than a flag we
	// inspect inside a RunE.
	installAgentHelp(root, flags)

	root.AddCommand(
		commands.NewAuthCmd(),
		commands.NewProfileCmd(),
		commands.NewDoctorCmd(),
		commands.NewCommandsCmd(),
		commands.NewSkillCmd(),
		commands.NewSetupCmd(),
		commands.NewVersionCmd(),
	)

	return root, flags
}

// buildApp resolves everything a command needs from flags, environment, and
// stored config.
func buildApp(flags *rootFlags) (*appctx.App, error) {
	profiles := bcprofile.NewStore(config.ProfilesPath())

	known, configuredDefault, err := profiles.List()
	if err != nil {
		return nil, output.ErrUsage(fmt.Sprintf("could not read profiles: %v", err))
	}

	name, err := bcprofile.Resolve(bcprofile.ResolveOptions{
		FlagValue:      flags.profile,
		EnvVar:         os.Getenv(config.EnvProfile),
		DefaultProfile: configuredDefault,
		Profiles:       known,
		// Never prompt. A CLI whose contract is "an agent can drive this"
		// cannot block on a picker, and a human who has several profiles and
		// named none gets a message telling them to pick.
		Interactive: false,
	})
	if err != nil {
		return nil, output.ErrUsage(err.Error())
	}

	var prof *bcprofile.Profile
	if name != "" {
		prof = known[name]
	}

	clientID := os.Getenv(config.EnvClientID)
	if clientID == "" && prof != nil {
		clientID = profileClientID(prof)
	}
	if clientID == "" {
		clientID = DefaultClientID
	}

	return &appctx.App{
		Out: output.New(output.Options{
			Format:  resolveFormat(flags),
			Writer:  os.Stdout,
			Verbose: flags.verbose,
		}),
		Profile:     name,
		BaseURL:     resolveBaseURL(flags.baseURL, prof),
		ClientID:    clientID,
		Agent:       flags.agent || flags.json,
		Interactive: interactive(flags),
		Confirm:     flags.confirm,
		Verbose:     flags.verbose,
		Profiles:    profiles,
	}, nil
}

func profileClientID(prof *bcprofile.Profile) string {
	if prof.Extra == nil {
		return ""
	}

	raw, ok := prof.Extra["client_id"]
	if !ok {
		return ""
	}

	var clientID string
	if err := json.Unmarshal(raw, &clientID); err != nil {
		return ""
	}
	return clientID
}
