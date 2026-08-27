package commands_test

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/weeks-app/weeks-cli/internal/commands"
)

func tree() *cobra.Command {
	root := &cobra.Command{Use: "weeks"}
	root.PersistentFlags().Bool("json", false, "Emit the JSON envelope")

	auth := &cobra.Command{Use: "auth", Short: "Sign in"}
	login := &cobra.Command{Use: "login", Short: "Sign in and store the credential"}
	login.Flags().Bool("browser", false, "Use the browser flow")
	auth.AddCommand(login)

	profile := &cobra.Command{Use: "profile", Short: "Manage profiles"}
	list := &cobra.Command{Use: "list", Short: "List profiles", Aliases: []string{"ls"}}
	profile.AddCommand(list)

	hidden := &cobra.Command{Use: "internal-only", Hidden: true}

	root.AddCommand(auth, profile, hidden)
	return root
}

func TestCatalogCoversTheWholeTree(t *testing.T) {
	paths := map[string]bool{}
	for _, info := range commands.Catalog(tree()) {
		paths[info.Path] = true
	}

	for _, want := range []string{"weeks auth", "weeks auth login", "weeks profile", "weeks profile list"} {
		if !paths[want] {
			t.Errorf("catalog is missing %q", want)
		}
	}
}

func TestCatalogOmitsWhatIsNotPartOfTheContract(t *testing.T) {
	for _, info := range commands.Catalog(tree()) {
		switch info.Command {
		case "internal-only":
			t.Error("a hidden command reached the catalog")
		case "help", "completion":
			t.Errorf("Cobra scaffolding %q reached the catalog", info.Command)
		}
	}
}

func TestCatalogIsSorted(t *testing.T) {
	catalog := commands.Catalog(tree())
	for i := 1; i < len(catalog); i++ {
		if catalog[i-1].Path > catalog[i].Path {
			t.Fatalf("catalog is unsorted at %d: %q before %q", i, catalog[i-1].Path, catalog[i].Path)
		}
	}
}

func TestEveryEntryCarriesAnID(t *testing.T) {
	// --ids-only reads item["id"]; without it the flag silently prints nothing.
	for _, info := range commands.Catalog(tree()) {
		if info.ID == "" || info.ID != info.Path {
			t.Errorf("%q: id = %q, want the path", info.Path, info.ID)
		}
	}
}

func TestDescribeSeparatesLocalAndInheritedFlags(t *testing.T) {
	root := tree()
	login, _, err := root.Find([]string{"auth", "login"})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}

	info := commands.Describe(login)

	if !hasFlag(info.Flags, "browser") {
		t.Error("the command's own --browser is missing from flags")
	}
	if hasFlag(info.Flags, "json") {
		t.Error("an inherited flag was reported as local")
	}
	if !hasFlag(info.InheritedFlags, "json") {
		t.Error("the global --json is missing from inherited_flags")
	}
}

func TestDescribeListsAliasesAsSubcommands(t *testing.T) {
	root := tree()
	profile, _, err := root.Find([]string{"profile"})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}

	info := commands.Describe(profile)

	var names []string
	for _, sub := range info.Subcommands {
		names = append(names, sub.Name)
	}

	// A script that calls `profile ls` breaks as hard when the alias goes away
	// as when the command does, so the surface snapshot has to see it.
	if !contains(names, "list") || !contains(names, "ls") {
		t.Errorf("subcommands = %v, want both list and its alias ls", names)
	}
}

func hasFlag(flags []commands.Flag, name string) bool {
	for _, f := range flags {
		if f.Name == name {
			return true
		}
	}
	return false
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
