# dandori - 設計書

## 1. アーキテクチャ概要

### 全体構成

```text
Client (Go SDK / CLI)          HTTP Client (curl, ブラウザ等)
    |                               |
    | gRPC                          | HTTP/JSON (grpc-gateway)
    v                               v
dandori-server (本リポジトリ)
    |
    | PostgreSQL
    v
PostgreSQL
    |
    v
dandori-worker (Go SDK側で実装, 1..N instances)
```

### サーバーの責務

サーバーはワークフローのステップ定義を理解しない。以下に徹する:

1. イベントの永続化（append-only）
2. ワーカーから返されたコマンドの処理（コマンド→イベント変換）
3. タスクキューの管理（Workflow Task + Activity Taskの2種類）
4. タイマーの管理（発火時刻到達の検知）
5. 外部APIの提供（StartWorkflow, DescribeWorkflow, SignalWorkflow, CancelWorkflow等）
6. Activityタイムアウトの監視（StartToCloseTimeout / ScheduleToCloseTimeout / ScheduleToStartTimeout超過の検知）
7. Activityハートビートの管理（heartbeat_at更新、heartbeat_timeout超過の検知）
8. 子ワークフローの管理（親子関係の記録、子の完了/失敗時に親へのイベント伝搬）
9. Query管理（workflow_queriesテーブルによるクエリ投入・結果返却、PollWorkflowTaskにpending_queries含む）
10. Continue-as-New管理（ワークフロー終了→新ワークフロー自動作成、continued_as_new_id追跡）
11. Cronスケジュール管理（cron_schedule検証、CompleteWorkflow時の自動Continue-as-New）
12. HTTP API提供（grpc-gatewayによるRESTful HTTPエンドポイント、OpenAPI仕様の提供）
13. Observability提供（OpenTelemetryトレーシング、Prometheusメトリクス、gRPC/HTTPヘルスチェック、pprofプロファイリング）
14. Namespace管理（マルチテナント分離、デフォルトnamespace "default"、全エンティティにnamespaceを付与）
15. Web UI提供（embed.FSでバイナリに組み込んだSPAを`/ui/`パスで配信、ワークフロー一覧・詳細・履歴の閲覧、ページネーション、Search Attributesフィルタ、オペレーター操作（Signal/Cancel/Terminate））
16. Search Attributes管理（JSONBカラム + GINインデックス、UpsertSearchAttributesコマンド処理、ListWorkflowsでの`@>`フィルタ）
16. pprofプロファイリング（`ENABLE_PPROF=true`で`/debug/pprof/`エンドポイントを有効化）

サーバーは「次に何をすべきか」を知らない。イベントが発生するたびにWorkflow Taskを生成し、ワーカーに判断を委ねる。

### ワーカーの責務（Go SDKリポジトリで実装）

2種類のタスクを処理する:

- Workflow Task: ワークフロー関数を最初からreplayし、イベント履歴と照合しながら実行。新しい副作用呼び出しに到達したらコマンドを生成してサーバーに返す。replayで非決定性エラーが発生した場合はFailWorkflowTaskでサーバーに報告する
- Activity Task: Activity関数を実際に実行し、結果をサーバーに報告

gRPC経由でサーバーに結果を返す。理由:

- ワーカーとDBが疎結合
- サーバー側でイベント記録とタスクスケジューリングをアトミックに実行可能
- proto定義をAPI契約とすることで、将来的に他言語SDKも作成可能

## 2. サーバー内部アーキテクチャ

Hexagonal Architecture（Ports and Adapters）を採用する。

```text
       Inbound Adapter                     Core                        Outbound Adapter
    +------------------+                                           +------------------------+
    |  adapter/grpc/   |          +----------------------+         |  adapter/postgres/     |
    |                  |-->       |                      |         |                        |--> PostgreSQL
    |  proto型 ->       |   port/  |  engine/             |  port/  |  Outbound Port の      |
    |  domain型変換    |-->Inbound|                      |  Out-   |  インターフェースを    |
    +------------------+   Port   |  ビジネスロジック    |  bound <|  暗黙的に実装         |
                           |      |  コマンド処理        |  Port   |                        |
    +------------------+   |      |  トランザクション制御|   |     |  migration/ も含む    |
    |  adapter/http/   |-->|      |                      |   |     +------------------------+
    |  grpc-gateway    |   |      +----------------------+   |
    +------------------+   |                |                |     +------------------------+
                           |                v                |     |  adapter/telemetry/    |
    +------------------+   |           domain/               |     |  (実装済み)             |
    |adapter/telemetry/|-->|           純粋な型定義           |     |                        |
    |  Inbound Port の |   |           エラー定義             |     |  Outbound Port の      |
    |  デコレータ      |                                     +---->|  デコレータ            |
    +------------------+                                           +------------------------+
```

### プロジェクト構成

```text
dandori/
├── api/
│   └── v1/                                   # proto定義 + 生成コード（SDKからimport可能）
│       ├── service.proto
│       ├── types.proto
│       ├── service.pb.go                     # generated
│       ├── service_grpc.pb.go                # generated
│       ├── service.pb.gw.go                  # generated（grpc-gatewayリバースプロキシ）
│       ├── service.swagger.json              # generated（OpenAPI v2仕様）
│       └── types.pb.go                       # generated
├── cmd/
│   ├── dandori/                              # サーバーバイナリ（DI、起動、shutdown）
│   │   └── main.go
│   └── dandori-cli/                          # CLIツール（cobra）
│       ├── main.go
│       └── cmd/
│           ├── root.go                       # rootコマンド + --server フラグ + --namespace フラグ（デフォルト "default"）+ newClient()
│           ├── start.go                      # start --id --type --queue --input --cron (--namespace)
│           ├── describe.go                   # describe <workflow_id>
│           ├── terminate.go                  # terminate <workflow_id> --reason
│           ├── cancel.go                     # cancel <workflow_id>
│           ├── signal.go                     # signal <workflow_id> --name --input
│           ├── list.go                       # list --status --type --queue --limit --token
│           └── history.go                    # history <workflow_id>
├── internal/
│   ├── adapter/
│   │   ├── grpc/                             # Inbound Adapter: gRPC
│   │   │   ├── handler.go
│   │   │   ├── interceptor.go               # OTelServerOptions（otelgrpc StatsHandler）
│   │   │   └── health.go                    # grpc.health.v1.Health サービス実装
│   │   ├── http/                             # Inbound Adapter: HTTP（grpc-gateway + Web UI）
│   │   │   ├── gateway.go                    # NewGatewayMux, NewHTTPHandler
│   │   │   ├── health.go                    # /healthz エンドポイント（DB ping）
│   │   │   ├── swagger.go                    # /swagger.json エンドポイント（embed）
│   │   │   ├── swagger.json                  # OpenAPI v2仕様（embed用コピー）
│   │   │   └── ui.go                         # /ui/ Web UIハンドラ（web.Content embed.FS、SPAフォールバック）
│   │   ├── telemetry/                        # Observability（デコレータパターン）
│   │   │   ├── tracer.go                    # OpenTelemetry TracerProvider初期化
│   │   │   ├── decorator.go                 # Tracing*Service デコレータ（3インターフェース）
│   │   │   ├── metrics.go                   # Prometheus メトリクス定義
│   │   │   └── metrics_decorator.go         # Metrics*Service デコレータ（3インターフェース）
│   │   └── postgres/                         # Outbound Adapter: PostgreSQL
│   │       ├── migration/                    # SQLマイグレーション
│   │       │   ├── 000001_initial.up.sql
│   │       │   ├── 000001_initial.down.sql
│   │       │   ├── 000002_heartbeat.up.sql
│   │       │   ├── 000002_heartbeat.down.sql
│   │       │   ├── 000003_activity_timeouts.up.sql
│   │       │   ├── 000003_activity_timeouts.down.sql
│   │       │   ├── 000004_child_workflow.up.sql
│   │       │   ├── 000004_child_workflow.down.sql
│   │       │   ├── 000005_query.up.sql
│   │       │   ├── 000005_query.down.sql
│   │       │   ├── 000006_cron.up.sql
│   │       │   ├── 000006_cron.down.sql
│   │       │   ├── 000007_namespace.up.sql
│   │       │   ├── 000007_namespace.down.sql
│   │       │   ├── 000009_partitioning.up.sql
│   │       │   └── 000009_partitioning.down.sql
│   │       ├── migrate.go                    # embed.FSマイグレーションランナー（schema_migrationsバージョン管理）
│   │       ├── store.go                      # コネクションプール、TxManager
│   │       ├── event.go                      # EventRepository実装
│   │       ├── workflow_task.go               # WorkflowTaskRepository実装
│   │       ├── activity_task.go              # ActivityTaskRepository実装
│   │       ├── timer.go                      # TimerRepository実装
│   │       ├── query.go                      # QueryRepository実装
│   │       ├── namespace.go                  # NamespaceRepository実装（NamespaceStore）
│   │       └── workflow.go                   # WorkflowRepository実装
│   ├── domain/                               # 純粋な型定義 + エラー定義（依存なし）
│   │   ├── command.go
│   │   ├── errors.go                         # ドメインエラー定義
│   │   ├── event.go
│   │   ├── namespace.go                      # Namespace型（Name, Description, CreatedAt）
│   │   ├── query.go                          # WorkflowQuery型、QueryStatus
│   │   ├── retry.go
│   │   ├── task.go
│   │   ├── timer.go
│   │   └── workflow.go
│   ├── engine/                               # アプリケーションコア（ビジネスロジック）
│   │   ├── engine.go                         # リクエスト駆動の操作（ClientService, WorkflowTaskService, ActivityTaskService を実装）
│   │   ├── command_processor.go              # コマンド→イベント変換パイプライン
│   │   ├── background.go                     # バックグラウンドプロセス（タイマー、タイムアウト監視）
│   │   ├── cron.go                           # Cronスケジュール検証（robfig/cron/v3）
│   │   └── retry.go                          # リトライポリシー
│   └── port/                                 # ポート定義（インターフェース）
│       ├── service.go                        # Inbound Port: 役割別インターフェース
│       └── repository.go                     # Outbound Port: 各Repository, TxManager
├── test/
│   ├── bench/                                # ベンチマークテスト（testcontainers）
│   │   ├── workflow_bench_test.go            # ワークフロー作成/イベント追記/タスクPoll・Completeのスループット
│   │   └── concurrent_bench_test.go          # N並行ワーカーでのタスクスループット
│   └── e2e/                                  # E2Eテスト（bufconn gRPC + httptest HTTP + testcontainers）
│       ├── setup_test.go                     # TestMain, bufconn server, httptest server, helpers
│       ├── sequential_activity_test.go       # 3ステップActivity + 結果取得
│       ├── replay_test.go                    # ワーカー再起動replay
│       ├── concurrent_poll_test.go           # 複数ワーカー重複なし
│       ├── retry_test.go                     # リトライ + non_retryable
│       ├── timeout_test.go                   # Activityタイムアウト
│       ├── terminate_test.go                 # Terminate + 結果破棄
│       ├── nondeterminism_test.go            # 非決定性エラー
│       ├── signal_test.go                    # Signal送信・受信・複数Signal
│       ├── cancel_test.go                    # CancelWorkflow
│       ├── parallel_activity_test.go         # 並行Activity実行
│       ├── heartbeat_test.go                 # Heartbeatタイムアウト・keepalive
│       ├── schedule_timeout_test.go         # ScheduleToClose/ScheduleToStartタイムアウト
│       ├── list_test.go                     # ListWorkflows API（一覧・フィルタ・ページネーション）
│       ├── saga_test.go                     # Saga補償トランザクション
│       ├── child_workflow_test.go           # Child Workflow（親→子起動・完了/失敗伝搬）
│       ├── sideeffect_test.go              # SideEffect記録
│       ├── query_test.go                   # Query送信→応答
│       ├── continue_as_new_test.go        # Continue-as-New（手動・型引き継ぎ）
│       ├── cron_test.go                   # Cron自動再起動・失敗時再起動なし
│       ├── http_api_test.go              # HTTP API（開始・取得・終了・履歴・一覧・フィルタ）
│       └── observability_test.go        # /healthz レスポンス検証
├── web/                                       # Web UI（vanilla JS SPA、embed.FSでバイナリ組み込み）
│   ├── embed.go                              # embed.FSエクスポート（web.Content）
│   ├── index.html                            # SPAエントリポイント（Tailwind CSS v4 CDN）
│   └── static/
│       ├── app.js                            # クライアントサイドルーティング、API呼び出し、ビュー描画
│       └── style.css                         # カスタムアニメーション定義
├── third_party/
│   └── google/api/                           # grpc-gateway用protoインクルード
│       ├── annotations.proto
│       └── http.proto
├── docker-compose.yml
└── go.mod
```

### 依存関係の方向

```text
cmd/dandori/main.go（依存の組み立て）
    |
    ├── adapter/grpc/ --> port/ (Inbound) <-- engine/ --> port/ (Outbound) <-- adapter/postgres/
    |                       |                    |              |                      |
    |                       v                    v              v                      v
    └──────────────────> domain/ <───────────────┴──────────────┴──────────────────────┘
```

**依存ルール**:

- `domain/`: 純粋な型・エラー定義。他のパッケージに依存しない。ドメインエラーもここに定義する
- `port/`: Inbound Port と Outbound Port の定義。domain/ のみに依存
- `engine/`: ビジネスロジック。port/ と domain/ に依存。adapter/ には依存しない
- `adapter/grpc/`: Inbound Adapter。port/ と domain/ に依存。**engine/ には依存しない**（エラー判定は domain/ のエラーを使う）
- `adapter/http/`: Inbound Adapter。grpc-gatewayの生成コード（api/v1）と web/ パッケージに依存。gRPCサーバーへのリバースプロキシとしてHTTP APIを提供し、`/ui/` パスでWeb UIを配信
- `web/`: フロントエンドアセット。embed.FSでエクスポートし、adapter/http/ から参照される。他のinternal/ パッケージには依存しない
- `adapter/postgres/`: Outbound Adapter。port/ と domain/ に依存。engine/ には依存しない
- `cmd/dandori/`: 全パッケージをimportし、依存を組み立てるエントリーポイント
- `cmd/dandori-cli/`: CLIツール。api/v1の生成コードとgRPCクライアントのみに依存。internal/は参照しない

### 各層の責務

#### domain/ - ドメインモデルとエラー

ビジネスロジックを持たない純粋な型定義とドメインエラー。全パッケージから参照される共有の語彙。

```go
// domain/errors.go
// ドメインエラーはdomain/に定義し、engine/とadapter/の両方から参照可能にする。
// これにより adapter/grpc/ が engine/ に依存することを防ぐ。
var (
    ErrWorkflowNotFound      = errors.New("workflow not found")
    ErrWorkflowAlreadyExists = errors.New("workflow already exists")
    ErrWorkflowNotRunning    = errors.New("workflow is not in running state")
    ErrTaskNotFound          = errors.New("task not found")
    ErrTaskAlreadyCompleted  = errors.New("task already completed")
    ErrNoTaskAvailable       = errors.New("no task available")
    ErrQueryNotFound         = errors.New("query not found")
    ErrQueryTimedOut         = errors.New("query timed out")
    ErrNamespaceNotFound     = errors.New("namespace not found")
)
```

```go
// domain/namespace.go
type Namespace struct {
    Name        string
    Description string
    CreatedAt   time.Time
}
```

```go
// domain/event.go
type EventType string

const (
    EventWorkflowExecutionStarted    EventType = "WorkflowExecutionStarted"
    EventWorkflowExecutionCompleted  EventType = "WorkflowExecutionCompleted"
    EventWorkflowExecutionFailed     EventType = "WorkflowExecutionFailed"
    EventWorkflowExecutionTerminated EventType = "WorkflowExecutionTerminated"
    EventActivityTaskScheduled       EventType = "ActivityTaskScheduled"
    EventActivityTaskCompleted       EventType = "ActivityTaskCompleted"
    EventActivityTaskFailed          EventType = "ActivityTaskFailed"
    EventActivityTaskTimedOut        EventType = "ActivityTaskTimedOut"

    EventTimerStarted               EventType = "TimerStarted"
    EventTimerFired                 EventType = "TimerFired"
    EventTimerCanceled              EventType = "TimerCanceled"

    EventWorkflowSignaled                   EventType = "WorkflowSignaled"
    EventWorkflowCancelRequested            EventType = "WorkflowCancelRequested"
    EventChildWorkflowExecutionStarted      EventType = "ChildWorkflowExecutionStarted"
    EventChildWorkflowExecutionCompleted    EventType = "ChildWorkflowExecutionCompleted"
    EventChildWorkflowExecutionFailed       EventType = "ChildWorkflowExecutionFailed"

    EventSideEffectRecorded                 EventType = "SideEffectRecorded"
    EventWorkflowExecutionContinuedAsNew    EventType = "WorkflowExecutionContinuedAsNew"
)

type HistoryEvent struct {
    ID           int64
    WorkflowID   uuid.UUID
    SequenceNum  int
    Type         EventType
    Data         json.RawMessage
    Timestamp    time.Time
}
```

```go
// domain/command.go
type CommandType string

const (
    CommandScheduleActivityTask CommandType = "ScheduleActivityTask"
    CommandCompleteWorkflow     CommandType = "CompleteWorkflow"
    CommandFailWorkflow         CommandType = "FailWorkflow"
    CommandStartTimer           CommandType = "StartTimer"
    CommandCancelTimer          CommandType = "CancelTimer"
    CommandStartChildWorkflow   CommandType = "StartChildWorkflow"
    CommandRecordSideEffect     CommandType = "RecordSideEffect"
    CommandContinueAsNew        CommandType = "ContinueAsNew"
)

type Command struct {
    Type       CommandType
    Attributes json.RawMessage
    Metadata   map[string]string
}

type ScheduleActivityTaskAttributes struct {
    SeqID                  int64           `json:"seq_id"`
    ActivityType           string          `json:"activity_type"`
    Input                  json.RawMessage `json:"input"`
    TaskQueue              string          `json:"task_queue,omitempty"`
    StartToCloseTimeout    time.Duration   `json:"start_to_close_timeout"`
    HeartbeatTimeout       time.Duration   `json:"heartbeat_timeout,omitempty"`
    ScheduleToCloseTimeout time.Duration   `json:"schedule_to_close_timeout,omitempty"`
    ScheduleToStartTimeout time.Duration   `json:"schedule_to_start_timeout,omitempty"`
    RetryPolicy            *RetryPolicy    `json:"retry_policy,omitempty"`
}

type CompleteWorkflowAttributes struct {
    Result json.RawMessage `json:"result"`
}

type FailWorkflowAttributes struct {
    ErrorMessage string `json:"error_message"`
}

type StartTimerAttributes struct {
    SeqID    int64         `json:"seq_id"`
    Duration time.Duration `json:"duration"`
}

type CancelTimerAttributes struct {
    SeqID int64 `json:"seq_id"`
}

type RecordSideEffectAttributes struct {
    SeqID int64           `json:"seq_id"`
    Value json.RawMessage `json:"value"`
}

type ContinueAsNewAttributes struct {
    WorkflowType string          `json:"workflow_type,omitempty"`
    TaskQueue    string          `json:"task_queue,omitempty"`
    Input        json.RawMessage `json:"input"`
}
```

```go
// domain/query.go
type QueryStatus string
const (
    QueryStatusPending  QueryStatus = "PENDING"
    QueryStatusAnswered QueryStatus = "ANSWERED"
)

type WorkflowQuery struct {
    ID           int64
    WorkflowID   uuid.UUID
    QueryType    string
    Input        json.RawMessage
    Result       json.RawMessage
    ErrorMessage string
    Status       QueryStatus
    CreatedAt    time.Time
    AnsweredAt   *time.Time
}
```

```go
// domain/retry.go
type RetryPolicy struct {
    MaxAttempts        int           `json:"max_attempts"`
    InitialInterval    time.Duration `json:"initial_interval"`
    BackoffCoefficient float64       `json:"backoff_coefficient"`
    MaxInterval        time.Duration `json:"max_interval"`
}
```

```go
// domain/workflow.go
type WorkflowStatus string

const (
    WorkflowStatusRunning        WorkflowStatus = "RUNNING"
    WorkflowStatusCompleted      WorkflowStatus = "COMPLETED"
    WorkflowStatusFailed         WorkflowStatus = "FAILED"
    WorkflowStatusTerminated     WorkflowStatus = "TERMINATED"
    WorkflowStatusContinuedAsNew WorkflowStatus = "CONTINUED_AS_NEW"
)

// IsTerminal はワークフローが終了状態かどうかを返す
func (s WorkflowStatus) IsTerminal() bool {
    return s == WorkflowStatusCompleted || s == WorkflowStatusFailed || s == WorkflowStatusTerminated || s == WorkflowStatusContinuedAsNew
}

type WorkflowExecution struct {
    ID               uuid.UUID
    Namespace        string
    WorkflowType     string
    TaskQueue        string
    Status           WorkflowStatus
    Input            json.RawMessage
    Result           json.RawMessage
    Error            string
    CreatedAt        time.Time
    ClosedAt         *time.Time
    ParentWorkflowID *uuid.UUID
    ParentSeqID      int64
    CronSchedule     string
    ContinuedAsNewID *uuid.UUID
}
```

```go
// domain/task.go
type TaskStatus string

const (
    TaskStatusPending   TaskStatus = "PENDING"
    TaskStatusRunning   TaskStatus = "RUNNING"
    TaskStatusCompleted TaskStatus = "COMPLETED"
)

// WorkflowTask はワーカーがポーリングで取得するWorkflow Task
type WorkflowTask struct {
    ID         int64
    Namespace  string
    QueueName  string
    WorkflowID uuid.UUID
    Status     TaskStatus
    ScheduledAt time.Time
}

// ActivityTask はワーカーがポーリングで取得するActivity Task
type ActivityTask struct {
    ID                       int64
    Namespace                string
    QueueName                string
    WorkflowID               uuid.UUID
    ActivityType             string
    ActivityInput            json.RawMessage
    ActivitySeqID            int64
    StartToCloseTimeout      time.Duration
    HeartbeatTimeout         time.Duration
    HeartbeatAt              *time.Time
    Attempt                  int
    MaxAttempts              int
    RetryPolicy              *RetryPolicy
    Status                   TaskStatus
    ScheduledAt              time.Time
    TimeoutAt                *time.Time
    ScheduleToCloseTimeout   time.Duration
    ScheduleToCloseTimeoutAt *time.Time
    ScheduleToStartTimeout   time.Duration
    ScheduleToStartTimeoutAt *time.Time
}

// ActivityFailure はActivity失敗の詳細情報
type ActivityFailure struct {
    Message      string
    Type         string
    NonRetryable bool
}
```

#### port/ - ポート定義

Inbound PortとOutbound Portの両方のインターフェースを定義する。domain/のみに依存する独立したパッケージ。

**Inbound Port は役割ごとに分離する（Interface Segregation Principle）**:

```go
// port/service.go - Inbound Port（役割別に分離）

// ClientService はクライアント向け操作を定義する。
// gRPCハンドラのクライアントAPIメソッドがこのインターフェースに依存する。
type ClientService interface {
    StartWorkflow(ctx context.Context, params StartWorkflowParams) (*domain.WorkflowExecution, error)
    DescribeWorkflow(ctx context.Context, namespace string, id uuid.UUID) (*domain.WorkflowExecution, error)
    GetWorkflowHistory(ctx context.Context, namespace string, workflowID uuid.UUID) ([]domain.HistoryEvent, error)
    TerminateWorkflow(ctx context.Context, namespace string, id uuid.UUID, reason string) error
    SignalWorkflow(ctx context.Context, namespace string, id uuid.UUID, signalName string, input json.RawMessage) error
    CancelWorkflow(ctx context.Context, namespace string, id uuid.UUID) error
    ListWorkflows(ctx context.Context, params ListWorkflowsParams) (*ListWorkflowsResult, error)
    QueryWorkflow(ctx context.Context, namespace string, id uuid.UUID, queryType string, input json.RawMessage) (*domain.WorkflowQuery, error)
}

// WorkflowTaskService はワーカーのWorkflow Task操作を定義する。
type WorkflowTaskService interface {
    PollWorkflowTask(ctx context.Context, namespace string, queueName string, workerID string) (*WorkflowTaskResult, error)
    CompleteWorkflowTask(ctx context.Context, taskID int64, commands []domain.Command) error
    FailWorkflowTask(ctx context.Context, taskID int64, cause string, message string) error
    RespondQueryTask(ctx context.Context, queryID int64, result json.RawMessage, errMsg string) error
}

// ActivityTaskService はワーカーのActivity Task操作を定義する。
type ActivityTaskService interface {
    PollActivityTask(ctx context.Context, namespace string, queueName string, workerID string) (*domain.ActivityTask, error)
    CompleteActivityTask(ctx context.Context, taskID int64, result json.RawMessage) error
    FailActivityTask(ctx context.Context, taskID int64, failure domain.ActivityFailure) error
    RecordActivityHeartbeat(ctx context.Context, taskID int64, details json.RawMessage) error
}

// StartWorkflowParams はワークフロー開始のパラメータ
type StartWorkflowParams struct {
    Namespace    string
    ID           uuid.UUID       // ゼロ値の場合はUUID自動生成
    WorkflowType string
    TaskQueue    string
    Input        json.RawMessage
    CronSchedule string
}

// WorkflowTaskResult はPollWorkflowTaskの結果
type WorkflowTaskResult struct {
    Task           domain.WorkflowTask
    Events         []domain.HistoryEvent
    WorkflowType   string
    PendingQueries []domain.WorkflowQuery
}
```

engine.Engineはこの3つのインターフェースを全て実装する。gRPCハンドラは必要なインターフェースのみを受け取る。

telemetryデコレータは関心のあるインターフェースだけを装飾できる。トレーシングとメトリクスで2段のデコレータチェーンを構成する:

```go
// トレーシングデコレータ（OpenTelemetryスパン生成）
type TracingClientService struct {
    next   port.ClientService
    tracer trace.Tracer
}

// メトリクスデコレータ（Prometheusカウンター/ヒストグラム記録）
type MetricsClientService struct {
    next    port.ClientService
    metrics *Metrics
}
```

```go
// port/repository.go - Outbound Port
type NamespaceRepository interface {
    GetByName(ctx context.Context, name string) (*domain.Namespace, error)
    Create(ctx context.Context, ns domain.Namespace) error
    List(ctx context.Context) ([]domain.Namespace, error)
}

type WorkflowRepository interface {
    Create(ctx context.Context, wf domain.WorkflowExecution) error
    Get(ctx context.Context, namespace string, id uuid.UUID) (*domain.WorkflowExecution, error)
    UpdateStatus(ctx context.Context, id uuid.UUID, status domain.WorkflowStatus, result json.RawMessage, errMsg string) error
    List(ctx context.Context, params ListWorkflowsParams) ([]domain.WorkflowExecution, error)
    SetContinuedAsNewID(ctx context.Context, id uuid.UUID, newID uuid.UUID) error
}

type EventRepository interface {
    // Append はイベントを追記する。sequence_numはリポジトリ内で自動採番される。
    Append(ctx context.Context, events []domain.HistoryEvent) error
    GetByWorkflowID(ctx context.Context, workflowID uuid.UUID) ([]domain.HistoryEvent, error)
    DeleteByWorkflowID(ctx context.Context, workflowID uuid.UUID) error
}

type WorkflowTaskRepository interface {
    Enqueue(ctx context.Context, task domain.WorkflowTask) error
    // Poll はキューからタスクを1件取得する。タスクがない場合はdomain.ErrNoTaskAvailableを返す。
    Poll(ctx context.Context, namespace string, queueName string, workerID string) (*domain.WorkflowTask, error)
    Complete(ctx context.Context, taskID int64) error
    GetByID(ctx context.Context, taskID int64) (*domain.WorkflowTask, error)
    RecoverStaleTasks(ctx context.Context) (int, error)
    DeleteByWorkflowID(ctx context.Context, workflowID uuid.UUID) error
}

type ActivityTaskRepository interface {
    Enqueue(ctx context.Context, task domain.ActivityTask) error
    // Poll はキューからタスクを1件取得する。タスクがない場合はdomain.ErrNoTaskAvailableを返す。
    Poll(ctx context.Context, namespace string, queueName string, workerID string) (*domain.ActivityTask, error)
    Complete(ctx context.Context, taskID int64) error
    CompletePending(ctx context.Context, taskID int64) error
    GetByID(ctx context.Context, taskID int64) (*domain.ActivityTask, error)
    GetTimedOut(ctx context.Context) ([]domain.ActivityTask, error)
    GetHeartbeatTimedOut(ctx context.Context) ([]domain.ActivityTask, error)
    GetScheduleToCloseTimedOut(ctx context.Context) ([]domain.ActivityTask, error)
    GetScheduleToStartTimedOut(ctx context.Context) ([]domain.ActivityTask, error)
    UpdateHeartbeat(ctx context.Context, taskID int64) error
    Requeue(ctx context.Context, taskID int64, scheduledAt time.Time) error
    RecoverStaleTasks(ctx context.Context) (int, error)
    DeleteByWorkflowID(ctx context.Context, workflowID uuid.UUID) error
}

type TimerRepository interface {
    Create(ctx context.Context, timer domain.Timer) error
    GetFired(ctx context.Context) ([]domain.Timer, error)
    MarkFired(ctx context.Context, timerID int64) (bool, error)
    Cancel(ctx context.Context, workflowID uuid.UUID, seqID int64) (bool, error)
    DeleteByWorkflowID(ctx context.Context, workflowID uuid.UUID) error
}

type QueryRepository interface {
    Create(ctx context.Context, query domain.WorkflowQuery) (int64, error)
    GetByID(ctx context.Context, queryID int64) (*domain.WorkflowQuery, error)
    GetPendingByWorkflowID(ctx context.Context, workflowID uuid.UUID) ([]domain.WorkflowQuery, error)
    SetResult(ctx context.Context, queryID int64, result json.RawMessage, errMsg string) error
    DeleteByWorkflowID(ctx context.Context, workflowID uuid.UUID) error
}

type TxManager interface {
    RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}
```

#### engine/ - アプリケーションコア

ビジネスロジックとトランザクション制御を担う。3つの Inbound Port を実装する。

```go
// engine/engine.go
// Engine は port.ClientService, port.WorkflowTaskService, port.ActivityTaskService を実装する。
// コマンド処理(processCommands)はEngineのメソッドとして統合し、
// CompleteWorkflowTask内から同一トランザクションで自然に呼び出す。
type Engine struct {
    namespaces     port.NamespaceRepository
    workflows      port.WorkflowRepository
    events         port.EventRepository
    workflowTasks  port.WorkflowTaskRepository
    activityTasks  port.ActivityTaskRepository
    timers         port.TimerRepository
    queries        port.QueryRepository
    tx             port.TxManager
    queryTimeout   time.Duration // デフォルト10秒、テスト時に短縮可能
}

// コンパイル時にインターフェース実装を保証
var _ port.ClientService       = (*Engine)(nil)
var _ port.WorkflowTaskService = (*Engine)(nil)
var _ port.ActivityTaskService = (*Engine)(nil)

func New(
    namespaces port.NamespaceRepository,
    workflows port.WorkflowRepository,
    events port.EventRepository,
    workflowTasks port.WorkflowTaskRepository,
    activityTasks port.ActivityTaskRepository,
    timers port.TimerRepository,
    queries port.QueryRepository,
    tx port.TxManager,
) *Engine {
    return &Engine{
        namespaces:    namespaces,
        workflows:     workflows,
        events:        events,
        workflowTasks: workflowTasks,
        activityTasks: activityTasks,
        timers:        timers,
        queries:       queries,
        tx:            tx,
        queryTimeout:  10 * time.Second,
    }
}

// resolveNamespace は空文字列を "default" に解決する
func (e *Engine) resolveNamespace(ns string) string {
    if ns == "" {
        return "default"
    }
    return ns
}

// --- ClientService の実装 ---

func (e *Engine) StartWorkflow(ctx context.Context, params port.StartWorkflowParams) (*domain.WorkflowExecution, error) {
    params.Namespace = e.resolveNamespace(params.Namespace)
    if params.ID == uuid.Nil {
        params.ID = uuid.New()
    }

    var wf *domain.WorkflowExecution
    err := e.tx.RunInTx(ctx, func(ctx context.Context) error {
        // 冪等性チェック: 同一IDのワークフローが存在するか
        existing, err := e.workflows.Get(ctx, params.Namespace, params.ID)
        if err != nil && !errors.Is(err, domain.ErrWorkflowNotFound) {
            return err
        }

        if existing != nil && existing.Status == domain.WorkflowStatusRunning {
            return domain.ErrWorkflowAlreadyExists
        }

        // 終了済みワークフローの再作成: 旧関連データを削除してからupsert
        if existing != nil && existing.Status.IsTerminal() {
            if err := e.events.DeleteByWorkflowID(ctx, params.ID); err != nil {
                return err
            }
            if err := e.workflowTasks.DeleteByWorkflowID(ctx, params.ID); err != nil {
                return err
            }
            if err := e.activityTasks.DeleteByWorkflowID(ctx, params.ID); err != nil {
                return err
            }
            if err := e.timers.DeleteByWorkflowID(ctx, params.ID); err != nil {
                return err
            }
        }

        newWF := domain.WorkflowExecution{
            ID:           params.ID,
            Namespace:    params.Namespace,
            WorkflowType: params.WorkflowType,
            TaskQueue:    params.TaskQueue,
            Status:       domain.WorkflowStatusRunning,
            Input:        params.Input,
        }
        if err := e.workflows.Create(ctx, newWF); err != nil {
            return err
        }

        eventData, err := json.Marshal(map[string]json.RawMessage{"input": params.Input})
        if err != nil {
            return err
        }
        if err := e.events.Append(ctx, []domain.HistoryEvent{{
            WorkflowID: params.ID,
            Type:       domain.EventWorkflowExecutionStarted,
            Data:       eventData,
        }}); err != nil {
            return err
        }
        if err := e.workflowTasks.Enqueue(ctx, domain.WorkflowTask{
            Namespace:  params.Namespace,
            QueueName:  params.TaskQueue,
            WorkflowID: params.ID,
        }); err != nil {
            return err
        }

        wf = &newWF
        return nil
    })
    if err != nil {
        return nil, err
    }
    return wf, nil
}

func (e *Engine) TerminateWorkflow(ctx context.Context, namespace string, id uuid.UUID, reason string) error {
    namespace = e.resolveNamespace(namespace)
    return e.tx.RunInTx(ctx, func(ctx context.Context) error {
        wf, err := e.workflows.Get(ctx, namespace, id)
        if err != nil {
            return err
        }
        if wf.Status != domain.WorkflowStatusRunning {
            return domain.ErrWorkflowNotRunning
        }

        if err := e.workflows.UpdateStatus(ctx, id, domain.WorkflowStatusTerminated, nil, reason); err != nil {
            return err
        }

        data, err := json.Marshal(map[string]string{"reason": reason})
        if err != nil {
            return err
        }
        return e.events.Append(ctx, []domain.HistoryEvent{{
            WorkflowID: id,
            Type:       domain.EventWorkflowExecutionTerminated,
            Data:       data,
        }})
    })
}

func (e *Engine) SignalWorkflow(ctx context.Context, namespace string, id uuid.UUID, signalName string, input json.RawMessage) error {
    namespace = e.resolveNamespace(namespace)
    return e.tx.RunInTx(ctx, func(ctx context.Context) error {
        wf, err := e.workflows.Get(ctx, namespace, id)
        if err != nil {
            return err
        }
        if wf.Status != domain.WorkflowStatusRunning {
            return domain.ErrWorkflowNotRunning
        }

        eventData, err := json.Marshal(map[string]any{
            "signal_name": signalName,
            "input":       input,
        })
        if err != nil {
            return err
        }
        if err := e.events.Append(ctx, []domain.HistoryEvent{{
            WorkflowID: id,
            Type:       domain.EventWorkflowSignaled,
            Data:       eventData,
        }}); err != nil {
            return err
        }
        return e.workflowTasks.Enqueue(ctx, domain.WorkflowTask{
            QueueName:  wf.TaskQueue,
            WorkflowID: id,
        })
    })
}

// --- WorkflowTaskService の実装 ---

func (e *Engine) PollWorkflowTask(ctx context.Context, namespace string, queueName string, workerID string) (*port.WorkflowTaskResult, error) {
    namespace = e.resolveNamespace(namespace)
    // タスクがない場合は(nil, nil)を返す。gRPCハンドラ側で空レスポンスに変換する。
    task, err := e.workflowTasks.Poll(ctx, namespace, queueName, workerID)
    if errors.Is(err, domain.ErrNoTaskAvailable) {
        return nil, nil
    }
    if err != nil {
        return nil, err
    }
    wf, err := e.workflows.Get(ctx, task.Namespace, task.WorkflowID)
    if err != nil {
        return nil, err
    }
    events, err := e.events.GetByWorkflowID(ctx, task.WorkflowID)
    if err != nil {
        return nil, err
    }
    return &port.WorkflowTaskResult{
        Task:         *task,
        Events:       events,
        WorkflowType: wf.WorkflowType,
    }, nil
}

func (e *Engine) CompleteWorkflowTask(ctx context.Context, taskID int64, commands []domain.Command) error {
    return e.tx.RunInTx(ctx, func(ctx context.Context) error {
        task, err := e.workflowTasks.GetByID(ctx, taskID)
        if err != nil {
            return err
        }

        // Advisory Lock: 同一ワークフローのWorkflow Task処理を直列化
        // pg_advisory_xact_lock はトランザクション終了時に自動解放
        // （adapter/postgres側でGetByID実行時にworkflowIDベースのAdvisory Lockを取得する）

        if err := e.workflowTasks.Complete(ctx, taskID); err != nil {
            return err
        }

        wf, err := e.workflows.Get(ctx, task.Namespace, task.WorkflowID)
        if err != nil {
            return err
        }
        return e.processCommands(ctx, wf, commands)
    })
}

// --- ActivityTaskService の実装 ---

func (e *Engine) CompleteActivityTask(ctx context.Context, taskID int64, result json.RawMessage) error {
    return e.tx.RunInTx(ctx, func(ctx context.Context) error {
        task, err := e.activityTasks.GetByID(ctx, taskID)
        if err != nil {
            return err
        }

        wf, err := e.workflows.Get(ctx, task.Namespace, task.WorkflowID)
        if err != nil {
            return err
        }

        if err := e.activityTasks.Complete(ctx, taskID); err != nil {
            return err
        }

        if wf.Status.IsTerminal() {
            // ワークフローが既に終了している場合、結果は破棄してタスクだけ完了する
            return nil
        }

        completedData, err := json.Marshal(map[string]any{
            "activity_seq_id": task.ActivitySeqID,
            "result":          result,
        })
        if err != nil {
            return err
        }
        if err := e.events.Append(ctx, []domain.HistoryEvent{{
            WorkflowID: task.WorkflowID,
            Type:       domain.EventActivityTaskCompleted,
            Data:       completedData,
        }}); err != nil {
            return err
        }
        return e.workflowTasks.Enqueue(ctx, domain.WorkflowTask{
            QueueName:  wf.TaskQueue,
            WorkflowID: task.WorkflowID,
        })
    })
}

func (e *Engine) FailActivityTask(ctx context.Context, taskID int64, failure domain.ActivityFailure) error {
    return e.tx.RunInTx(ctx, func(ctx context.Context) error {
        task, err := e.activityTasks.GetByID(ctx, taskID)
        if err != nil {
            return err
        }

        wf, err := e.workflows.Get(ctx, task.Namespace, task.WorkflowID)
        if err != nil {
            return err
        }
        if wf.Status.IsTerminal() {
            return e.activityTasks.Complete(ctx, taskID)
        }

        // リトライ判定: non_retryable=true or 最大試行回数到達 → 失敗確定
        if failure.NonRetryable || task.Attempt >= task.MaxAttempts {
            if err := e.activityTasks.Complete(ctx, taskID); err != nil {
                return err
            }
            failedData, err := json.Marshal(map[string]any{
                "activity_seq_id": task.ActivitySeqID,
                "failure":         failure,
            })
            if err != nil {
                return err
            }
            if err := e.events.Append(ctx, []domain.HistoryEvent{{
                WorkflowID: task.WorkflowID,
                Type:       domain.EventActivityTaskFailed,
                Data:       failedData,
            }}); err != nil {
                return err
            }
            return e.workflowTasks.Enqueue(ctx, domain.WorkflowTask{
                QueueName:  wf.TaskQueue,
                WorkflowID: task.WorkflowID,
            })
        }

        // リトライ可能 → 指数バックオフで再キューイング
        return e.activityTasks.Requeue(ctx, taskID, computeNextRetryTime(task))
    })
}
```

```go
// engine/background.go
// BackgroundWorker はバックグラウンドプロセスを管理する。
// Engine (リクエスト処理) とは別の構造体として定義し、責務を分離する。
type BackgroundWorker struct {
    workflows     port.WorkflowRepository
    events        port.EventRepository
    workflowTasks port.WorkflowTaskRepository
    activityTasks port.ActivityTaskRepository
    timers        port.TimerRepository
    tx            port.TxManager
}

func NewBackgroundWorker(
    workflows port.WorkflowRepository,
    events port.EventRepository,
    workflowTasks port.WorkflowTaskRepository,
    activityTasks port.ActivityTaskRepository,
    timers port.TimerRepository,
    tx port.TxManager,
) *BackgroundWorker {
    return &BackgroundWorker{
        workflows: workflows, events: events,
        workflowTasks: workflowTasks,
        activityTasks: activityTasks,
        timers: timers, tx: tx,
    }
}

// RunActivityTimeoutChecker はtimeout_at超過のActivity Taskを検知し、
// ActivityTaskTimedOutイベントを記録してWorkflow Taskを生成する。
// ctx.Done()でループを終了する。
func (w *BackgroundWorker) RunActivityTimeoutChecker(ctx context.Context, interval time.Duration) error {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            if err := w.checkActivityTimeouts(ctx); err != nil {
                slog.Error("activity timeout check failed", "error", err)
            }
        }
    }
}

// RunTaskRecovery はlocked_untilを超えたタスクをPENDINGに戻す。
func (w *BackgroundWorker) RunTaskRecovery(ctx context.Context, interval time.Duration) error {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            if n, err := w.workflowTasks.RecoverStaleTasks(ctx); err != nil {
                slog.Error("workflow task recovery failed", "error", err)
            } else if n > 0 {
                slog.Info("recovered stale workflow tasks", "count", n)
            }
            if n, err := w.activityTasks.RecoverStaleTasks(ctx); err != nil {
                slog.Error("activity task recovery failed", "error", err)
            } else if n > 0 {
                slog.Info("recovered stale activity tasks", "count", n)
            }
        }
    }
}
```

```go
// engine/command_processor.go
// コマンド処理はEngineのメソッドとして統合。
// CompleteWorkflowTask内から同一トランザクションで呼び出される。

// processCommands: コマンドリストを順次処理し、イベント記録とタスク生成を行う
// *domain.WorkflowExecution を受け取り、Namespace/TaskQueue等をコマンド処理に伝搬する
func (e *Engine) processCommands(ctx context.Context, wf *domain.WorkflowExecution, commands []domain.Command) error {
    for _, cmd := range commands {
        switch cmd.Type {
        case domain.CommandScheduleActivityTask:
            if err := e.processScheduleActivity(ctx, wf.ID, wf.TaskQueue, cmd.Attributes, cmd.Metadata); err != nil {
                return err
            }
        case domain.CommandCompleteWorkflow:
            if err := e.processCompleteWorkflow(ctx, wf.ID, cmd.Attributes, cmd.Metadata); err != nil {
                return err
            }
        case domain.CommandFailWorkflow:
            if err := e.processFailWorkflow(ctx, wf.ID, cmd.Attributes, cmd.Metadata); err != nil {
                return err
            }
        case domain.CommandStartTimer:
            if err := e.processStartTimer(ctx, wf.ID, cmd.Attributes, cmd.Metadata); err != nil {
                return err
            }
        case domain.CommandCancelTimer:
            if err := e.processCancelTimer(ctx, wf.ID, cmd.Attributes, cmd.Metadata); err != nil {
                return err
            }
        case domain.CommandStartChildWorkflow:
            if err := e.processStartChildWorkflow(ctx, wf.ID, wf.TaskQueue, cmd.Attributes, cmd.Metadata); err != nil {
                return err
            }
        case domain.CommandRecordSideEffect:
            if err := e.processRecordSideEffect(ctx, wf.ID, cmd.Attributes, cmd.Metadata); err != nil {
                return err
            }
        case domain.CommandContinueAsNew:
            if err := e.processContinueAsNew(ctx, wf.ID, cmd.Attributes, cmd.Metadata); err != nil {
                return err
            }
        default:
            return fmt.Errorf("unknown command type: %s", cmd.Type)
        }
    }
    return nil
}

func (e *Engine) processScheduleActivity(ctx context.Context, workflowID uuid.UUID, taskQueue string, attrs json.RawMessage, metadata map[string]string) error {
    var a domain.ScheduleActivityTaskAttributes
    if err := json.Unmarshal(attrs, &a); err != nil {
        return err
    }

    // TaskQueueが未指定ならワークフローのTaskQueueを使用
    queue := a.TaskQueue
    if queue == "" {
        queue = taskQueue
    }

    // RetryPolicy未指定時のデフォルトMaxAttempts=1（リトライなし）
    maxAttempts := 1
    if a.RetryPolicy != nil && a.RetryPolicy.MaxAttempts > 0 {
        maxAttempts = a.RetryPolicy.MaxAttempts
    }

    if err := e.activityTasks.Enqueue(ctx, domain.ActivityTask{
        QueueName:           queue,
        WorkflowID:          workflowID,
        ActivityType:        a.ActivityType,
        ActivityInput:       a.Input,
        ActivitySeqID:       a.SeqID,
        StartToCloseTimeout: a.StartToCloseTimeout,
        HeartbeatTimeout:    a.HeartbeatTimeout,
        Attempt:             1,
        MaxAttempts:         maxAttempts,
        RetryPolicy:         a.RetryPolicy,
    }); err != nil {
        return err
    }

    // marshalEventData: metadataが空ならjson.Marshal(a)と同等、
    // 非空ならevent_dataに"metadata"キーを追加して保存
    eventData, _ := marshalEventData(a, metadata)
    return e.events.Append(ctx, []domain.HistoryEvent{{
        WorkflowID: workflowID,
        Type:       domain.EventActivityTaskScheduled,
        Data:       eventData,
    }})
}
```

#### adapter/grpc/ - Inbound Adapter

proto型とdomain型の変換を行う薄い層。ビジネスロジックは一切持たない。**役割別のインターフェースを受け取る**。エラーハンドリングは **domain/ のエラーを使い、engine/ には依存しない**。

```go
// adapter/grpc/handler.go
type Handler struct {
    apiv1.UnimplementedDandoriServiceServer
    client   port.ClientService
    wfTask   port.WorkflowTaskService
    actTask  port.ActivityTaskService
}

func NewHandler(
    client port.ClientService,
    wfTask port.WorkflowTaskService,
    actTask port.ActivityTaskService,
) *Handler {
    return &Handler{client: client, wfTask: wfTask, actTask: actTask}
}

// resolveNamespace は空文字列を "default" に解決する
func resolveNamespace(ns string) string {
    if ns == "" {
        return "default"
    }
    return ns
}

func (h *Handler) StartWorkflow(ctx context.Context, req *apiv1.StartWorkflowRequest) (*apiv1.StartWorkflowResponse, error) {
    var id uuid.UUID
    if req.WorkflowId != "" {
        var err error
        id, err = uuid.Parse(req.WorkflowId)
        if err != nil {
            return nil, status.Errorf(codes.InvalidArgument, "invalid workflow_id: %v", err)
        }
    }
    wf, err := h.client.StartWorkflow(ctx, port.StartWorkflowParams{
        Namespace:    resolveNamespace(req.Namespace),
        ID:           id,
        WorkflowType: req.WorkflowType,
        TaskQueue:    req.TaskQueue,
        Input:        req.Input,
    })
    if err != nil {
        return nil, domainErrorToGRPC(err)
    }
    return &apiv1.StartWorkflowResponse{WorkflowId: wf.ID.String()}, nil
}

func (h *Handler) PollWorkflowTask(ctx context.Context, req *apiv1.PollWorkflowTaskRequest) (*apiv1.PollWorkflowTaskResponse, error) {
    result, err := h.wfTask.PollWorkflowTask(ctx, resolveNamespace(req.Namespace), req.QueueName, req.WorkerId)
    if err != nil {
        return nil, domainErrorToGRPC(err)
    }
    if result == nil {
        // タスクなし: 空レスポンスを返す（エラーではない）
        return &apiv1.PollWorkflowTaskResponse{}, nil
    }
    return &apiv1.PollWorkflowTaskResponse{
        TaskId:       result.Task.ID,
        WorkflowId:   result.Task.WorkflowID.String(),
        WorkflowType: result.WorkflowType,
        Events:       domainEventsToProto(result.Events),
    }, nil
}

// domainErrorToGRPC はドメインエラーをgRPCステータスコードに変換する。
// engine/には依存せず、domain/のエラーのみを使う。
func domainErrorToGRPC(err error) error {
    switch {
    case errors.Is(err, domain.ErrWorkflowNotFound):
        return status.Errorf(codes.NotFound, "%v", err)
    case errors.Is(err, domain.ErrWorkflowAlreadyExists):
        return status.Errorf(codes.AlreadyExists, "%v", err)
    case errors.Is(err, domain.ErrWorkflowNotRunning):
        return status.Errorf(codes.FailedPrecondition, "%v", err)
    case errors.Is(err, domain.ErrTaskNotFound):
        return status.Errorf(codes.NotFound, "%v", err)
    case errors.Is(err, domain.ErrTaskAlreadyCompleted):
        return status.Errorf(codes.FailedPrecondition, "%v", err)
    case errors.Is(err, domain.ErrNamespaceNotFound):
        return status.Errorf(codes.NotFound, "%v", err)
    default:
        return status.Errorf(codes.Internal, "internal error: %v", err)
    }
}
```

#### adapter/postgres/ - Outbound Adapter

```go
// adapter/postgres/store.go
type Store struct {
    db *sql.DB
}

// RunInTx: port.TxManager を満たす
// context経由でトランザクションを伝搬する。
// 全てのリポジトリ実装はconn()メソッドでcontextからトランザクションを取得する。
// 既にトランザクション内の場合はネストせず再利用する。
func (s *Store) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
    if txFromContext(ctx) != nil {
        return fn(ctx)
    }

    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    if err := fn(withTx(ctx, tx)); err != nil {
        return err
    }
    return tx.Commit()
}

func (s *Store) Namespaces() port.NamespaceRepository         { return &NamespaceStore{store: s} }
func (s *Store) Workflows() port.WorkflowRepository          { return &WorkflowStore{store: s} }
func (s *Store) Events() port.EventRepository                { return &EventStore{store: s} }
func (s *Store) WorkflowTasks() port.WorkflowTaskRepository  { return &WorkflowTaskStore{store: s} }
func (s *Store) ActivityTasks() port.ActivityTaskRepository   { return &ActivityTaskStore{store: s} }
func (s *Store) Timers() port.TimerRepository                { return &TimerStore{store: s} }
func (s *Store) Queries() port.QueryRepository               { return &QueryStore{store: s} }
```

### 依存の組み立て（cmd/dandori/main.go）

```go
func main() {
    // 環境変数（DATABASE_URL, GRPC_PORT, HTTP_PORT）
    databaseURL := envOrDefault("DATABASE_URL", "postgres://dandori:dandori@localhost:5432/dandori?sslmode=disable")
    grpcPort := envOrDefault("GRPC_PORT", "7233")
    httpPort := envOrDefault("HTTP_PORT", "8080")

    // OpenTelemetry初期化（OTEL_EXPORTER_OTLP_ENDPOINT未設定時はno-op）
    tracer, tracerShutdown, _ := telemetry.InitTracer(ctx)
    defer tracerShutdown(context.Background())

    // Prometheusレジストリ
    reg := prometheus.NewRegistry()
    reg.MustRegister(collectors.NewProcessCollector(...), collectors.NewGoCollector())
    metrics := telemetry.NewMetrics(reg)

    // DB接続 + ping + プール設定
    db, _ := sql.Open("postgres", databaseURL)
    db.SetMaxOpenConns(25)
    db.SetMaxIdleConns(5)
    db.SetConnMaxLifetime(5 * time.Minute)

    // embed.FSマイグレーション（schema_migrationsテーブルによるバージョン管理）
    postgres.RunMigrations(context.Background(), db)

    // Outbound Adapter
    store := postgres.New(db)

    // Application Core
    eng := engine.New(
        store.Namespaces(),
        store.Workflows(),
        store.Events(),
        store.WorkflowTasks(),
        store.ActivityTasks(),
        store.Timers(),
        store.Queries(),
        store, // TxManager
    )

    // Background Worker（Engineとは別の構造体）
    bgWorker := engine.NewBackgroundWorker(
        store.Workflows(),
        store.Events(),
        store.WorkflowTasks(),
        store.ActivityTasks(),
        store.Timers(),
        store,
    )

    // Observabilityデコレータチェーン: engine → tracing → metrics → handler
    tracedClient := telemetry.NewTracingClientService(eng, tracer)
    tracedWFTask := telemetry.NewTracingWorkflowTaskService(eng, tracer)
    tracedActTask := telemetry.NewTracingActivityTaskService(eng, tracer)

    metricsClient := telemetry.NewMetricsClientService(tracedClient, metrics)
    metricsWFTask := telemetry.NewMetricsWorkflowTaskService(tracedWFTask, metrics)
    metricsActTask := telemetry.NewMetricsActivityTaskService(tracedActTask, metrics)

    // Inbound Adapter（デコレータ済みインターフェースを渡す）
    handler := grpcadapter.NewHandler(metricsClient, metricsWFTask, metricsActTask)

    // gRPCサーバー起動（OTel StatsHandler + Health + reflection有効）
    srv := grpc.NewServer(grpcadapter.OTelServerOptions()...)
    apiv1.RegisterDandoriServiceServer(srv, handler)
    grpc_health_v1.RegisterHealthServer(srv, grpcadapter.NewHealthServer(db))
    reflection.Register(srv)
    go srv.Serve(lis)

    // HTTPサーバー（gRPC-Gateway + /healthz + /metrics + /ui/ Web UI + pprof）
    gatewayMux, _ := httpadapter.NewGatewayMux(ctx, grpcAddr)
    extraHandlers := map[string]http.Handler{
        "/healthz": httpadapter.NewHealthHandler(db),
        "/metrics": promhttp.HandlerFor(reg, promhttp.HandlerOpts{}),
        "/ui/":     httpadapter.NewUIHandler(),
    }
    // ENABLE_PPROF=true で /debug/pprof/ エンドポイントを有効化
    if os.Getenv("ENABLE_PPROF") == "true" {
        // net/http/pprof の Index, Cmdline, Profile, Symbol, Trace を登録
        extraHandlers["/debug/pprof/"] = pprofMux
    }
    httpHandler := httpadapter.NewHTTPHandler(gatewayMux, extraHandlers)
    go httpSrv.ListenAndServe()

    // graceful shutdown（SIGINT/SIGTERM → cancel → HTTP Shutdown → GracefulStop → db.Close）
    sig := <-sigCh
    cancel()
    httpSrv.Shutdown(shutdownCtx)
    srv.GracefulStop()
    db.Close()
}
```

### トランザクション境界

以下の操作は1トランザクション内で実行し、原子性を保証する:

| 操作 | トランザクション内で行うこと |
|------|---------------------------|
| StartWorkflow | 冪等性チェック + (終了済み再作成時は旧関連データ削除+クエリ削除) + WorkflowExecution作成(upsert) + イベント記録 + Workflow Task投入 |
| CompleteWorkflowTask | Advisory Lock取得 + タスク解決 + コマンド→イベント変換 + タスク生成 + ステータス更新 |
| FailWorkflowTask | タスク完了 + ワークフローFAILED更新 + WorkflowExecutionFailedイベント記録 |
| CompleteActivityTask | ワークフロー状態チェック + イベント記録 + Workflow Task投入 |
| FailActivityTask | ワークフロー状態チェック + リトライ判定 + イベント記録 + タスク再投入 or Workflow Task投入 |
| TerminateWorkflow | ワークフロー状態チェック + イベント記録 + ステータスをTERMINATEDに更新 |
| SignalWorkflow | ワークフロー状態チェック（RUNNING確認） + WorkflowSignaledイベント記録 + Workflow Task投入 |
| CancelWorkflow | ワークフロー状態チェック（RUNNING確認） + WorkflowCancelRequestedイベント記録 + Workflow Task投入 |

### バックグラウンドプロセス

BackgroundWorkerが管理する:

- タスクタイムアウト回収（`RunTaskRecovery`）: locked_untilを超えたタスクをPENDINGに戻す（visibility timeout）
- Activityタイムアウト監視（`RunActivityTimeoutChecker`）: 3種のタイムアウトを順次チェックする。(1) StartToCloseTimeout: timeout_at超過のRUNNINGタスクを検知。(2) ScheduleToCloseTimeout: schedule_to_close_timeout_at超過のPENDING/RUNNINGタスクを検知。(3) ScheduleToStartTimeout: schedule_to_start_timeout_at超過のPENDINGタスクを検知（CompletePendingで完了）。いずれもActivityTaskTimedOutイベントを記録してWorkflow Taskを生成する
- Heartbeatタイムアウト監視（`RunHeartbeatTimeoutChecker`）: heartbeat_at + heartbeat_timeout を超過したActivity Taskを検知し、ActivityTaskTimedOutイベントを記録してWorkflow Taskを生成する（handleTimedOutTaskを再利用）
- タイマーポーラー（`RunTimerPoller`）: 発火時刻到達のtimerを検知し、`MarkFired`（PENDINGガード付き）で二重発火を防止した上で、TimerFiredイベントを記録してWorkflow Taskを生成する

各プロセスはcontext.Done()でグレースフルに停止する。

### エラーモデル

各操作で返しうるドメインエラーを定義する:

| 操作 | 正常 | エラー |
|------|------|--------|
| StartWorkflow | *WorkflowExecution | ErrWorkflowAlreadyExists（同一ID+RUNNING） |
| DescribeWorkflow | *WorkflowExecution | ErrWorkflowNotFound |
| GetWorkflowHistory | []HistoryEvent | ErrWorkflowNotFound |
| TerminateWorkflow | nil | ErrWorkflowNotFound, ErrWorkflowNotRunning |
| SignalWorkflow | nil | ErrWorkflowNotFound, ErrWorkflowNotRunning |
| CancelWorkflow | nil | ErrWorkflowNotFound, ErrWorkflowNotRunning |
| PollWorkflowTask | *WorkflowTaskResult (nil=タスクなし) | — |
| CompleteWorkflowTask | nil | ErrTaskNotFound, ErrTaskAlreadyCompleted |
| FailWorkflowTask | nil | ErrTaskNotFound |
| PollActivityTask | *ActivityTask (nil=タスクなし) | — |
| CompleteActivityTask | nil | ErrTaskNotFound, ErrTaskAlreadyCompleted |
| FailActivityTask | nil | ErrTaskNotFound, ErrTaskAlreadyCompleted |
| RecordActivityHeartbeat | nil | ErrTaskNotFound |
| QueryWorkflow | *WorkflowQuery | ErrWorkflowNotFound, ErrWorkflowNotRunning, ErrQueryTimedOut |
| RespondQueryTask | nil | ErrQueryNotFound |

gRPCハンドラはdomainErrorToGRPC()で一元的にgRPCステータスコードに変換する:

| ドメインエラー | gRPC ステータス |
|---|---|
| ErrWorkflowNotFound | NOT_FOUND |
| ErrWorkflowAlreadyExists | ALREADY_EXISTS |
| ErrWorkflowNotRunning | FAILED_PRECONDITION |
| ErrTaskNotFound | NOT_FOUND |
| ErrTaskAlreadyCompleted | FAILED_PRECONDITION |
| ErrNoTaskAvailable | （エラーではなく空レスポンス） |
| ErrQueryNotFound | NOT_FOUND |
| ErrQueryTimedOut | DEADLINE_EXCEEDED |
| ErrNamespaceNotFound | NOT_FOUND |
| その他 | INTERNAL |

## 3. Deterministic Replayの仕組み

### 処理シーケンス

1. クライアントがStartWorkflowを呼ぶ
2. サーバーがWorkflowExecutionStartedイベントを記録し、Workflow Taskをキューに投入
3. ワーカーがWorkflow Taskを取得し、イベント履歴とともにワークフロー関数を最初から実行
4. ワークフロー関数内でExecuteActivityが呼ばれると、SDKがイベント履歴を確認:
   - 完了イベントがあれば記録された結果を即座に返す（replay）
   - なければScheduleActivityTaskコマンドを生成し、ワークフロー関数をサスペンド
5. コマンドリストがCompleteWorkflowTaskでサーバーに返される
6. サーバーがコマンドをイベントに変換し、Activity Taskをキューに投入
7. ワーカーがActivity Taskを取得し、Activity関数を実際に実行して結果を報告
8. サーバーがActivityTaskCompletedイベントを記録し、新しいWorkflow Taskを生成
9. ワーカーが再度ワークフロー関数を最初からreplay。既完了のActivityは履歴から結果が返り、新しいActivityに到達する
10. ワークフロー関数が最後まで実行されるとCompleteWorkflowコマンドが返される

### 非決定性エラーの処理

ワーカーがreplay中にseqIDの不一致を検出した場合:

1. ワーカーはFailWorkflowTask APIを呼び、cause="NonDeterminismError"と詳細メッセージをサーバーに報告する
2. サーバーはタスクを完了としてマークし、エラーをログに記録する
3. ワークフロー自体は停止状態となり、コード修正後にワーカーが再起動されると新しいWorkflow Taskで再試行される

### seqIDによるコマンドとイベントの対応付け

ワークフロー関数内でExecuteActivityやSleepが呼ばれるたびにseqIDがインクリメントされる。
replay時にこのseqIDをキーとしてイベント履歴を検索する。

例: 3つのActivityを順次呼ぶワークフロー

```text
[0] WorkflowExecutionStarted
[1] ActivityTaskScheduled  {seqID: 0, activityType: "validate-order"}
[2] ActivityTaskCompleted  {seqID: 0, result: {...}}
[3] ActivityTaskScheduled  {seqID: 1, activityType: "charge-payment"}
[4] ActivityTaskCompleted  {seqID: 1, result: {...}}
```

## 4. コマンドとイベントの関係

### コマンド一覧（ワーカー → サーバー）

| コマンド | 説明 |
|---------|------|
| ScheduleActivityTask | Activity実行を要求（RetryPolicy, StartToCloseTimeout付き） |
| CompleteWorkflow | ワークフロー正常完了 |
| FailWorkflow | ワークフロー異常終了 |
| StartTimer | タイマー開始（SeqID, Duration指定） |
| CancelTimer | タイマーキャンセル（PENDING状態のタイマーのみキャンセル可能） |
| StartChildWorkflow | 子ワークフロー起動（SeqID, WorkflowType, TaskQueue, Input指定） |
| RecordSideEffect | 非決定的な値の記録（SeqID, Value指定。WorkflowTask未生成） |
| ContinueAsNew | ワークフローを終了し新ワークフローを自動作成（WorkflowType, TaskQueue, Input指定。未指定は現WF値を引き継ぐ） |

Saga関連のコマンドはない（Pure SDKパターンのため、サーバーは通常のActivityコマンドとして処理する）。observability用にCompleteWorkflowTaskのmetadataフィールドを追加済み（Sprint 12）。SDK側がsaga実行中かどうかのヒントを付与可能

### コマンド → イベント変換

| コマンド | 生成されるイベント | 副作用 |
|---------|-------------------|--------|
| ScheduleActivityTask | ActivityTaskScheduled | Activity Taskをキューに投入 |
| CompleteWorkflow | WorkflowExecutionCompleted | ステータスをCOMPLETEDに更新。親WFがある場合はChildWorkflowExecutionCompletedイベント伝搬。cron_schedule付きWFの場合はContinue-as-Newで自動再起動 |
| FailWorkflow | WorkflowExecutionFailed | ステータスをFAILEDに更新。親WFがある場合はChildWorkflowExecutionFailedイベント伝搬 |
| StartTimer | TimerStarted | timersテーブルにPENDINGレコード作成 |
| CancelTimer | TimerCanceled（PENDINGの場合のみ） | timersテーブルのstatusをCANCELEDに更新。既にFIREDの場合はno-op |
| StartChildWorkflow | ChildWorkflowExecutionStarted（親）+ WorkflowExecutionStarted（子） | 子ワークフロー作成 + 子のWorkflow Task投入 |
| RecordSideEffect | SideEffectRecorded | なし（イベント記録のみ、WorkflowTask未生成） |
| ContinueAsNew | WorkflowExecutionContinuedAsNew（旧WF）+ WorkflowExecutionStarted（新WF） | 旧WFをCONTINUED_AS_NEWに更新、新WF作成、continued_as_new_id設定、新WFのWorkflow Task投入 |

### 外部トリガー → イベント

| トリガー | 生成されるイベント | 副作用 |
|---------|-------------------|--------|
| StartWorkflow API | WorkflowExecutionStarted | Workflow Task生成 |
| Activity完了報告 | ActivityTaskCompleted | Workflow Task生成 |
| Activity失敗報告（リトライ不可） | ActivityTaskFailed | Workflow Task生成 |
| Activity失敗報告（リトライ可） | （イベントなし） | 同一Activity Taskを再キューイング |
| Activityタイムアウト検知 | ActivityTaskTimedOut | Workflow Task生成 |
| タイマー発火検知（TimerPoller） | TimerFired | Workflow Task生成 |
| TerminateWorkflow API | WorkflowExecutionTerminated | ステータスをTERMINATEDに更新 |
| SignalWorkflow API | WorkflowSignaled | Workflow Task生成 |
| CancelWorkflow API | WorkflowCancelRequested | Workflow Task生成（ワークフロー状態は変更しない） |
| Heartbeatタイムアウト検知 | ActivityTaskTimedOut | Workflow Task生成 |
| 子ワークフロー完了 | ChildWorkflowExecutionCompleted（親） | 親のWorkflow Task生成 |
| 子ワークフロー失敗 | ChildWorkflowExecutionFailed（親） | 親のWorkflow Task生成 |
| QueryWorkflow API | （イベントなし） | workflow_queriesにPENDINGクエリ作成 + Workflow Task生成 |

### イベントタイプ一覧

MVP + Phase 2実装済み: WorkflowExecutionStarted, WorkflowExecutionCompleted, WorkflowExecutionFailed, WorkflowExecutionTerminated, ActivityTaskScheduled, ActivityTaskCompleted, ActivityTaskFailed, ActivityTaskTimedOut, TimerStarted, TimerFired, TimerCanceled, WorkflowSignaled, WorkflowCancelRequested

Phase 3実装済み: ChildWorkflowExecutionStarted, ChildWorkflowExecutionCompleted, ChildWorkflowExecutionFailed, SideEffectRecorded, WorkflowExecutionContinuedAsNew

## 5. Saga / 補償トランザクションの設計

### 設計判断: Pure SDKパターン

Sagaの補償ロジックはTemporalと同じくSDK側で完結するパターンを採用する。サーバーはsagaという概念を持たない。

**サーバー側の変更が不要な理由:**

dandoriサーバーは既に多ラウンド実行（Activity完了 → 新Workflow Task → 次のActivity）をサポートしている。Sagaの補償実行は「エラー発生後に逆順でActivityを実行する」だけであり、サーバーから見ると通常のActivity実行と全く同じに見える。

### 多ラウンド実行シーケンス（Saga補償の4ラウンド例）

```text
Round 1: book-flight (成功)
  Worker → CompleteWorkflowTask [ScheduleActivity("book-flight")]
  Server → ActivityTaskScheduled → Activity完了 → ActivityTaskCompleted → WorkflowTask

Round 2: book-hotel (成功)
  Worker → CompleteWorkflowTask [ScheduleActivity("book-hotel")]
  Server → ActivityTaskScheduled → Activity完了 → ActivityTaskCompleted → WorkflowTask

Round 3: book-car (失敗)
  Worker → CompleteWorkflowTask [ScheduleActivity("book-car")]
  Server → ActivityTaskScheduled → Activity失敗 → ActivityTaskFailed → WorkflowTask

Round 4: 補償実行 (cancel-hotel, cancel-flight を逆順で実行)
  Worker → CompleteWorkflowTask [ScheduleActivity("cancel-hotel")]
  Server → ActivityTaskScheduled → Activity完了 → ActivityTaskCompleted → WorkflowTask
  Worker → CompleteWorkflowTask [ScheduleActivity("cancel-flight")]
  Server → ActivityTaskScheduled → Activity完了 → ActivityTaskCompleted → WorkflowTask
  Worker → CompleteWorkflowTask [FailWorkflow(original error)]
  Server → WorkflowExecutionFailed
```

サーバーは各ラウンドを既存の仕組みで処理するだけであり、sagaであることを意識しない。

### Observability用metadataフィールド（実装済み）

observability向上のためにCompleteWorkflowTaskRequestにオプショナルなmetadataフィールドを追加している:

```protobuf
message CompleteWorkflowTaskRequest {
  int64 task_id = 1;
  repeated Command commands = 2;
  map<string, string> metadata = 3;
}
```

SDK側がsaga実行中に `{"saga_compensating": "true"}` のようなヒントを設定することで、ログやトレーシングでsaga補償フェーズを識別可能にする。

metadataの伝搬フロー:

1. gRPCハンドラがリクエストのmetadataを全domain.Commandに設定
2. command_processor.goの各process関数がmarshalEventDataヘルパーでevent_dataにmetadataを含めて保存
3. metadataが空の場合は既存と同じJSON構造を維持（後方互換性）
4. GetWorkflowHistoryでevent_data内のmetadataを取得可能

### SDK側saga APIの設計（dandori-sdk-goリポジトリで実装）

```go
// saga/saga.go
type Saga struct {
    compensations []compensation
    options       Options
}

type Options struct {
    ParallelCompensation bool // true: 補償を並行実行、false: 逆順で直列実行
    ContinueWithError    bool // true: 補償失敗時も残りの補償を続行
}

func New(opts Options) *Saga { ... }

// AddCompensation は補償アクションを登録する。
// 内部的には通常のActivityと同じ。
func (s *Saga) AddCompensation(ctx workflow.Context, activityType string, input any, opts ...workflow.ActivityOption) { ... }

// Compensate は登録された補償を逆順で実行する。
// 各補償は通常のExecuteActivityとして実行される。
// originalErr は補償完了後に返される元のエラー。
func (s *Saga) Compensate(ctx workflow.Context, originalErr error) error { ... }
```

### Compensate実行フロー

```text
1. Compensate(ctx, err) が呼ばれる
2. 登録された補償リストを逆順に取り出す
3. 各補償に対して workflow.ExecuteActivity() を呼ぶ
   → サーバーから見ると通常のActivity実行
4. ContinueWithError=false の場合: 補償失敗で即座にエラー返却
   ContinueWithError=true の場合: 補償失敗を記録し、残りの補償を続行
5. 全補償完了後、originalErr を返す（補償エラーがあればラップして返す）
```

### エッジケースと対応方針

| ケース | 対応 |
|--------|------|
| 補償Activity自体が失敗 | ContinueWithError設定に従う。RetryPolicyで自動リトライも可能 |
| ワークフローがTerminateされた | 補償は実行されない（Terminateはgracefulではない） |
| ワーカーがクラッシュ | deterministic replayで状態復元。補償の途中からでも再開可能 |
| 補償が冪等でない | ユーザー責任。ドキュメントで冪等性の重要性を明記する |

## 5b. Child Workflowの設計

### 概要

親ワークフローから子ワークフローを起動し、子の完了/失敗が親に自動伝搬される仕組み。

### データモデル

`workflow_executions`テーブルに`parent_workflow_id`と`parent_seq_id`カラムを追加。子ワークフローは親のWorkflow IDとStartChildWorkflowコマンドのSeqIDを保持する。

### StartChildWorkflowコマンドの処理フロー

1. ワーカーがStartChildWorkflowコマンドを送信（SeqID, WorkflowType, TaskQueue, Input指定）
2. Engine（processStartChildWorkflow）が子ワークフローを作成（ParentWorkflowID, ParentSeqID付き）
3. 親ワークフローにChildWorkflowExecutionStartedイベントを記録
4. 子ワークフローにWorkflowExecutionStartedイベントを記録
5. 子ワークフローのWorkflow Taskをキューに投入
6. TaskQueue未指定の場合は親のTaskQueueを継承

### 子ワークフロー完了/失敗時の親への伝搬

propagateToParentヘルパーで共通処理:

1. 子ワークフローのParentWorkflowIDがnilなら何もしない（通常のワークフロー）
2. 親ワークフローをGet
3. 親ワークフローがRUNNINGでなければ何もしない（既に終了済み）
4. 親ワークフローにChildWorkflowExecutionCompleted/Failedイベントを記録
5. 親ワークフローのWorkflow Taskをキューに投入

この伝搬はprocessCompleteWorkflow、processFailWorkflow、FailWorkflowTaskの各処理末尾で実行される。

### Child Workflowのエッジケース

| ケース | 対応 |
|--------|------|
| 親が既に終了済み | 伝搬をスキップ（親のステータスがRUNNINGでない場合） |
| 子ワークフローが通常のワークフロー | ParentWorkflowIDがnilなので伝搬処理は発生しない |
| FailWorkflowTaskで子が失敗 | 親にChildWorkflowExecutionFailedイベントを伝搬 |

## 5c. SideEffectの設計

### 概要

ワークフロー関数内で非決定的な値（UUID生成、現在時刻取得等）を安全に使用するための仕組み。初回実行時に値をイベントとして記録し、リプレイ時にはイベントから値を復元する。

### サーバー側の実装

サーバーの責務は最小限:

1. `RecordSideEffect`コマンドを受け取る（SeqID + Value）
2. `SideEffectRecorded`イベントを記録する
3. WorkflowTaskは生成しない（値の記録のみで次のステップに進む必要がないため）

リプレイ時の値復元はSDK側の責務。SDKはイベント履歴にSideEffectRecordedイベントがあれば記録された値を返し、なければ関数を実行して値を生成しRecordSideEffectコマンドをサーバーに送信する。

### コマンドとイベント

| コマンド | 生成されるイベント | 副作用 |
|---------|-------------------|--------|
| RecordSideEffect | SideEffectRecorded | なし（イベント記録のみ） |

DBスキーマ変更不要。既存の`workflow_events`テーブルにイベントが保存される。

## 5d. Queryの設計

### 概要

外部からRUNNINGワークフローの状態を問い合わせる仕組み。Queryはイベント履歴に記録されないside channel通信。

### データモデル

`workflow_queries`テーブルにクエリを永続化する。ステータスはPENDING → ANSWEREDの遷移。

### 処理フロー

1. クライアントがQueryWorkflow APIを呼ぶ
2. Engine.QueryWorkflow:
   - ワークフローがRUNNINGであることを確認
   - workflow_queriesにPENDINGクエリを作成
   - Workflow Taskをキューに投入（ワーカーにクエリ処理を促す）
   - ポーリングループでクエリ結果を待つ（100msインターバル、10秒タイムアウト）
3. ワーカーがPollWorkflowTaskでタスクを取得すると、レスポンスにpending_queriesが含まれる
4. ワーカーはワークフロー関数をリプレイし、登録されたクエリハンドラで結果を生成
5. ワーカーがRespondQueryTask APIで結果を返す
6. Engine.RespondQueryTaskがworkflow_queriesのステータスをANSWEREDに更新
7. QueryWorkflowのポーリングループが結果を検出し、クライアントに返す

### 設計判断

- Queryはワークフローイベント履歴に記録しない（side channelであり履歴の一部ではない）
- ポーリング待ちはEngine層で実装。将来的にLISTEN/NOTIFYで最適化可能
- queryTimeoutはEngineのフィールドで管理し、テスト時に短縮可能
- BackgroundWorkerにqueries不要（Queryのライフサイクルはリクエスト駆動）
- CLIのqueryサブコマンドは今後のスコープ

## 5e. Continue-as-New / Cronスケジュールの設計

### 概要

Continue-as-Newはワークフローを終了し、同じワークフロータイプで新しいワークフローを自動作成する仕組み。イベント履歴の肥大化を防ぎ、長期実行ワークフローを効率的に管理する。Cronスケジュールはワークフロー完了時に自動でContinue-as-Newを実行し、定期実行を実現する。

### データモデル

`workflow_executions`テーブルに`cron_schedule`と`continued_as_new_id`カラムを追加（マイグレーション000006）。`continued_as_new_id`は次のワークフローへの外部キー参照を持つ。

### ContinueAsNewコマンドの処理フロー

1. ワーカーがContinueAsNewコマンドを送信（WorkflowType, TaskQueue, Input指定。未指定は現WF値を引き継ぐ）
2. Engine（processContinueAsNew）がcontinueAsNewヘルパーを呼び出す
3. 旧ワークフローをCONTINUED_AS_NEWステータスに更新
4. 新ワークフローを作成（CronScheduleを引き継ぐ）
5. continued_as_new_idを設定（旧WF → 新WF追跡用）
6. 旧WFにWorkflowExecutionContinuedAsNewイベントを記録
7. 新WFにWorkflowExecutionStartedイベントを記録
8. 新WFのWorkflow Taskをキューに投入

### Cronスケジュール

StartWorkflow時にcron_scheduleを指定可能。5フィールドcron式（分 時 日 月 曜日）。`robfig/cron/v3`で検証。

CompleteWorkflow時にcron_schedule付きのワークフローは:

1. WorkflowExecutionCompletedイベントを記録
2. continueAsNewヘルパーで自動的に新ワークフローを作成
3. 新ワークフローにcron_scheduleが引き継がれ、連鎖的に再起動する

FailWorkflow時はCron再起動しない。失敗は明示的な対処が必要なため。

### エッジケース

| ケース | 対応 |
|--------|------|
| ContinueAsNew + 親ワークフロー | 新WFにはparent_workflow_idを引き継がない。旧WFでpropagateToParentも行わない |
| CronWorkflow + FailWorkflow | 自動再起動しない（processCompleteWorkflowのみCron処理を実行） |
| CronSchedule引き継ぎ | continueAsNewで新WFにcron_scheduleを引き継ぐことで連鎖再起動が機能する |
| 不正なcron式 | StartWorkflow時にバリデーションエラーを返す |

## 6. workflow.Contextの設計（Go SDKリポジトリで実装）

### 内部構造

```go
type Context struct {
    env *workflowEnvironment
}

type workflowEnvironment struct {
    events       []HistoryEvent
    commands     []Command
    isReplaying  bool
    scheduler    *coroutineScheduler
    nextSeqID    int64
}
```

### coroutineScheduler

ワークフロー関数は独立したgoroutine上で実行され、yieldでサスペンドされる。
協調スケジューラパターンでメインgoroutineとワークフローgoroutineの間の制御を切り替える。

```go
type coroutineScheduler struct {
    mainCh     chan struct{}
    workflowCh chan struct{}
    ctx        context.Context
    completed  bool
    err        error
}
```

### ExecuteActivityのreplayロジック

```go
func ExecuteActivity[I, O any](ctx Context, activityType string, input I, opts ...ActivityOption) (O, error) {
    env := ctx.env
    seqID := env.nextSeqID
    env.nextSeqID++

    if completedEvent := env.findCompletionEvent(seqID); completedEvent != nil {
        var result O
        json.Unmarshal(completedEvent.Result, &result)
        return result, completedEvent.Error
    }

    if failedEvent := env.findFailureEvent(seqID); failedEvent != nil {
        var zero O
        return zero, errors.New(failedEvent.ErrorMessage)
    }

    if scheduledEvent := env.findScheduledEvent(seqID); scheduledEvent != nil {
        env.scheduler.yield()
        var zero O
        return zero, workflow.ErrDestroyWorkflow
    }

    options := applyActivityOptions(opts...)
    env.commands = append(env.commands, Command{
        Type: CommandScheduleActivityTask,
        Attributes: marshalJSON(ScheduleActivityTaskAttributes{
            SeqID:               seqID,
            ActivityType:        activityType,
            Input:               marshalJSON(input),
            StartToCloseTimeout: options.StartToCloseTimeout,
            RetryPolicy:         options.RetryPolicy,
        }),
    })
    env.scheduler.yield()
    var zero O
    return zero, workflow.ErrDestroyWorkflow
}
```

yield()後のコードパスについて: Workflow Task処理完了時にcontextがキャンセルされ、goroutineは終了する。`ErrDestroyWorkflow` はgoroutineクリーンアップ時の安全ガードである。

## 7. PostgreSQLスキーマ

### namespaces テーブル

```sql
CREATE TABLE namespaces (
    name         VARCHAR(255) PRIMARY KEY,
    description  TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- デフォルトnamespaceの挿入
INSERT INTO namespaces (name) VALUES ('default');
```

### workflow_executions テーブル

```sql
CREATE TABLE workflow_executions (
    id              UUID PRIMARY KEY,
    namespace       VARCHAR(255) NOT NULL DEFAULT 'default' REFERENCES namespaces(name),
    workflow_type   TEXT NOT NULL,
    task_queue      TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'RUNNING',
    input           JSONB,
    result          JSONB,
    error_message   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at       TIMESTAMPTZ,
    parent_workflow_id UUID REFERENCES workflow_executions(id),
    parent_seq_id  INTEGER,
    cron_schedule  VARCHAR(255),
    continued_as_new_id UUID REFERENCES workflow_executions(id)
);

CREATE INDEX idx_workflow_executions_status ON workflow_executions(status);
CREATE INDEX idx_workflow_executions_parent ON workflow_executions(parent_workflow_id)
    WHERE parent_workflow_id IS NOT NULL;
```

### workflow_events テーブル

ハッシュパーティショニング（workflow_idベース、16分割）を採用。`WHERE workflow_id = $1` によるパーティションプルーニングが有効。マイグレーション000009で通常テーブルからパーティションテーブルに変換される（冪等）。

```sql
CREATE TABLE workflow_events (
    id           BIGINT NOT NULL DEFAULT nextval('workflow_events_id_seq'),
    workflow_id  UUID NOT NULL,
    sequence_num INT NOT NULL,
    event_type   TEXT NOT NULL,
    event_data   JSONB NOT NULL,
    timestamp    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (workflow_id, sequence_num)
) PARTITION BY HASH (workflow_id);

-- 16分割パーティション（p0〜p15）
CREATE TABLE workflow_events_p0  PARTITION OF workflow_events FOR VALUES WITH (MODULUS 16, REMAINDER 0);
-- ... p1〜p14 ...
CREATE TABLE workflow_events_p15 PARTITION OF workflow_events FOR VALUES WITH (MODULUS 16, REMAINDER 15);

CREATE INDEX idx_workflow_events_workflow_id ON workflow_events (workflow_id);
```

### workflow_tasks テーブル

WorkflowTaskとActivityTaskはスキーマが異なるため、テーブルを分離する。

```sql
CREATE TABLE workflow_tasks (
    id              BIGSERIAL PRIMARY KEY,
    namespace       VARCHAR(255) NOT NULL DEFAULT 'default' REFERENCES namespaces(name),
    queue_name      TEXT NOT NULL,
    workflow_id     UUID NOT NULL REFERENCES workflow_executions(id),
    status          TEXT NOT NULL DEFAULT 'PENDING',
    scheduled_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at      TIMESTAMPTZ,
    locked_by       TEXT,
    locked_until    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_workflow_tasks_poll
    ON workflow_tasks(queue_name, status, scheduled_at)
    WHERE status = 'PENDING';
```

### activity_tasks テーブル

```sql
CREATE TABLE activity_tasks (
    id                     BIGSERIAL PRIMARY KEY,
    namespace              VARCHAR(255) NOT NULL DEFAULT 'default' REFERENCES namespaces(name),
    queue_name             TEXT NOT NULL,
    workflow_id            UUID NOT NULL REFERENCES workflow_executions(id),
    activity_type          TEXT NOT NULL,
    activity_input         JSONB,
    activity_seq_id        BIGINT NOT NULL,
    start_to_close_timeout INTERVAL,
    heartbeat_timeout      INTERVAL,
    heartbeat_at           TIMESTAMPTZ,
    schedule_to_close_timeout  INTERVAL,
    schedule_to_close_timeout_at TIMESTAMPTZ,
    schedule_to_start_timeout  INTERVAL,
    schedule_to_start_timeout_at TIMESTAMPTZ,
    retry_policy           JSONB,
    attempt                INT NOT NULL DEFAULT 1,
    max_attempts           INT NOT NULL DEFAULT 3,
    status                 TEXT NOT NULL DEFAULT 'PENDING',
    scheduled_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at             TIMESTAMPTZ,
    locked_by              TEXT,
    locked_until           TIMESTAMPTZ,
    timeout_at             TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_activity_tasks_poll
    ON activity_tasks(queue_name, status, scheduled_at)
    WHERE status = 'PENDING';

CREATE INDEX idx_activity_tasks_timeout
    ON activity_tasks(timeout_at)
    WHERE status = 'RUNNING' AND timeout_at IS NOT NULL;

CREATE INDEX idx_activity_tasks_heartbeat
    ON activity_tasks(heartbeat_at)
    WHERE status = 'RUNNING' AND heartbeat_timeout IS NOT NULL;

CREATE INDEX idx_activity_tasks_schedule_to_close_timeout
    ON activity_tasks(schedule_to_close_timeout_at)
    WHERE schedule_to_close_timeout_at IS NOT NULL;

CREATE INDEX idx_activity_tasks_schedule_to_start_timeout
    ON activity_tasks(schedule_to_start_timeout_at)
    WHERE status = 'PENDING' AND schedule_to_start_timeout_at IS NOT NULL;
```

タスク取得クエリ（Activity Task）:

```sql
UPDATE activity_tasks
SET status = 'RUNNING',
    locked_by = $1,
    locked_until = NOW() + INTERVAL '30 seconds',
    started_at = NOW(),
    timeout_at = CASE
        WHEN start_to_close_timeout IS NOT NULL
        THEN NOW() + start_to_close_timeout
        ELSE NULL
    END,
    heartbeat_at = CASE
        WHEN heartbeat_timeout IS NOT NULL
        THEN NOW()
        ELSE NULL
    END,
    schedule_to_start_timeout_at = NULL
WHERE id = (
    SELECT id FROM activity_tasks
    WHERE queue_name = $2
      AND status = 'PENDING'
      AND scheduled_at <= NOW()
    ORDER BY scheduled_at ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
RETURNING *;
```

### timers テーブル

```sql
CREATE TABLE timers (
    id              BIGSERIAL PRIMARY KEY,
    namespace       VARCHAR(255) NOT NULL DEFAULT 'default' REFERENCES namespaces(name),
    workflow_id     UUID NOT NULL REFERENCES workflow_executions(id),
    seq_id          BIGINT NOT NULL,
    fire_at         TIMESTAMPTZ NOT NULL,
    status          TEXT NOT NULL DEFAULT 'PENDING',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_timers_pending ON timers(fire_at) WHERE status = 'PENDING';
```

### workflow_queries テーブル

```sql
CREATE TABLE IF NOT EXISTS workflow_queries (
    id            BIGSERIAL PRIMARY KEY,
    workflow_id   UUID NOT NULL REFERENCES workflow_executions(id),
    query_type    TEXT NOT NULL,
    input         JSONB,
    result        JSONB,
    error_message TEXT,
    status        TEXT NOT NULL DEFAULT 'PENDING',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    answered_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_workflow_queries_pending
    ON workflow_queries(workflow_id, status)
    WHERE status = 'PENDING';
```

### PostgreSQL活用ポイント

- SKIP LOCKED: タスクキューの排他制御
- JSONB: ワークフロー入出力、イベントデータ、RetryPolicyの柔軟な格納
- Advisory Lock: CompleteWorkflowTask時にworkflow_idハッシュでpg_advisory_xact_lockを取得し、同一ワークフローのWorkflow Task処理を直列化
- UNIQUE(workflow_id, sequence_num): イベントの重複挿入を防止
- タイムアウト検知: timeout_atカラムによるActivity StartToCloseTimeout監視、heartbeat_at + heartbeat_timeoutによるHeartbeatタイムアウト監視、schedule_to_close_timeout_atによるScheduleToCloseTimeout監視、schedule_to_start_timeout_atによるScheduleToStartTimeout監視
- ハッシュパーティショニング: workflow_eventsテーブルをworkflow_idベースで16分割し、大量イベントの読み書きを高速化（パーティションプルーニング有効）
- LISTEN/NOTIFY: タスク投入の即時通知（Phase 2）

## 8. gRPCサービス定義

```protobuf
service DandoriService {
  // === クライアント向け API ===
  rpc StartWorkflow(StartWorkflowRequest) returns (StartWorkflowResponse);
  rpc DescribeWorkflow(DescribeWorkflowRequest) returns (DescribeWorkflowResponse);
  rpc GetWorkflowHistory(GetWorkflowHistoryRequest) returns (GetWorkflowHistoryResponse);
  rpc TerminateWorkflow(TerminateWorkflowRequest) returns (TerminateWorkflowResponse);
  rpc SignalWorkflow(SignalWorkflowRequest) returns (SignalWorkflowResponse);
  rpc CancelWorkflow(CancelWorkflowRequest) returns (CancelWorkflowResponse);
  rpc ListWorkflows(ListWorkflowsRequest) returns (ListWorkflowsResponse);
  rpc QueryWorkflow(QueryWorkflowRequest) returns (QueryWorkflowResponse);

  // === ワーカー向け API: Workflow Task ===
  rpc PollWorkflowTask(PollWorkflowTaskRequest) returns (PollWorkflowTaskResponse);
  rpc CompleteWorkflowTask(CompleteWorkflowTaskRequest) returns (CompleteWorkflowTaskResponse);
  rpc FailWorkflowTask(FailWorkflowTaskRequest) returns (FailWorkflowTaskResponse);
  rpc RespondQueryTask(RespondQueryTaskRequest) returns (RespondQueryTaskResponse);

  // === ワーカー向け API: Activity Task ===
  rpc PollActivityTask(PollActivityTaskRequest) returns (PollActivityTaskResponse);
  rpc CompleteActivityTask(CompleteActivityTaskRequest) returns (CompleteActivityTaskResponse);
  rpc FailActivityTask(FailActivityTaskRequest) returns (FailActivityTaskResponse);
  rpc RecordActivityHeartbeat(RecordActivityHeartbeatRequest) returns (RecordActivityHeartbeatResponse);
}
```

メッセージ定義は前回のものと同一（§7の主要メッセージ定義を参照）。省略。

## 9. Go SDKリポジトリ構成（dandori-sdk-go）

```text
dandori-sdk-go/
├── client/
│   ├── client.go              # Client interface, Dial()
│   ├── options.go             # Options, StartWorkflowOptions
│   └── workflow_run.go        # WorkflowRun interface 実装
├── worker/
│   ├── worker.go
│   ├── workflow_task_processor.go
│   └── activity_task_processor.go
├── workflow/
│   ├── context.go
│   ├── activity.go            # ExecuteActivity[I,O]
│   ├── options.go             # ActivityOption, WithStartToCloseTimeout, WithRetryPolicy
│   ├── scheduler.go
│   └── env.go
├── saga/
│   ├── saga.go                # Saga struct, New(), AddCompensation(), Compensate()
│   └── options.go             # Options: ParallelCompensation, ContinueWithError
├── examples/
│   └── order/
└── go.mod
```

## 10. 実装の難所と対処方針

### トランザクションの一貫性

CompleteWorkflowTaskでは複数のイベント記録、タスク生成、ステータス更新を1トランザクションで行う。port.TxManagerとcontext伝搬でこれを実現する。

### Workflow Taskの直列化

CompleteWorkflowTask実行時にpg_advisory_xact_lock(hash(workflowID))を取得する。adapter/postgres/のWorkflowTaskStore.GetByID内でロックを取得し、トランザクション終了時に自動解放する。

### ワークフロー状態の整合性

CompleteActivityTask / FailActivityTask は対象ワークフローの状態をチェックする。ワークフローが既に終了（COMPLETED/FAILED/TERMINATED）している場合、結果は破棄してタスクだけ完了させる。

### ActivityリトライとseqIDの対応

リトライはサーバー側で管理。non_retryable=true または残り試行回数0で ActivityTaskFailed イベントが記録される。リトライ中はイベントを記録しない（ワークフローから見ると「まだ完了していないActivity」）。

### Activity StartToCloseTimeout

タスク取得時にtimeout_atを設定する。BackgroundWorkerのRunActivityTimeoutCheckerがtimeout_at超過のタスクを検知する。

### Activity ScheduleToCloseTimeout

Activityスケジュール時にschedule_to_close_timeout_atを設定する。リトライを跨いで有効（Requeue時もtimeout_atは維持される）。PENDING/RUNNING両方のタスクが対象。

### Activity ScheduleToStartTimeout

Activityスケジュール時にschedule_to_start_timeout_atを設定する。PENDINGタスクのみ対象。Poll時にschedule_to_start_timeout_atをNULLにクリアする。Requeue時は新しいscheduled_at + schedule_to_start_timeoutで再計算する。CompletePendingメソッドでPENDINGタスクを完了させる。

### ListWorkflows API

cursor-based paginationによるワークフロー一覧取得。カーソルは`created_at` + `id`の組み合わせをbase64 JSON化したトークン。フィルタ（status, workflow_type, task_queue, search_attributes）は動的WHERE句で構築。PageSizeデフォルト20、上限100。Engine層でPageSize+1件取得し、超過分で次ページ有無を判定する。

### Search Attributes

ワークフローに任意のキー-バリューペアをビジネスメタデータとして付与する機能。`workflow_executions`テーブルにJSONB型の`search_attributes`カラムを追加し、GINインデックスで高速な検索を実現する。

データモデル:

- `search_attributes JSONB DEFAULT '{}'::jsonb`: ワークフロー実行テーブルのカラム
- `idx_workflow_executions_search_attributes`: GINインデックス

コマンド: `UpsertSearchAttributes` - ワーカーがワークフロー実行中にSearch Attributesを追加・更新する。既存のattributesにマージされる（`search_attributes || $1::jsonb`）。

フィルタリング: ListWorkflows APIの`search_attributes_filter`パラメータで、JSONB `@>` 演算子による包含検索を行う。例: `{"outcome":"payment_success"}` を指定すると、そのキー-バリューを含むワークフローのみが返る。

Web UIでは「Outcome」カラムとしてSearch Attributesのうち`outcome`キーの値を人間に分かりやすいラベルで表示する。Outcomeドロップダウンでフィルタリングも可能。

### CLIツール（dandori-cli）

`cmd/dandori-cli/`にcobra製CLIバイナリを配置。`--server`フラグ（デフォルト`localhost:7233`）で接続先を指定。サブコマンド: start, describe, terminate, cancel, signal, list, history。api/v1の生成コードとgRPCクライアントのみに依存し、internal/パッケージは参照しない。`grpc.NewClient`で遅延接続するため、サーバー未起動時はRPC呼び出し時にエラーとなる。

### StartWorkflowの冪等性

同一IDで非終了状態のワークフローが存在する場合はErrWorkflowAlreadyExistsを返す。終了済みの場合は新規作成を許可する。チェックと作成は同一トランザクション内で実行する。

### エラー定義の配置

ドメインエラーはdomain/に定義する。これによりadapter/grpc/がengine/に依存することを防ぎ、Hexagonal Architectureの依存方向を維持する。

### Poll操作の振る舞い

タスクがない場合、リポジトリはdomain.ErrNoTaskAvailableを返す。gRPCハンドラはこれを空レスポンスに変換する（エラーではない）。SDKのワーカーは空レスポンスを受け取ると一定間隔後に再度Pollする。

### Saga補償の状態復元（Go SDK側）

ワーカーがクラッシュした場合、deterministic replayによりワークフロー関数が再実行される。Sagaの補償リストはワークフロー関数の通常の実行フロー内で再構築されるため、特別な永続化は不要。replayでbook-flight成功 → AddCompensation(cancel-flight) → book-hotel成功 → AddCompensation(cancel-hotel) → book-car失敗 → Compensate()と再実行されるので、補償リストは自然に復元される。Compensate内のExecuteActivityもreplayで既完了分はスキップされ、未完了の補償から再開される。

### goroutineの管理とリーク防止（Go SDK側）

Workflow Task処理完了時にcontextをcancelし、goroutineを適切に終了させる。

### WorkflowRun.Get()の実装（Go SDK側）

MVPではDescribeWorkflowのポーリングで実現する。

## 11. テスト戦略

### テスト構成

| テスト層 | パッケージ | テスト内容 | テスト数 |
|---------|-----------|-----------|---------|
| ユニットテスト | engine | モック構造体によるビジネスロジック検証 | 78 |
| ユニットテスト | adapter/grpc (grpc_test) | モックサービスによるハンドラ・エラーマッピング検証 | 16 |
| インテグレーションテスト | adapter/postgres (postgres_test) | testcontainers + PostgreSQL 16による全CRUD・Advisory Lock検証 | 76 |
| インテグレーションテスト | adapter/grpc (grpc_test) | testcontainers + Engine + PostgreSQL Storeの実スタック検証 | 8 |
| E2Eテスト | test/e2e (e2e_test) | bufconn gRPC + httptest HTTP + testcontainers + BackgroundWorker による全主要シナリオ検証 | 47 |
| ベンチマーク | test/bench (bench_test) | testcontainers + PostgreSQL 16によるスループット・レイテンシ計測（ワークフロー作成、イベント追記、タスクPoll/Complete、N並行ワーカー） | 10 |

### テストパターン

**モック構造体（関数フィールド型）:**

engine/ と adapter/grpc/ の両方で同一パターンを使用する。インターフェースの各メソッドに対応する関数フィールドを持ち、nil時はデフォルト値を返す。

```go
type mockClientService struct {
    StartWorkflowFn    func(ctx context.Context, params port.StartWorkflowParams) (*domain.WorkflowExecution, error)
    DescribeWorkflowFn func(ctx context.Context, id uuid.UUID) (*domain.WorkflowExecution, error)
    // ...
}
```

**testcontainersパターン:**

adapter/postgres/ と adapter/grpc/ の両方でTestMainを使い、postgres:16-alpineコンテナを起動する。adapter/grpc/ のTestMainでは postgres.RunMigrations() を再利用し、newTestHandler() で postgres.New → engine.New → grpc.NewHandler のDIを組み立てる。

**gRPCテストのアプローチ:**

adapter/grpc/ のインテグレーションテストではbufconn不使用。ハンドラメソッドを直接呼び出すことで、トランスポート層のオーバーヘッドなしにビジネスロジックの検証に集中する。全テストを `grpc_test` 外部テストパッケージに統一し、TestMainを1つに保つ。domainErrorToGRPC（unexported）はハンドラメソッド経由で間接テストする。

**E2Eテストのアプローチ:**

test/e2e/ では `google.golang.org/grpc/test/bufconn` を使い、実際のgRPCトランスポートスタック（シリアライズ/デシリアライズ、ステータスコード変換）を経由してテストする。HTTP APIテストでは `httptest.Server` + `RegisterDandoriServiceHandlerServer`（in-process方式）を使い、gRPC-Gateway経由のHTTPエンドポイントを検証する。Go SDK未完成の段階で、生gRPCクライアント（`apiv1.DandoriServiceClient`）によるワーカー動作シミュレーションで全主要シナリオを検証する。BackgroundWorker（timeout_checker=500ms, timer_poller=500ms, task_recovery=2s）もgoroutineで起動し、タイムアウト検知・タイマー発火・タスク回復も含めた統合動作を確認する。各テストは `truncateAll()` でテーブルを初期化し、Sequential実行で分離する。

**Advisory Lockテスト:**

同一ワークフローのWorkflow Task処理が直列化されることを、並行goroutine + time.Sleep(200ms) + タイムスタンプ比較で検証する。異なるワークフローのtaskが並行実行可能なことは総実行時間で検証する。

### CI（GitHub Actions）

```yaml
# .github/workflows/ci.yml
- go vet ./...          # 静的解析
- go build ./cmd/dandori # サーバービルド確認
- go build ./cmd/dandori-cli # CLIビルド確認
- go test -v -race -count=1 ./...  # 全テスト（ユニット + インテグレーション + E2E、race detector有効）
- アーキテクチャ制約チェック: adapter/grpc/ が engine/ をimportしていないことを検証
```

## 12. Phase 1 MVPの実装ステップ

### サーバー

1. プロジェクト骨格: go mod, docker-compose.yml, proto定義
2. domain/: 型定義 + エラー定義（ErrWorkflowNotFound等）
3. port/: 役割別Inbound Port（ClientService, WorkflowTaskService, ActivityTaskService）+ Outbound Port（WorkflowTaskRepository, ActivityTaskRepository を分離）
4. adapter/postgres/: PostgreSQL実装、テーブル分離（workflow_tasks, activity_tasks）
5. engine/: Engine（3つのInbound Port実装）、CommandProcessor、BackgroundWorker（別構造体）
6. adapter/grpc/: gRPCハンドラ（役割別インターフェース受取、domainErrorToGRPC）
7. cmd/dandori/: DI、サーバー起動、BackgroundWorker起動、graceful shutdown
8. ユニットテスト + インテグレーションテスト
9. E2Eテスト: bufconn gRPC + testcontainers + BackgroundWorkerによる全主要シナリオ検証（19テスト）

### Go SDK

1. クライアントSDK: Dial, ExecuteWorkflow, GetWorkflow, WorkflowRun
2. ワーカーSDK - Activityサイド
3. ワーカーSDK - Workflowサイド（核心）
4. 非決定性検出 + FailWorkflowTask報告
5. サンプルワークフロー
6. テスト
