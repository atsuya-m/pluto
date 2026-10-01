# pluto

Protobuf RPC API を **人間は REPL で**、**AI / script は決定的な CLI で** 探索・実行するためのツール。

```text
REPL for humans.
Deterministic CLI primitives for agents.
```

- proto ファイル、または gRPC server reflection からスキーマを読み込む
- Connect / gRPC / gRPC-Web の unary・server streaming・client streaming・bidi streaming を呼べる
- REPL では evans のような補完と、フィールドを上下に移動して編集できる request editor を使える
- CLI は ANSI なし・キー順固定の JSON / NDJSON を出力するので、AI エージェントやスクリプトからそのまま扱える

```text
$ pluto -r -t https://demo.connectrpc.com repl
pluto  target https://demo.connectrpc.com (connect) • schema reflection • type `help` for commands
pluto> Say

Say  ›  SayRequest

❯ sentence  string  ""

↑/k/C-p up • ↓/j/C-n down • enter edit/open • x/d unset/delete • e edit as json • C-s preview/send • esc/C-g back
```

## 目次

- [インストール](#インストール)
- [クイックスタート](#クイックスタート)
- [REPL](#repl)
- [CLI（AI / script 向け）](#cliai--script-向け)
- [設定ファイルとプロファイル](#設定ファイルとプロファイル)
- [スキーマの読み込み](#スキーマの読み込み)
- [接続](#接続)
- [グローバルフラグ](#グローバルフラグ)
- [開発](#開発)
- [制限事項](#制限事項)

## インストール

Go 1.27 以上が必要。

```bash
go install github.com/atsuya-m/pluto/cmd/pluto@latest
# mise を使う場合
mise use go:github.com/atsuya-m/pluto/cmd/pluto@latest
```

ソースからビルドする場合:

```bash
git clone https://github.com/atsuya-m/pluto.git
cd pluto
make build          # bin/pluto と bin/userserver (サンプルサーバー) ができる
```

## クイックスタート

### 公開デモに繋ぐ

[Connect](https://connectrpc.com) 公式の公開デモ `demo.connectrpc.com` (ElizaService) は reflection を公開しているので、proto ファイルなしで試せる。

```bash
./bin/pluto -r -t https://demo.connectrpc.com repl
```

```text
pluto> Say                  # enter で sentence を編集 → C-s でプレビュー → enter で送信
pluto> Introduce            # server streaming
pluto> Converse             # bidi streaming
```

入力した文は外部の公開サーバーに送られるので、個人情報や機密情報は入れないこと。

### 同梱のサンプルサーバーで試す

`testdata/proto` の `UserService` / `AdminService` を実装したサーバーを同梱している。unary / 各種 streaming / reflection にすべて対応している。

```bash
make server         # localhost:8080 で起動 (connect / grpc / grpc-web / reflection)
make repl           # 別ターミナルで REPL を起動 (--schema testdata/proto)
```

| RPC | 種類 | 内容 |
|---|---|---|
| `CreateUser` / `GetUser` / `ListUsers` / `UpdateUser` / `DeleteUser` | unary | インメモリのユーザー CRUD |
| `WatchUsers` | server streaming | 既存ユーザーを流したあと、作成・更新されたユーザーを流し続ける |
| `ImportUsers` | client streaming | 送られた request の数だけユーザーを作り、件数を返す |
| `Chat` | bidi streaming | 送ったテキストをエコーする |
| `admin.v1.AdminService.GetUser` / `BanUser` | unary | `GetUser` が曖昧になる例として同名の RPC を持つ |

## REPL

```bash
pluto --schema ./proto --target http://localhost:8080 repl
```

AltScreen は使わず、コマンドの結果・送信した request・response は通常のターミナル出力としてスクロールバックに残る（[ADR 0001](docs/adr/0001-repl-inline-rendering.md)）。

### コマンド

| コマンド | 内容 |
|---|---|
| `<rpc>` | `call <rpc>` と同じ |
| `call [rpc]` | request editor を開く。引数なしなら RPC 一覧から選ぶ |
| `rpcs [service]` | RPC を一覧から選ぶ（`/` で絞り込み）。service を指定するとその RPC を一覧表示する |
| `desc [rpc\|message] <name>` | RPC / message の定義を表示する |
| `services` | service の一覧 |
| `edit` | 直前の request editor に戻る |
| `header` | 現在の request header を表示する（機微な値は伏せ字） |
| `header set\|add <Key> <value>` | request header を設定 / 追加する。以降のすべての呼び出しに付く |
| `header rm <Key>` / `header clear` | request header を削除する |
| `reload` | RPC 一覧を読み直す（補完用） |
| `clear` / `help` / `exit` | 画面クリア / ヘルプ / 終了（`C-d`、`C-c` でも終了） |

RPC 名は `CreateUser`、`UserService.CreateUser`、`user.v1.UserService.CreateUser`、`user.v1.UserService/CreateUser` のどれでも指定できる。候補が複数ある場合は勝手に選ばず、候補一覧を表示する。

### 入力と補完

- 入力中は補完候補がポップアップする。先頭では コマンドと RPC、`call ` の後では RPC、`rpcs ` の後では service、`header rm ` の後では header 名が候補になる
- `tab` / `shift+tab` で候補を選ぶ。候補が1つなら `tab` で確定する。`esc` / `C-g` でポップアップを閉じる
- 候補を選んでいないときの `↑` / `↓` / `C-p` / `C-n` は履歴移動
- 入力欄は emacs キーバインド（`C-a` `C-e` `C-b` `C-f` `C-k` `C-u` `C-w` `C-d` `M-b` `M-f` `M-d`）で編集できる

### Request editor

```text
CreateUser  ›  CreateUserRequest

  name       string               "Taro"
❯ nickname   optional string      <unset>
  age        optional int32       20
  status     UserStatus           USER_STATUS_ACTIVE
  tags       repeated string      [2 items] ["go","grpc"]
  profile    Profile              {...}
  labels     map<string,string>   {1 entries} {"env":"dev"}
  contact    oneof                email: "taro@example.com"
```

| キー | 操作 |
|---|---|
| `↑` `↓` / `k` `j` / `C-p` `C-n` | 移動 |
| `g` `G` / `M-<` `M->` / `C-v` `M-v` | 先頭 / 末尾 / ページ送り |
| `enter` / `l` / `C-f` | 編集する。message・repeated・map なら中に入る。enum・bool・oneof は選択肢から選ぶ |
| `x` / `d` / `C-d` | unset する（repeated / map の中では要素を削除） |
| `a` | repeated / map に要素を追加する |
| `K` / `J` | repeated の要素を上下に入れ替える |
| `e` | そのフィールドを JSON で直接編集する（空にすると unset） |
| `C-s` | プレビュー（streaming RPC では送信） |
| `esc` / `h` / `C-b` / `C-g` | 親の message に戻る。最上位ではコマンド入力に戻る |

テキスト入力中は `enter` で確定、`esc` / `C-g` でキャンセル。不正な値はエラーを表示したまま入力を続けられる。

値の表示:

- `<unset>` は presence のあるフィールド（`optional`、message、oneof のメンバー）が未設定であることを示す。`optional string` に空文字を入れると `""` と表示され、unset とは区別される
- presence のないフィールドは既定値を薄い色で表示する
- repeated は要素の一覧と `+ Add` を、map はキー順のエントリと `+ Add` を表示する。map に追加するときはキー → 値の順に入力し、キーは型に合わせて検証される
- oneof は1行にまとめて表示し、`enter` でメンバーを選ぶとそのメンバーの編集に進む。別のメンバーを設定すると既存の値は protobuf の仕様どおりクリアされる
- `google.protobuf.*`（Timestamp など）は JSON で入力する（例: `"2026-09-29T12:00:00Z"`）

### 送信とレスポンス

`C-s` でプレビューを開き、`enter` で送信する。送信中は `esc` でキャンセルできる。

| キー | 操作 |
|---|---|
| `e` | request editor に戻って編集する |
| `v` | response を折りたたみビューで開く（streaming では最後に受信したメッセージ） |
| `p` | response の全文をスクロールバックに出す |
| `q` / `esc` | コマンド入力に戻る（折りたたみビューを開いていないときは `enter` も） |

#### 折りたたみビュー

端末の高さに収まらない response はスクロールバックに流さず、折りたたみビューで開く（収まる response も `v` で開ける）。最初は画面に収まる深さまで展開した状態で開く。

```text
✔ OK user.v1.UserService.ListUsers (4ms)
 ▾ users: [40 items]
   ▸ [0]: {6 keys}
   ▾ [1]: {6 keys}
❯      id: "u-002"
       name: "user02"
     ▸ tags: [2 items]
.users[1].id  4/43  /user02: 1/1
```

| キー | 操作 |
|---|---|
| `↑` `↓` / `k` `j` / `C-p` `C-n` | 移動（`g` `G` 先頭 / 末尾、`C-v` `M-v` ページ送り） |
| `enter` / `space` / `tab` | 折りたたみを切り替える |
| `l` / `→` | 開く（開いていれば最初の子へ） |
| `h` / `←` | 閉じる（閉じていれば親へ） |
| `L` / `H` | カーソル以下をすべて開く / すべて閉じる |
| `1`〜`9` | その深さまで開く |
| `/` | キーと値を検索する（ヒットした位置まで自動で開く） |
| `n` / `N` | 次 / 前のヒット |
| `y` | カーソル位置の値をクリップボードにコピーする（文字列は引用符を外した中身、object / array は整形した JSON） |
| `Y` | カーソル位置の jq 形式のパスをコピーする |

下端にはカーソル位置の jq 形式のパス（`.users[1].id`）を表示する。長い値は画面幅で `…` と省略して表示するが、`y` でコピーすれば全文を取り出せる。コピーは OS のクリップボード（macOS なら `pbcopy`）を使い、使えない環境では端末のクリップボード機能（OSC 52）を使う。

### Streaming

- **server streaming**: 通常どおり送信すると、受信したメッセージが `← #n` として届いた順に流れる。`esc` / `C-g` で受信を止める
- **client streaming / bidi streaming**: request editor で `C-s` を押すたびに今のメッセージを送る（`→ #n`）。最初の `C-s` でストリームが開き、bidi では返ってきたメッセージがその場で流れる。`C-x` で送信を終了し、最上位で `esc` を押すとストリームをキャンセルする


## CLI（AI / script 向け）

CLI のコマンドは Bubble Tea を一切起動しない。

```bash
pluto desc services
pluto desc rpcs [service]
pluto desc rpc <name>
pluto desc message <name>
pluto call <rpc> [-d <json>]
```

### 例

```bash
pluto -s ./proto desc rpcs -o json
pluto -s ./proto desc rpc CreateUser -o json
pluto -s ./proto -t http://localhost:8080 call CreateUser -d '{"name":"Taro"}' -o json
pluto -s ./proto --protocol grpc call UserService/GetUser -d @req.json
echo '{"id":"u-001"}' | pluto -s ./proto call UserService.GetUser -d -

# client streaming / bidi streaming: 改行区切りの JSON か JSON 配列で複数の request を渡す
pluto -r -t http://localhost:8080 call ImportUsers -d '{"name":"A"}
{"name":"B"}'
pluto -r -t https://demo.connectrpc.com call Converse -d '[{"sentence":"Hello"}]' -o json
```

`-d` には JSON 文字列、`@file`（ファイル）、`-`（標準入力）を渡せる。

### 出力

- `-o text`（既定）: 定義は proto 風のテキスト、response は整形した JSON
- `-o json`: キー順が固定された JSON。ANSI エスケープや進捗表示は含まず、stdout にだけ出す
- `desc` の一覧は名前順、フィールドは field number 順に並ぶ

`call -o json` の出力:

```json
{
  "message": { "user": { "id": "u-001", "name": "Taro" } },
  "headers": { "Content-Type": ["application/proto"] },
  "trailers": {}
}
```

streaming RPC は `-o json` のとき NDJSON（1行1オブジェクト）になる。

```text
{"message":{"id":"u-001"}}
{"message":{"id":"u-002"}}
{"summary":{"count":2,"headers":{...},"trailers":{...}}}
```

client / bidi streaming のサマリーは `{"summary":{"sent":N,"received":M,...}}`。途中でエラーになった場合は最後の行が `{"error":{...}}` になる。

### エラーと終了コード

`-o json` のエラーは stdout に JSON で出す。text の場合は stderr に出す。

```json
{
  "error": {
    "code": "ambiguous_symbol",
    "message": "ambiguous symbol \"GetUser\" matches 2 candidates",
    "candidates": ["admin.v1.AdminService.GetUser", "user.v1.UserService.GetUser"]
  }
}
```

RPC エラーでは、サーバーが header / trailer で返したアプリケーション固有のメタデータ（エラー理由など）を `metadata` に含める。`Date` や `Content-Type`、`Grpc-*` / `Connect-*` などの通信上の定型ヘッダーは除く。text 出力と REPL のエラー表示にも同じ内容を出す。

```json
{
  "error": {
    "code": "not_found",
    "message": "account not found",
    "metadata": { "X-Error-Reason": ["1001"] }
  }
}
```

| 終了コード | 意味 |
|---|---|
| `0` | 成功 |
| `1` | その他のエラー（スキーマの読み込み失敗、request JSON の不正など） |
| `2` | 使い方の誤り（引数・フラグの不足や不正） |
| `3` | シンボルが見つからない、または曖昧 |
| `4` | RPC がエラーを返した（`code` は Connect / gRPC のステータスコード） |

## 設定ファイルとプロファイル

接続先・スキーマ・ヘッダーの組み合わせをプロファイルとして保存し、`-p <name>` で切り替えられる。

```yaml
# .pluto.yaml
default_profile: local
profiles:
  local:
    schema: proto/api/api.proto          # 文字列でもリストでもよい
    import_paths: [proto, ~/googleapis]
    protocol: grpc
    target: http://localhost:50051
  dev:
    schema: proto/api/api.proto
    import_paths: [proto, ~/googleapis]
    protocol: grpc
    target: https://api.example.com
    headers:
      x-api-key: ${EXAMPLE_API_KEY}    # 秘密はファイルに書かず環境変数から読む
      User-Agent: my-client/1.0.0
  demo:
    reflection: true
    target: https://demo.connectrpc.com
```

```bash
pluto repl                      # default_profile (local) を使う
pluto -p dev repl
pluto -p dev -t https://other.example.com call GetUser -d '{"id":"1"}'   # フラグはプロファイルより優先
pluto profiles                  # プロファイル一覧 (ヘッダーは名前だけ表示)
```

- 設定ファイルは `--config` (または `PLUTO_CONFIG`) → カレントディレクトリから親をたどって最初に見つかった `.pluto.yaml` → `<ユーザー設定ディレクトリ>/pluto/config.yaml` の順に探す
- プロファイルは `-p` → `PLUTO_PROFILE` → `default_profile` の順に決まる
- 使えるキー: `schema` / `import_paths` / `reflection` / `target` / `protocol` / `json_codec` / `headers`。未知のキーはエラーになる
- 値の中の `${VAR}` / `$VAR` は環境変数で展開する。参照した環境変数が未設定または空ならエラーにする（空の認証ヘッダーを送らないため）
- パスの `~` はホームディレクトリ、相対パスは設定ファイルのあるディレクトリからの相対として扱う
- コマンドラインで明示したフラグはプロファイルより優先する。`-H` で同じ名前のヘッダーを渡すとプロファイルの値を置き換える
- REPL のバナーに使用中のプロファイル名を表示する

## スキーマの読み込み

| 方法 | フラグ | 補足 |
|---|---|---|
| proto ファイル / ディレクトリ | `-s, --schema`（既定 `.`） | ディレクトリは再帰的に探索する（`.` で始まるディレクトリ、`vendor`、`node_modules` は除外）。well-known types は同梱 |
| 追加の import path | `-I, --import-path` | 単一ファイルを指定するときなどに使う |
| gRPC server reflection | `-r, --reflection` | v1 → v1alpha の順に試す。`--target` のサーバーから取得し、公開されている service だけを扱う |

## 接続

| プロトコル | フラグ | 補足 |
|---|---|---|
| Connect | `--protocol connect`（既定） | unary / server streaming は標準の HTTP クライアント（`https://` では自動で HTTP/2）、client / bidi streaming は HTTP/2（`http://` では h2c） |
| gRPC | `--protocol grpc` | HTTP/2。`http://` では h2c を使う |
| gRPC-Web | `--protocol grpcweb` | |

- `--json-codec` で wire format を binary protobuf から JSON に変える
- `-H 'Key: Value'` で request header を付ける（複数指定可）。REPL では `header` コマンドの初期値になる
- `HTTP_PROXY` / `HTTPS_PROXY` などのプロキシ設定に従う

## グローバルフラグ

| フラグ | 既定値 | 内容 |
|---|---|---|
| `-s, --schema` | `.` | proto ファイルまたはディレクトリ（複数指定可） |
| `-I, --import-path` | | 追加の import path |
| `-r, --reflection` | `false` | proto ファイルの代わりに gRPC server reflection でスキーマを取得する |
| `-t, --target` | `http://localhost:8080` | サーバーの base URL（スキームを省略すると `http://`） |
| `--protocol` | `connect` | `connect` / `grpc` / `grpcweb` |
| `--json-codec` | `false` | wire format を JSON にする |
| `-H, --header` | | `'Key: Value'` 形式の request header（複数指定可） |
| `-o, --output` | `text` | `text` / `json` |
| `--config` | 自動で探索 | 設定ファイル（`PLUTO_CONFIG`） |
| `-p, --profile` | `default_profile` | 使うプロファイル（`PLUTO_PROFILE`） |

## 開発

```bash
make build     # bin/pluto, bin/userserver
make test      # go test -race ./...
make lint      # golangci-lint run ./...
make golden    # CLI の golden file (internal/adapter/cli/testdata/golden) を更新
```

golangci-lint のバージョンは `.mise.toml` で固定している。CI（GitHub Actions）では `go vet`、`go test -race`、golangci-lint を実行する。

### 構成

Clean Architecture で、中心は Protobuf のスキーマ。Bubble Tea も Plain CLI も同格の adapter として同じ UseCase を使う。

```text
cmd/pluto                     エントリポイント
internal/
  domain/
    schema/                   Schema / RPC / Message / Field と名前解決
    request/                  DynamicMessageBuilder (request の組み立て), FieldPath
  application/
    port/                     SchemaLoader / RPCInvoker / StreamOpener など
    usecase/                  ListRPCs / DescribeRPC / PrepareRequest / InvokeRPC / OpenStream など
  adapter/
    cli/                      cobra による Plain CLI と repl コマンド
    presenter/                text / json 出力と ErrorDetail への変換
    tui/                      Bubble Tea (app, commandline, rpcselector, requesteditor)
  infrastructure/
    schema/                   proto (protocompile) / reflection / キャッシュ
    transport/                connect-go による Connect / gRPC / gRPC-Web
  bootstrap/                  依存の組み立て (手動 DI)
  testutil/                   テスト用の fixture / テストサーバー
examples/userserver           サンプルサーバー
testdata/proto                サンプル / テスト用の proto
```

依存方向は golangci-lint の depguard で検査している。

- Bubble Tea / Bubbles / Lip Gloss は `internal/adapter/tui` 配下でのみ import できる
- `domain` は `application` / `adapter` / `infrastructure` / transport に依存しない
- `application` は `adapter` / `infrastructure` / transport に依存しない

`testdata/proto` を変更したら `internal/testutil/fixture/proto` にもコピーする（同一であることをテストで検査している）。

## 制限事項

- TLS の詳細設定（証明書検証の無効化、独自 CA、クライアント証明書）とタイムアウトの指定には未対応
- スキーマの読み込み元は proto ファイルと reflection のみ（descriptor set / buf には未対応）
- REPL で成功時の response header / trailer は表示しない（CLI の `-o json` では出力する。エラー時のメタデータは REPL でも表示する）
