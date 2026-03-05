# dandori

Temporalにインスパイアされたワークフローエンジン。PostgreSQLとGoで構築し、deterministic replay方式を採用。

## 特徴

- **Deterministic Replay**: ワークフロー関数を最初から再実行し、イベント履歴と照合して状態を復元
- **Event Sourcing**: 全状態変化をイベントとして記録。イベント履歴がSource of Truth
- **PostgreSQL**: 外部依存を最小限に。タスクキューは `SELECT FOR UPDATE SKIP LOCKED` で実現
- **gRPC API**: proto定義をAPI契約とし、将来的に他言語SDKにも対応可能
- **Hexagonal Architecture**: domain/ -> port/ -> adapter/ の依存方向でテスタビリティを確保

## アーキテクチャ

```text
Client (Go SDK / CLI)
    |
    | gRPC
    v
dandori-server (本リポジトリ)
    |
    | PostgreSQL
    v
PostgreSQL
    |
    v
dandori-worker (Go SDK側で実装, 1..N instances)
```

サーバーはワークフローのステップ定義を理解しない。イベントの永続化、コマンド処理、タスクキュー管理に徹し、「次に何をすべきか」の判断はワーカーに委ねる。

## プロジェクト構成

```text
dandori/
├── api/v1/               # proto定義 + 生成コード
├── cmd/dandori/           # サーバーバイナリ
├── internal/
│   ├── domain/            # ドメインモデル（型定義、エラー定義）
│   ├── port/              # ポート（インターフェース定義）
│   ├── adapter/
│   │   ├── grpc/          # Inbound Adapter: gRPCハンドラ
│   │   └── postgres/      # Outbound Adapter: PostgreSQL実装
│   │       └── migration/ # SQLマイグレーション
│   └── engine/            # ビジネスロジック、コマンドプロセッサ
├── test/e2e/              # E2Eテスト
└── docs/                  # 設計書、スプリント管理
```

## 必要要件

- Go 1.24+
- Docker / Docker Compose（PostgreSQL用）
- protoc + protoc-gen-go + protoc-gen-go-grpc（proto再生成時のみ）

## セットアップ

### PostgreSQL起動

```bash
docker compose up -d
```

デフォルト接続情報: `postgres://dandori:dandori@localhost:5432/dandori?sslmode=disable`

### ビルド

```bash
go build ./cmd/dandori
```

### サーバー起動

```bash
./dandori
```

環境変数:

| 変数 | デフォルト | 説明 |
|------|-----------|------|
| `DATABASE_URL` | `postgres://dandori:dandori@localhost:5432/dandori?sslmode=disable` | PostgreSQL接続文字列 |
| `GRPC_PORT` | `7233` | gRPCリッスンポート |

マイグレーションはサーバー起動時に自動実行される。

## gRPC API

| カテゴリ | RPC | 説明 |
|---------|-----|------|
| Client | `StartWorkflow` | ワークフロー開始（冪等） |
| Client | `DescribeWorkflow` | ワークフロー状態取得 |
| Client | `GetWorkflowHistory` | イベント履歴取得 |
| Client | `TerminateWorkflow` | ワークフロー強制終了 |
| Worker | `PollWorkflowTask` | Workflow Taskの取得 |
| Worker | `CompleteWorkflowTask` | Workflow Task完了（コマンド返却） |
| Worker | `FailWorkflowTask` | Workflow Task失敗報告 |
| Worker | `PollActivityTask` | Activity Taskの取得 |
| Worker | `CompleteActivityTask` | Activity Task完了（結果返却） |
| Worker | `FailActivityTask` | Activity Task失敗報告 |

gRPC reflectionが有効なため、grpcurlで直接操作可能:

```bash
grpcurl -plaintext localhost:7233 list
grpcurl -plaintext -d '{"workflow_type": "MyWorkflow", "task_queue": "default"}' \
  localhost:7233 dandori.api.v1.DandoriService/StartWorkflow
```

## テスト

```bash
# 全テスト実行（PostgreSQL testcontainersが自動起動）
go test -v -race ./...

# 個別テスト
go test -v -race ./internal/engine/...         # Engine ユニットテスト
go test -v -race ./internal/adapter/postgres/... # PostgreSQL インテグレーションテスト
go test -v -race ./internal/adapter/grpc/...     # gRPC ハンドラテスト
go test -v -race ./test/e2e/...                  # E2Eテスト
```

テストにはDockerが必要（testcontainers-goがPostgreSQLコンテナを自動起動）。

## 関連リポジトリ

| リポジトリ | 役割 |
|-----------|------|
| dandori（本リポジトリ） | サーバー、proto定義（API契約のSource of Truth） |
| dandori-sdk-go | Go SDK（クライアント、ワーカー、deterministic replay） |

## ドキュメント

- [設計書](docs/design.md) - アーキテクチャ、データモデル、API仕様の詳細
- [プロダクト要求仕様書](docs/prd.md) - コアコンセプト、機能要件、フェーズ計画
- [スプリント管理](docs/sprints.md) - Sprint 1-20の詳細タスクと進捗

## 開発状況

- **Phase 1 (MVP)**: 完了 - Sprint 1-5 + E2Eテスト（104テスト通過）
- **Phase 2 (信頼性と機能拡張)**: Sprint 6-11（Timer, Signal, Cancel, Heartbeat, LISTEN/NOTIFY, CLI）
- **Phase 3 (高度な機能)**: Sprint 12-16（Child Workflow, SideEffect, Cron, HTTP API, Observability）
- **Phase 4 (運用性と最適化)**: Sprint 17-20（Namespace, Web UI, パフォーマンス, ドキュメント）

## ライセンス

TBD
