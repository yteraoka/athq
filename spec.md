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
- `ATHQ_VIM` (`--vim`、既定 off (TUI のエディタは素の textarea)。
  `1`/`true`/`yes`/`on` で vi キーバインドにする)

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
| `ctrl+e` | クエリを `$EDITOR` で編集し、保存内容で置き換える |
| `esc` | エディタから抜ける (vim モードでは insert を抜けてからもう一度) |
| `tab` (エディタで入力中) | カーソル直前の名前を補完 |
| `ctrl+y` (エディタ) | 選択範囲をシステムのクリップボードにコピー |
| `ctrl+v` / 端末のペースト (エディタ) | クリップボードから貼り付け |
| `f2` | マウスを端末に返す / 取り戻す |
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
- マウス操作にも対応します。画面を開いている間はマウスを athq が受け取るため、
  カタログ・カラム・結果ペインで端末自身のテキスト選択をするには修飾キー
  (Windows Terminal と Ghostty は `shift`、Terminal.app は `fn`) を押すか、
  `f2` でマウスを端末に返します (`f2` でまた取り戻します)。

#### エディタのモード (vim キーバインド、オプトイン)

既定ではエディタは emacs 風のキー (`ctrl+a`、`ctrl+k`、`alt+f` など) で操作する
素の textarea です。`ctrl+e` で大きな編集を `$EDITOR` に任せられるので、これを
既定の挙動としています。`--vim` (`ATHQ_VIM=1`) を指定すると vi と同じモード式に
なります。normal モードで開き、`i` (や `a` `A` `I` `o` `O` `c` `s`) で insert に
入り、`esc` で normal に戻ります。normal モードでの `esc` はペインから抜けます。
どのモードかはペインのタイトル (`query — NORMAL` / `INSERT` / `VISUAL` /
`V-LINE`) と、カーソルが点滅するかどうかで分かります。insert モードでも
emacs 風のキーはそのまま使えます。

| 分類 | キー |
| --- | --- |
| 移動 | `h` `j` `k` `l`、`w` `W` `b` `B` `e` `E`、`0` `^` `$`、`{` `}`、`gg` `G`、`5G`、`ctrl+d` `ctrl+u` `ctrl+f` `ctrl+b`、矢印キー |
| 挿入 | `i` `a` `I` `A` `o` `O`、`s` `S`、`c{motion}` `cc` `C` |
| 削除 | `x` `X`、`d{motion}` `dd` `D` |
| コピー / 貼り付け | `y{motion}` `yy` `Y`、`p` `P` |
| 選択 | `v` (文字単位)、`V` (行単位)、続けて `y` `d` `c` |
| その他 | `r{char}`、`J`、`u` (undo、200 回まで)、数値プレフィックス (`3dd`、`2w`) |

`ctrl+r` はどのモードでもクエリの実行なので、vim の redo には割り当てません。
補完の `tab` は insert モード専用で、normal モードの `tab` は他のペインと同じく
ペイン移動です。

#### `$EDITOR` への退避

上表より先 (検索、マーク、マクロ、`:` コマンドなど) はわざと実装していません。
`ctrl+e` がその代わりです。クエリを一時ファイル (`.sql`) に書き出し、athq を
中断して `$EDITOR` (`-e`/`--editor` と同じ解決順: `ATHQ_EDITOR`、`$VISUAL`、
`$EDITOR`、`vi`) を起動し、終了したら保存内容を読み込んで TUI に戻ります。
戻った直後は vi がファイルを開いたときと同じく normal モード・カーソルは
先頭です。`ctrl+r` と同様どのペインからでも使え、置き換え自体も 1 つの変更
として扱うので vim モードでは `u` で元に戻せます。エディタが失敗して終了した
場合 (vim の `:cq` など) はクエリを変更しません。

#### コピーとペースト

コピー (`y`、`ctrl+y`) は 2 つの経路に同時に送ります。どちらか一方だけでは
すべての端末をカバーできないためです。

- OSC 52 (端末自身が処理するエスケープシーケンス)。ssh 越しでも効く唯一の方法で、
  Windows Terminal と Ghostty は対応、Terminal.app は無視します。
- ローカルの外部コマンド (`pbcopy`、`wl-copy`、`xclip`、`xsel`、WSL では
  `clip.exe`)。Terminal.app や OSC 52 を無効にしている端末はこちらで拾えます。

コピーした文字数はステータス行に出し、外部コマンドが見つからなかった場合は
その旨も添えます。`ctrl+shift+c` は使いません: Windows Terminal と Ghostty は
その組み合わせを端末側のコピーとして奪い、Terminal.app はそもそも送信できず
`ctrl+c` になって athq が終了してしまうためです。

ペーストは端末自身のペースト (`cmd+v`、`ctrl+shift+v`、middle click) がそのまま
使えます (bracketed paste として届きます)。insert 中はカーソル位置に、normal
モードでは `p` と同じくカーソルの隣に入れます。`ctrl+v` は上記の外部コマンドで
クリップボードを読んで同じことをします。

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
