package athq

import (
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func TestWorkGroupPrefersTheFlagOverTheEnvironment(t *testing.T) {
	t.Setenv(envWorkGroup, "from-env")
	opts.workGroup = "from-flag"
	t.Cleanup(func() { opts.workGroup = "" })

	if got := workGroup(); got != "from-flag" {
		t.Errorf("got = %q, want %q", got, "from-flag")
	}
}

func TestWorkGroupFallsBackToTheEnvironment(t *testing.T) {
	t.Setenv(envWorkGroup, "analytics")
	opts.workGroup = ""

	if got := workGroup(); got != "analytics" {
		t.Errorf("got = %q, want %q", got, "analytics")
	}
}

func TestWorkGroupDefaultsToPrimary(t *testing.T) {
	t.Setenv(envWorkGroup, "")
	opts.workGroup = ""

	if got := workGroup(); got != defaultWorkGroup {
		t.Errorf("got = %q, want %q", got, defaultWorkGroup)
	}
}

func TestCatalogDefaultsToTheGlueCatalog(t *testing.T) {
	t.Setenv(envCatalog, "")
	opts.catalog = ""

	if got := catalog(); got != defaultCatalog {
		t.Errorf("got = %q, want %q", got, defaultCatalog)
	}
}

func TestRequireDatabaseFailsWhenUnset(t *testing.T) {
	t.Setenv(envDatabase, "")
	opts.database = ""

	if _, err := requireDatabase(); err == nil {
		t.Error("got no error without a database, want one")
	}
}

func TestNormalizeFlagNameMapsTheShortAliases(t *testing.T) {
	if got := normalizeFlagName(nil, "wg"); got != pflag.NormalizedName("workgroup") {
		t.Errorf("wg: got = %q, want %q", got, "workgroup")
	}
	if got := normalizeFlagName(nil, "db"); got != pflag.NormalizedName("database") {
		t.Errorf("db: got = %q, want %q", got, "database")
	}
	if got := normalizeFlagName(nil, "region"); got != pflag.NormalizedName("region") {
		t.Errorf("region: got = %q, want it unchanged", got)
	}
}

func TestRootHasARunEForTheDefaultTUI(t *testing.T) {
	if rootCmd.RunE == nil {
		t.Error("expected the root command to run something with no subcommand")
	}
}

func TestRootFileFlagIsLocalWithShorthandF(t *testing.T) {
	f := rootCmd.Flags().Lookup("file")
	if f == nil {
		t.Fatal("expected a --file flag on the root command")
	}
	if f.Shorthand != "f" {
		t.Errorf("got shorthand %q, want %q", f.Shorthand, "f")
	}
	// It must stay local: query defines its own -f/--file, and a persistent
	// flag of the same name on the root would collide with it.
	if rootCmd.PersistentFlags().Lookup("file") != nil {
		t.Error("expected --file to be a local flag, not a persistent one")
	}
}

func TestRootRejectsAPositionalArgument(t *testing.T) {
	rootCmd.SetArgs([]string{"SELECT 1"})
	defer rootCmd.SetArgs(nil)

	err := rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("got = %v, want an \"unknown command\" error", err)
	}
}

func TestShortAliasSetsTheCanonicalFlag(t *testing.T) {
	opts.workGroup = ""
	t.Cleanup(func() { opts.workGroup = "" })

	if err := rootCmd.PersistentFlags().Parse([]string{"--wg", "analytics"}); err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if opts.workGroup != "analytics" {
		t.Errorf("got = %q, want %q", opts.workGroup, "analytics")
	}
}
