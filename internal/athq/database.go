package athq

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/athena"
	"github.com/aws/aws-sdk-go-v2/service/athena/types"
	"github.com/spf13/cobra"
)

var databaseCmd = &cobra.Command{
	Use:     "database",
	Aliases: []string{"db"},
	Short:   "Show databases in the data catalog",
}

var databaseListCmd = &cobra.Command{
	Use:   "list",
	Short: "List databases",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, stop := signalContext()
		defer stop()

		c, err := newClients(ctx)
		if err != nil {
			return err
		}

		databases, err := listDatabases(ctx, c)
		if err != nil {
			return err
		}
		rows := make([][]string, 0, len(databases))
		for _, db := range databases {
			rows = append(rows, []string{
				aws.ToString(db.Name),
				aws.ToString(db.Description),
			})
		}
		return renderTable(cmd.OutOrStdout(), []string{"NAME", "DESCRIPTION"}, rows, nil, terminalWidth(os.Stdout))
	},
}

// listDatabases reads the catalog directly, so no query is run and nothing is
// billed.
func listDatabases(ctx context.Context, c *clients) ([]types.Database, error) {
	pager := athena.NewListDatabasesPaginator(c.athena, &athena.ListDatabasesInput{
		CatalogName: aws.String(catalog()),
		WorkGroup:   aws.String(workGroup()),
	})
	var databases []types.Database
	for pager.HasMorePages() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list databases: %w", err)
		}
		databases = append(databases, page.DatabaseList...)
	}
	return databases, nil
}

func init() {
	databaseCmd.AddCommand(databaseListCmd)
	rootCmd.AddCommand(databaseCmd)
}
