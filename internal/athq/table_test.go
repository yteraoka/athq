package athq

import "testing"

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
