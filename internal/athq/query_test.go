package athq

import (
	"bytes"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/athena/types"
)

func TestResolveOutputFormatPrefersTheFlag(t *testing.T) {
	got, err := resolveOutputFormat("out.csv", "json")
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if got != formatJSON {
		t.Errorf("got = %v, want %v", got, formatJSON)
	}
}

func TestResolveOutputFormatFollowsTheExtension(t *testing.T) {
	got, err := resolveOutputFormat("out.tsv", "")
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if got != formatTSV {
		t.Errorf("got = %v, want %v", got, formatTSV)
	}
}

func TestResolveOutputFormatDefaultsToTheTable(t *testing.T) {
	got, err := resolveOutputFormat("", "")
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if got != formatTable {
		t.Errorf("got = %v, want %v", got, formatTable)
	}
}

func TestResolveOutputFormatRejectsAnUnknownName(t *testing.T) {
	if _, err := resolveOutputFormat("", "parquet"); err == nil {
		t.Error("got no error for an unknown format, want one")
	}
}

func TestQueryFailureUsesTheStateChangeReason(t *testing.T) {
	qe := &types.QueryExecution{Status: &types.QueryExecutionStatus{
		State:             types.QueryExecutionStateFailed,
		StateChangeReason: aws.String("SYNTAX_ERROR: line 1:8 mismatched input"),
	}}
	err := queryFailure(qe)
	if err == nil {
		t.Fatal("got no error, want one")
	}
	if !strings.Contains(err.Error(), "SYNTAX_ERROR") {
		t.Errorf("got = %q, want it to carry the reason", err)
	}
}

func TestQueryFailureFallsBackToTheAthenaError(t *testing.T) {
	qe := &types.QueryExecution{Status: &types.QueryExecutionStatus{
		State:       types.QueryExecutionStateFailed,
		AthenaError: &types.AthenaError{ErrorMessage: aws.String("table not found")},
	}}
	if err := queryFailure(qe); err == nil || !strings.Contains(err.Error(), "table not found") {
		t.Errorf("got = %v, want it to carry the Athena error message", err)
	}
}

func TestResultLocationOfAnEmptyExecution(t *testing.T) {
	if got := resultLocation(nil); got != "" {
		t.Errorf("got = %q, want an empty string", got)
	}
	qe := &types.QueryExecution{ResultConfiguration: &types.ResultConfiguration{
		OutputLocation: aws.String("s3://bucket/key.csv"),
	}}
	if got := resultLocation(qe); got != "s3://bucket/key.csv" {
		t.Errorf("got = %q, want the output location", got)
	}
}

func TestPrintStatsShowsTheScannedBytesAndCost(t *testing.T) {
	qe := &types.QueryExecution{
		QueryExecutionId: aws.String("abc-123"),
		Status:           &types.QueryExecutionStatus{State: types.QueryExecutionStateSucceeded},
		Statistics: &types.QueryExecutionStatistics{
			DataScannedInBytes:         aws.Int64(2 * 1024 * 1024 * 1024),
			TotalExecutionTimeInMillis: aws.Int64(2300),
		},
	}
	var buf bytes.Buffer
	printStats(&buf, qe)

	got := buf.String()
	for _, want := range []string{"SUCCEEDED", "2.3s", "2.00 GiB", "abc-123"} {
		if !strings.Contains(got, want) {
			t.Errorf("got = %q, want it to contain %q", got, want)
		}
	}
}
