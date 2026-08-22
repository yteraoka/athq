package athq

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/athena"
	"github.com/aws/aws-sdk-go-v2/service/athena/types"
	"github.com/mattn/go-runewidth"
)

// fakeQueryResults serves canned GetQueryResults pages to the paginator.
type fakeQueryResults struct {
	pages []*athena.GetQueryResultsOutput
	calls int
}

func (f *fakeQueryResults) GetQueryResults(_ context.Context, _ *athena.GetQueryResultsInput, _ ...func(*athena.Options)) (*athena.GetQueryResultsOutput, error) {
	i := f.calls
	f.calls++
	if i >= len(f.pages) {
		return &athena.GetQueryResultsOutput{}, nil
	}
	return f.pages[i], nil
}

func resultPage(columns []string, rows ...[]*string) *athena.GetQueryResultsOutput {
	meta := &types.ResultSetMetadata{}
	for _, c := range columns {
		meta.ColumnInfo = append(meta.ColumnInfo, types.ColumnInfo{
			Name: aws.String(c),
			Type: aws.String("varchar"),
		})
	}
	rs := &types.ResultSet{ResultSetMetadata: meta}
	for _, r := range rows {
		row := types.Row{}
		for _, v := range r {
			row.Data = append(row.Data, types.Datum{VarCharValue: v})
		}
		rs.Rows = append(rs.Rows, row)
	}
	return &athena.GetQueryResultsOutput{ResultSet: rs}
}

func TestFormatFromPath(t *testing.T) {
	if got := formatFromPath("out.json"); got != formatJSON {
		t.Errorf("out.json: got = %v, want %v", got, formatJSON)
	}
	if got := formatFromPath("out.JSONL"); got != formatJSONL {
		t.Errorf("out.JSONL: got = %v, want %v", got, formatJSONL)
	}
	if got := formatFromPath("out.tsv"); got != formatTSV {
		t.Errorf("out.tsv: got = %v, want %v", got, formatTSV)
	}
	if got := formatFromPath("out.txt"); got != formatRaw {
		t.Errorf("out.txt: got = %v, want %v", got, formatRaw)
	}
	if got := formatFromPath("result"); got != formatCSV {
		t.Errorf("no extension: got = %v, want %v", got, formatCSV)
	}
}

func TestParseFormatRejectsUnknown(t *testing.T) {
	if _, err := parseFormat("yaml"); err == nil {
		t.Error("got no error for an unknown format, want one")
	}
	f, err := parseFormat("JSON")
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if f != formatJSON {
		t.Errorf("got = %v, want %v", f, formatJSON)
	}
}

func TestFetchResultsSkipsHeaderRowOfDML(t *testing.T) {
	api := &fakeQueryResults{pages: []*athena.GetQueryResultsOutput{
		resultPage([]string{"id", "name"},
			[]*string{aws.String("id"), aws.String("name")},
			[]*string{aws.String("1"), aws.String("alice")},
		),
	}}

	rt, err := fetchResults(context.Background(), api, "abc", types.StatementTypeDml, 0)
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if len(rt.rows) != 1 {
		t.Fatalf("row count: got = %d, want 1", len(rt.rows))
	}
	if got := *rt.rows[0][0]; got != "1" {
		t.Errorf("first value: got = %q, want %q", got, "1")
	}
}

func TestFetchResultsKeepsFirstRowOfUtility(t *testing.T) {
	// SHOW CREATE TABLE and friends do not repeat the column names.
	api := &fakeQueryResults{pages: []*athena.GetQueryResultsOutput{
		resultPage([]string{"createtab_stmt"},
			[]*string{aws.String("CREATE EXTERNAL TABLE t(")},
			[]*string{aws.String(")")},
		),
	}}

	rt, err := fetchResults(context.Background(), api, "abc", types.StatementTypeUtility, 0)
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if len(rt.rows) != 2 {
		t.Fatalf("row count: got = %d, want 2", len(rt.rows))
	}
	if got := *rt.rows[0][0]; got != "CREATE EXTERNAL TABLE t(" {
		t.Errorf("first value: got = %q, want the CREATE statement", got)
	}
}

func TestFetchResultsStopsAtLimit(t *testing.T) {
	api := &fakeQueryResults{pages: []*athena.GetQueryResultsOutput{
		resultPage([]string{"n"},
			[]*string{aws.String("n")},
			[]*string{aws.String("1")},
			[]*string{aws.String("2")},
			[]*string{aws.String("3")},
		),
	}}

	rt, err := fetchResults(context.Background(), api, "abc", types.StatementTypeDml, 2)
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if len(rt.rows) != 2 {
		t.Errorf("row count: got = %d, want 2", len(rt.rows))
	}
	if !rt.truncated {
		t.Error("truncated: got = false, want true")
	}
}

func TestWriteSeparatedEscapesAndBlanksNull(t *testing.T) {
	rt := &resultTable{
		columns: []column{{name: "a", typ: "varchar"}, {name: "b", typ: "varchar"}},
		rows: [][]*string{
			{aws.String(`x,y "z"`), nil},
			{aws.String("line1\nline2"), aws.String("")},
		},
	}
	var buf bytes.Buffer
	if err := writeSeparated(&buf, rt, ','); err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	want := "a,b\n\"x,y \"\"z\"\"\",\n\"line1\nline2\",\n"
	if got := buf.String(); got != want {
		t.Errorf("got = %q, want %q", got, want)
	}
}

func TestWriteJSONUsesColumnTypes(t *testing.T) {
	rt := &resultTable{
		columns: []column{
			{name: "id", typ: "bigint"},
			{name: "ratio", typ: "double"},
			{name: "ok", typ: "boolean"},
			{name: "name", typ: "varchar"},
			{name: "missing", typ: "varchar"},
		},
		rows: [][]*string{{
			aws.String("42"), aws.String("1.5"), aws.String("true"), aws.String("a&b"), nil,
		}},
	}
	var buf bytes.Buffer
	if err := writeJSON(&buf, rt); err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	want := "[\n  {\"id\":42,\"ratio\":1.5,\"ok\":true,\"name\":\"a&b\",\"missing\":null}\n]\n"
	if got := buf.String(); got != want {
		t.Errorf("got = %q, want %q", got, want)
	}
}

func TestWriteJSONWithNoRows(t *testing.T) {
	rt := &resultTable{columns: []column{{name: "a", typ: "varchar"}}}
	var buf bytes.Buffer
	if err := writeJSON(&buf, rt); err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if got := buf.String(); got != "[]\n" {
		t.Errorf("got = %q, want %q", got, "[]\n")
	}
}

func TestWriteJSONLWritesOneObjectPerLine(t *testing.T) {
	rt := &resultTable{
		columns: []column{{name: "n", typ: "integer"}},
		rows:    [][]*string{{aws.String("1")}, {aws.String("2")}},
	}
	var buf bytes.Buffer
	if err := writeJSONL(&buf, rt); err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	want := "{\"n\":1}\n{\"n\":2}\n"
	if got := buf.String(); got != want {
		t.Errorf("got = %q, want %q", got, want)
	}
}

func TestJSONValueFallsBackToString(t *testing.T) {
	got := jsonValue(aws.String("not a number"), "bigint")
	if got != "not a number" {
		t.Errorf("got = %v, want the original string", got)
	}
	if v := jsonValue(nil, "varchar"); v != nil {
		t.Errorf("NULL: got = %v, want nil", v)
	}
}

func TestRenderTableAlignsNumbersRight(t *testing.T) {
	var buf bytes.Buffer
	err := renderTable(&buf, []string{"NAME", "N"}, [][]string{{"ab", "1"}, {"c", "22"}}, []bool{false, true}, 0)
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	want := "NAME   N\n----  --\nab     1\nc     22\n"
	if got := buf.String(); got != want {
		t.Errorf("got = %q, want %q", got, want)
	}
}

func TestRenderTableTruncatesToWidth(t *testing.T) {
	var buf bytes.Buffer
	long := strings.Repeat("x", 40)
	if err := renderTable(&buf, []string{"COL"}, [][]string{{long}}, nil, 20); err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		if w := runewidth.StringWidth(line); w > 20 {
			t.Errorf("line %q is %d wide, want at most 20", line, w)
		}
	}
	if !strings.Contains(buf.String(), "…") {
		t.Error("got no ellipsis, want the truncated cell marked")
	}
}

func TestSanitizeCellFoldsNewlines(t *testing.T) {
	if got := sanitizeCell("a\nb\tc"); got != "a b c" {
		t.Errorf("got = %q, want %q", got, "a b c")
	}
}
