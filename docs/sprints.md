# dandori - Sprint 進捗管理

## 概要

Phase単位の実装計画をSprint（1週間）に分割して管理する。
各Sprintにはゴール・タスク・完了条件を定義し、チェックボックスで進捗を追跡する。

本ドキュメントはサーバーリポジトリ（dandori）の進捗を管理する。
Go SDKリポジトリ（dandori-sdk-go）の進捗は当該リポジトリで管理する。

ステータス凡例:

- `未着手` - Sprint開始前
- `進行中` - Sprint実施中
- `完了` - 全タスク完了、完了条件クリア
- `持越し` - 未完了タスクあり、次Sprintへ繰越

## Phase 1: MVP（サーバー側）

### Sprint 1 - プロジェクト基盤

ステータス: `完了`

ゴール: ビルド・起動・DB接続ができるプロジェクト骨格を構築する

タスク:

- [x] go mod init, .gitignore
- [x] docker-compose.yml（PostgreSQL 18-alpine3.23）
- [x] api/v1/service.proto の定義（全10メソッド: StartWorkflow, DescribeWorkflow, GetWorkflowHistory, TerminateWorkflow, PollWorkflowTask, CompleteWorkflowTask, FailWorkflowTask, PollActivityTask, CompleteActivityTask, FailActivityTask）
- [x] api/v1/types.proto の定義（HistoryEvent, Command, FailActivityTaskRequest に non_retryable/error_type 含む）
- [x] protoc-gen-goによるコード生成の確認
- [x] internal/adapter/postgres/migration/000001_initial.up.sql
  - workflow_executions テーブル
  - workflow_events テーブル（UNIQUE(workflow_id, sequence_num)）
  - workflow_tasks テーブル（Workflow Task専用）
  - activity_tasks テーブル（Activity Task専用、timeout_at/start_to_close_timeout/retry_policy カラム含む）
  - timers テーブル
- [x] internal/adapter/postgres/migration/000001_initial.down.sql
- [x] golang-migrateでマイグレーション実行確認

完了条件:

- PostgreSQLが起動し、マイグレーションが通る（5テーブル作成）
- gRPCコードが生成される（全10メソッド分）
- `go build ./cmd/dandori` でサーバーバイナリがビルドできる

### Sprint 2 - ドメインモデルとストア層

ステータス: `完了`

ゴール: domain/の型定義、port/のインターフェース定義、adapter/postgres/のPostgreSQL実装を完成させる

技術選定変更: PostgreSQLドライバを pgx/v5 から database/sql + github.com/lib/pq に変更

タスク:

- [x] internal/domain/errors.go（ErrWorkflowNotFound, ErrWorkflowAlreadyExists, ErrWorkflowNotRunning, ErrTaskNotFound, ErrTaskAlreadyCompleted, ErrNoTaskAvailable）
- [x] internal/domain/event.go（EventType定数: Started, Completed, Failed, Terminated, ActivityScheduled, ActivityCompleted, ActivityFailed, ActivityTimedOut）
- [x] internal/domain/command.go（CommandType定数、ScheduleActivityTaskAttributes に RetryPolicy/StartToCloseTimeout、CompleteWorkflowAttributes, FailWorkflowAttributes）
- [x] internal/domain/retry.go（RetryPolicy: MaxAttempts, InitialInterval, BackoffCoefficient, MaxInterval）
- [x] internal/domain/task.go（WorkflowTask, ActivityTask を別型で定義、TaskStatus、ActivityFailure）
- [x] internal/domain/workflow.go（WorkflowStatus に TERMINATED、IsTerminal()メソッド）
- [x] internal/domain/timer.go
- [x] internal/port/service.go（役割別 Inbound Port: ClientService, WorkflowTaskService, ActivityTaskService）
- [x] internal/port/repository.go（Outbound Port: WorkflowRepository, EventRepository, WorkflowTaskRepository, ActivityTaskRepository, TimerRepository, TxManager）
- [x] internal/adapter/postgres/store.go（database/sql、TxManager、context経由のトランザクション伝搬、ネストTx再利用、Workflows()/Events()/WorkflowTasks()/ActivityTasks()/Timers() ファクトリメソッド）
- [x] internal/adapter/postgres/event.go（Append で sequence_num を自動採番、GetByWorkflowID）
- [x] internal/adapter/postgres/workflow_task.go（Enqueue, Poll with SKIP LOCKED, Complete, GetByID with Advisory Lock, RecoverStaleTasks）
- [x] internal/adapter/postgres/activity_task.go（Enqueue, Poll with SKIP LOCKED + timeout_at 設定, Complete, GetByID, GetTimedOut, Requeue, RecoverStaleTasks）
- [x] internal/adapter/postgres/workflow.go（Create, Get で ErrWorkflowNotFound 返却, UpdateStatus）
- [x] internal/adapter/postgres/timer.go（Create, GetFired, MarkFired）
- [x] adapter/postgres/のインテグレーションテスト（testcontainers-go、全32テスト通過）

完了条件:

- [x] testcontainersでPostgreSQLを起動し、全リポジトリのCRUD操作がテスト通過
- [x] TxManagerで複数リポジトリ操作が1トランザクションで実行されることを確認
- [x] SKIP LOCKEDによるタスク取得の排他制御をテストで確認
- [x] 存在しないワークフローでErrWorkflowNotFoundが返ることを確認
- [x] タスクなし時にErrNoTaskAvailableが返ることを確認
- [x] timeout_atがActivity Task取得時に正しく設定されることを確認

### Sprint 3 - Engineとコマンドプロセッサ

ステータス: `完了`

ゴール: engine/のビジネスロジックを実装し、StartWorkflowからコマンド処理までの一連のフローを動作させる

タスク:

- [x] internal/port/repository.go にDeleteByWorkflowIDを追加（EventRepository, WorkflowTaskRepository, ActivityTaskRepository, TimerRepository）
- [x] adapter/postgres/ にDeleteByWorkflowID実装（event.go, workflow_task.go, activity_task.go, timer.go）
- [x] adapter/postgres/workflow.go のCreateをupsert化（ON CONFLICT + terminal状態チェック → ErrWorkflowAlreadyExists）
- [x] adapter/postgres/ の新規テスト追加（DeleteByWorkflowID×4, Create upsert×2、計38テスト通過）
- [x] internal/engine/engine.go
  - port.ClientService 実装: StartWorkflow（冪等性チェック + ID自動生成 + 終了済みワークフローの関連データ削除→再作成）、DescribeWorkflow、TerminateWorkflow（状態チェック + ErrWorkflowNotRunning）、GetWorkflowHistory
  - port.WorkflowTaskService 実装: PollWorkflowTask（ErrNoTaskAvailable→nil,nil）、CompleteWorkflowTask（Advisory Lock + taskID→workflowID解決）、FailWorkflowTask
  - port.ActivityTaskService 実装: PollActivityTask、CompleteActivityTask（ワークフロー状態チェック）、FailActivityTask（non_retryable + リトライ判定 + ワークフロー状態チェック）
  - var _ port.ClientService = (*Engine)(nil) 等のコンパイル時保証
- [x] internal/engine/command_processor.go
  - processScheduleActivity: TaskQueue未指定時にワークフローのTaskQueueを使用、RetryPolicy/Timeout伝搬、デフォルトMaxAttempts=1
  - processCompleteWorkflow, processFailWorkflow
  - 未知のCommandTypeでエラーを返す
- [x] internal/engine/background.go（BackgroundWorker: RunActivityTimeoutChecker, RunTaskRecovery。Engineとは別構造体）
- [x] internal/engine/retry.go（指数バックオフリトライ、computeNextRetryTime、MaxIntervalキャップ）
- [x] engineのユニットテスト（手書きモック構造体でロジックを検証、35テスト通過）

実装上の設計判断:

- CommandProcessorを独立構造体にせず、Engineのメソッドとして統合（processCommandsがCompleteWorkflowTask内から呼ばれるため、同一トランザクション内で自然に動作）
- PollWorkflowTask/PollActivityTaskはErrNoTaskAvailableをengine層で(nil, nil)に変換し、gRPCハンドラの簡素化に備える
- StartWorkflowで終了済みワークフロー再作成時に旧events/tasks/timersを削除してからCreate（upsert）

完了条件:

- [x] StartWorkflowで冪等性チェック + イベント記録 + タスク投入が1トランザクションで実行される
- [x] 同一IDで2回StartWorkflow → ErrWorkflowAlreadyExists
- [x] 終了済みIDで再度StartWorkflow → 新規作成成功（関連データ削除→再作成）
- [x] TerminateWorkflowで状態チェック → RUNNING以外は ErrWorkflowNotRunning
- [x] CompleteWorkflowTaskでtaskIDからworkflowIDを解決し、コマンド→イベント変換が正しく動作する
- [x] CompleteActivityTaskでワークフローがTERMINATED済みの場合、結果が破棄されタスクだけ完了する
- [x] FailActivityTaskでnon_retryable=trueの場合は即座にActivityTaskFailed
- [x] FailActivityTaskでリトライ可能な場合はRequeueされる
- [x] BackgroundWorkerのRunActivityTimeoutCheckerのロジックがテスト通過
- [x] go build ./cmd/dandori, go vet ./... がクリーン

### Sprint 4 - gRPCハンドラとサーバー起動

ステータス: `完了`

ゴール: gRPCサーバーが起動し、全APIが呼び出し可能な状態にする

タスク:

- [x] internal/port/service.go に WorkflowType string を WorkflowTaskResult に追加
- [x] internal/engine/engine.go の PollWorkflowTask でワークフロー取得し WorkflowType を設定
- [x] internal/engine/engine_test.go の PollWorkflowTask テストで WorkflowType を検証
- [x] internal/adapter/grpc/handler.go
  - NewHandler(client, wfTask, actTask) で役割別インターフェースを受け取る
  - domainErrorToGRPC() でドメインエラー→gRPCステータス変換を一元化（engine/に依存しない）
  - 全10メソッドのハンドラ実装
  - 型変換ヘルパー: workflowStatusToProto, domainWorkflowToProto, domainEventsToProto, protoCommandsToDomain, commandTypeFromProto
  - PollWorkflowTask/PollActivityTask: nil結果 → 空レスポンス（エラーではない）
- [x] internal/adapter/postgres/migrate.go（embed.FSによるマイグレーションランナー、冪等性あり）
- [x] cmd/dandori/main.go
  - 環境変数: DATABASE_URL, GRPC_PORT
  - DB接続 + ping + プール設定
  - embed.FSマイグレーション実行
  - DI: adapter/postgres → engine.New + engine.NewBackgroundWorker → adapter/grpc.NewHandler(eng, eng, eng)
  - gRPCサーバー起動（reflection.Register でgRPCurl対応）
  - BackgroundWorker.RunActivityTimeoutChecker(5s), RunTaskRecovery(10s) をgoroutineで起動
  - graceful shutdown（SIGINT/SIGTERM → context cancel → GracefulStop → db.Close）
- [x] go build ./cmd/dandori, go vet ./..., go test ./internal/engine/... がクリーン

実装上の設計判断:

- マイグレーションに golang-migrate を使わず embed.FS + 冪等チェック（information_schema.tables）で実現。外部依存を削減
- PollWorkflowTask で WorkflowType を返すために、engine 層でワークフロー取得を追加。PollWorkflowTaskResponse の workflow_type フィールドを活用
- gRPC reflection を有効化し、grpcurl でのデバッグを容易にした

完了条件:

- [x] サーバーが起動し、gRPCurlでStartWorkflow → DescribeWorkflowが動作する
- [x] 同一IDでStartWorkflow 2回 → ALREADY_EXISTS (gRPC)
- [x] 存在しないIDでDescribeWorkflow → NOT_FOUND (gRPC)
- [x] COMPLETED状態のWFにTerminateWorkflow → FAILED_PRECONDITION (gRPC)
- [x] PollWorkflowTaskでタスクなし → 空レスポンス（エラーなし）
- [x] FailWorkflowTask, FailActivityTask（non_retryable含む）が正常に動作する
- [x] adapter/grpc/ が engine/ を import していないことを確認

### Sprint 5 - テストと品質

ステータス: `完了`

ゴール: サーバー側のテストスイートを整備し、Go SDKとのE2E検証に備える

タスク:

- [x] internal/adapter/grpc/mock_test.go（mockClientService, mockWorkflowTaskService, mockActivityTaskService — 関数フィールド型モック）
- [x] internal/adapter/grpc/handler_test.go — gRPCハンドラのユニットテスト
  - TestErrorMapping（7サブテスト）: 各ドメインエラーがハンドラ経由で正しいgRPCステータスに変換されることを検証（ErrWorkflowNotFound→NotFound, ErrWorkflowAlreadyExists→AlreadyExists, ErrWorkflowNotRunning→FailedPrecondition, ErrTaskNotFound→NotFound, ErrTaskAlreadyCompleted→FailedPrecondition, unknown→Internal, wrapped error→unwrapping確認）
  - TestStartWorkflow_InvalidUUID → codes.InvalidArgument
  - TestPollWorkflowTask_NoTask → 空レスポンス（エラーなし）
  - TestPollActivityTask_NoTask → 空レスポンス（エラーなし）
  - TestCompleteWorkflowTask_InvalidCommand → codes.InvalidArgument
- [x] internal/adapter/grpc/testhelper_test.go — testcontainers postgres:16-alpine セットアップ、postgres.RunMigrations()による冪等マイグレーション、newTestHandler()ヘルパー（postgres.New → engine.New → grpc.NewHandler）
- [x] internal/adapter/grpc/integration_test.go — gRPCインテグレーションテスト（Engine + PostgreSQL Storeの実スタック）
  - TestIntegration_StartWorkflow_PollComplete: StartWorkflow → PollWT → CompleteWT(CompleteWorkflow) → Describe=COMPLETED
  - TestIntegration_ActivityFlow: StartWorkflow → PollWT → CompleteWT(ScheduleActivity) → PollAT → CompleteAT → PollWT → CompleteWT(CompleteWorkflow) → COMPLETED
  - TestIntegration_TerminateWorkflow: StartWorkflow → Terminate → Describe=TERMINATED + History確認
  - TestIntegration_FailWorkflowTask: StartWorkflow → PollWT → FailWT → Describe=FAILED
  - TestIntegration_StartWorkflow_Duplicate: 同一ID2回 → AlreadyExists
  - TestIntegration_DescribeWorkflow_NotFound → NotFound
  - TestIntegration_PollWorkflowTask_Empty → 空レスポンス
- [x] internal/adapter/postgres/advisory_lock_test.go — Advisory Lockテスト
  - TestAdvisoryLock_SameWorkflow_Serialized: 同一workflowの2 taskが直列化されることをタイムスタンプ差で検証
  - TestAdvisoryLock_DifferentWorkflows_Concurrent: 異なるworkflowのtaskが並行実行可能なことを総実行時間で検証
  - TestAdvisoryLock_NoLockOutsideTransaction: トランザクション外のGetByIDではロックなしで成功
- [x] .github/workflows/ci.yml — GitHub Actions CI（go vet, go build, go test -v -race -count=1, adapter/grpc→engine依存チェック）

実装上の設計判断:

- bufconn不使用: gRPCトランスポート層のテストは不要。ハンドラメソッド直接呼び出しで十分
- 全テスト `grpc_test` パッケージ: TestMainが1つのみ必要なため統一。domainErrorToGRPC（unexported）はハンドラメソッド経由で間接テスト
- マイグレーション: `postgres.RunMigrations()` を再利用（embed.FS活用、SQLファイル直接読み込みではなく）
- Advisory Lockテスト: time.Sleep(200ms) + タイムスタンプ比較で直列化を検証。異なるworkflowのテストは総実行時間で並行性を検証

完了条件:

- [x] `go vet ./...` がクリーン
- [x] `go test -v -race ./internal/adapter/grpc/...` — ユニット（11テスト）+ インテグレーション（7テスト）全通過
- [x] `go test -v -race ./internal/adapter/postgres/...` — Advisory Lock含め全通過（41テスト）
- [x] `go test -v -race ./...` — 全テスト通過（engine 35 + postgres 41 + grpc 18 = 94テスト）
- [x] `go list -f '{{ join .Imports "\n" }}' ./internal/adapter/grpc/` に engine がないことを確認
- [x] CI設定が正しいYAML構造であることを確認

### E2Eテスト（サーバー単体）

ステータス: `完了`

ゴール: Go SDK未完成の段階で、生gRPCクライアントによるワーカーシミュレーションでサーバーMVPの全主要シナリオをE2E検証する

ブランチ: `feature/e2e`

設計判断:

- bufconn使用: `google.golang.org/grpc/test/bufconn` でポート競合なし・高速なgRPCスタック経由テスト（既存インテグレーションテストはハンドラ直接呼び出し）
- BackgroundWorker高速化: timeout_checker=500ms, task_recovery=2s（本番は5s/10s）
- テスト分離: 各テストで `truncateAll()` 実行、Sequential実行（t.Parallel()不使用）
- ワーカーシミュレーション: `pollWorkflowTaskUntil` / `pollActivityTaskUntil` ヘルパーで共通化

タスク:

- [x] test/e2e/setup_test.go — TestMain（testcontainers postgres + bufconn gRPCサーバー + BackgroundWorker）、ヘルパー関数群
- [x] test/e2e/sequential_activity_test.go（シナリオ1,2）
  - TestE2E_ThreeStepSequentialActivity: 3ステップActivity順次実行 → COMPLETED、8イベント検証
  - TestE2E_ResultRetrieval: CompleteWorkflow結果が DescribeWorkflow で取得できること
- [x] test/e2e/terminate_test.go（シナリオ8,9）
  - TestE2E_TerminateRunningWorkflow: Activity中のTerminate → TERMINATED + 履歴イベント確認
  - TestE2E_TerminatedActivityResultDiscarded: Terminate後のActivity完了が破棄、WT再発行なし
- [x] test/e2e/nondeterminism_test.go（シナリオ10）
  - TestE2E_NonDeterminismFailWorkflowTask: replay時FailWorkflowTask → FAILED + WorkflowExecutionFailed イベント
- [x] test/e2e/retry_test.go（シナリオ5,6）
  - TestE2E_ActivityRetry: 3回リトライ（attempt 1,2 fail → attempt 3 complete） → COMPLETED
  - TestE2E_NonRetryableFailure: non_retryable=true で即座にActivityTaskFailed → FailWorkflow → FAILED
- [x] test/e2e/replay_test.go（シナリオ3）
  - TestE2E_WorkerRestartReplay: WT取得後未完了 → ロック失効 → RunTaskRecovery回復 → 別ワーカーがreplay（全履歴確認） → COMPLETED
- [x] test/e2e/concurrent_poll_test.go（シナリオ4）
  - TestE2E_MultipleWorkersNoDuplicate: 5ワークフロー × 3ワーカー並行Poll → 重複なし（SKIP LOCKED検証）
- [x] test/e2e/timeout_test.go（シナリオ7）
  - TestE2E_ActivityTimeout: start_to_close_timeout=1s → Activity未完了 → BackgroundWorkerがActivityTaskTimedOut検知 → 新WT発行

完了条件:

- [x] `go test -v -race -count=1 ./test/e2e/...` — 10テスト全通過
- [x] `go test -v -race ./...` — 既存テスト含め全通過（engine 35 + postgres 41 + grpc 18 + e2e 10 = 104テスト）
- [x] `go build ./cmd/dandori` — ビルド成功
- [x] `go vet ./...` — クリーン

### E2E検証（Go SDKリポジトリと合同）

Go SDKリポジトリの開発完了後、SDK経由でのE2E検証を実施する:

- client.ExecuteWorkflow → WorkflowRun.Get() で結果が取得できる
- SDK経由のdeterministic replayが正しく動作する

---

## Phase 2: 信頼性と機能拡張

### Sprint 6 - Timer / Sleep

ステータス: `未着手`

ゴール: ワークフロー内でタイマー（Sleep）を開始・キャンセルできるようにし、バックグラウンドポーラーで発火を検知する

設計判断:

- `TimerRepository.Cancel` を新設し、`MarkFired` の戻り値を `error` → `(bool, error)` に変更。`WHERE status = 'PENDING'` ガードで二重発火を防止
- `BackgroundWorker` に `timers port.TimerRepository` フィールドを追加し、`NewBackgroundWorker` シグネチャを変更
- CancelTimer時にTimerが既にFIRED → no-op（TimerCanceledイベントを記録しない）
- Engine command processor に `processStartTimer`, `processCancelTimer` を追加

タスク:

- [ ] internal/domain/event.go — 新EventType追加: `TimerStarted`, `TimerFired`, `TimerCanceled`
- [ ] internal/domain/command.go — 新CommandType追加: `StartTimer`, `CancelTimer`。`StartTimerAttributes`（timer_id, duration）、`CancelTimerAttributes`（timer_id）を定義
- [ ] internal/port/repository.go — `TimerRepository` に `Cancel(ctx, workflowID, seqID) (bool, error)` 追加。`MarkFired` 戻り値を `(bool, error)` に変更
- [ ] internal/adapter/postgres/timer.go — `Cancel` 実装（`UPDATE timers SET status = 'CANCELED' WHERE ... AND status = 'PENDING'`）。`MarkFired` に `WHERE status = 'PENDING'` ガード追加、戻り値 `(bool, error)` 化
- [ ] internal/engine/command_processor.go — `processStartTimer`: Timer作成 + TimerStartedイベント記録。`processCancelTimer`: Timer Cancel呼び出し、成功時のみTimerCanceledイベント記録
- [ ] internal/engine/background.go — `BackgroundWorker` に `timers` フィールド追加。`NewBackgroundWorker(txm, wfTasks, actTasks, timers, events, workflows)` シグネチャ変更。`RunTimerPoller(ctx, interval)` 追加（1秒間隔でGetFired → MarkFired → TimerFiredイベント記録 + WorkflowTask投入）
- [ ] cmd/dandori/main.go — `NewBackgroundWorker` 呼び出しにTimerRepository追加。`RunTimerPoller` goroutine起動
- [ ] api/v1/types.proto — `CommandType` に `START_TIMER = 4`, `CANCEL_TIMER = 5` 追加。`StartTimerAttributes`, `CancelTimerAttributes` メッセージ追加。`Command` に属性フィールド追加
- [ ] internal/adapter/grpc/handler.go — `commandTypeFromProto`, `protoCommandsToDomain` にTimer系コマンドのマッピング追加
- [ ] internal/adapter/postgres/timer_test.go — Cancel/MarkFiredの新テスト追加
- [ ] internal/engine/engine_test.go — processStartTimer, processCancelTimer のユニットテスト追加
- [ ] test/e2e/ — Timer発火シナリオ、Timerキャンセルシナリオの E2Eテスト追加

完了条件:

- [ ] StartTimerコマンド → Timer作成 → TimerPoller発火 → TimerFiredイベント + WorkflowTask投入
- [ ] CancelTimerコマンド → Timer CANCELED → TimerCanceledイベント記録
- [ ] 既にFIREDのTimerに対するCancelTimer → no-op（イベント記録なし）
- [ ] MarkFiredが二重発火しないこと（`WHERE status = 'PENDING'` ガード）
- [ ] `go test -v -race ./...` — 全テスト通過
- [ ] `go vet ./...` — クリーン

### Sprint 7 - Signal / Channel

ステータス: `未着手`

ゴール: 外部からワークフローにシグナルを送信し、ワークフロー側でシグナルを受信・処理できるようにする

設計判断:

- シグナルは `WorkflowSignaled` イベントとして記録し、同時にWorkflowTaskを投入してワーカーに通知
- 非RUNNINGワークフローへのSignal → `ErrWorkflowNotRunning`
- 各Signalで独立したイベント + WorkflowTask を生成

タスク:

- [ ] api/v1/service.proto — `SignalWorkflow` RPC追加。`SignalWorkflowRequest`（workflow_id, signal_name, input）、`SignalWorkflowResponse` メッセージ追加
- [ ] api/v1/types.proto — `EventType` に `WORKFLOW_SIGNALED` 追加。`WorkflowSignaledAttributes`（signal_name, input）メッセージ追加
- [ ] internal/domain/event.go — `EventType` に `WorkflowSignaled` 追加
- [ ] internal/port/service.go — `ClientService` に `SignalWorkflow(ctx, workflowID, signalName, input) error` 追加
- [ ] internal/engine/engine.go — `SignalWorkflow` 実装: ワークフロー取得 → RUNNING確認 → WorkflowSignaledイベント記録 → WorkflowTask投入
- [ ] internal/adapter/grpc/handler.go — `SignalWorkflow` ハンドラ実装。`domainEventsToProto` にWorkflowSignaled変換追加
- [ ] internal/engine/engine_test.go — SignalWorkflow: 正常系、非RUNNING時のエラー、複数Signal連続送信のテスト
- [ ] internal/adapter/grpc/handler_test.go — SignalWorkflow ハンドラのユニットテスト
- [ ] test/e2e/ — Signal送信 → WorkflowTask受信 → 処理完了のE2Eテスト

完了条件:

- [ ] SignalWorkflow → WorkflowSignaledイベント記録 + WorkflowTask投入
- [ ] 非RUNNINGワークフローへのSignal → FAILED_PRECONDITION (gRPC)
- [ ] 複数Signal連続送信で各シグナルが独立したイベント・タスクとして処理される
- [ ] `go test -v -race ./...` — 全テスト通過
- [ ] `go vet ./...` — クリーン

### Sprint 8 - CancelWorkflow + Parallel Activity + Heartbeat

ステータス: `未着手`

ゴール: ワークフローのグレースフルキャンセル、並行Activity実行の検証、Activityハートビートの実装

設計判断:

- CancelWorkflow: graceful方式。CancelRequestedイベントを記録しWorkflowTaskを投入するが、ワークフロー状態は変更しない（ワーカー側でキャンセル処理を実装）
- Parallel Activity: サーバー側の変更不要（既存の仕組みで複数ActivityをCompleteWorkflowTaskで同時にスケジュール可能）。E2E検証のみ
- RecordActivityHeartbeat: migration 000002 で `heartbeat_at`, `heartbeat_timeout` カラム追加。Poll時に `heartbeat_at` を初期化、RecordHeartbeat で更新
- BackgroundWorker: heartbeat_timeout超過チェックをActivityTimeoutCheckerに統合

タスク:

- [ ] api/v1/service.proto — `CancelWorkflow` RPC追加。`CancelWorkflowRequest`（workflow_id）、`CancelWorkflowResponse` メッセージ追加。`RecordActivityHeartbeat` RPC追加。`RecordActivityHeartbeatRequest`（task_id, details）、`RecordActivityHeartbeatResponse` メッセージ追加
- [ ] api/v1/types.proto — `EventType` に `WORKFLOW_CANCEL_REQUESTED` 追加。`CommandType` に `REQUEST_CANCEL_ACTIVITY = 6` 追加。`ScheduleActivityTaskAttributes` に `heartbeat_timeout` フィールド追加
- [ ] internal/domain/event.go — `EventType` に `WorkflowCancelRequested` 追加
- [ ] internal/domain/task.go — `ActivityTask` に `HeartbeatAt`, `HeartbeatTimeout` フィールド追加
- [ ] internal/adapter/postgres/migration/000002_heartbeat.up.sql — `activity_tasks` に `heartbeat_at TIMESTAMPTZ`, `heartbeat_timeout INTERVAL` カラム追加（`ALTER TABLE IF EXISTS` + カラム存在チェック）
- [ ] internal/adapter/postgres/migration/000002_heartbeat.down.sql
- [ ] internal/port/service.go — `ClientService` に `CancelWorkflow(ctx, workflowID) error` 追加。`ActivityTaskService` に `RecordActivityHeartbeat(ctx, taskID, details) error` 追加
- [ ] internal/port/repository.go — `ActivityTaskRepository` に `UpdateHeartbeat(ctx, taskID) error`、`GetHeartbeatTimedOut(ctx) ([]ActivityTask, error)` 追加
- [ ] internal/adapter/postgres/activity_task.go — `UpdateHeartbeat` 実装（`UPDATE activity_tasks SET heartbeat_at = NOW()`）。`GetHeartbeatTimedOut` 実装。`Poll` で `heartbeat_at = NOW()` 初期化追加。`Enqueue` で `heartbeat_timeout` 保存追加
- [ ] internal/engine/engine.go — `CancelWorkflow` 実装: RUNNING確認 → WorkflowCancelRequestedイベント → WorkflowTask投入。`RecordActivityHeartbeat` 実装: タスク取得 → UpdateHeartbeat
- [ ] internal/engine/background.go — `RunActivityTimeoutChecker` にheartbeatタイムアウトチェック追加（GetHeartbeatTimedOut → ActivityTimedOutイベント → WorkflowTask投入）
- [ ] internal/adapter/grpc/handler.go — `CancelWorkflow`, `RecordActivityHeartbeat` ハンドラ実装
- [ ] internal/engine/engine_test.go — CancelWorkflow/RecordActivityHeartbeat ユニットテスト
- [ ] internal/adapter/postgres/activity_task_test.go — UpdateHeartbeat/GetHeartbeatTimedOut テスト
- [ ] test/e2e/ — CancelWorkflowシナリオ、並行Activity（2+ Activity同時スケジュール → 全完了）シナリオ、Heartbeatタイムアウトシナリオの E2Eテスト

完了条件:

- [ ] CancelWorkflow → WorkflowCancelRequestedイベント + WorkflowTask投入（ワークフロー状態はRUNNINGのまま）
- [ ] 非RUNNINGワークフローへのCancel → FAILED_PRECONDITION
- [ ] 並行Activity: 1回のCompleteWorkflowTaskで複数ScheduleActivityコマンド → 各Activityが独立にPoll・Complete可能
- [ ] RecordActivityHeartbeat → heartbeat_at が更新される
- [ ] HeartbeatTimeout超過 → ActivityTimedOutイベント + WorkflowTask投入
- [ ] migration 000002 が冪等に適用される（カラム存在チェック）
- [ ] `go test -v -race ./...` — 全テスト通過
- [ ] `go vet ./...` — クリーン

### Sprint 9 - 追加タイムアウト + 構造化ログ

ステータス: `未着手`

ゴール: ScheduleToCloseTimeout / ScheduleToStartTimeout を実装し、ログ出力を slog に統一する

設計判断:

- migration 000003: `activity_tasks` に `schedule_to_close_timeout/schedule_to_close_timeout_at`, `schedule_to_start_timeout/schedule_to_start_timeout_at` カラム追加
- ScheduleToCloseTimeout: Activityスケジュール時にtimeout_atを設定し、Requeue時にも持続（リトライを跨いで有効）
- ScheduleToStartTimeout: PENDINGタスクのみ対象。Poll時にtimeout_atをクリア
- slog化: `internal/` 配下の `log.Printf` を `slog.Error`/`slog.Info`/`slog.Warn` に置換

タスク:

- [ ] internal/adapter/postgres/migration/000003_activity_timeouts.up.sql — `activity_tasks` に `schedule_to_close_timeout INTERVAL`, `schedule_to_close_timeout_at TIMESTAMPTZ`, `schedule_to_start_timeout INTERVAL`, `schedule_to_start_timeout_at TIMESTAMPTZ` カラム追加（カラム存在チェック）
- [ ] internal/adapter/postgres/migration/000003_activity_timeouts.down.sql
- [ ] internal/domain/task.go — `ActivityTask` に `ScheduleToCloseTimeout`, `ScheduleToCloseTimeoutAt`, `ScheduleToStartTimeout`, `ScheduleToStartTimeoutAt` フィールド追加
- [ ] internal/domain/command.go — `ScheduleActivityTaskAttributes` に `ScheduleToCloseTimeout`, `ScheduleToStartTimeout` フィールド追加
- [ ] internal/port/repository.go — `ActivityTaskRepository` に `GetScheduleToCloseTimedOut(ctx) ([]ActivityTask, error)`, `GetScheduleToStartTimedOut(ctx) ([]ActivityTask, error)` 追加
- [ ] internal/adapter/postgres/activity_task.go — `Enqueue` で新カラム保存。`Poll` で `schedule_to_start_timeout_at = NULL` クリア。`Requeue` で `schedule_to_close_timeout_at` を維持。`GetScheduleToCloseTimedOut`, `GetScheduleToStartTimedOut` 実装
- [ ] internal/engine/command_processor.go — `processScheduleActivity` でタイムアウト値をActivityTaskに伝搬
- [ ] internal/engine/background.go — `RunActivityTimeoutChecker` にScheduleToClose/ScheduleToStartタイムアウトチェック追加
- [ ] api/v1/types.proto — `ScheduleActivityTaskAttributes` に `schedule_to_close_timeout`, `schedule_to_start_timeout` フィールド追加
- [ ] internal/adapter/grpc/handler.go — 新タイムアウトフィールドのマッピング追加
- [ ] internal/ 全体 — `log.Printf` → `slog.Error`/`slog.Info`/`slog.Warn` に置換。`cmd/dandori/main.go` で `slog.SetDefault` 設定
- [ ] internal/adapter/postgres/activity_task_test.go — 新タイムアウト関連テスト追加
- [ ] internal/engine/engine_test.go — タイムアウト伝搬テスト追加
- [ ] test/e2e/ — ScheduleToCloseTimeout / ScheduleToStartTimeout シナリオの E2Eテスト

完了条件:

- [ ] ScheduleToCloseTimeout設定 → タイムアウト超過でActivityTimedOut（リトライ跨ぎでも有効）
- [ ] ScheduleToStartTimeout設定 → PENDINGのままタイムアウト超過でActivityTimedOut
- [ ] ScheduleToStartTimeout → Poll後はtimeout_atがクリアされタイムアウトしない
- [ ] migration 000003 が冪等に適用される
- [ ] `internal/` 配下に `log.Printf` が残っていないこと
- [ ] `go test -v -race ./...` — 全テスト通過
- [ ] `go vet ./...` — クリーン

### Sprint 10 - LISTEN/NOTIFY + Sticky Execution

ステータス: `未着手`

ゴール: PostgreSQL LISTEN/NOTIFYによるリアルタイムタスク通知と、Sticky Executionによるキャッシュ効率の向上を実装する

設計判断:

- NOTIFY: タスクEnqueue時に `NOTIFY 'task:{queue_name}'` を発行。SDK側のListenerがイベントを受信してPollをトリガー
- Listener: `pq.NewListener` をラップした `postgres.Listener` を提供。SDK消費用のチャネルベースAPI
- Sticky Execution: 2段階Poll（1. sticky_worker_idが一致するタスク → 2. sticky_worker_id未設定 or sticky_timeout超過のタスク）
- migration 000004: `workflow_tasks.sticky_worker_id`, `workflow_executions.sticky_worker_id`, `workflow_executions.sticky_timeout` カラム追加

タスク:

- [ ] internal/adapter/postgres/migration/000004_sticky_notify.up.sql — `workflow_tasks` に `sticky_worker_id VARCHAR(255)` カラム追加。`workflow_executions` に `sticky_worker_id VARCHAR(255)`, `sticky_timeout INTERVAL` カラム追加（カラム存在チェック）
- [ ] internal/adapter/postgres/migration/000004_sticky_notify.down.sql
- [ ] internal/adapter/postgres/notify.go（新規）— `NotifyTaskAvailable(ctx, queueName) error`: `SELECT pg_notify('task:' || $1, '')` 実行
- [ ] internal/adapter/postgres/listener.go（新規）— `Listener` 構造体: `pq.NewListener` ラップ、`Listen(channel) error`, `NotificationChannel() <-chan *pq.Notification`, `Close() error`
- [ ] internal/adapter/postgres/workflow_task.go — `Enqueue` 後に `NotifyTaskAvailable` 呼び出し追加。`Poll` を2段階化: まず `WHERE sticky_worker_id = $worker_id` で取得、なければ `WHERE (sticky_worker_id IS NULL OR sticky_timeout超過)` で取得
- [ ] internal/adapter/postgres/activity_task.go — `Enqueue` 後に `NotifyTaskAvailable` 呼び出し追加
- [ ] internal/domain/workflow.go — `WorkflowExecution` に `StickyWorkerID`, `StickyTimeout` フィールド追加
- [ ] internal/domain/task.go — `WorkflowTask` に `StickyWorkerID` フィールド追加
- [ ] internal/port/repository.go — `WorkflowTaskRepository.Poll` シグネチャに `workerID string` パラメータ追加。`WorkflowRepository` に `UpdateStickyWorker(ctx, workflowID, workerID, timeout) error` 追加
- [ ] internal/engine/engine.go — `CompleteWorkflowTask` でstickyWorkerIDをワークフローに設定。`PollWorkflowTask` にworkerID伝搬
- [ ] internal/adapter/grpc/handler.go — PollWorkflowTaskRequestの `worker_id` フィールドを伝搬
- [ ] api/v1/service.proto — `PollWorkflowTaskRequest` に `worker_id` フィールド追加
- [ ] internal/adapter/postgres/listener_test.go — Listener接続・通知受信テスト
- [ ] internal/adapter/postgres/workflow_task_test.go — 2段階Poll・Sticky Executionテスト追加
- [ ] test/e2e/ — NOTIFY受信テスト、Sticky Execution優先Pollテスト

完了条件:

- [ ] タスクEnqueue時にNOTIFY発行され、Listenerで受信できる
- [ ] Sticky Execution: 同一workerIDのPollが優先される
- [ ] sticky_timeout超過のタスクが他ワーカーに取得される
- [ ] 2段階Pollで既存の非Stickyワークフローが引き続き動作する
- [ ] migration 000004 が冪等に適用される
- [ ] `go test -v -race ./...` — 全テスト通過
- [ ] `go vet ./...` — クリーン

### Sprint 11 - CLI + ListWorkflows

ステータス: `未着手`

ゴール: CLIツールでワークフロー操作を可能にし、ListWorkflows APIでワークフロー一覧を取得できるようにする

設計判断:

- CLI: `cmd/dandori-cli/` に新バイナリ。`spf13/cobra` を使用
- ListWorkflows: cursor-based pagination（`created_at`, `id` の組み合わせ）。フィルタ: status, workflow_type, task_queue
- CLIサブコマンド: `start`, `describe`, `terminate`, `cancel`, `signal`, `list`, `history`

タスク:

- [ ] api/v1/service.proto — `ListWorkflows` RPC追加。`ListWorkflowsRequest`（page_size, next_page_token, status_filter, type_filter, queue_filter）、`ListWorkflowsResponse`（workflows, next_page_token）メッセージ追加
- [ ] internal/port/service.go — `ClientService` に `ListWorkflows(ctx, params ListWorkflowsParams) (ListWorkflowsResult, error)` 追加。`ListWorkflowsParams`（PageSize, Cursor, StatusFilter, TypeFilter, QueueFilter）、`ListWorkflowsResult`（Workflows, NextCursor）型定義
- [ ] internal/adapter/postgres/workflow.go — `List(ctx, params) ([]WorkflowExecution, nextCursor, error)` 実装: `WHERE created_at < $cursor_time OR (created_at = $cursor_time AND id < $cursor_id) ORDER BY created_at DESC, id DESC LIMIT $page_size + 1`
- [ ] internal/engine/engine.go — `ListWorkflows` 実装: パラメータバリデーション + リポジトリ呼び出し + カーソル生成
- [ ] internal/adapter/grpc/handler.go — `ListWorkflows` ハンドラ実装。cursor encode/decode（base64 JSON）
- [ ] cmd/dandori-cli/main.go（新規）— cobra rootコマンド。`--server` フラグ（デフォルト `localhost:50051`）
- [ ] cmd/dandori-cli/cmd/start.go（新規）— `start` サブコマンド: `--id`, `--type`, `--queue`, `--input` フラグ
- [ ] cmd/dandori-cli/cmd/describe.go（新規）— `describe` サブコマンド: workflow_id引数
- [ ] cmd/dandori-cli/cmd/terminate.go（新規）— `terminate` サブコマンド: workflow_id引数、`--reason` フラグ
- [ ] cmd/dandori-cli/cmd/cancel.go（新規）— `cancel` サブコマンド: workflow_id引数
- [ ] cmd/dandori-cli/cmd/signal.go（新規）— `signal` サブコマンド: workflow_id引数、`--name`, `--input` フラグ
- [ ] cmd/dandori-cli/cmd/list.go（新規）— `list` サブコマンド: `--status`, `--type`, `--queue`, `--limit` フラグ
- [ ] cmd/dandori-cli/cmd/history.go（新規）— `history` サブコマンド: workflow_id引数
- [ ] internal/adapter/postgres/workflow_test.go — List（ページネーション、フィルタ）テスト追加
- [ ] internal/engine/engine_test.go — ListWorkflows ユニットテスト
- [ ] test/e2e/ — ListWorkflows API E2Eテスト

完了条件:

- [ ] `go build ./cmd/dandori-cli` — CLIバイナリがビルドできる
- [ ] `dandori-cli start --type MyWorkflow` → ワークフロー開始
- [ ] `dandori-cli list --status RUNNING` → RUNNINGワークフロー一覧表示
- [ ] `dandori-cli list --limit 5` → 5件取得 + next_page_token で次ページ取得可能
- [ ] ListWorkflows API: cursor-based paginationが正しく動作する
- [ ] フィルタ（status/type/queue）が正しく適用される
- [ ] `go test -v -race ./...` — 全テスト通過
- [ ] `go vet ./...` — クリーン

---

## Phase 3: 高度な機能

### Sprint 12 - Child Workflow

ステータス: `未着手`

ゴール: ワークフローから子ワークフローを起動し、子の完了/失敗が親に伝搬される仕組みを実装する

設計判断:

- 新コマンド `StartChildWorkflow` → Engine内で子ワークフロー作成 + `ChildWorkflowExecutionStarted` イベント記録
- 子ワークフロー完了/失敗時にEngine層で親ワークフローへ `ChildWorkflowExecutionCompleted` / `ChildWorkflowExecutionFailed` イベント伝搬 + WorkflowTask投入
- migration 000005: `workflow_executions` に `parent_workflow_id`, `parent_seq_id` カラム追加
- 子ワークフローの `parent_seq_id` は親の `ScheduleChildWorkflow` コマンドに対応するイベントのsequence_num

タスク:

- [ ] internal/adapter/postgres/migration/000005_child_workflow.up.sql — `workflow_executions` に `parent_workflow_id UUID REFERENCES workflow_executions(id)`, `parent_seq_id INTEGER` カラム追加（カラム存在チェック）
- [ ] internal/adapter/postgres/migration/000005_child_workflow.down.sql
- [ ] internal/domain/event.go — 新EventType追加: `ChildWorkflowExecutionStarted`, `ChildWorkflowExecutionCompleted`, `ChildWorkflowExecutionFailed`
- [ ] internal/domain/command.go — 新CommandType追加: `StartChildWorkflow`。`StartChildWorkflowAttributes`（workflow_id, workflow_type, task_queue, input）定義
- [ ] internal/domain/workflow.go — `WorkflowExecution` に `ParentWorkflowID`, `ParentSeqID` フィールド追加
- [ ] internal/port/repository.go — `WorkflowRepository` に `GetByParentWorkflowID(ctx, parentID) ([]WorkflowExecution, error)` 追加。`WorkflowRepository.Create` に `parentWorkflowID`, `parentSeqID` パラメータ追加
- [ ] internal/adapter/postgres/workflow.go — `Create` で `parent_workflow_id`, `parent_seq_id` 保存。`GetByParentWorkflowID` 実装
- [ ] internal/engine/command_processor.go — `processStartChildWorkflow`: 子ワークフロー作成（parent情報付き） + ChildWorkflowExecutionStartedイベント + 子のWorkflowTask投入
- [ ] internal/engine/engine.go — `CompleteWorkflowTask`（processCompleteWorkflow / processFailWorkflow）内で parent_workflow_id が非NULLの場合、親へイベント伝搬 + WorkflowTask投入
- [ ] api/v1/types.proto — `CommandType` に `START_CHILD_WORKFLOW = 7` 追加。`StartChildWorkflowAttributes` メッセージ追加。`EventType` に `CHILD_WORKFLOW_EXECUTION_STARTED/COMPLETED/FAILED` 追加
- [ ] internal/adapter/grpc/handler.go — ChildWorkflow関連のマッピング追加
- [ ] internal/adapter/postgres/workflow_test.go — 親子関係のCRUDテスト
- [ ] internal/engine/engine_test.go — ChildWorkflow開始・完了・失敗伝搬のユニットテスト
- [ ] test/e2e/ — 親→子→完了→親通知の E2Eテスト。子失敗→親通知のE2Eテスト

完了条件:

- [ ] StartChildWorkflowコマンド → 子ワークフロー作成 + ChildWorkflowExecutionStartedイベント
- [ ] 子ワークフロー完了 → 親に ChildWorkflowExecutionCompleted イベント + WorkflowTask投入
- [ ] 子ワークフロー失敗 → 親に ChildWorkflowExecutionFailed イベント + WorkflowTask投入
- [ ] migration 000005 が冪等に適用される
- [ ] `go test -v -race ./...` — 全テスト通過
- [ ] `go vet ./...` — クリーン

### Sprint 13 - SideEffect + Query

ステータス: `未着手`

ゴール: SideEffect（非決定的な値の記録）とQuery（ワークフロー状態の問い合わせ）を実装する

設計判断:

- SideEffect: `RecordSideEffect` コマンド → `SideEffectRecorded` イベント記録のみ（WorkflowTask生成なし）。リプレイ時にイベントから値を復元
- Query: 非同期方式。`workflow_queries` テーブルにクエリを投入 → WorkflowTask投入 → ワーカーがクエリ結果をRespondQueryTaskで返す
- QueryはRUNNINGワークフローのみ対象

タスク:

- [ ] internal/adapter/postgres/migration/000006_query.up.sql — `workflow_queries` テーブル作成（id, workflow_id, query_type, query_input, result, status, created_at, answered_at）
- [ ] internal/adapter/postgres/migration/000006_query.down.sql
- [ ] internal/domain/event.go — 新EventType追加: `SideEffectRecorded`
- [ ] internal/domain/command.go — 新CommandType追加: `RecordSideEffect`。`RecordSideEffectAttributes`（result）定義
- [ ] internal/domain/query.go（新規）— `WorkflowQuery` 型（ID, WorkflowID, QueryType, Input, Result, Status, CreatedAt, AnsweredAt）、QueryStatus（PENDING, ANSWERED）
- [ ] internal/port/service.go — `ClientService` に `QueryWorkflow(ctx, workflowID, queryType, input) (QueryResult, error)` 追加。`WorkflowTaskService` に `RespondQueryTask(ctx, queryID, result) error` 追加
- [ ] internal/port/repository.go — `QueryRepository` インターフェース追加: `Create`, `GetPendingByWorkflowID`, `SetResult`
- [ ] internal/adapter/postgres/query.go（新規）— `QueryRepository` 実装
- [ ] internal/engine/command_processor.go — `processRecordSideEffect`: SideEffectRecordedイベント記録のみ
- [ ] internal/engine/engine.go — `QueryWorkflow` 実装: RUNNING確認 → クエリ作成 → WorkflowTask投入 → ポーリングで結果待ち。`RespondQueryTask` 実装: クエリ結果設定
- [ ] api/v1/service.proto — `QueryWorkflow` RPC、`RespondQueryTask` RPC追加
- [ ] api/v1/types.proto — `CommandType` に `RECORD_SIDE_EFFECT = 8` 追加。`EventType` に `SIDE_EFFECT_RECORDED` 追加。対応属性メッセージ追加
- [ ] internal/adapter/grpc/handler.go — QueryWorkflow, RespondQueryTask ハンドラ実装。SideEffect関連マッピング追加
- [ ] internal/adapter/postgres/query_test.go — QueryRepository テスト
- [ ] internal/engine/engine_test.go — SideEffect/Query ユニットテスト
- [ ] test/e2e/ — SideEffect記録・リプレイ検証、Query送信→応答のE2Eテスト

完了条件:

- [ ] RecordSideEffectコマンド → SideEffectRecordedイベント記録（WorkflowTask未生成）
- [ ] QueryWorkflow → クエリ投入 + WorkflowTask → ワーカー応答 → 結果取得
- [ ] 非RUNNINGワークフローへのQuery → FAILED_PRECONDITION
- [ ] `go test -v -race ./...` — 全テスト通過
- [ ] `go vet ./...` — クリーン

### Sprint 14 - Cron / Schedule + Continue-as-New

ステータス: `未着手`

ゴール: Continue-as-New（ワークフローの引き継ぎ再起動）とCronスケジュールを実装する

設計判断:

- Continue-as-New: `ContinueAsNew` コマンド → 現ワークフローを `CONTINUED_AS_NEW` ステータスに変更 → 新ワークフロー作成（同一ID + 新run_id、もしくは新規ID）
- Cron: `StartWorkflowRequest.cron_schedule` に cron式を設定 → ワークフロー完了時（`processCompleteWorkflow`）に自動でContinue-as-New
- migration 000007: `workflow_executions` に `cron_schedule`, `continued_as_new_id` カラム追加

タスク:

- [ ] internal/adapter/postgres/migration/000007_cron.up.sql — `workflow_executions` に `cron_schedule VARCHAR(255)`, `continued_as_new_id UUID` カラム追加（カラム存在チェック）
- [ ] internal/adapter/postgres/migration/000007_cron.down.sql
- [ ] internal/domain/workflow.go — `WorkflowStatus` に `ContinuedAsNew` 追加。`WorkflowExecution` に `CronSchedule`, `ContinuedAsNewID` フィールド追加。`IsTerminal()` に `ContinuedAsNew` 追加
- [ ] internal/domain/command.go — 新CommandType追加: `ContinueAsNew`。`ContinueAsNewAttributes`（workflow_type, task_queue, input）定義
- [ ] internal/domain/event.go — 新EventType追加: `WorkflowExecutionContinuedAsNew`
- [ ] internal/adapter/postgres/workflow.go — `Create` で `cron_schedule` 保存。`UpdateStatus` で `continued_as_new_id` 設定対応
- [ ] internal/engine/command_processor.go — `processContinueAsNew`: 現WFをCONTINUED_AS_NEWステータス + continued_as_new_id設定 → 新WF作成 + WorkflowTask投入
- [ ] internal/engine/engine.go — `processCompleteWorkflow` 内でcron_scheduleが設定されている場合、次回実行時刻を計算して自動Continue-as-New
- [ ] internal/engine/cron.go（新規）— cron式パース + 次回実行時刻計算。外部ライブラリ（`robfig/cron/v3`）使用
- [ ] api/v1/types.proto — `CommandType` に `CONTINUE_AS_NEW = 9` 追加。`EventType` に `WORKFLOW_EXECUTION_CONTINUED_AS_NEW` 追加。`StartWorkflowRequest` に `cron_schedule` フィールド追加
- [ ] internal/adapter/grpc/handler.go — ContinueAsNew関連マッピング追加。StartWorkflowでcron_schedule伝搬
- [ ] internal/adapter/postgres/workflow_test.go — ContinuedAsNew / CronSchedule テスト
- [ ] internal/engine/engine_test.go — ContinueAsNew / Cron ユニットテスト
- [ ] test/e2e/ — ContinueAsNew手動実行、Cron自動再起動のE2Eテスト

完了条件:

- [ ] ContinueAsNewコマンド → 現WF CONTINUED_AS_NEW + 新WF作成・開始
- [ ] Cronスケジュール設定 → ワークフロー完了時に自動Continue-as-New
- [ ] CONTINUED_AS_NEW状態がIsTerminal()でtrueを返す
- [ ] migration 000007 が冪等に適用される
- [ ] `go test -v -race ./...` — 全テスト通過
- [ ] `go vet ./...` — クリーン

### Sprint 15 - HTTP API (grpc-gateway)

ステータス: `未着手`

ゴール: gRPC-Gatewayを導入し、RESTful HTTP APIとSwaggerドキュメントを提供する

設計判断:

- proto ファイルにHTTPアノテーション追加（`google.api.http`）
- `runtime.NewServeMux()` でgRPC-Gatewayマルチプレクサを生成し、HTTPサーバーで提供
- Swagger（OpenAPI）JSON自動生成、`/swagger.json` エンドポイントで提供

タスク:

- [ ] api/v1/service.proto — 各RPCに `option (google.api.http)` アノテーション追加（POST /v1/workflows, GET /v1/workflows/{id}, POST /v1/workflows/{id}/terminate, etc.）
- [ ] buf.gen.yaml / protoc設定 — `grpc-gateway`, `openapiv2` プラグイン追加
- [ ] internal/adapter/http/gateway.go（新規）— `NewGatewayMux(ctx, grpcAddr) (*runtime.ServeMux, error)`: gRPC-Gateway設定。JSON marshaler設定（EmitUnpopulated等）
- [ ] internal/adapter/http/swagger.go（新規）— embed.FSでSwagger JSON提供。`/swagger.json` ハンドラ
- [ ] cmd/dandori/main.go — HTTP_PORT環境変数追加。HTTPサーバー起動（Gateway + Swagger）。graceful shutdown統合
- [ ] api/v1/service.swagger.json（自動生成）
- [ ] test/e2e/ — HTTP API経由のStartWorkflow、DescribeWorkflow E2Eテスト

完了条件:

- [ ] `curl -X POST http://localhost:8080/v1/workflows` でワークフロー開始可能
- [ ] `curl http://localhost:8080/v1/workflows/{id}` でワークフロー詳細取得可能
- [ ] `/swagger.json` でOpenAPI仕様が取得できる
- [ ] gRPCとHTTPの両方が同時に動作する
- [ ] `go test -v -race ./...` — 全テスト通過
- [ ] `go vet ./...` — クリーン

### Sprint 16 - Observability (OpenTelemetry + Prometheus)

ステータス: `未着手`

ゴール: 分散トレーシング、メトリクス収集、ヘルスチェックを実装し、本番運用の可観測性を確保する

設計判断:

- OpenTelemetry: デコレータパターンで `port.ClientService` 等をラップ（既存コードに侵入しない）。gRPC interceptorでトレースコンテキスト伝搬
- Prometheus: メトリクスデコレータで各操作のカウンター/ヒストグラムを記録。`/metrics` エンドポイントで提供
- Health check: `grpc.health.v1.Health` サービス実装 + HTTP `/healthz` エンドポイント

タスク:

- [ ] internal/adapter/telemetry/tracer.go（新規）— OpenTelemetry TracerProvider初期化。OTLP exporter設定。環境変数による設定（`OTEL_EXPORTER_OTLP_ENDPOINT`）
- [ ] internal/adapter/telemetry/decorator.go（新規）— `TracingClientService`: `port.ClientService` ラッパー。各メソッドでspan作成、属性設定、エラー記録
- [ ] internal/adapter/telemetry/metrics.go（新規）— Prometheus metrics: `dandori_workflow_started_total`, `dandori_workflow_completed_total`, `dandori_task_poll_duration_seconds`, `dandori_active_workflows` 等
- [ ] internal/adapter/telemetry/metrics_decorator.go（新規）— `MetricsClientService`: `port.ClientService` ラッパー。各メソッドでメトリクス記録
- [ ] internal/adapter/grpc/interceptor.go（新規）— OpenTelemetry gRPC server interceptor（UnaryInterceptor, StreamInterceptor）
- [ ] internal/adapter/http/health.go（新規）— HTTP `/healthz` エンドポイント（DB ping含む）
- [ ] internal/adapter/grpc/health.go（新規）— `grpc.health.v1.Health` サービス実装
- [ ] cmd/dandori/main.go — TracerProvider初期化、デコレータ適用、interceptor設定、`/metrics` + `/healthz` ハンドラ追加
- [ ] internal/adapter/telemetry/tracer_test.go — トレーサー初期化テスト
- [ ] internal/adapter/telemetry/decorator_test.go — デコレータがspan作成することを検証
- [ ] test/e2e/ — `/healthz` レスポンス検証、`/metrics` レスポンス検証

完了条件:

- [ ] gRPC呼び出しでOpenTelemetryスパンが生成される
- [ ] Prometheusメトリクスが `/metrics` で取得できる
- [ ] `grpc.health.v1.Health.Check` が SERVING を返す
- [ ] HTTP `/healthz` が 200 OK を返す
- [ ] デコレータパターンで既存のengine/adapter層に変更がないこと
- [ ] `go test -v -race ./...` — 全テスト通過
- [ ] `go vet ./...` — クリーン

---

## Phase 4: 運用性と最適化

### Sprint 17 - Namespace（マルチテナント）

ステータス: `未着手`

ゴール: Namespaceによるマルチテナント分離を実装し、ワークフローとタスクをNamespace単位で管理する

設計判断:

- `namespaces` テーブル作成。デフォルトnamespace（`default`）を初期データとして投入
- 全テーブルに `namespace_id` カラム追加。全クエリに `WHERE namespace_id = $ns` 条件追加
- 全APIに `namespace` パラメータ追加。未指定時は `default` を使用
- CLIに `--namespace` グローバルフラグ追加

タスク:

- [ ] internal/adapter/postgres/migration/000008_namespace.up.sql — `namespaces` テーブル作成（id, name UNIQUE, description, created_at）。デフォルトnamespace投入。`workflow_executions`, `workflow_tasks`, `activity_tasks`, `timers` に `namespace_id` カラム追加 + 外部キー + インデックス
- [ ] internal/adapter/postgres/migration/000008_namespace.down.sql
- [ ] internal/domain/namespace.go（新規）— `Namespace` 型（ID, Name, Description, CreatedAt）
- [ ] internal/port/repository.go — `NamespaceRepository` インターフェース追加: `GetByName`, `Create`, `List`。全既存リポジトリメソッドに `namespaceID` パラメータ追加
- [ ] internal/adapter/postgres/namespace.go（新規）— `NamespaceRepository` 実装
- [ ] internal/adapter/postgres/workflow.go — 全クエリに `namespace_id` 条件追加
- [ ] internal/adapter/postgres/workflow_task.go — 全クエリに `namespace_id` 条件追加
- [ ] internal/adapter/postgres/activity_task.go — 全クエリに `namespace_id` 条件追加
- [ ] internal/adapter/postgres/timer.go — 全クエリに `namespace_id` 条件追加
- [ ] internal/engine/engine.go — 全メソッドでnamespace解決（名前→ID変換）を追加
- [ ] api/v1/service.proto — 全Request メッセージに `namespace` フィールド追加
- [ ] internal/adapter/grpc/handler.go — namespace伝搬追加。未指定時 `default` 設定
- [ ] cmd/dandori-cli/ — `--namespace` グローバルフラグ追加、全サブコマンドで伝搬
- [ ] internal/adapter/postgres/namespace_test.go — Namespace CRUDテスト
- [ ] internal/adapter/postgres/ 全テスト — namespace_id対応
- [ ] internal/engine/engine_test.go — namespace分離テスト
- [ ] test/e2e/ — 異なるnamespace間のワークフロー分離E2Eテスト

完了条件:

- [ ] デフォルトnamespace（`default`）で既存機能がそのまま動作する
- [ ] 異なるnamespaceのワークフローが互いに見えない
- [ ] `dandori-cli --namespace production list` でnamespace指定が動作する
- [ ] migration 000008 が冪等に適用される
- [ ] `go test -v -race ./...` — 全テスト通過
- [ ] `go vet ./...` — クリーン

### Sprint 18 - Web UI

ステータス: `未着手`

ゴール: ワークフローの一覧・詳細・履歴をブラウザで確認できるWeb UIを提供する

設計判断:

- SPA（Single Page Application）を `embed.FS` でバイナリに組み込み
- HTTP API（Sprint 15のgrpc-gateway）経由でデータ取得
- `/ui/` パスで提供。最小限の機能に絞る（閲覧のみ、操作は将来拡張）

タスク:

- [ ] web/（新規ディレクトリ）— フロントエンドプロジェクト初期化（軽量フレームワーク選定: Preact or vanilla JS + HTML templates）
- [ ] web/index.html — メインページ（ワークフロー一覧）
- [ ] web/workflow.html — ワークフロー詳細ページ（ステータス、履歴タイムライン）
- [ ] web/static/ — CSS、JavaScript
- [ ] internal/adapter/http/ui.go（新規）— `embed.FS` でweb/ディレクトリを組み込み、`/ui/` パスで提供。SPA用のフォールバックルーティング
- [ ] cmd/dandori/main.go — UIハンドラをHTTPサーバーに統合
- [ ] web/ のビルド・テスト手順をREADMEに追加

完了条件:

- [ ] `http://localhost:8080/ui/` でワークフロー一覧が表示される
- [ ] ワークフロー詳細ページでステータスと履歴が表示される
- [ ] バイナリ単体でUI含めてデプロイ可能（外部ファイル不要）
- [ ] `go build ./cmd/dandori` — UI組み込みでビルド成功
- [ ] `go vet ./...` — クリーン

### Sprint 19 - パフォーマンス最適化

ステータス: `未着手`

ゴール: 大規模ワークフロー環境でのパフォーマンスを改善し、ベンチマークで性能特性を可視化する

設計判断:

- `workflow_events` テーブルのハッシュパーティショニング（workflow_idベース、16分割）で大量イベントの読み書きを高速化
- ベンチマークテストで定量的な性能測定を実施
- pprofエンドポイントで実行時プロファイリングを可能にする

タスク:

- [ ] internal/adapter/postgres/migration/000009_partitioning.up.sql — `workflow_events` テーブルをハッシュパーティショニングに変換（16分割）。既存データの移行SQL
- [ ] internal/adapter/postgres/migration/000009_partitioning.down.sql
- [ ] internal/adapter/postgres/event.go — パーティション対応のクエリ最適化（`WHERE workflow_id = $1` がパーティションプルーニングに効くことを確認）
- [ ] test/bench/workflow_bench_test.go（新規）— ベンチマークテスト: ワークフロー作成スループット、イベント追記スループット、タスクPoll/Completeレイテンシ
- [ ] test/bench/concurrent_bench_test.go（新規）— 並行ベンチマーク: N並行ワーカーでのタスクスループット
- [ ] cmd/dandori/main.go — `net/http/pprof` エンドポイント追加（`/debug/pprof/`）。環境変数 `ENABLE_PPROF=true` でのみ有効化
- [ ] internal/adapter/postgres/ — N+1クエリの検出と最適化（必要に応じてバッチクエリ化）

完了条件:

- [ ] `go test -bench=. ./test/bench/...` — ベンチマーク実行可能
- [ ] パーティショニング後も全既存テストが通過する
- [ ] `/debug/pprof/` でプロファイリングデータが取得できる（ENABLE_PPROF=true時）
- [ ] migration 000009 が冪等に適用される
- [ ] `go test -v -race ./...` — 全テスト通過
- [ ] `go vet ./...` — クリーン

### Sprint 20 - ドキュメント整備

ステータス: `未着手`

ゴール: ユーザー・開発者・運用者向けの包括的なドキュメントを整備し、プロジェクトの利用・貢献・運用を容易にする

設計判断:

- ドキュメントは `docs/` ディレクトリに Markdown で管理
- ユーザードキュメント、開発者ドキュメント、運用ドキュメントの3カテゴリに分類
- コード例はリポジトリ内の実コードから抽出し、常に最新を維持

タスク:

- [ ] docs/getting-started.md（新規）— クイックスタートガイド: Docker Compose起動、最初のワークフロー実行、CLIの使い方
- [ ] docs/concepts.md（新規）— コンセプトガイド: ワークフロー、アクティビティ、タスクキュー、シグナル、タイマー、子ワークフロー等の概念説明
- [ ] docs/cli-reference.md（新規）— CLIリファレンス: 全サブコマンド・フラグの説明と使用例
- [ ] docs/api-reference.md（新規）— API リファレンス: 全gRPC RPC / HTTP APIの仕様（Swagger JSONへのリンク含む）
- [ ] docs/configuration.md（新規）— 設定リファレンス: 環境変数一覧、デフォルト値、推奨設定
- [ ] docs/architecture.md（新規）— アーキテクチャガイド: Hexagonal Architecture、ディレクトリ構成、データフロー図
- [ ] docs/contributing.md（新規）— コントリビューションガイド: 開発環境構築、テスト実行、コーディング規約、PR手順
- [ ] docs/sdk-guide.md（新規）— SDK開発ガイド: Go SDK連携方法、カスタムSDK開発のためのプロトコル仕様
- [ ] docs/deployment.md（新規）— デプロイメントガイド: Docker、Kubernetes、バイナリ直接デプロイ
- [ ] docs/monitoring.md（新規）— 監視ガイド: Prometheus/Grafanaダッシュボード設定、アラート設定例
- [ ] docs/troubleshooting.md（新規）— トラブルシューティング: よくある問題と解決方法
- [ ] README.md — 更新: 各ドキュメントへのリンク、バッジ、簡潔なプロジェクト説明

完了条件:

- [ ] 全ドキュメントファイルが作成され、Markdown構文エラーがない
- [ ] getting-started.mdの手順に従って新規ユーザーがワークフロー実行まで到達できる
- [ ] READMEから全ドキュメントへのリンクが有効
- [ ] コード例が実際のリポジトリコードと整合している
- [ ] `docs/` 配下の全 `.md` ファイルが目次リンクで相互参照されている
