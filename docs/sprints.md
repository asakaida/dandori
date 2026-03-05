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

ステータス: `未着手`

ゴール: ビルド・起動・DB接続ができるプロジェクト骨格を構築する

タスク:

- [ ] go mod init, .gitignore
- [ ] docker-compose.yml（PostgreSQL）
- [ ] api/v1/service.proto, types.proto の定義
- [ ] protoc-gen-goによるコード生成の確認
- [ ] internal/adapter/postgres/migration/000001_initial.up.sql（4テーブル作成）
- [ ] internal/adapter/postgres/migration/000001_initial.down.sql
- [ ] golang-migrateでマイグレーション実行確認

完了条件:

- PostgreSQLが起動し、マイグレーションが通る
- gRPCコードが生成される
- `go build ./cmd/dandori` でサーバーバイナリがビルドできる

### Sprint 2 - ドメインモデルとストア層

ステータス: `未着手`

ゴール: domain/の型定義、port/のインターフェース定義、adapter/postgres/のPostgreSQL実装を完成させる

タスク:

- [ ] internal/domain/ の型定義（Event, Command, Task, Timer, WorkflowExecution）
- [ ] internal/port/service.go（Inbound Port: WorkflowService, StartWorkflowParams, WorkflowTask）
- [ ] internal/port/repository.go（Outbound Port: WorkflowRepository, EventRepository, TaskRepository, TimerRepository, TxManager）
- [ ] internal/adapter/postgres/store.go（コネクションプール、TxManager、context経由のトランザクション伝搬）
- [ ] internal/adapter/postgres/event.go（Append, GetByWorkflowID, GetNextSequenceNum）
- [ ] internal/adapter/postgres/task.go（Enqueue, Poll with SKIP LOCKED, Complete）
- [ ] internal/adapter/postgres/workflow.go（Create, Get, UpdateStatus）
- [ ] adapter/postgres/のユニットテスト（testcontainers-go）

完了条件:

- testcontainersでPostgreSQLを起動し、全リポジトリのCRUD操作がテスト通過
- TxManagerで複数リポジトリ操作が1トランザクションで実行されることを確認
- SKIP LOCKEDによるタスク取得の排他制御をテストで確認

### Sprint 3 - Engineとコマンドプロセッサ

ステータス: `未着手`

ゴール: engine/のビジネスロジックを実装し、StartWorkflowからコマンド処理までの一連のフローを動作させる

タスク:

- [ ] internal/engine/engine.go（port.WorkflowServiceを実装するEngine struct、StartWorkflow, GetWorkflow, CompleteWorkflowTask, CompleteActivityTask, FailActivityTask）
- [ ] internal/engine/command_processor.go（ScheduleActivityTask, CompleteWorkflow, FailWorkflowの処理）
- [ ] internal/engine/retry.go（固定間隔リトライポリシー）
- [ ] engineのユニットテスト（port/のインターフェースをモックしてロジックを検証）

完了条件:

- StartWorkflowでイベント記録 + タスク投入が1トランザクションで実行される
- CompleteWorkflowTaskでコマンド→イベント変換 + タスク生成が正しく動作する
- リトライポリシーのロジックがテスト通過

### Sprint 4 - gRPCハンドラとサーバー起動

ステータス: `未着手`

ゴール: gRPCサーバーが起動し、全APIが呼び出し可能な状態にする

タスク:

- [ ] internal/adapter/grpc/handler.go（proto型 ↔ domain型の変換、port.WorkflowServiceインターフェース経由で委譲）
- [ ] cmd/dandori/main.go（DI: adapter/postgres → engine → port.WorkflowService → adapter/grpc、gRPCサーバー起動、graceful shutdown）
- [ ] PollWorkflowTask（イベント履歴を添付して返す）
- [ ] PollActivityTask
- [ ] GetWorkflowHistory
- [ ] gRPCurlで全APIの動作確認

完了条件:

- サーバーが起動し、gRPCurlでStartWorkflow → GetWorkflowExecutionが動作する
- PollWorkflowTaskでイベント履歴付きのWorkflow Taskが取得できる
- CompleteActivityTask後にWorkflow Taskが自動生成される

### Sprint 5 - テストと品質

ステータス: `未着手`

ゴール: サーバー側のテストスイートを整備し、Go SDKとのE2E検証に備える

タスク:

- [ ] adapter/grpc経由のインテグレーションテスト（StartWorkflow → Poll → Complete のフロー）
- [ ] engine/command_processorのエッジケーステスト
- [ ] engine/retryのテスト（成功、全リトライ失敗）
- [ ] adapter/postgres内のAdvisory Lockによるワークフロー直列化のテスト
- [ ] CI設定（GitHub Actions: lint, test, build）

完了条件:

- `go test ./...` が全件通過
- testcontainersを使用したインテグレーションテストが通過
- CIが緑

### E2E検証（Go SDKリポジトリと合同）

Sprint 5完了後、Go SDKリポジトリの開発と合わせてE2E検証を実施する:

- サーバー + ワーカー起動でサンプルワークフロー（3ステップ順次Activity）が完了する
- ワーカー強制停止 → 再起動でreplayが正しく動作する
- 複数ワーカー起動でタスク重複実行が起きない

---

## Phase 2: 信頼性と機能拡張

Sprint構成はPhase 1完了後に詳細化する。想定Sprint:

- Sprint 6: Timer / Sleep
- Sprint 7: Signal / Channel
- Sprint 8: 並行Activity対応（サーバー側）
- Sprint 9: ワークフローキャンセル、ハートビート
- Sprint 10: LISTEN/NOTIFY、sticky execution対応
- Sprint 11: CLIツール、構造化ログ整備

## Phase 3: 高度な機能

Sprint構成はPhase 2完了後に詳細化する。

## Phase 4: 運用性と最適化

Sprint構成はPhase 3完了後に詳細化する。
