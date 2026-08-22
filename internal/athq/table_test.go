package athq

import (
	"testing"
	"time"
)

func TestSplitTableNameWithDatabase(t *testing.T) {
	db, table, err := splitTableName("analytics.events")
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if db != "analytics" || table != "events" {
		t.Errorf("got = %q,%q, want analytics,events", db, table)
	}
}

func TestSplitTableNameUsesTheConfiguredDatabase(t *testing.T) {
	t.Setenv(envDatabase, "logs")
	opts.database = ""

	db, table, err := splitTableName("events")
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if db != "logs" || table != "events" {
		t.Errorf("got = %q,%q, want logs,events", db, table)
	}
}

func TestSplitTableNameWithoutAnyDatabase(t *testing.T) {
	t.Setenv(envDatabase, "")
	opts.database = ""

	if _, _, err := splitTableName("events"); err == nil {
		t.Error("got no error without a database, want one")
	}
}

func TestSplitTableNameRejectsAnEmptyPart(t *testing.T) {
	if _, _, err := splitTableName("db."); err == nil {
		t.Error("got no error for a missing table name, want one")
	}
}

func TestQuoteIdentifierLeavesPlainNamesAlone(t *testing.T) {
	if got := quoteIdentifier("my_table1"); got != "my_table1" {
		t.Errorf("got = %q, want it unquoted", got)
	}
}

func TestQuoteIdentifierQuotesUnusualNames(t *testing.T) {
	if got := quoteIdentifier("my-table"); got != `"my-table"` {
		t.Errorf("got = %q, want it quoted", got)
	}
	if got := quoteIdentifier(`we"ird`); got != `"we""ird"` {
		t.Errorf("got = %q, want the inner quote doubled", got)
	}
}

func TestFormatTimestampTreatsTheEpochAsNever(t *testing.T) {
	epoch := time.Unix(0, 0)
	if got := formatTimestamp(&epoch); got != "" {
		t.Errorf("got = %q, want an empty string", got)
	}
	if got := formatTimestamp(nil); got != "" {
		t.Errorf("nil: got = %q, want an empty string", got)
	}
	when := time.Date(2026, 8, 22, 10, 30, 0, 0, time.Local)
	if got := formatTimestamp(&when); got != "2026-08-22 10:30:00" {
		t.Errorf("got = %q, want %q", got, "2026-08-22 10:30:00")
	}
}

func TestTablePatternWrapsAPlainWord(t *testing.T) {
	if got := tablePattern("cloudtrail"); got != "*cloudtrail*" {
		t.Errorf("got = %q, want %q", got, "*cloudtrail*")
	}
	if got := tablePattern("log*"); got != "log*" {
		t.Errorf("a pattern with a wildcard: got = %q, want it unchanged", got)
	}
	if got := tablePattern(""); got != "" {
		t.Errorf("empty: got = %q, want it unchanged", got)
	}
}
