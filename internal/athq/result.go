package athq

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/athena"
	"github.com/aws/aws-sdk-go-v2/service/athena/types"
	"github.com/mattn/go-runewidth"
)

type outputFormat string

const (
	formatTable outputFormat = "table"
	formatCSV   outputFormat = "csv"
	formatTSV   outputFormat = "tsv"
	formatJSON  outputFormat = "json"
	formatJSONL outputFormat = "jsonl"
	formatRaw   outputFormat = "raw"
)

// formatFromPath guesses the output format from the extension of the
// destination path. Anything unknown is treated as CSV, which is what Athena
// itself writes to S3.
func formatFromPath(path string) outputFormat {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".tsv":
		return formatTSV
	case ".json":
		return formatJSON
	case ".jsonl", ".ndjson":
		return formatJSONL
	case ".txt":
		return formatRaw
	default:
		return formatCSV
	}
}

func parseFormat(s string) (outputFormat, error) {
	switch outputFormat(strings.ToLower(s)) {
	case formatTable:
		return formatTable, nil
	case formatCSV:
		return formatCSV, nil
	case formatTSV:
		return formatTSV, nil
	case formatJSON:
		return formatJSON, nil
	case formatJSONL:
		return formatJSONL, nil
	case formatRaw:
		return formatRaw, nil
	}
	return "", fmt.Errorf("unknown format %q: want table, csv, tsv, json, jsonl or raw", s)
}

type column struct {
	name string
	typ  string
}

// resultTable is the in-memory form of a query result. Values are pointers so
// that SQL NULL stays distinguishable from an empty string.
type resultTable struct {
	columns   []column
	rows      [][]*string
	truncated bool
}

func (rt *resultTable) value(row []*string, i int) *string {
	if i < len(row) {
		return row[i]
	}
	return nil
}

// fetchResults pages through GetQueryResults. When limit is greater than zero
// it stops after that many rows and marks the table as truncated.
func fetchResults(ctx context.Context, api athena.GetQueryResultsAPIClient, executionID string, statementType types.StatementType, limit int) (*resultTable, error) {
	pager := athena.NewGetQueryResultsPaginator(api, &athena.GetQueryResultsInput{
		QueryExecutionId: aws.String(executionID),
		MaxResults:       aws.Int32(1000),
	})

	rt := &resultTable{}
	firstPage := true
	for pager.HasMorePages() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get query results: %w", err)
		}
		if page.ResultSet == nil {
			break
		}
		if rt.columns == nil && page.ResultSet.ResultSetMetadata != nil {
			for _, ci := range page.ResultSet.ResultSetMetadata.ColumnInfo {
				rt.columns = append(rt.columns, column{
					name: aws.ToString(ci.Name),
					typ:  aws.ToString(ci.Type),
				})
			}
		}

		rows := page.ResultSet.Rows
		if firstPage {
			firstPage = false
			// For DML statements Athena repeats the column names as the first
			// row of the first page. DDL and UTILITY statements (SHOW CREATE
			// TABLE, DESCRIBE, ...) do not, so their first row is real data.
			if statementType == types.StatementTypeDml && len(rows) > 0 {
				rows = rows[1:]
			}
		}

		for _, r := range rows {
			if limit > 0 && len(rt.rows) >= limit {
				rt.truncated = true
				return rt, nil
			}
			vals := make([]*string, len(r.Data))
			for i, d := range r.Data {
				vals[i] = d.VarCharValue
			}
			rt.rows = append(rt.rows, vals)
		}
	}
	return rt, nil
}

func writeResult(w io.Writer, rt *resultTable, f outputFormat, maxWidth int) error {
	switch f {
	case formatCSV:
		return writeSeparated(w, rt, ',')
	case formatTSV:
		return writeSeparated(w, rt, '\t')
	case formatJSON:
		return writeJSON(w, rt)
	case formatJSONL:
		return writeJSONL(w, rt)
	case formatRaw:
		return writeRaw(w, rt)
	default:
		return writeTable(w, rt, maxWidth)
	}
}

func writeSeparated(w io.Writer, rt *resultTable, comma rune) error {
	cw := csv.NewWriter(w)
	cw.Comma = comma

	rec := make([]string, len(rt.columns))
	for i, c := range rt.columns {
		rec[i] = c.name
	}
	if err := cw.Write(rec); err != nil {
		return err
	}
	for _, row := range rt.rows {
		for i := range rec {
			rec[i] = ""
			if v := rt.value(row, i); v != nil {
				rec[i] = *v
			}
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func writeJSON(w io.Writer, rt *resultTable) error {
	bw := bufio.NewWriter(w)
	if len(rt.rows) == 0 {
		if _, err := bw.WriteString("[]\n"); err != nil {
			return err
		}
		return bw.Flush()
	}
	if _, err := bw.WriteString("[\n"); err != nil {
		return err
	}
	for i, row := range rt.rows {
		if i > 0 {
			if _, err := bw.WriteString(",\n"); err != nil {
				return err
			}
		}
		if _, err := bw.WriteString("  "); err != nil {
			return err
		}
		if err := encodeRowObject(bw, rt, row); err != nil {
			return err
		}
	}
	if _, err := bw.WriteString("\n]\n"); err != nil {
		return err
	}
	return bw.Flush()
}

func writeJSONL(w io.Writer, rt *resultTable) error {
	bw := bufio.NewWriter(w)
	for _, row := range rt.rows {
		if err := encodeRowObject(bw, rt, row); err != nil {
			return err
		}
		if err := bw.WriteByte('\n'); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// writeRaw prints the values without any decoration, which is what a single
// column result such as SHOW CREATE TABLE wants.
func writeRaw(w io.Writer, rt *resultTable) error {
	bw := bufio.NewWriter(w)
	for _, row := range rt.rows {
		parts := make([]string, 0, len(row))
		for i := range rt.columns {
			if v := rt.value(row, i); v != nil {
				parts = append(parts, *v)
			} else {
				parts = append(parts, "")
			}
		}
		if _, err := bw.WriteString(strings.Join(parts, "\t") + "\n"); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// encodeRowObject writes one JSON object, keeping the column order that maps
// cannot preserve.
func encodeRowObject(bw *bufio.Writer, rt *resultTable, row []*string) error {
	if err := bw.WriteByte('{'); err != nil {
		return err
	}
	for i, c := range rt.columns {
		if i > 0 {
			if err := bw.WriteByte(','); err != nil {
				return err
			}
		}
		key, err := marshalJSON(c.name)
		if err != nil {
			return err
		}
		if _, err := bw.Write(key); err != nil {
			return err
		}
		if err := bw.WriteByte(':'); err != nil {
			return err
		}
		val, err := marshalJSON(jsonValue(rt.value(row, i), c.typ))
		if err != nil {
			return err
		}
		if _, err := bw.Write(val); err != nil {
			return err
		}
	}
	return bw.WriteByte('}')
}

// jsonValue converts an Athena value into a Go value using the column type, so
// numbers and booleans are not quoted in the JSON output. Anything that does
// not parse stays a string.
func jsonValue(v *string, typ string) any {
	if v == nil {
		return nil
	}
	switch typ {
	case "boolean":
		if b, err := strconv.ParseBool(*v); err == nil {
			return b
		}
	case "tinyint", "smallint", "integer", "int", "bigint":
		if n, err := strconv.ParseInt(*v, 10, 64); err == nil {
			return n
		}
	case "float", "real", "double":
		if f, err := strconv.ParseFloat(*v, 64); err == nil {
			return f
		}
	}
	return *v
}

func marshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func isNumericType(typ string) bool {
	switch typ {
	case "tinyint", "smallint", "integer", "int", "bigint", "float", "real", "double", "decimal":
		return true
	}
	return false
}

func writeTable(w io.Writer, rt *resultTable, maxWidth int) error {
	headers := make([]string, len(rt.columns))
	rightAlign := make([]bool, len(rt.columns))
	for i, c := range rt.columns {
		headers[i] = c.name
		rightAlign[i] = isNumericType(c.typ)
	}
	rows := make([][]string, 0, len(rt.rows))
	for _, row := range rt.rows {
		cells := make([]string, len(rt.columns))
		for i := range rt.columns {
			if v := rt.value(row, i); v != nil {
				cells[i] = *v
			} else {
				cells[i] = "NULL"
			}
		}
		rows = append(rows, cells)
	}
	return renderTable(w, headers, rows, rightAlign, maxWidth)
}

const (
	tableGap         = "  "
	maxColumnWidth   = 120
	minShrunkenWidth = 8
)

// renderTable prints a fixed width table. Cells wider than the column are cut
// with an ellipsis, and columns are shrunk until the whole table fits maxWidth
// (zero means unlimited).
func renderTable(w io.Writer, headers []string, rows [][]string, rightAlign []bool, maxWidth int) error {
	if len(headers) == 0 {
		return nil
	}
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = runewidth.StringWidth(h)
	}
	for _, row := range rows {
		for i := range headers {
			if i >= len(row) {
				continue
			}
			if n := runewidth.StringWidth(sanitizeCell(row[i])); n > widths[i] {
				widths[i] = n
			}
		}
	}
	// A hard cap would cut identifiers that the reader needs in full, so it
	// only applies when the width to fit is unknown; otherwise the columns are
	// shrunk just enough to fit the screen.
	if maxWidth <= 0 {
		for i := range widths {
			if widths[i] > maxColumnWidth {
				widths[i] = maxColumnWidth
			}
		}
	}
	shrinkToWidth(widths, maxWidth)

	bw := bufio.NewWriter(w)
	writeRow := func(cells []string) error {
		var sb strings.Builder
		for i := range headers {
			cell := ""
			if i < len(cells) {
				cell = sanitizeCell(cells[i])
			}
			if runewidth.StringWidth(cell) > widths[i] {
				cell = runewidth.Truncate(cell, widths[i], "…")
			}
			if i > 0 {
				sb.WriteString(tableGap)
			}
			if i == len(headers)-1 && (len(rightAlign) <= i || !rightAlign[i]) {
				// no trailing padding on the last left aligned column
				sb.WriteString(cell)
				continue
			}
			if len(rightAlign) > i && rightAlign[i] {
				sb.WriteString(runewidth.FillLeft(cell, widths[i]))
			} else {
				sb.WriteString(runewidth.FillRight(cell, widths[i]))
			}
		}
		_, err := bw.WriteString(strings.TrimRight(sb.String(), " ") + "\n")
		return err
	}

	if err := writeRow(headers); err != nil {
		return err
	}
	rule := make([]string, len(headers))
	for i := range rule {
		rule[i] = strings.Repeat("-", widths[i])
	}
	if err := writeRow(rule); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writeRow(row); err != nil {
			return err
		}
	}
	return bw.Flush()
}

func shrinkToWidth(widths []int, maxWidth int) {
	if maxWidth <= 0 {
		return
	}
	total := len(tableGap) * (len(widths) - 1)
	for _, w := range widths {
		total += w
	}
	for total > maxWidth {
		widest, idx := 0, -1
		for i, w := range widths {
			if w > widest && w > minShrunkenWidth {
				widest, idx = w, i
			}
		}
		if idx < 0 {
			return
		}
		widths[idx]--
		total--
	}
}

var cellReplacer = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ")

func sanitizeCell(s string) string {
	return cellReplacer.Replace(s)
}
