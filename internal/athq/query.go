package athq

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/athena"
	"github.com/aws/aws-sdk-go-v2/service/athena/types"
	"github.com/spf13/cobra"
)

var errCanceled = errors.New("interrupted")

// defaultMaxRows is how many result rows are shown when --max-rows is not
// given, both on stdout and in the TUI result pane.
const defaultMaxRows = 100

var queryOpts struct {
	output  string
	file    string
	editor  bool
	format  string
	maxRows int
	id      string
	last    bool
	quiet   bool
	tui     bool
}

var queryCmd = &cobra.Command{
	Use:     "query [SQL]",
	Aliases: []string{"q"},
	Short:   "Run a query and print or save the result",
	Long: `Run a query and print or save the result.

The SQL can be given as an argument, read from a file with --file, written in
$EDITOR with --editor, or piped on stdin. Without --output the result is shown
as a table on stdout; with --output the format follows the file extension
(.csv, .tsv, .json, .jsonl, .txt) unless --format says otherwise.`,
	Example: `  athq q "SELECT * FROM logs LIMIT 10"
  athq q -f query.sql -o result.csv
  athq q -e --db mydb
  echo "SELECT 1" | athq q
  athq q --last -o result.json
  athq q --tui`,
	RunE: runQuery,
}

func init() {
	f := queryCmd.Flags()
	f.StringVarP(&queryOpts.output, "output", "o", "", "write the result to this file instead of stdout")
	f.StringVarP(&queryOpts.file, "file", "f", "", "read the query from this file (- for stdin)")
	f.BoolVarP(&queryOpts.editor, "editor", "e", false, "write the query in $EDITOR")
	f.StringVar(&queryOpts.format, "format", "", "output format: table, csv, tsv, json, jsonl or raw (raw copies the file Athena wrote)")
	f.IntVar(&queryOpts.maxRows, "max-rows", defaultMaxRows, "rows to show on stdout (0 for all)")
	f.StringVar(&queryOpts.id, "id", "", "fetch the result of an existing query execution instead of running one")
	f.BoolVar(&queryOpts.last, "last", false, "fetch the result of the last successful query")
	f.BoolVar(&queryOpts.quiet, "quiet", false, "do not print the execution statistics")
	f.BoolVarP(&queryOpts.tui, "tui", "t", false, "open the interactive browser: table definitions beside the query editor")
	rootCmd.AddCommand(queryCmd)
}

func runQuery(cmd *cobra.Command, args []string) error {
	if queryOpts.id != "" && queryOpts.last {
		return errors.New("--id and --last cannot be used together")
	}
	// Fail on a bad format before anything is sent to AWS.
	format, err := resolveOutputFormat(queryOpts.output, queryOpts.format)
	if err != nil {
		return err
	}

	ctx, stop := signalContext()
	defer stop()

	c, err := newClients(ctx)
	if err != nil {
		return err
	}

	if queryOpts.tui {
		if queryOpts.id != "" || queryOpts.last {
			return errors.New("--tui cannot be combined with --id or --last")
		}
		initial, err := initialTUIQuery(args, queryOpts.file, queryOpts.editor, os.Stdin)
		if err != nil {
			return err
		}
		return runTUI(ctx, c, initial, queryOpts.maxRows)
	}

	executionID := queryOpts.id
	if queryOpts.last {
		path, err := historyPath()
		if err != nil {
			return err
		}
		entries, err := readHistory(path, 0)
		if err != nil {
			return err
		}
		e, ok := lastSucceeded(entries)
		if !ok {
			return errors.New("no successful query in the history")
		}
		executionID = e.QueryExecutionID
	}

	var qe *types.QueryExecution
	if executionID != "" {
		if len(args) > 0 || queryOpts.file != "" || queryOpts.editor {
			return errors.New("--id and --last take no query")
		}
		qe, err = getQueryExecution(ctx, c, executionID)
		if err != nil {
			return err
		}
		if qe.Status == nil || qe.Status.State != types.QueryExecutionStateSucceeded {
			return queryFailure(qe)
		}
		if !queryOpts.quiet {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "athq: result of execution %s\n", executionID)
		}
	} else {
		sql, err := resolveQuery(args, queryOpts.file, queryOpts.editor, os.Stdin)
		if err != nil {
			return err
		}
		qe, err = runStatement(ctx, c, sql, newStderrProgress())
		if err != nil {
			return err
		}
		if !queryOpts.quiet {
			printStats(cmd.ErrOrStderr(), qe)
		}
	}

	return emitResults(ctx, c, cmd.ErrOrStderr(), qe, queryOpts.output, format, queryOpts.maxRows)
}

// resolveOutputFormat picks the format from --format, then from the extension
// of the destination, and finally falls back to the stdout table.
func resolveOutputFormat(dest, formatFlag string) (outputFormat, error) {
	switch {
	case formatFlag != "":
		return parseFormat(formatFlag)
	case dest != "":
		return formatFromPath(dest), nil
	default:
		return formatTable, nil
	}
}

// runStatement starts a query, waits for it to finish and records it in the
// history. Progress is drawn through p, so a caller that owns the screen (the
// TUI) can pass a silent one.
func runStatement(ctx context.Context, c *clients, sql string, p *progress) (*types.QueryExecution, error) {
	in := &athena.StartQueryExecutionInput{
		QueryString: aws.String(sql),
		WorkGroup:   aws.String(workGroup()),
	}
	if db := database(); db != "" {
		in.QueryExecutionContext = &types.QueryExecutionContext{
			Database: aws.String(db),
			Catalog:  aws.String(catalog()),
		}
	}
	// Without an explicit location the work group setting is used.
	if loc := outputLocation(); loc != "" {
		in.ResultConfiguration = &types.ResultConfiguration{OutputLocation: aws.String(loc)}
	}

	out, err := c.athena.StartQueryExecution(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("failed to start the query: %w", err)
	}
	id := aws.ToString(out.QueryExecutionId)

	qe, waitErr := waitForQuery(ctx, c, id, p)

	entry := historyEntry{
		Time:             time.Now(),
		QueryExecutionID: id,
		WorkGroup:        workGroup(),
		Database:         database(),
		State:            "UNKNOWN",
		Query:            sql,
	}
	if errors.Is(waitErr, errCanceled) {
		entry.State = string(types.QueryExecutionStateCancelled)
	}
	if qe != nil {
		if qe.Status != nil {
			entry.State = string(qe.Status.State)
		}
		if qe.Statistics != nil {
			entry.DataScannedInBytes = aws.ToInt64(qe.Statistics.DataScannedInBytes)
		}
	}
	recordHistory(entry)

	if waitErr != nil {
		return qe, waitErr
	}
	return qe, nil
}

func getQueryExecution(ctx context.Context, c *clients, id string) (*types.QueryExecution, error) {
	out, err := c.athena.GetQueryExecution(ctx, &athena.GetQueryExecutionInput{
		QueryExecutionId: aws.String(id),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get the query execution %s: %w", id, err)
	}
	if out.QueryExecution == nil {
		return nil, fmt.Errorf("query execution %s not found", id)
	}
	return out.QueryExecution, nil
}

// waitForQuery polls until the query settles, showing a status line while it
// runs. An interrupt stops the query on the Athena side too.
func waitForQuery(ctx context.Context, c *clients, id string, p *progress) (*types.QueryExecution, error) {
	const maxDelay = 2 * time.Second

	defer p.clear()

	start := time.Now()
	delay := 200 * time.Millisecond
	for {
		qe, err := getQueryExecution(ctx, c, id)
		if err != nil {
			if ctx.Err() != nil {
				return nil, stopQuery(c, id)
			}
			return nil, err
		}
		state := types.QueryExecutionStateQueued
		if qe.Status != nil {
			state = qe.Status.State
		}
		switch state {
		case types.QueryExecutionStateSucceeded:
			p.clear()
			return qe, nil
		case types.QueryExecutionStateFailed, types.QueryExecutionStateCancelled:
			p.clear()
			return qe, queryFailure(qe)
		}

		p.update(string(state), time.Since(start))
		select {
		case <-ctx.Done():
			p.clear()
			return nil, stopQuery(c, id)
		case <-time.After(delay):
		}
		if delay < maxDelay {
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}
}

// stopQuery cancels the execution on the Athena side. It runs on a fresh
// context because the original one is already canceled.
func stopQuery(c *clients, id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := c.athena.StopQueryExecution(ctx, &athena.StopQueryExecutionInput{
		QueryExecutionId: aws.String(id),
	}); err != nil {
		return fmt.Errorf("%w: failed to stop the query %s: %w", errCanceled, id, err)
	}
	return fmt.Errorf("%w: stopped the query %s", errCanceled, id)
}

func queryFailure(qe *types.QueryExecution) error {
	if qe == nil || qe.Status == nil {
		return errors.New("the query did not succeed")
	}
	state := strings.ToLower(string(qe.Status.State))
	reason := aws.ToString(qe.Status.StateChangeReason)
	if reason == "" && qe.Status.AthenaError != nil {
		reason = aws.ToString(qe.Status.AthenaError.ErrorMessage)
	}
	if reason == "" {
		return fmt.Errorf("the query %s", state)
	}
	return fmt.Errorf("the query %s: %s", state, reason)
}

func printStats(w io.Writer, qe *types.QueryExecution) {
	if qe == nil {
		return
	}
	var scanned, millis int64
	if qe.Statistics != nil {
		scanned = aws.ToInt64(qe.Statistics.DataScannedInBytes)
		millis = aws.ToInt64(qe.Statistics.TotalExecutionTimeInMillis)
	}
	state := ""
	if qe.Status != nil {
		state = string(qe.Status.State)
	}
	_, _ = fmt.Fprintf(w, "athq: %s in %s, scanned %s (~$%.4f), execution id %s\n",
		state,
		formatDuration(time.Duration(millis)*time.Millisecond),
		humanBytes(scanned),
		estimateCost(scanned, pricePerTB()),
		aws.ToString(qe.QueryExecutionId),
	)
}

func resultLocation(qe *types.QueryExecution) string {
	if qe == nil || qe.ResultConfiguration == nil {
		return ""
	}
	return aws.ToString(qe.ResultConfiguration.OutputLocation)
}

// emitResults writes the query result in the requested format.
func emitResults(ctx context.Context, c *clients, notices io.Writer, qe *types.QueryExecution, dest string, format outputFormat, maxRows int) error {
	// Athena already wrote a CSV to S3, so hand that file over untouched
	// instead of paging through the API.
	if format == formatCSV || format == formatRaw {
		if loc := resultLocation(qe); loc != "" {
			return withOutput(dest, func(w io.Writer) error {
				_, err := copyS3Object(ctx, c, loc, w)
				return err
			})
		}
	}

	limit := 0
	if format == formatTable && dest == "" {
		limit = maxRows
	}
	rt, err := fetchResults(ctx, c.athena, aws.ToString(qe.QueryExecutionId), qe.StatementType, limit)
	if err != nil {
		return err
	}

	width := 0
	if format == formatTable && dest == "" {
		width = terminalWidth(os.Stdout)
	}
	if err := withOutput(dest, func(w io.Writer) error {
		return writeResult(w, rt, format, width)
	}); err != nil {
		return err
	}
	if rt.truncated {
		_, _ = fmt.Fprintf(notices, "athq: showing the first %d rows (--max-rows 0 for all)\n", limit)
	}
	return nil
}

// withOutput runs fn against dest, or against stdout when dest is empty.
func withOutput(dest string, fn func(io.Writer) error) error {
	if dest == "" {
		return fn(os.Stdout)
	}
	f, err := createFile(dest)
	if err != nil {
		return err
	}
	if err := fn(f); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to write %s: %w", dest, err)
	}
	return nil
}
