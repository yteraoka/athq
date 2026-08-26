package athq

import (
	"context"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/athena/types"
)

// The catalog is held in memory for the life of the TUI session: databases are
// listed once at start up, and a database's tables are fetched the first time
// it is expanded. ListTableMetadata returns the columns too, so one call per
// database is enough to describe all of its tables.

type tuiColumn struct {
	name      string
	typ       string
	comment   string
	partition bool
	// projection summarizes the partition's projection configuration, e.g.
	// "date yyyy/MM/dd" or "enum a, b, c". Empty when projection is not
	// enabled for the column, which is the common case for a plain
	// Hive-partitioned table with no format to show.
	projection string
}

type tuiTable struct {
	name    string
	columns []tuiColumn
}

type tuiDatabase struct {
	name     string
	tables   []tuiTable
	expanded bool
	loaded   bool
	loading  bool
	loadErr  string
}

// catalogRow is one visible line of the catalog pane. table is -1 on the line
// of a database itself.
type catalogRow struct {
	db    int
	table int
}

func (r catalogRow) isDatabase() bool { return r.table < 0 }

// catalogRows flattens the expanded tree into the lines to draw.
func catalogRows(databases []tuiDatabase) []catalogRow {
	rows := make([]catalogRow, 0, len(databases))
	for i, db := range databases {
		rows = append(rows, catalogRow{db: i, table: -1})
		if !db.expanded {
			continue
		}
		for j := range db.tables {
			rows = append(rows, catalogRow{db: i, table: j})
		}
	}
	return rows
}

type msgTUIDatabases struct {
	databases []string
	err       error
}

type msgTUITables struct {
	database string
	tables   []tuiTable
	err      error
}

func loadDatabasesCmd(ctx context.Context, c *clients) tea.Cmd {
	return func() tea.Msg {
		databases, err := listDatabases(ctx, c)
		if err != nil {
			return msgTUIDatabases{err: err}
		}
		names := make([]string, 0, len(databases))
		for _, db := range databases {
			names = append(names, aws.ToString(db.Name))
		}
		sort.Strings(names)
		return msgTUIDatabases{databases: names}
	}
}

func loadTablesCmd(ctx context.Context, c *clients, database string) tea.Cmd {
	return func() tea.Msg {
		metadata, err := listTables(ctx, c, database, "")
		if err != nil {
			return msgTUITables{database: database, err: err}
		}
		tables := make([]tuiTable, 0, len(metadata))
		for _, m := range metadata {
			tables = append(tables, tuiTable{
				name:    aws.ToString(m.Name),
				columns: toTUIColumns(m),
			})
		}
		sort.Slice(tables, func(i, j int) bool { return tables[i].name < tables[j].name })
		return msgTUITables{database: database, tables: tables}
	}
}

// toTUIColumns puts the partition keys after the ordinary columns, marked, the
// way SHOW CREATE TABLE presents them.
func toTUIColumns(m types.TableMetadata) []tuiColumn {
	columns := make([]tuiColumn, 0, len(m.Columns)+len(m.PartitionKeys))
	for _, c := range m.Columns {
		columns = append(columns, tuiColumn{
			name:    aws.ToString(c.Name),
			typ:     aws.ToString(c.Type),
			comment: aws.ToString(c.Comment),
		})
	}
	for _, c := range m.PartitionKeys {
		name := aws.ToString(c.Name)
		columns = append(columns, tuiColumn{
			name:       name,
			typ:        aws.ToString(c.Type),
			comment:    aws.ToString(c.Comment),
			partition:  true,
			projection: partitionProjection(m.Parameters, name),
		})
	}
	return columns
}

// partitionProjection reads a partition column's projection configuration
// (https://docs.aws.amazon.com/athena/latest/ug/partition-projection-supported-types.html)
// out of the table's parameters and summarizes it, e.g. "date yyyy/MM/dd".
// Most tables do not use projection, in which case this is empty and the
// partition's actual format can only be told from its real values.
func partitionProjection(params map[string]string, column string) string {
	if params["projection.enabled"] != "true" {
		return ""
	}
	prefix := "projection." + column + "."
	typ := params[prefix+"type"]
	if typ == "" {
		return ""
	}
	switch typ {
	case "date":
		if format := params[prefix+"format"]; format != "" {
			return "date " + format
		}
	case "integer":
		if r := params[prefix+"range"]; r != "" {
			return "integer " + strings.Replace(r, ",", "–", 1)
		}
	case "enum":
		if values := params[prefix+"values"]; values != "" {
			return "enum " + strings.ReplaceAll(values, ",", ", ")
		}
	}
	return typ
}
