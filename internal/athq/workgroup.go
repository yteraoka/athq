package athq

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/athena"
	"github.com/spf13/cobra"
)

var workGroupCmd = &cobra.Command{
	Use:     "workgroup",
	Aliases: []string{"wg"},
	Short:   "Show Athena work groups",
}

var workGroupListCmd = &cobra.Command{
	Use:   "list",
	Short: "List work groups",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, stop := signalContext()
		defer stop()

		c, err := newClients(ctx)
		if err != nil {
			return err
		}

		pager := athena.NewListWorkGroupsPaginator(c.athena, &athena.ListWorkGroupsInput{})
		var rows [][]string
		for pager.HasMorePages() {
			page, err := pager.NextPage(ctx)
			if err != nil {
				return fmt.Errorf("failed to list work groups: %w", err)
			}
			for _, wg := range page.WorkGroups {
				engine := ""
				if wg.EngineVersion != nil {
					engine = aws.ToString(wg.EngineVersion.EffectiveEngineVersion)
				}
				rows = append(rows, []string{
					aws.ToString(wg.Name),
					string(wg.State),
					engine,
					aws.ToString(wg.Description),
				})
			}
		}
		return renderTable(cmd.OutOrStdout(), []string{"NAME", "STATE", "ENGINE", "DESCRIPTION"}, rows, nil, terminalWidth(os.Stdout))
	},
}

var workGroupDescCmd = &cobra.Command{
	Use:     "describe [NAME]",
	Aliases: []string{"desc"},
	Short:   "Show the details of a work group",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := workGroup()
		if len(args) == 1 {
			name = args[0]
		}

		ctx, stop := signalContext()
		defer stop()

		c, err := newClients(ctx)
		if err != nil {
			return err
		}

		out, err := c.athena.GetWorkGroup(ctx, &athena.GetWorkGroupInput{WorkGroup: aws.String(name)})
		if err != nil {
			return fmt.Errorf("failed to get the work group %s: %w", name, err)
		}
		wg := out.WorkGroup
		if wg == nil {
			return fmt.Errorf("work group %s not found", name)
		}

		w := cmd.OutOrStdout()
		printField(w, "Name", aws.ToString(wg.Name))
		printField(w, "State", string(wg.State))
		printField(w, "Description", aws.ToString(wg.Description))
		if wg.CreationTime != nil {
			printField(w, "CreationTime", wg.CreationTime.Local().Format("2006-01-02 15:04:05"))
		}

		cfg := wg.Configuration
		if cfg == nil {
			return nil
		}
		if cfg.ResultConfiguration != nil {
			printField(w, "OutputLocation", aws.ToString(cfg.ResultConfiguration.OutputLocation))
			if enc := cfg.ResultConfiguration.EncryptionConfiguration; enc != nil {
				printField(w, "EncryptionOption", string(enc.EncryptionOption))
				printField(w, "KmsKey", aws.ToString(enc.KmsKey))
			}
			printField(w, "ExpectedBucketOwner", aws.ToString(cfg.ResultConfiguration.ExpectedBucketOwner))
		}
		if cfg.EngineVersion != nil {
			printField(w, "EngineVersion", aws.ToString(cfg.EngineVersion.EffectiveEngineVersion))
		}
		printField(w, "EnforceWorkGroupConfiguration", formatBool(cfg.EnforceWorkGroupConfiguration))
		printField(w, "PublishCloudWatchMetrics", formatBool(cfg.PublishCloudWatchMetricsEnabled))
		printField(w, "RequesterPays", formatBool(cfg.RequesterPaysEnabled))
		if cfg.BytesScannedCutoffPerQuery != nil {
			printField(w, "BytesScannedCutoffPerQuery", humanBytes(*cfg.BytesScannedCutoffPerQuery))
		}
		printField(w, "ExecutionRole", aws.ToString(cfg.ExecutionRole))
		printField(w, "AdditionalConfiguration", aws.ToString(cfg.AdditionalConfiguration))
		return nil
	},
}

// printField prints one key/value line, skipping the ones that are not set.
func printField(w io.Writer, key, value string) {
	if value == "" {
		return
	}
	_, _ = fmt.Fprintf(w, "%-30s %s\n", key+":", value)
}

func formatBool(b *bool) string {
	if b == nil {
		return ""
	}
	return strconv.FormatBool(*b)
}

func init() {
	workGroupCmd.AddCommand(workGroupListCmd, workGroupDescCmd)
	rootCmd.AddCommand(workGroupCmd)
}
