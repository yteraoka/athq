package athq

import (
	"context"
	"sort"

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
		columns = append(columns, tuiColumn{
			name:      aws.ToString(c.Name),
			typ:       aws.ToString(c.Type),
			comment:   aws.ToString(c.Comment),
			partition: true,
		})
	}
	return columns
}
