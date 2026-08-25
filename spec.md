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
- `ATHQ_SAVED_QUERIES_FILE` (既定は `~/.config/athq/queries.jsonl`、
  `XDG_CONFIG_HOME` が設定されていればその下の `athq/queries.jsonl`)

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

- 最上段: 使用中の work group と結果の出力先 (S3 の location)
- 左上: database とその table のツリー
- 右上: 選択中の table のカラム定義 (パーティションキーは色を変えて `(partition)` 表示)
- 中央: クエリエディタ
- 下: 実行結果 (スクロール可能)
- 最下段: ステータス行とキーヘルプ

| キー | 動作 |
| --- | --- |
| `tab` / `shift+tab` | ペイン移動 (エディタでは `tab` は補完) |
| `↑` `↓` `←` `→` (`k` `j` `h` `l`) | ペイン内の移動 |
| `enter` | database を展開 / カラムを挿入 |
| `i` | 選択中の名前をエディタに挿入 (`database.table` またはカラム名) |
| `r` | カタログを再取得 |
| `ctrl+r` | エディタの内容を実行 |
| `ctrl+s` | 結果をファイルに保存 (path を入力、拡張子で形式判定、全行) |
| `ctrl+w` | エディタのクエリに name と description を付けて保存 |
| `ctrl+o` | 保存済みクエリを一覧から開く |
| `esc` | エディタから抜ける |
| `tab` (エディタ) | カーソル直前の名前を補完 |
| `ctrl+c` | 実行中ならクエリを中止、そうでなければ終了 |
| `q` | 終了 (エディタ以外) |

- ヘッダには起動時に `GetWorkGroup` で取得した情報を表示します。出力先は
  `--output-location` (`ATHQ_OUTPUT_LOCATION`) があればそちらを、なければ
  work group の設定を表示し、上書きしている場合はその旨を括弧で添えます。
  work group が `EnforceWorkGroupConfiguration` の場合は指定が無視されるため、
  work group 側の location を「指定は無視される」と添えて表示します。
  work group を取得できなかった場合は出力先を unknown と表示し、
  理由をステータス行に出します (クエリの実行は可能です)。
- エディタにフォーカスがあるときの `tab` は、カーソル直前の語をカタログから補完します。
  補完候補はすでにメモリにあるカタログから作るので API は呼ばず、クエリも実行されません。
  エディタから抜けるには `esc` (カタログへ) か `shift+tab` を使います。
  - 修飾なしの語は database 名・取得済み database の table 名・選択中 table の
    カラム名から探します
  - `db.` はその database の table 名、`db.tbl.` はその table のカラム名を候補にします
  - まだ table を取得していない database の候補は出ません (補完のために取得はしません)
  - 候補が 0 件なら何もしません (タブ文字は挿入しません)
  - 候補が複数あるときは共通の接頭辞まで補完してステータス行に候補を並べ
    (入りきらない分は省略)、続けて `tab` を押すと候補を順に挿入して切り替えます
  - 大文字・小文字は区別せずに前方一致し、挿入されるのはカタログ側の表記です
- マウス操作にも対応します (画面を開いている間はマウスを athq が受け取るため、
  端末側のテキスト選択には `shift` が必要になります)。

| 操作 | 動作 |
| --- | --- |
| database をクリック | 展開 / 折りたたみ |
| table をクリック | 選択 (カラム定義を表示)、選択済みの table を再度クリックでエディタに `database.table` を挿入 |
| カラムをクリック | カラム名をエディタに挿入 |
| エディタ・結果ペインをクリック | そのペインにフォーカス |
| ホイール | ポインタが乗っているペインをスクロール |
| 保存済みクエリ一覧でクリック | 選択、選択済みの行を再度クリックで開く |

- カタログはメタデータ API で取得するのでクエリは実行されず課金もありません。
  database は展開したときに初めて table を取得し、セッション中はメモリに保持します。
- 結果ペインには `--max-rows` 行 (既定 100、0 ですべて) まで表示します。保存時は常に全行です。
- 実行中はステータス行に経過時間、完了後は行数・スキャン量・概算コストを表示します。
- `ctrl+w` は name、続いて description を入力させ、保存日時とクエリを合わせて
  `~/.config/athq/queries.jsonl` に JSONL 形式で name 順に保存します
  (パーミッション 0600)。同じ name で保存すると上書きします。
- `ctrl+o` は保存済みクエリの一覧を画面全体に表示します。上段のリストに
  name・保存日時・description を並べ、下段にカーソル位置のエントリ全体
  (description、保存日時、クエリ本文) を表示するので、内容を確認してから
  `enter` でエディタに読み込めます。`esc` で何もせずに戻ります。
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
