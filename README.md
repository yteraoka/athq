# athq

A small command line tool to query Amazon Athena and look at the result.

- Run a query from an argument, a file, `$EDITOR` or stdin
- Show the result as a table on stdout, or save it as CSV / TSV / JSON / JSONL
- See how much data was scanned and roughly what it costs
- Ctrl-C stops the query on the Athena side too
- List work groups and databases, and print the DDL of a table
- `--tui` opens an interactive browser: write the query with the table
  definitions next to it

## Install

```sh
go install github.com/yteraoka/athq/cmd/athq@latest
```

Or download a binary from the [releases](https://github.com/yteraoka/athq/releases).

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
athq q --tui                       # or -t
athq q --tui "SELECT * FROM events"   # start with a query in the editor
athq q --tui -f query.sql
```

The screen has the databases and their tables on the top left, the columns of
the selected table on the top right, the query editor in the middle and the
result at the bottom, so the definition stays visible while the query is
written.

| Key | |
| --- | --- |
| `tab` / `shift+tab` | move between the panes |
| `↑` `↓` `←` `→` (or `k` `j` `h` `l`) | move around inside a pane |
| `enter` | expand a database, or insert the selected column |
| `i` | insert the selected name into the editor (`database.table` or a column) |
| `r` | reload the catalog |
| `ctrl+r` | run the query in the editor |
| `ctrl+s` | save the whole result; the format follows the extension typed |
| `esc` | leave the editor |
| `ctrl+c` | stop a running query, or quit |
| `q` | quit (outside the editor) |

The catalog is read with the metadata API, so browsing it runs no query and
costs nothing. The result pane shows the first `--max-rows` rows (100 by
default, `0` for all); saving always writes every row.

### Work groups, databases and tables

```sh
athq wg list
athq wg desc                 # the work group in use
athq wg desc analytics

athq db list

athq tbl list                # needs --db or ATHQ_DATABASE
athq tbl desc events         # SHOW CREATE TABLE
athq tbl desc mydb.events
athq tbl desc events --metadata   # catalog metadata, no query is run
```

## License

MIT
