# athq

A small command line tool to query Amazon Athena and look at the result.

- Run a query from an argument, a file, `$EDITOR` or stdin
- Show the result as a table on stdout, or save it as CSV / TSV / JSON / JSONL
- See how much data was scanned and roughly what it costs
- Ctrl-C stops the query on the Athena side too
- List work groups and databases, and print the DDL of a table
- Run with no subcommand (or `athq query --tui`) to open an interactive
  browser: write the query with the table definitions next to it

## Installation

### Homebrew

```bash
brew tap yteraoka/cask
brew trust yteraoka/cask
brew install --cask athq
```

### mise

```bash
mise use -g github:yteraoka/athq
```

Pin a version instead of following the latest release:

```bash
mise use -g github:yteraoka/athq@0.1.0
```

### Go

```sh
go install github.com/yteraoka/athq/cmd/athq@latest
```

Or build from a checkout:

```sh
go build -o athq ./cmd/athq
```

Binaries for each release are also on the
[releases page](https://github.com/yteraoka/athq/releases).

## Configuration

AWS credentials and the region are resolved the usual way (environment
variables, `~/.aws/config`, instance metadata, ...). `--region` and `--profile`
override them.

Everything else is optional:

| Environment variable | Flag | Default |
| --- | --- | --- |
| `ATHQ_WORK_GROUP` | `--workgroup`, `--wg` | `primary` |
| `ATHQ_DATABASE` | `--database`, `--db` | none |
| `ATHQ_OUTPUT_LOCATION` | `--output-location` | the work group setting |
| `ATHQ_CATALOG` | `--catalog` | `AwsDataCatalog` |
| `ATHQ_PRICE_PER_TB` | – | `5.0` (for the cost estimate) |
| `ATHQ_EDITOR` | – | `$VISUAL`, `$EDITOR`, then `vi` |
| `ATHQ_HISTORY_FILE` | – | `<user cache dir>/athq/history.jsonl` |
| `ATHQ_SAVED_QUERIES_FILE` | – | `~/.config/athq/queries.jsonl` |

A flag beats the environment variable, which beats the default.

## Usage

### Queries

```sh
# print the result as a table (the first 100 rows)
athq q "SELECT * FROM logs LIMIT 10"

# the format follows the extension of the output file
athq q "SELECT * FROM logs" -o result.csv
athq q "SELECT * FROM logs" -o result.json
athq q "SELECT * FROM logs" -o result.tsv --db mydb

# read the query from a file, from $EDITOR or from stdin
athq q -f query.sql
athq q -e
echo "SELECT 1" | athq q

# fetch the result of an earlier execution again
athq history
athq q --last -o result.json
athq q --id 12345678-1234-1234-1234-123456789012
```

`.csv` is handed over straight from the CSV that Athena wrote to S3, so quoting
is preserved exactly and large results do not have to be paged through the API.
The other formats are built from `GetQueryResults`, which is also where the
column types come from — numbers and booleans are unquoted in JSON.

While the query runs, its state and elapsed time are shown on stderr, and when
it finishes so are the scanned bytes and a rough cost:

```
athq: SUCCEEDED in 2.3s, scanned 12.40 MiB (~$0.0001), execution id 1234abcd-...
```

Press Ctrl-C to stop a query; `StopQueryExecution` is called so it stops
running on the Athena side as well.

### Interactive mode

```sh
athq                                   # opens with an empty editor
athq -f query.sql                      # opens with the file's query
athq q --tui                           # or -t
athq q --tui "SELECT * FROM events"    # start with a query in the editor
athq q --tui -f query.sql
```

Running `athq` with no subcommand opens the same interactive browser as
`athq q --tui`. `-f`/`--file` (`-` for stdin) preloads the editor with a
query from a file, but unlike `athq q --tui` the query cannot be given as a
plain argument — `athq "SELECT ..."` is rejected as an unknown command, so
use `athq q --tui "SELECT ..."` for that instead.

The screen has the databases and their tables on the top left, the columns of
the selected table on the top right, the query editor in the middle and the
result at the bottom, so the definition stays visible while the query is
written. A header above them names the work group the queries run in and the
S3 location their results are written to:

```
work group: primary   output: s3://aws-athena-query-results-…/
```

The location is read from the work group unless `--output-location` (or
`ATHQ_OUTPUT_LOCATION`) sets one, and the header says so when it does. A work
group that enforces its own configuration ignores that override, and the
header says that too.

| Key | |
| --- | --- |
| `tab` / `shift+tab` | move between the panes (in the editor `tab` completes) |
| `↑` `↓` `←` `→` (or `k` `j` `h` `l`) | move around inside a pane |
| `enter` | expand a database, or insert the selected column |
| `i` | insert the selected name into the editor (`database.table` or a column) |
| `r` | reload the catalog |
| `ctrl+r` | run the query in the editor |
| `ctrl+s` | save the whole result; the format follows the extension typed |
| `ctrl+w` | save the query in the editor under a name and a description |
| `ctrl+o` | open a saved query |
| `esc` | leave the editor |
| `tab` (in the editor) | complete the name before the cursor |
| `click`+drag (in the editor) | select text |
| `shift` + arrow keys (in the editor) | select text without the mouse |
| `ctrl+shift+c` (in the editor) | copy the selection to the clipboard |
| `ctrl+v` or a terminal paste (in the editor) | paste from the clipboard |
| `ctrl+c` | stop a running query, or quit (works in the editor too) |
| `q` | quit (outside the editor, since `q` is a character there) |

In the editor `tab` completes the name being typed from the catalog that is
already on screen, so no API is called and no query is run. A bare word is
matched against the database names, the tables of the databases that have been
opened and the columns of the table being looked at; `analytics.` offers the
tables of that database and `analytics.events.` its columns. A database whose
tables have not been fetched yet has nothing to offer, and a word with no
candidate leaves the editor alone — no tab character is inserted. When several
names match, the first `tab` fills in as much as they all share and lists them
on the status line, and the ones after it insert each candidate in turn. The
editor is still left with `esc` or `shift+tab`.

The mouse works as well, so a name can go into the query without walking there
with `tab` and `i`: clicking a database opens or closes it, clicking a table
selects it and clicking the selected table inserts `database.table`, and
clicking a column inserts its name. Clicking the editor or the result moves the
focus there, and the wheel scrolls whatever the pointer is over.

In the query editor, clicking and dragging selects text directly, and
`ctrl+shift+c` copies the selection to the system clipboard; `shift` with the
arrow keys selects without the mouse at all. Pasting works both with `ctrl+v`
and with however the terminal itself pastes (e.g. cmd+v on a Mac); most
terminals, Ghostty included, send that as a bracketed paste rather than a
literal ctrl+v, and both are handled. Elsewhere — the catalog, the columns and
the result — athq still takes the mouse while the TUI is open, so selecting
text there needs the `shift` key to fall back to the terminal's own selection,
the way it does in other full screen terminal programs.

The catalog is read with the metadata API, so browsing it runs no query and
costs nothing. The result pane shows the first `--max-rows` rows (100 by
default, `0` for all); saving always writes every row.

`ctrl+w` puts the query aside: it asks for a name, then for a description, and
writes both together with the query and the time into
`~/.config/athq/queries.jsonl` (`$XDG_CONFIG_HOME/athq` when that is set).
Saving under a name that is already there replaces it.

`ctrl+o` lists what has been saved. The list shows the name, when it was saved
and the description, with the whole entry — including the query itself — below
it, so it can be read before `enter` puts it into the editor. `esc` leaves the
list without touching the editor.

When a query fails, the whole message goes into the result pane, wrapped and
scrollable, together with the execution id — the status line at the bottom has
room for its first line only. The same message can be printed again later with
`athq q --id <execution id>`.

### Work groups, databases and tables

```sh
athq wg list
athq wg desc                 # the work group in use
athq wg desc analytics

athq db list

athq tbl list                # needs --db or ATHQ_DATABASE
athq tbl list cloudtrail     # tables whose name contains cloudtrail
athq tbl list 'log*'         # * is the wildcard, matched against the whole name
athq tbl desc events         # SHOW CREATE TABLE
athq tbl desc mydb.events
athq tbl desc events --metadata   # catalog metadata, no query is run
```

## License

MIT
