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

### E2E検証（Go SDKリポジトリと合同）

Sprint 5完了後、Go SDKリポジトリの開発と合わせてE2E検証を実施する:

- サーバー + ワーカー起動でサンプルワークフロー（3ステップ順次Activity）が完了する
- client.ExecuteWorkflow → WorkflowRun.Get() で結果が取得できる
- ワーカー強制停止 → 再起動でreplayが正しく動作する
- 複数ワーカー起動でタスク重複実行が起きない
- Activity失敗時のリトライが正しく動作する（non_retryable=trueで即座に失敗）
- Activityタイムアウトが検知されてワークフローに通知される
- TerminateWorkflowで実行中ワークフローが即座に終了する
- TERMINATED後のActivity完了が正しく破棄される
- ワークフロー関数変更後のreplayで非決定性エラーがFailWorkflowTaskで報告される

---

## Phase 2: 信頼性と機能拡張

Sprint構成はPhase 1完了後に詳細化する。想定Sprint:

- Sprint 6: Timer / Sleep
- Sprint 7: Signal / Channel
- Sprint 8: 並行Activity対応（サーバー側）
- Sprint 9: ワークフローキャンセル（CancelWorkflow API）、ハートビート（RecordActivityHeartbeat API）
- Sprint 10: LISTEN/NOTIFY、sticky execution対応
- Sprint 11: CLIツール、ListWorkflows API、構造化ログ整備

## Phase 3: 高度な機能

Sprint構成はPhase 2完了後に詳細化する。

## Phase 4: 運用性と最適化

Sprint構成はPhase 3完了後に詳細化する。
