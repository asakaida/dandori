# クイックスタートガイド

dandoriワークフローエンジンを起動し、最初のワークフローを実行するまでの手順。

## 必要要件

- Go 1.24+
- Docker / Docker Compose
- git

## 1. リポジトリの取得

```bash
git clone https://github.com/asakaida/dandori.git
git clone https://github.com/asakaida/dandori-sdk-go.git
```

2つのリポジトリは同じ親ディレクトリに配置する。

```text
projects/
├── dandori/           # サーバー
└── dandori-sdk-go/    # Go SDK
```

## 2. PostgreSQLの起動

```bash
cd dandori
docker compose up -d
```

PostgreSQLがポート5432で起動する。接続情報: `postgres://dandori:dandori@localhost:5432/dandori?sslmode=disable`

## 3. サーバーのビルドと起動

```bash
cd dandori
go build -o dandori-server ./cmd/dandori
./dandori-server
```

起動すると以下のログが出力される。

```text
{"level":"INFO","msg":"connected to database"}
{"level":"INFO","msg":"migrations complete"}
{"level":"INFO","msg":"dandori gRPC server listening","port":"7233"}
{"level":"INFO","msg":"dandori HTTP server listening","port":"8080"}
```

サーバーが以下のポートでリッスンする。

| ポート | プロトコル | 用途 |
| --- | --- | --- |
| 7233 | gRPC | SDK / CLIからの接続 |
| 8080 | HTTP | REST API / Web UI / メトリクス |

## 4. Hello Worldサンプルの実行

dandori-sdk-goリポジトリにサンプルが含まれている。

### ワーカーの起動（ターミナル2）

```bash
cd dandori-sdk-go
go run ./examples/hello/worker/
```

ワーカーがサーバーに接続し、タスクキュー `hello-queue` のポーリングを開始する。

### ワークフローの開始（ターミナル3）

```bash
cd dandori-sdk-go
go run ./examples/hello/starter/
```

出力例:

```text
Started workflow: 550e8400-e29b-41d4-a716-446655440000
Result: Hello, dandori!
```

ワークフローIDはUUIDが自動生成される。
このIDを使って以降の確認を行う。

## 5. CLIで状態を確認

dandori-cliでワークフローの状態を確認できる。
`<workflow-id>` は上の出力で表示されたIDに置き換える。

```bash
cd dandori
go run ./cmd/dandori-cli describe <workflow-id>
go run ./cmd/dandori-cli history <workflow-id>
go run ./cmd/dandori-cli list
```

## 6. Web UIで確認

ブラウザで `http://localhost:8080/ui/` を開くと、
ワークフロー一覧と詳細を確認できる。

## 7. HTTP APIで確認

```bash
# ワークフロー一覧
curl -s http://localhost:8080/v1/workflows | jq .

# ワークフロー詳細（IDを置き換える）
curl -s http://localhost:8080/v1/workflows/<workflow-id> | jq .

# イベント履歴
curl -s http://localhost:8080/v1/workflows/<workflow-id>/history | jq .
```

## 処理の流れ

```text
starter (Go)              dandori-server              worker (Go)
  |                            |                         |
  |-- ExecuteWorkflow -------->|                         |
  |   (gRPC: StartWorkflow)    |                         |
  |                            |-- WorkflowTask -------->|
  |                            |                         |-- HelloWorkflow()
  |                            |                         |   "Greetを実行して"
  |                            |<-- ScheduleActivity ---- |
  |                            |-- ActivityTask -------->|
  |                            |                         |-- Greet() 実行
  |                            |<-- Complete (結果) ------ |
  |                            |-- WorkflowTask -------->|
  |                            |                         |-- replay + 完了
  |                            |<-- CompleteWorkflow ---- |
  |<-- Result ----------------- |                         |
  |   "Hello, dandori!"        |                         |
```

## 次のステップ

- 独自のワークフローとアクティビティを書く → dandori-sdk-go/README.md のAPIリファレンス
- 複数アクティビティの順次/並列実行を試す
- Sagaパターンで補償トランザクションを実装する
- dandori本体のREADMEにある旅行予約サンプルを参考にする
