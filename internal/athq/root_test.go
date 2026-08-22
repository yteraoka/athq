package athq

import (
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
