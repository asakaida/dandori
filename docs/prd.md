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
- Saga / 補償トランザクション（Phase 3）: Temporalと同じPure SDKパターン。サーバーはsagaの概念を持たず、SDK側でAddCompensation/Compensateを提供する。ワークフロー関数内で補償アクションを登録し、エラー発生時に逆順で実行する。サーバーにとっては通常のActivity実行と区別がつかない
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

- gRPC APIで開始（`StartWorkflow`）
- Workflow IDの指定が可能（未指定時はUUID生成）
- 入力パラメータをJSONで渡す
- 冪等性の保証: 同一Workflow IDで実行中のワークフローが存在する場合は `ALREADY_EXISTS` エラーを返す。完了済みのワークフローと同一IDの場合は新規作成を許可する。将来的にWorkflowIdConflictPolicy（USE_EXISTINGなど）を追加可能な設計とする

(c) Activity実行

- ワーカーがActivity Taskを取得して実行
- 任意のGoの関数（`func(ctx context.Context, input T) (R, error)`）
- タイムアウトサポート:
  - StartToCloseTimeout: 1回のActivity実行の制限時間。サーバー側でバックグラウンド監視し、超過時にActivityTaskTimedOutイベントを記録する
  - ScheduleToCloseTimeout / ScheduleToStartTimeout はPhase 2以降で追加

(d) タスクキュー

- PostgreSQLベース（SKIP LOCKED）
- Workflow TaskとActivity Taskの2種類
- 複数ワーカーの同時ポーリング対応
- キュー名による振り分け

(e) リトライ機構

- Activity失敗時の自動リトライ
- RetryPolicyによる制御:
  - MaxAttempts: 最大試行回数（デフォルト3、0で無制限）
  - InitialInterval: 初回リトライ間隔（デフォルト1秒）
  - BackoffCoefficient: 指数バックオフ係数（delay = InitialInterval * BackoffCoefficient^(attempt-1)、MaxIntervalでキャップ）
  - MaxInterval: 最大リトライ間隔
- リトライ不可能なエラーの区別: FailActivityTask時にnon_retryableフラグを指定可能。trueの場合はRetryPolicyに関係なく即座にActivityTaskFailedイベントを記録する
- RetryPolicyはSDKがScheduleActivityTaskコマンドに含めてサーバーに伝達する

(f) イベントストア

- append-only
- seqIDによるコマンドとイベントの対応付け
- ワークフロー状態の復元に使用

(g) Deterministic Replay

- ワーカー側でワークフロー関数を再実行
- イベント履歴との照合によるreplay
- 非決定性の検出（seqID不一致でエラー）
- 非決定性エラー発生時、ワーカーはFailWorkflowTask APIでサーバーに報告する

(h) gRPC API

クライアント向け:

| メソッド | 説明 |
|---------|------|
| StartWorkflow | ワークフロー開始 |
| DescribeWorkflow | ワークフローの状態・メタデータ取得 |
| GetWorkflowHistory | ワークフローのイベント履歴取得 |
| TerminateWorkflow | ワークフロー強制終了（即座に終了、クリーンアップなし） |

ワーカー向け - Workflow Task:

| メソッド | 説明 |
|---------|------|
| PollWorkflowTask | Workflow Taskの取得（イベント履歴を添付） |
| CompleteWorkflowTask | Workflow Task完了報告（コマンドリストを返す） |
| FailWorkflowTask | Workflow Task失敗報告（非決定性エラー等） |

ワーカー向け - Activity Task:

| メソッド | 説明 |
|---------|------|
| PollActivityTask | Activity Taskの取得 |
| CompleteActivityTask | Activity完了報告（結果を返す） |
| FailActivityTask | Activity失敗報告（エラーメッセージ、エラータイプ、non_retryableフラグ） |

### 3.2 オプション機能（段階的に追加）

Phase 2:

- Timer / Sleep
- Signal / Channel
- 並行Activity実行（Future / Fan-out / Fan-in）
- ワークフローキャンセル（CancelWorkflow API、gracefulなキャンセル通知）
- Activityハートビート（RecordActivityHeartbeat API）
- ScheduleToCloseTimeout / ScheduleToStartTimeout
- CLIツール
- LISTEN/NOTIFYによるタスク即時通知
- sticky execution（ワーカー側イベントキャッシュ）
- ListWorkflows API（ワークフロー一覧取得）

Phase 3:

- Child Workflow
- Saga / 補償トランザクション: Temporalと同じPure SDKパターン。サーバー側はobservability用のmetadataフィールド追加とE2Eテストのみ。SDK側でsagaパッケージ（AddCompensation, Compensate, ContinueWithError）を提供
- SideEffect
- Query
- Cron / スケジュール実行
- HTTP API（grpc-gateway）
- OpenTelemetryトレーシング
- Prometheusメトリクス

Phase 4:

- Web UI
- Continue-as-New
- イベントテーブルのパーティショニング
- Namespace（マルチテナント）
- パフォーマンスベンチマーク
- ドキュメント整備

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
- testcontainersでPostgreSQL起動（adapter/postgres/, adapter/grpc/, test/e2e/ の各パッケージで使用）
- E2Eテスト: bufconn経由の実gRPCスタックでワーカー動作をシミュレートし、全主要シナリオを検証
- CI: GitHub Actions（go vet, go build, go test -race, アーキテクチャ制約チェック）

### 多言語対応

- gRPC + proto定義をAPI契約とし、Go以外の言語からもSDKを作成可能な設計にする
- サーバーは特定のSDK言語に依存しない

## 5. SDK利用イメージ

以下はGo SDKリポジトリ（dandori-sdk-go）で提供されるAPIのイメージ。

### クライアント

```go
c, _ := client.Dial(client.Options{
    HostPort: "localhost:7233",
})
defer c.Close()

// ワークフロー開始（WorkflowRunを返す）
run, err := c.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
    ID:        "order-123",
    TaskQueue: "order-queue",
    WorkflowType: "order-workflow",
}, orderInput)

// ブロッキングで結果を待つ
var result OrderResult
err = run.Get(ctx, &result)
```

```go
// 既存ワークフローのハンドル取得と結果待ち
run := c.GetWorkflow(ctx, "order-123")
var result OrderResult
err := run.Get(ctx, &result)
```

```go
// ワークフロー情報の取得
execution, err := c.DescribeWorkflow(ctx, "order-123")
fmt.Println(execution.Status) // "RUNNING", "COMPLETED", "FAILED"
```

```go
// イベント履歴の取得
events, err := c.GetWorkflowHistory(ctx, "order-123")
```

```go
// ワークフロー強制終了
err := c.TerminateWorkflow(ctx, "order-123", "manual termination")
```

### WorkflowRun インターフェース

```go
type WorkflowRun interface {
    GetID() string
    Get(ctx context.Context, valuePtr interface{}) error // 完了までブロッキング待機
}
```

### ワークフロー関数

```go
func OrderWorkflow(ctx workflow.Context, input OrderInput) (OrderResult, error) {
    validation, err := workflow.ExecuteActivity[OrderInput, OrderValidation](
        ctx, "validate-order", input,
        workflow.WithStartToCloseTimeout(10*time.Second),
        workflow.WithRetryPolicy(workflow.RetryPolicy{MaxAttempts: 3}),
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
        workflow.WithStartToCloseTimeout(30*time.Second),
    )
    if err != nil {
        return OrderResult{}, err
    }

    _, err = workflow.ExecuteActivity[ConfirmInput, struct{}](
        ctx, "send-confirmation",
        ConfirmInput{OrderID: input.OrderID, Email: input.CustomerEmail},
        workflow.WithStartToCloseTimeout(10*time.Second),
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
c, _ := client.Dial(client.Options{HostPort: "localhost:7233"})
defer c.Close()

w := worker.New(c, "order-queue")
w.RegisterWorkflow("order-workflow", workflows.OrderWorkflow)
w.RegisterActivity("validate-order", activities.ValidateOrder)
w.RegisterActivity("charge-payment", activities.ChargePayment)
w.RegisterActivity("send-confirmation", activities.SendConfirmation)
w.Start(ctx)
```

### Saga利用例（旅行予約ワークフロー）

```go
func TripBookingWorkflow(ctx workflow.Context, input TripInput) (TripResult, error) {
    s := saga.New(saga.Options{ParallelCompensation: false})

    // 1. フライト予約
    flightRes, err := workflow.ExecuteActivity[FlightInput, FlightResult](
        ctx, "book-flight", input.Flight,
        workflow.WithStartToCloseTimeout(30*time.Second),
    )
    if err != nil {
        // 補償不要（まだ何も予約していない）
        return TripResult{}, err
    }
    s.AddCompensation(ctx, "cancel-flight", CancelFlightInput{ReservationID: flightRes.ReservationID})

    // 2. ホテル予約
    hotelRes, err := workflow.ExecuteActivity[HotelInput, HotelResult](
        ctx, "book-hotel", input.Hotel,
        workflow.WithStartToCloseTimeout(30*time.Second),
    )
    if err != nil {
        // フライトをキャンセル
        return TripResult{}, s.Compensate(ctx, err)
    }
    s.AddCompensation(ctx, "cancel-hotel", CancelHotelInput{ReservationID: hotelRes.ReservationID})

    // 3. レンタカー予約
    carRes, err := workflow.ExecuteActivity[CarInput, CarResult](
        ctx, "book-car", input.Car,
        workflow.WithStartToCloseTimeout(30*time.Second),
    )
    if err != nil {
        // ホテル → フライトの順でキャンセル（逆順補償）
        return TripResult{}, s.Compensate(ctx, err)
    }

    return TripResult{
        FlightReservation: flightRes.ReservationID,
        HotelReservation:  hotelRes.ReservationID,
        CarReservation:    carRes.ReservationID,
    }, nil
}
```

sagaパッケージはSDK側で提供する。サーバーにとっては補償Activity（cancel-flight等）も通常のActivityと同じに見える。

## 6. 段階的実装計画

### Phase 1: MVP

目標: deterministic replayでワークフローを実行し、Activityを順次実行して完了できること

サーバーとGo SDKの両リポジトリで並行して開発する。

学習ポイント: gRPC、SKIP LOCKEDキュー、イベントソーシング、deterministic replay、goroutine/context/graceful shutdown

### Phase 2: 信頼性と機能拡張

- Timer / Sleep
- Signal / Channel
- 並行Activity実行（Future / Fan-out / Fan-in）
- ワークフローキャンセル（CancelWorkflow）
- Activityハートビート
- CLIツール
- LISTEN/NOTIFYによるタスク即時通知
- sticky execution（ワーカー側イベントキャッシュ）
- ListWorkflows API
- 構造化ログ整備
- インテグレーションテスト充実

### Phase 3: 高度な機能

- Saga / 補償トランザクション（サーバー: metadataフィールド + E2E、SDK: sagaパッケージ）
- Child Workflow
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
- PostgreSQLドライバ: database/sql + github.com/lib/pq
- マイグレーション: embed.FS（標準ライブラリ、外部依存なし）
- CLI: github.com/spf13/cobra（Phase 2）
- ログ: 標準ライブラリ log/slog
- UUID: github.com/google/uuid
- テスト: 標準ライブラリ + github.com/stretchr/testify
### 使わないもの

- ORM（生SQL、学習目的）
- Kafka, RabbitMQ等
- Redis（全てPostgreSQLで実現）
- Make、Taskfile等のタスクランナー（goコマンドとシェルスクリプトで十分）
