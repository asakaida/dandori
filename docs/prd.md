# dandori - プロダクト要求仕様書

## Context

Temporalのようなワークフローエンジンを学習と実用の目的でゼロから作成する。
PostgreSQLとGolangを使い、外部依存を最小限にとどめる。
Temporalと同じdeterministic replay方式を採用し、任意のGoコードでワークフローを記述できるようにする。

通信プロトコルにgRPCを採用し、proto定義をAPIの契約とすることで、将来的にTypeScript等の他言語からも利用可能な設計とする。

## 1. リポジトリ構成

サーバーとSDKは別リポジトリで管理する。

| リポジトリ | 役割 |
|-----------|------|
| dandori（本リポジトリ） | サーバー、proto定義（API契約のSource of Truth） |
| dandori-sdk-go（別リポジトリ） | Go SDK（クライアント、ワーカー、deterministic replay） |

proto定義は本リポジトリでホストし、SDK側がGoモジュールとしてimportする。

## 2. コアコンセプト

### 取り入れる概念

- Workflow: 任意のGoの関数として記述されるdurable execution。プロセスが落ちてもイベント履歴のreplayで途中から再開できる
- Activity: 副作用のある処理（外部API、DB操作等）。リトライポリシーを持つ
- Deterministic Replay: ワークフロー関数を最初から再実行し、イベント履歴と照合して状態を復元する仕組み
- Task Queue: Workflow TaskとActivity Taskの2種類。PostgreSQLのSELECT FOR UPDATE SKIP LOCKEDで実現
- Worker: Workflow TaskとActivity Taskの両方を処理する。複数起動でスケールアウト可能
- Event Sourcing: 全状態変化をイベントとして記録。イベント履歴がSource of Truth
- Command: ワーカーがWorkflow Task実行の結果としてサーバーに返す指示（ScheduleActivityTask, CompleteWorkflow等）
- Timer / Sleep: PostgreSQLに発火時刻を記録し、ポーリングで検知する待機機能
- Signal: 外部から実行中のワークフローにデータを注入する仕組み

### 段階的に追加する概念

- Sleep / Timer（Phase 2）
- Signal / Channel（Phase 2）
- 並行Activity実行 / Future（Phase 2）
- Child Workflow（Phase 3）
- Saga / 補償トランザクション（Phase 3）
- Continue-as-New / Namespace（Phase 4）
- SideEffect, Query, Update（Phase 3-4）

## 3. 機能要件

### 3.1 必須機能（MVP）

(a) ワークフロー定義

- 任意のGoの関数として記述
- `func(ctx workflow.Context, input T) (R, error)` のシグネチャ
- 関数内でif/for等の制御フローが自由に使える
- workflow.Contextを通じてActivity呼び出し

(b) ワークフローの開始

- gRPC APIで開始
- Workflow IDの指定が可能（未指定時はUUID生成）
- 入力パラメータをJSONで渡す
- 冪等性の保証

(c) Activity実行

- ワーカーがActivity Taskを取得して実行
- 任意のGoの関数（`func(ctx context.Context, input T) (R, error)`）
- タイムアウト（StartToClose）サポート

(d) タスクキュー

- PostgreSQLベース（SKIP LOCKED）
- Workflow TaskとActivity Taskの2種類
- 複数ワーカーの同時ポーリング対応
- キュー名による振り分け

(e) リトライ機構

- Activity失敗時の自動リトライ
- 最大リトライ回数、バックオフ間隔の指定
- リトライ不可能なエラーの区別

(f) イベントストア

- append-only
- seqIDによるコマンドとイベントの対応付け
- ワークフロー状態の復元に使用

(g) Deterministic Replay

- ワーカー側でワークフロー関数を再実行
- イベント履歴との照合によるreplay
- 非決定性の検出（seqID不一致でエラー）

(h) gRPC API

- クライアント向け: StartWorkflow, GetWorkflowExecution, TerminateWorkflow
- ワーカー向け: PollWorkflowTask, CompleteWorkflowTask, PollActivityTask, CompleteActivityTask, FailActivityTask

### 3.2 オプション機能（段階的に追加）

- Timer / Sleep
- Signal / Channel
- 並行Activity実行（Future / Fan-out / Fan-in）
- ワークフローキャンセル
- ハートビート
- CLIツール
- Child Workflow
- Saga / 補償トランザクション
- Cron / スケジュール実行
- Query / Search Attribute
- HTTP API
- Web UI

## 4. 非機能要件

### 耐久性

- サーバー再起動後もワークフローは中断地点からreplayで再開
- ワーカーダウン時、タスクは一定時間後に他のワーカーへ再配分（visibility timeout）
- PostgreSQLコミットをもって状態変化が確定

### 一貫性

- 同一ワークフローのWorkflow Taskは常に1つだけが処理中（pg_advisory_xact_lock）
- イベントのシーケンス番号による楽観的排他制御
- タスク二重実行の検知

### スケーラビリティ

- ワーカーの水平スケール可能
- サーバーはステートレス設計
- MVPでは単一サーバー構成

### パフォーマンス

- MVPでは秒間数十ワークフロー
- タスクキューのポーリング間隔は設定可能（デフォルト1秒）
- 長期実行ワークフロー対応

### 可観測性

- 構造化ログ（slog）
- OpenTelemetryトレーシング（Phase 2以降）
- Prometheusメトリクス（Phase 2以降）

### テスタビリティ

- ワークフローとActivityの単体テスト可能
- テスト用モックActivity実行環境
- testcontainersでPostgreSQL起動

### 多言語対応

- gRPC + proto定義をAPI契約とし、Go以外の言語からもSDKを作成可能な設計にする
- サーバーは特定のSDK言語に依存しない

## 5. SDK利用イメージ

以下はGo SDKリポジトリ（dandori-sdk-go）で提供されるAPIのイメージ。

### ワークフロー関数

```go
func OrderWorkflow(ctx workflow.Context, input OrderInput) (OrderResult, error) {
    validation, err := workflow.ExecuteActivity[OrderInput, OrderValidation](
        ctx, "validate-order", input,
        workflow.WithActivityTimeout(10*time.Second),
        workflow.WithActivityRetry(workflow.RetryPolicy{MaxAttempts: 3}),
    )
    if err != nil {
        return OrderResult{}, err
    }

    // 通常のGoのif文がそのまま使える
    if !validation.Valid {
        return OrderResult{}, fmt.Errorf("validation failed: %s", validation.Reason)
    }

    chargeResult, err := workflow.ExecuteActivity[ChargeInput, ChargeResult](
        ctx, "charge-payment",
        ChargeInput{OrderID: input.OrderID, Amount: validation.Amount},
        workflow.WithActivityTimeout(30*time.Second),
    )
    if err != nil {
        return OrderResult{}, err
    }

    _, err = workflow.ExecuteActivity[ConfirmInput, struct{}](
        ctx, "send-confirmation",
        ConfirmInput{OrderID: input.OrderID, Email: input.CustomerEmail},
        workflow.WithActivityTimeout(10*time.Second),
    )
    if err != nil {
        return OrderResult{}, err
    }

    return OrderResult{OrderID: input.OrderID, Status: "completed"}, nil
}
```

### Activity関数

```go
func ValidateOrder(ctx context.Context, input OrderInput) (OrderValidation, error) {
    // 副作用OK。deterministicである必要はない
    return OrderValidation{Valid: true, Amount: 1000}, nil
}
```

### ワーカー起動

```go
w := worker.New(client, "order-queue")
w.RegisterWorkflow("order-workflow", workflows.OrderWorkflow)
w.RegisterActivity("validate-order", activities.ValidateOrder)
w.RegisterActivity("charge-payment", activities.ChargePayment)
w.RegisterActivity("send-confirmation", activities.SendConfirmation)
w.Start(ctx)
```

### クライアント

```go
c := client.New("localhost:7233")
run, err := c.StartWorkflow(ctx, client.StartWorkflowOptions{
    ID:        "order-123",
    Type:      "order-workflow",
    TaskQueue: "order-queue",
    Input:     orderInput,
})
```

## 6. 段階的実装計画

### Phase 1: MVP

目標: deterministic replayでワークフローを実行し、Activityを順次実行して完了できること

サーバーとGo SDKの両リポジトリで並行して開発する。

学習ポイント: gRPC、SKIP LOCKEDキュー、イベントソーシング、deterministic replay、goroutine/context/graceful shutdown

### Phase 2: 信頼性と機能拡張

- Timer / Sleep
- Signal / Channel
- 並行Activity実行（Future / Fan-out / Fan-in）
- ワークフローキャンセル
- Activityハートビート
- 指数バックオフリトライ
- CLIツール
- LISTEN/NOTIFYによるタスク即時通知
- sticky execution（ワーカー側イベントキャッシュ）
- 構造化ログ整備
- インテグレーションテスト充実

### Phase 3: 高度な機能

- Child Workflow
- Saga / 補償トランザクション
- SideEffect
- Query
- Cron / スケジュール実行
- HTTP API（grpc-gateway）
- OpenTelemetryトレーシング
- Prometheusメトリクス

### Phase 4: 運用性と最適化

- Web UI
- Continue-as-New
- イベントテーブルのパーティショニング
- Namespace（マルチテナント）
- パフォーマンスベンチマーク
- ドキュメント整備

## 7. 技術選定

### 使用ライブラリ（最小構成）

- gRPC: google.golang.org/grpc + protoc-gen-go
- PostgreSQLドライバ: github.com/jackc/pgx/v5
- マイグレーション: github.com/golang-migrate/migrate
- CLI: github.com/spf13/cobra（Phase 2）
- ログ: 標準ライブラリ log/slog
- UUID: github.com/google/uuid
- テスト: 標準ライブラリ + github.com/stretchr/testify
### 使わないもの

- ORM（生SQL、学習目的）
- Kafka, RabbitMQ等
- Redis（全てPostgreSQLで実現）
- Make、Taskfile等のタスクランナー（goコマンドとシェルスクリプトで十分）
