package athq

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/athena"
	"github.com/aws/aws-sdk-go-v2/service/athena/types"
	"github.com/spf13/cobra"
)

var tableCmd = &cobra.Command{
	Use:     "table",
	Aliases: []string{"tbl"},
	Short:   "Show tables and their definitions",
}

var tableListCmd = &cobra.Command{
	Use:   "list [PATTERN]",
	Short: "List tables in the database",
	Long: `List tables in the database.

PATTERN is matched against the whole table name with * as the wildcard, so
"log*" finds the tables whose name starts with log. A pattern without a
wildcard is looked for anywhere in the name.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := requireDatabase()
		if err != nil {
			return err
		}

		ctx, stop := signalContext()
		defer stop()

		c, err := newClients(ctx)
		if err != nil {
			return err
		}

		expression := ""
		if len(args) == 1 {
			expression = tablePattern(args[0])
		}
		tables, err := listTables(ctx, c, db, expression)
		if err != nil {
			return err
		}

		rows := make([][]string, 0, len(tables))
		for _, t := range tables {
			created := formatTimestamp(t.CreateTime)
			rows = append(rows, []string{
				aws.ToString(t.Name),
				aws.ToString(t.TableType),
				fmt.Sprintf("%d", len(t.Columns)),
				fmt.Sprintf("%d", len(t.PartitionKeys)),
				created,
			})
		}
		return renderTable(cmd.OutOrStdout(),
			[]string{"NAME", "TYPE", "COLUMNS", "PARTITIONS", "CREATED"},
			rows,
			[]bool{false, false, true, true, false},
			terminalWidth(os.Stdout))
	},
}

var tableDescOpts struct {
	metadata bool
}

var tableDescCmd = &cobra.Command{
	Use:     "describe [DATABASE.]TABLE",
	Aliases: []string{"desc"},
	Short:   "Show the DDL of a table",
	Long: `Show the DDL of a table.

By default SHOW CREATE TABLE is run so that Athena's own DDL is printed. With
--metadata the catalog metadata is read through the API instead, which runs no
query.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db, table, err := splitTableName(args[0])
		if err != nil {
			return err
		}

		ctx, stop := signalContext()
		defer stop()

		c, err := newClients(ctx)
		if err != nil {
			return err
		}

		if tableDescOpts.metadata {
			return describeTableMetadata(ctx, c, cmd, db, table)
		}

		sql := fmt.Sprintf("SHOW CREATE TABLE %s.%s", quoteIdentifier(db), quoteIdentifier(table))
		qe, err := runStatement(ctx, c, sql, newDiscardProgress())
		if err != nil {
			return err
		}
		rt, err := fetchResults(ctx, c.athena, aws.ToString(qe.QueryExecutionId), qe.StatementType, 0)
		if err != nil {
			return err
		}
		return writeRaw(cmd.OutOrStdout(), rt)
	},
}

// tablePattern prepares the filter for ListTableMetadata, which matches the
// whole table name with * as the wildcard. A pattern without a wildcard would
// therefore find nothing unless it happened to be the entire name, so it is
// taken as a substring instead.
func tablePattern(s string) string {
	if s == "" || strings.Contains(s, "*") {
		return s
	}
	return "*" + s + "*"
}

// formatTimestamp renders a catalog timestamp, treating the Unix epoch as
// "never": Glue reports it that way for a table that has not been accessed.
func formatTimestamp(t *time.Time) string {
	if t == nil || t.Unix() <= 0 {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

// listTables reads the table metadata of a database from the catalog. The
// result carries the columns and partition keys as well, so one call is enough
// to describe every table in the database.
func listTables(ctx context.Context, c *clients, db, expression string) ([]types.TableMetadata, error) {
	in := &athena.ListTableMetadataInput{
		CatalogName:  aws.String(catalog()),
		DatabaseName: aws.String(db),
		WorkGroup:    aws.String(workGroup()),
	}
	if expression != "" {
		in.Expression = aws.String(expression)
	}

	pager := athena.NewListTableMetadataPaginator(c.athena, in)
	var tables []types.TableMetadata
	for pager.HasMorePages() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list tables in %s: %w", db, err)
		}
		tables = append(tables, page.TableMetadataList...)
	}
	return tables, nil
}

func describeTableMetadata(ctx context.Context, c *clients, cmd *cobra.Command, db, table string) error {
	out, err := c.athena.GetTableMetadata(ctx, &athena.GetTableMetadataInput{
		CatalogName:  aws.String(catalog()),
		DatabaseName: aws.String(db),
		TableName:    aws.String(table),
		WorkGroup:    aws.String(workGroup()),
	})
	if err != nil {
		return fmt.Errorf("failed to get the metadata of %s.%s: %w", db, table, err)
	}
	t := out.TableMetadata
	if t == nil {
		return fmt.Errorf("table %s.%s not found", db, table)
	}

	w := cmd.OutOrStdout()
	printField(w, "Name", aws.ToString(t.Name))
	printField(w, "Database", db)
	printField(w, "TableType", aws.ToString(t.TableType))
	printField(w, "CreateTime", formatTimestamp(t.CreateTime))
	printField(w, "LastAccessTime", formatTimestamp(t.LastAccessTime))

	width := terminalWidth(os.Stdout)
	if len(t.Columns) > 0 {
		_, _ = fmt.Fprintln(w, "\nColumns:")
		if err := renderTable(w, []string{"NAME", "TYPE", "COMMENT"}, columnRows(t.Columns), nil, width); err != nil {
			return err
		}
	}
	if len(t.PartitionKeys) > 0 {
		_, _ = fmt.Fprintln(w, "\nPartition keys:")
		if err := renderTable(w, []string{"NAME", "TYPE", "COMMENT"}, columnRows(t.PartitionKeys), nil, width); err != nil {
			return err
		}
	}
	if len(t.Parameters) > 0 {
		_, _ = fmt.Fprintln(w, "\nParameters:")
		keys := make([]string, 0, len(t.Parameters))
		for k := range t.Parameters {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		rows := make([][]string, 0, len(keys))
		for _, k := range keys {
			rows = append(rows, []string{k, t.Parameters[k]})
		}
		if err := renderTable(w, []string{"KEY", "VALUE"}, rows, nil, width); err != nil {
			return err
		}
	}
	return nil
}

func columnRows(cols []types.Column) [][]string {
	rows := make([][]string, 0, len(cols))
	for _, c := range cols {
		rows = append(rows, []string{
			aws.ToString(c.Name),
			aws.ToString(c.Type),
			aws.ToString(c.Comment),
		})
	}
	return rows
}

// splitTableName accepts "table" or "database.table" and falls back to the
// configured database.
func splitTableName(arg string) (db, table string, err error) {
	if i := strings.Index(arg, "."); i >= 0 {
		db, table = arg[:i], arg[i+1:]
		if db == "" || table == "" {
			return "", "", fmt.Errorf("invalid table name %q: want [database.]table", arg)
		}
		return db, table, nil
	}
	db, err = requireDatabase()
	if err != nil {
		return "", "", err
	}
	return db, arg, nil
}

var plainIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// quoteIdentifier leaves ordinary names alone and double quotes anything else,
// which is how Athena's SQL accepts unusual identifiers.
func quoteIdentifier(s string) string {
	if plainIdentifier.MatchString(s) {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func init() {
	tableDescCmd.Flags().BoolVar(&tableDescOpts.metadata, "metadata", false, "read the catalog metadata through the API instead of running SHOW CREATE TABLE")
	tableCmd.AddCommand(tableListCmd, tableDescCmd)
	rootCmd.AddCommand(tableCmd)
}
