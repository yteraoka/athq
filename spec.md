Amazon Athena に簡単にクエリして結果を確認するためのコマンドラインツール

## 設定

次の環境変数を参照します。すべてオプショナルです。

- `ATHQ_WORK_GROUP` (`--workgroup`, `--wg`)
- `ATHQ_DATABASE` (`--database`, `--db`)
- `ATHQ_OUTPUT_LOCATION` (`--output-location`)
- `ATHQ_CATALOG` (`--catalog`)
- `ATHQ_PRICE_PER_TB` (スキャン量からの概算コスト計算に使う TB 単価、既定 5.0)
- `ATHQ_EDITOR` (未設定なら `$VISUAL`、`$EDITOR`、`vi` の順)
- `ATHQ_HISTORY_FILE` (既定はユーザーキャッシュディレクトリの `athq/history.jsonl`)

フラグ > 環境変数 > 既定値 の優先順で解決します。

`ATHQ_WORK_GROUP` が未指定の場合はデフォルトの `primary` という work group を使います。
`ATHQ_OUTPUT_LOCATION` が未指定の場合は work group で指定されている location を使います。
`ATHQ_CATALOG` が未指定の場合は `AwsDataCatalog` を使います。

AWS の認証情報とリージョンは通常の解決順 (環境変数、`~/.aws/config`、インスタンスメタデータ)
に従います。`--region` と `--profile` で上書きできます。

## q, query コマンド

クエリの指定方法は次の 4 つで、同時に複数を指定するとエラーになります。

- コマンドライン引数
- `-f, --file <path>` (`-` で標準入力)
- `-e, --editor` — その場でエディタを起動し、保存して終了したらその内容を実行する
- パイプされた標準入力

`-o, --output <path>` で結果を保存するファイルの path を指定します。
省略した場合は整形したテーブルを標準出力に表示します (`--max-rows`、既定 100 行)。

出力形式は拡張子で自動判定し、`--format` で明示指定できます。

| 拡張子 | 形式 | 取得方法 |
| --- | --- | --- |
| `.csv` (および不明な拡張子) | CSV | Athena が S3 に書いた結果オブジェクトをそのままダウンロード |
| `.tsv` | TSV | `GetQueryResults` |
| `.json` | JSON 配列 | `GetQueryResults` |
| `.jsonl`, `.ndjson` | 1 行 1 オブジェクト | `GetQueryResults` |
| `.txt` | Athena が書いたファイルをそのまま | S3 からダウンロード |

JSON では列の型 (`bigint`, `double`, `boolean` など) に従って数値・真偽値をクォートせずに
出力し、NULL は `null` にします。

その他の挙動:

- 実行中は状態 (QUEUED/RUNNING) と経過時間を stderr に表示します (stderr が端末のときだけ)
- 完了時にスキャン量・実行時間・概算コスト・実行 ID を stderr に表示します (`--quiet` で抑止)
- Ctrl-C で `StopQueryExecution` を呼び、Athena 側の実行も止めます
- 実行のたびに履歴を JSONL に記録します (最大 1000 件、パーミッション 0600)
- `--id <QueryExecutionId>` で既存の実行の結果を取得します
- `--last` で直近に成功した実行の結果を取得します

### TUI モード (`-t`, `--tui`)

table の定義を見ながらクエリを書くための画面を開きます。
引数・`-f`・`-e`・標準入力でクエリを渡すとエディタの初期内容になります (省略可)。

画面は 3 ペイン構成です。

- 左上: database とその table のツリー
- 右上: 選択中の table のカラム定義 (パーティションキーは色を変えて `(partition)` 表示)
- 中央: クエリエディタ
- 下: 実行結果 (スクロール可能)
- 最下段: ステータス行とキーヘルプ

| キー | 動作 |
| --- | --- |
| `tab` / `shift+tab` | ペイン移動 |
| `↑` `↓` `←` `→` (`k` `j` `h` `l`) | ペイン内の移動 |
| `enter` | database を展開 / カラムを挿入 |
| `i` | 選択中の名前をエディタに挿入 (`database.table` またはカラム名) |
| `r` | カタログを再取得 |
| `ctrl+r` | エディタの内容を実行 |
| `ctrl+s` | 結果をファイルに保存 (path を入力、拡張子で形式判定、全行) |
| `esc` | エディタから抜ける |
| `ctrl+c` | 実行中ならクエリを中止、そうでなければ終了 |
| `q` | 終了 (エディタ以外) |

- カタログはメタデータ API で取得するのでクエリは実行されず課金もありません。
  database は展開したときに初めて table を取得し、セッション中はメモリに保持します。
- 結果ペインには `--max-rows` 行 (既定 100、0 ですべて) まで表示します。保存時は常に全行です。
- 実行中はステータス行に経過時間、完了後は行数・スキャン量・概算コストを表示します。
- クエリが失敗したときは、エラー全文を折り返して結果ペインに表示します (実行 ID 付き、
  スクロール可能)。ステータス行は 1 行しかないため先頭しか表示できないためです。
  同じ内容は後から `athq q --id <実行ID>` でも確認できます。

## hist, history コマンド

実行履歴を新しい順に表示します。`-n` で件数 (既定 20、0 ですべて)、
`--full` で 1 件ずつブロック形式でクエリ全文を表示します。

## wg, workgroup コマンド

- `list` で work group のリストを出力します
- `desc`, `describe [NAME]` で work group の詳細を出力します
  - NAME を省略すると使用中の work group を表示します

## db, database コマンド

- `list` でデータベースのリストを出力します
  - `ListDatabases` API を使うのでクエリは実行されず課金もありません

## tbl, table コマンド

- `desc`, `describe [DATABASE.]TABLE` で table の DDL を出力します
  - `db.table` で指定するか `ATHQ_DATABASE` を参照します
  - 既定では `SHOW CREATE TABLE` を実行して Athena 自身の DDL を出力します
  - `--metadata` を付けると `GetTableMetadata` API で列・パーティションキー・
    パラメータを表示します (クエリは実行されません)
- `list [PATTERN]` でデータベース内の table のリストを出力します
  - PATTERN は table 名の全体に対するマッチで `*` がワイルドカードです (`log*` など)
  - ワイルドカードを含まない場合は部分一致として扱います (`*PATTERN*` に展開)
