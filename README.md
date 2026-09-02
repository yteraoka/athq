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
| `ATHQ_VIM` | `--vim` | `1` (vi keys in the interactive editor) |

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

A partition key in the columns pane is marked `(partition)`, and when the
table uses [partition
projection](https://docs.aws.amazon.com/athena/latest/ug/partition-projection.html)
its configuration is summarized there too, e.g. `(partition: date yyyy/MM/dd)`
or `(partition: enum dev, staging, prod)` — read from the table's own
properties, so this costs nothing extra. A plainly Hive-partitioned table has
no such format on file; its columns just say `(partition)`, and the actual
value format can only be told from the real partition values.

| Key | |
| --- | --- |
| `tab` / `shift+tab` | move between the panes (while typing in the editor `tab` completes) |
| `↑` `↓` `←` `→` (or `k` `j` `h` `l`) | move around inside a pane |
| `enter` | expand a database, or insert the selected column |
| `i` | insert the selected name into the editor (`database.table` or a column) |
| `r` | reload the catalog |
| `ctrl+r` | run the query in the editor |
| `ctrl+s` | save the whole result; the format follows the extension typed |
| `ctrl+w` | save the query in the editor under a name and a description |
| `ctrl+o` | open a saved query |
| `esc` | leave the editor (in vim mode, stop inserting first) |
| `tab` (while typing in the editor) | complete the name before the cursor |
| `click`+drag (in the editor) | select text |
| `shift` + arrow keys (while typing in the editor) | select text without the mouse |
| `ctrl+y` (in the editor) | copy the selection to the system clipboard |
| `ctrl+v` or a terminal paste (in the editor) | paste from the system clipboard |
| `f2` | hand the mouse to the terminal, and take it back |
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
editor is left with `shift+tab`, or with `esc` once nothing is being typed.

The mouse works as well, so a name can go into the query without walking there
with `tab` and `i`: clicking a database opens or closes it, clicking a table
selects it and clicking the selected table inserts `database.table`, and
clicking a column inserts its name. Clicking the editor or the result moves the
focus there, and the wheel scrolls whatever the pointer is over.

#### The query editor is modal

The editor uses vi keys: it starts in **normal** mode, where the keys are
commands, and `i` (or `a`, `A`, `I`, `o`, `O`, `c`, `s`) starts **inserting**.
`esc` stops inserting; a second `esc` leaves the pane. The pane title says
which mode it is in — `query — NORMAL`, `query — INSERT`, `query — VISUAL`
— and the cursor stands still while commanding and blinks while typing.
`--vim=false` or `ATHQ_VIM=0` turns all of it off and leaves the plain text
editor of before, with its emacs style keys (`ctrl+a`, `ctrl+k`, `alt+f`, …),
which are also what insert mode still has.

| | |
| --- | --- |
| move | `h` `j` `k` `l`, `w` `W` `b` `B` `e` `E`, `0` `^` `$`, `{` `}`, `gg` `G`, `5G`, `ctrl+d` `ctrl+u` `ctrl+f` `ctrl+b`, and the arrow keys |
| insert | `i` `a` `I` `A` `o` `O`, `s` `S`, `c{motion}` `cc` `C` |
| delete | `x` `X`, `d{motion}` `dd` `D` |
| copy and paste | `y{motion}` `yy` `Y`, `p` `P` |
| select | `v` charwise, `V` whole lines, then `y` `d` `c` |
| the rest | `r{char}` replace one character, `J` join the lines, `u` undo, a count before any of them (`3dd`, `2w`) |

`ctrl+r` runs the query in every mode, so it is not vim's redo; `u` goes back
one change at a time, up to 200 of them. Tab completion belongs to insert
mode, where a name is being typed; in normal mode `tab` moves between the
panes like everywhere else.

#### Copying and pasting

**Copying.** `y` in normal or visual mode — or `ctrl+y`, which also copies a
selection made with the mouse or with `shift` and the arrow keys — puts the
text on the system clipboard two ways at once, because no single one works in
every terminal:

* as an [OSC 52](https://invisible-island.net/xterm/ctlseqs/ctlseqs.html)
  sequence, which the terminal itself acts on and which is the only way that
  survives an `ssh` session. Windows Terminal and Ghostty do it; Terminal.app
  ignores it.
* through a helper program on the machine athq runs on: `pbcopy`, `wl-copy`,
  `xclip`, `xsel`, or `clip.exe` under WSL. This is what covers Terminal.app
  and any terminal with OSC 52 turned off.

The status line says how much was copied, and mentions it when there was no
helper program to use. `ctrl+shift+c` is deliberately *not* the key: Windows
Terminal and Ghostty keep that combination for their own copy, and Terminal.app
cannot send it at all — it arrives as a bare `ctrl+c`, which would quit athq.

**Pasting.** However the terminal itself pastes works (`cmd+v`, `ctrl+shift+v`,
middle click): terminals send that as a bracketed paste, which athq puts into
the editor at the cursor, or beside it like `p` when the editor is not
inserting. `ctrl+v` does the same by reading the clipboard through the helper
program, for terminals that have no paste of their own.

**Selecting with the terminal instead.** athq holds the mouse while the TUI is
open, so dragging in the catalog, the columns or the result pane does not
select text the way it does in a shell. Either use the terminal's override
(`shift` in Windows Terminal and Ghostty, `fn` in Terminal.app) or press `f2`,
which hands the mouse back to the terminal until `f2` takes it again. In the
query editor athq does the selecting itself: click and drag selects, and the
selection carries on into visual mode, so `y` copies it.

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
