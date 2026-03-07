# アーキテクチャガイド

## 設計思想

dandoriはHexagonal Architecture（ポートアンドアダプタ）を採用している。
ビジネスロジックを外部の技術的関心事から分離し、テスト容易性と変更容易性を確保する。

依存関係のルール:

```text
domain ← port ← engine ← adapter
```

内側のレイヤーは外側のレイヤーを知らない。
adapter層はport層のインターフェースを通じてengine層と通信する。

## ディレクトリ構成

```text
dandori/
├── api/v1/                    # Protocol Buffers定義と生成コード
│   ├── service.proto          # gRPC サービス定義
│   ├── types.proto            # メッセージ型定義
│   ├── service.pb.go          # 生成: メッセージ型
│   ├── service_grpc.pb.go     # 生成: gRPCサーバー/クライアント
│   ├── service.pb.gw.go       # 生成: grpc-gateway
│   └── service.swagger.json   # 生成: OpenAPI v2仕様
│
├── cmd/
│   ├── dandori/               # サーバーバイナリ
│   │   └── main.go            # DI、起動、グレースフルシャットダウン
│   └── dandori-cli/           # CLIツール
│       ├── main.go
│       └── cmd/               # Cobraサブコマンド
│
├── internal/
│   ├── domain/                # ドメインモデル（純粋な型定義、外部依存なし）
│   ├── port/                  # ポート（インターフェース定義）
│   ├── engine/                # ビジネスロジック（ポートの実装）
│   ├── adapter/               # アダプタ（外部システムとの接続）
│   │   ├── grpc/              # Inbound: gRPCハンドラ
│   │   ├── http/              # Inbound: HTTP/Web UI/SSE
│   │   ├── postgres/          # Outbound: PostgreSQL永続化
│   │   └── telemetry/         # Cross-cutting: トレーシング/メトリクス
│   └── test/
│       ├── e2e/               # E2Eテスト（bufconn + testcontainers）
│       └── bench/             # ベンチマークテスト
│
├── web/                       # Web UI（vanilla JS + Tailwind CSS v4）
│   ├── embed.go               # embed.FS宣言
│   ├── index.html
│   └── static/
│
├── third_party/               # grpc-gateway用protoインクルード
└── docker-compose.yml         # PostgreSQL開発環境
```

## レイヤー詳細

### Domain層 (`internal/domain/`)

外部依存を持たない純粋な型定義。
ワークフロー、イベント、コマンド、タスク、エラーなどのドメインモデルを定義する。

主要な型:

- `WorkflowExecution`: ワークフローの実行状態
- `HistoryEvent`: イベント履歴の1レコード
- `Command`: ワーカーからサーバーへの指示
- `WorkflowTask` / `ActivityTask`: タスクキューのタスク
- `RetryPolicy`: リトライポリシー
- `Timer`: タイマー
- `WorkflowQuery`: クエリ
- `Namespace`: 名前空間

### Port層 (`internal/port/`)

Inbound PortとOutbound Portのインターフェースを定義する。

Inbound Port（サービス境界）:

- `ClientService`: クライアント向けAPI（StartWorkflow, DescribeWorkflow等 8メソッド）
- `WorkflowTaskService`: ワーカー向けWorkflow Task API（4メソッド）
- `ActivityTaskService`: ワーカー向けActivity Task API（4メソッド）

Outbound Port（永続化境界）:

- `WorkflowRepository`: ワークフローのCRUD
- `EventRepository`: イベント履歴の追記・取得
- `WorkflowTaskRepository`: Workflow Taskキュー操作
- `ActivityTaskRepository`: Activity Taskキュー操作
- `TimerRepository`: タイマー管理
- `QueryRepository`: クエリ管理
- `NamespaceRepository`: 名前空間管理
- `TxManager`: トランザクション管理

### Engine層 (`internal/engine/`)

ビジネスロジックの中核。3つのInbound Portを全て実装する。

- `engine.go`: Engineメイン（StartWorkflow, Poll, Complete等の実装）
- `command_processor.go`: コマンドからイベントへの変換パイプライン
- `background.go`: バックグラウンドワーカー（タイムアウト検出、タイマーポーリング、タスクリカバリ）
- `retry.go`: 指数バックオフリトライ計算
- `cron.go`: Cronスケジュール検証
- `broadcaster.go`: SSE/WebSocket通知ブロードキャスター

### Adapter層 (`internal/adapter/`)

外部システムとの接続を担う。

Inbound Adapter:

- `grpc/handler.go`: gRPCハンドラ。proto型とドメイン型の相互変換を行う
- `grpc/interceptor.go`: OpenTelemetryインストルメンテーション
- `grpc/health.go`: gRPC Healthチェック
- `http/gateway.go`: grpc-gatewayリバースプロキシ
- `http/health.go`: HTTP /healthzエンドポイント
- `http/swagger.go`: OpenAPI仕様配信
- `http/ui.go`: Web UI（embed.FSによるSPA配信）
- `http/sse.go`: Server-Sent Eventsエンドポイント

Outbound Adapter:

- `postgres/store.go`: コネクションプール管理、TxManager実装
- `postgres/workflow.go` 〜 `namespace.go`: 各Repositoryインターフェースの実装
- `postgres/migrate.go`: embed.FSベースのマイグレーションランナー
- `postgres/migration/`: SQLマイグレーションファイル

Cross-cutting:

- `telemetry/tracer.go`: OpenTelemetry TracerProvider
- `telemetry/decorator.go`: トレーシングデコレータ
- `telemetry/metrics.go`: Prometheusメトリクス定義
- `telemetry/metrics_decorator.go`: メトリクスデコレータ

## データフロー

### ワークフロー開始

```text
Client
  → gRPC Handler (proto → domain変換)
    → Tracing Decorator (スパン作成)
      → Metrics Decorator (カウンタ増加)
        → Engine.StartWorkflow
          → TxManager.RunInTx
            → WorkflowRepository.Create
            → EventRepository.Append (WorkflowExecutionStarted)
            → WorkflowTaskRepository.Enqueue
```

### Workflow Taskの処理

```text
Worker
  → PollWorkflowTask
    → WorkflowTaskRepository.Poll (SKIP LOCKED)
    → EventRepository.GetByWorkflowID
    → 全イベント履歴をレスポンス

Worker (ワークフローロジック実行後)
  → CompleteWorkflowTask (コマンド配列)
    → Advisory Lock (pg_advisory_xact_lock)
    → CommandProcessor.Process
      → 各コマンド → イベント変換
      → アクティビティスケジュール / タイマー作成 / ワークフロー完了等
```

### バックグラウンドワーカー

```text
ActivityTimeoutChecker (5秒間隔)
  → ActivityTaskRepository.GetTimedOut / GetHeartbeatTimedOut / ...
  → イベント記録 + Workflow Task投入

TimerPoller (1秒間隔)
  → TimerRepository.GetFired
  → MarkFired + イベント記録 + Workflow Task投入

TaskRecovery (10秒間隔)
  → WorkflowTaskRepository.RecoverStaleTasks
  → ActivityTaskRepository.RecoverStaleTasks
```

## DI（依存性注入）

`cmd/dandori/main.go` でデコレータパターンによるDIを行う:

```text
PostgreSQL Store
  → Engine (全Repositoryを注入)
    → Tracing Decorator (Engine + Tracer)
      → Metrics Decorator (Traced + Metrics)
        → gRPC Handler (Metricsed)
```

この構成により、各レイヤーの関心事が分離され、テスト時にはモックへの差し替えが容易になる。

## データベース設計

PostgreSQLの機能を活用した設計:

- `SELECT FOR UPDATE SKIP LOCKED`: タスクキューの排他制御
- `pg_advisory_xact_lock`: ワークフロー単位の排他ロック
- イベントテーブルのハッシュパーティショニング（workflow_idで16分割）
- JSONB + GINインデックス: Search Attributesの高速フィルタリング
- UNIQUE制約 `(workflow_id, sequence_num)`: イベント順序の保証

## 技術選定の方針

dandoriは外部依存を最小化する方針を取っている:

- メッセージキュー不使用: PostgreSQLのSKIP LOCKEDでタスクキューを実現
- キャッシュ不使用: PostgreSQLの接続プールで十分なパフォーマンスを確保
- ORM不使用: database/sql + lib/pqで直接SQL
- フロントエンドフレームワーク不使用: vanilla JS + Tailwind CSS v4
