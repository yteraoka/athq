package athq

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	defaultWorkGroup = "primary"
	defaultCatalog   = "AwsDataCatalog"

	envWorkGroup      = "ATHQ_WORK_GROUP"
	envDatabase       = "ATHQ_DATABASE"
	envOutputLocation = "ATHQ_OUTPUT_LOCATION"
	envCatalog        = "ATHQ_CATALOG"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// globalOptions holds the values given by the persistent flags. Every accessor
// below falls back to an environment variable and then to a default, so the
// precedence is always flag > env > default.
type globalOptions struct {
	workGroup      string
	database       string
	outputLocation string
	catalog        string
	region         string
	profile        string
}

var opts globalOptions

// rootOpts holds the flags accepted when athq is run with no subcommand,
// which opens the same interactive browser as `athq query --tui`.
var rootOpts struct {
	file string
}

var rootCmd = &cobra.Command{
	Use:   "athq",
	Short: "Query Amazon Athena from the command line",
	Long: "athq runs queries against Amazon Athena and shows or saves the results,\n" +
		"and lists work groups, databases and table definitions.\n\n" +
		"Run with no subcommand to open the interactive browser (like `athq query --tui`).",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runRoot,
}

// runRoot opens the interactive browser, optionally preloaded with a query
// read from --file or piped on stdin. Unlike `athq query`, the query cannot
// be given as a plain argument, since a bare word there would be ambiguous
// with a subcommand name.
func runRoot(_ *cobra.Command, _ []string) error {
	ctx, stop := signalContext()
	defer stop()

	c, err := newClients(ctx)
	if err != nil {
		return err
	}

	initial, err := initialTUIQuery(nil, rootOpts.file, false, os.Stdin)
	if err != nil {
		return err
	}
	return runTUI(ctx, c, initial, defaultMaxRows)
}

// SetVersionInfo records the build information injected by the linker.
func SetVersionInfo(ver, cmt, dt string) {
	version, commit, date = ver, cmt, dt
	rootCmd.Version = versionString()
}

func versionString() string {
	return fmt.Sprintf("athq version %s (commit %s, built at %s)", version, commit, date)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "athq:", err)
		os.Exit(1)
	}
}

func init() {
	pf := rootCmd.PersistentFlags()
	pf.StringVar(&opts.workGroup, "workgroup", "", "Athena work group (alias --wg, env "+envWorkGroup+", default "+defaultWorkGroup+")")
	pf.StringVar(&opts.database, "database", "", "default database (alias --db, env "+envDatabase+")")
	pf.StringVar(&opts.outputLocation, "output-location", "", "s3:// location for query results (env "+envOutputLocation+", default: the work group setting)")
	pf.StringVar(&opts.catalog, "catalog", "", "data catalog name (env "+envCatalog+", default "+defaultCatalog+")")
	pf.StringVar(&opts.region, "region", "", "AWS region (default: the usual AWS resolution)")
	pf.StringVar(&opts.profile, "profile", "", "AWS shared config profile")

	// Local (not persistent) so it does not collide with query's own -f/--file.
	rootCmd.Flags().StringVarP(&rootOpts.file, "file", "f", "", "read the query from this file (- for stdin) and preload it into the interactive browser")

	// cobra/pflag allow only one long name per flag, so --wg and --db are
	// accepted by normalizing them onto their canonical names.
	rootCmd.SetGlobalNormalizationFunc(normalizeFlagName)

	rootCmd.SetVersionTemplate(`{{printf "%s\n" .Version}}`)
	rootCmd.Version = versionString()

	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), versionString())
		},
	})
}

func normalizeFlagName(_ *pflag.FlagSet, name string) pflag.NormalizedName {
	switch name {
	case "wg":
		name = "workgroup"
	case "db":
		name = "database"
	}
	return pflag.NormalizedName(name)
}

func resolveOption(flagValue, envKey, fallback string) string {
	if flagValue != "" {
		return flagValue
	}
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	return fallback
}

func workGroup() string      { return resolveOption(opts.workGroup, envWorkGroup, defaultWorkGroup) }
func database() string       { return resolveOption(opts.database, envDatabase, "") }
func outputLocation() string { return resolveOption(opts.outputLocation, envOutputLocation, "") }
func catalog() string        { return resolveOption(opts.catalog, envCatalog, defaultCatalog) }

// requireDatabase returns the configured database or an error explaining how to
// set one.
func requireDatabase() (string, error) {
	db := database()
	if db == "" {
		return "", fmt.Errorf("no database given: use --database (--db) or set %s", envDatabase)
	}
	return db, nil
}
