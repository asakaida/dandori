# dandori - 設計書

## 1. アーキテクチャ概要

### 全体構成

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

### サーバーの責務

サーバーはワークフローのステップ定義を理解しない。以下に徹する:

1. イベントの永続化（append-only）
2. ワーカーから返されたコマンドの処理（コマンド→イベント変換）
3. タスクキューの管理（Workflow Task + Activity Taskの2種類）
4. タイマーの管理（発火時刻到達の検知）
5. 外部APIの提供（StartWorkflow, DescribeWorkflow等）
6. Activityタイムアウトの監視（StartToCloseTimeout超過の検知）

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
    |  (Phase 3)       |   |      +----------------------+   |
    +------------------+   |                |                |     +------------------------+
                           |                v                |     |  adapter/telemetry/    |
    +------------------+   |           domain/               |     |  (Phase 2-3)           |
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
│       └── types.pb.go                       # generated
├── cmd/
│   └── dandori/                              # サーバーバイナリ（DI、起動、shutdown）
│       └── main.go
├── internal/
│   ├── adapter/
│   │   ├── grpc/                             # Inbound Adapter: gRPC
│   │   │   └── handler.go
│   │   └── postgres/                         # Outbound Adapter: PostgreSQL
│   │       ├── migration/                    # SQLマイグレーション
│   │       │   ├── 000001_initial.up.sql
│   │       │   └── 000001_initial.down.sql
│   │       ├── migrate.go                    # embed.FSマイグレーションランナー
│   │       ├── store.go                      # コネクションプール、TxManager
│   │       ├── event.go                      # EventRepository実装
│   │       ├── workflow_task.go               # WorkflowTaskRepository実装
│   │       ├── activity_task.go              # ActivityTaskRepository実装
│   │       ├── timer.go                      # TimerRepository実装
│   │       └── workflow.go                   # WorkflowRepository実装
│   ├── domain/                               # 純粋な型定義 + エラー定義（依存なし）
│   │   ├── command.go
│   │   ├── errors.go                         # ドメインエラー定義
│   │   ├── event.go
│   │   ├── retry.go
│   │   ├── task.go
│   │   ├── timer.go
│   │   └── workflow.go
│   ├── engine/                               # アプリケーションコア（ビジネスロジック）
│   │   ├── engine.go                         # リクエスト駆動の操作（ClientService, WorkflowTaskService, ActivityTaskService を実装）
│   │   ├── command_processor.go              # コマンド→イベント変換パイプライン
│   │   ├── background.go                     # バックグラウンドプロセス（タイマー、タイムアウト監視）
│   │   └── retry.go                          # リトライポリシー
│   └── port/                                 # ポート定義（インターフェース）
│       ├── service.go                        # Inbound Port: 役割別インターフェース
│       └── repository.go                     # Outbound Port: 各Repository, TxManager
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
- `adapter/postgres/`: Outbound Adapter。port/ と domain/ に依存。engine/ には依存しない
- `cmd/dandori/`: 全パッケージをimportし、依存を組み立てるエントリーポイント

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
)
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

    // Phase 2 で追加:
    // EventWorkflowExecutionCancelRequested
    // EventWorkflowExecutionCanceled
    // EventTimerStarted
    // EventTimerFired
    // EventTimerCanceled
    // EventWorkflowSignaled
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

    // Phase 2 で追加:
    // CommandStartTimer
    // CommandCancelTimer
)

type Command struct {
    Type       CommandType
    Attributes json.RawMessage
}

type ScheduleActivityTaskAttributes struct {
    SeqID               int64           `json:"seq_id"`
    ActivityType        string          `json:"activity_type"`
    Input               json.RawMessage `json:"input"`
    TaskQueue           string          `json:"task_queue,omitempty"`
    StartToCloseTimeout time.Duration   `json:"start_to_close_timeout"`
    RetryPolicy         *RetryPolicy    `json:"retry_policy,omitempty"`
}

type CompleteWorkflowAttributes struct {
    Result json.RawMessage `json:"result"`
}

type FailWorkflowAttributes struct {
    ErrorMessage string `json:"error_message"`
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
    WorkflowStatusRunning    WorkflowStatus = "RUNNING"
    WorkflowStatusCompleted  WorkflowStatus = "COMPLETED"
    WorkflowStatusFailed     WorkflowStatus = "FAILED"
    WorkflowStatusTerminated WorkflowStatus = "TERMINATED"
)

// IsTerminal はワークフローが終了状態かどうかを返す
func (s WorkflowStatus) IsTerminal() bool {
    return s == WorkflowStatusCompleted || s == WorkflowStatusFailed || s == WorkflowStatusTerminated
}

type WorkflowExecution struct {
    ID           uuid.UUID
    WorkflowType string
    TaskQueue    string
    Status       WorkflowStatus
    Input        json.RawMessage
    Result       json.RawMessage
    Error        string
    CreatedAt    time.Time
    ClosedAt     *time.Time
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
    QueueName  string
    WorkflowID uuid.UUID
    Status     TaskStatus
    ScheduledAt time.Time
}

// ActivityTask はワーカーがポーリングで取得するActivity Task
type ActivityTask struct {
    ID                  int64
    QueueName           string
    WorkflowID          uuid.UUID
    ActivityType        string
    ActivityInput       json.RawMessage
    ActivitySeqID       int64
    StartToCloseTimeout time.Duration
    Attempt             int
    MaxAttempts         int
    RetryPolicy         *RetryPolicy
    Status              TaskStatus
    ScheduledAt         time.Time
    TimeoutAt           *time.Time
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
    DescribeWorkflow(ctx context.Context, id uuid.UUID) (*domain.WorkflowExecution, error)
    GetWorkflowHistory(ctx context.Context, workflowID uuid.UUID) ([]domain.HistoryEvent, error)
    TerminateWorkflow(ctx context.Context, id uuid.UUID, reason string) error
}

// WorkflowTaskService はワーカーのWorkflow Task操作を定義する。
type WorkflowTaskService interface {
    PollWorkflowTask(ctx context.Context, queueName string, workerID string) (*WorkflowTaskResult, error)
    CompleteWorkflowTask(ctx context.Context, taskID int64, commands []domain.Command) error
    FailWorkflowTask(ctx context.Context, taskID int64, cause string, message string) error
}

// ActivityTaskService はワーカーのActivity Task操作を定義する。
type ActivityTaskService interface {
    PollActivityTask(ctx context.Context, queueName string, workerID string) (*domain.ActivityTask, error)
    CompleteActivityTask(ctx context.Context, taskID int64, result json.RawMessage) error
    FailActivityTask(ctx context.Context, taskID int64, failure domain.ActivityFailure) error
}

// StartWorkflowParams はワークフロー開始のパラメータ
type StartWorkflowParams struct {
    ID           uuid.UUID       // ゼロ値の場合はUUID自動生成
    WorkflowType string
    TaskQueue    string
    Input        json.RawMessage
}

// WorkflowTaskResult はPollWorkflowTaskの結果
type WorkflowTaskResult struct {
    Task         domain.WorkflowTask
    Events       []domain.HistoryEvent
    WorkflowType string
}
```

engine.Engineはこの3つのインターフェースを全て実装する。gRPCハンドラは必要なインターフェースのみを受け取る。

telemetryデコレータは関心のあるインターフェースだけを装飾できる:

```go
// 例: ClientServiceだけにメトリクスを付ける
type metricsClientService struct {
    next    port.ClientService
    metrics MetricsHandler
}
```

```go
// port/repository.go - Outbound Port
type WorkflowRepository interface {
    Create(ctx context.Context, wf domain.WorkflowExecution) error
    Get(ctx context.Context, id uuid.UUID) (*domain.WorkflowExecution, error)
    UpdateStatus(ctx context.Context, id uuid.UUID, status domain.WorkflowStatus, result json.RawMessage, errMsg string) error
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
    Poll(ctx context.Context, queueName string, workerID string) (*domain.WorkflowTask, error)
    Complete(ctx context.Context, taskID int64) error
    GetByID(ctx context.Context, taskID int64) (*domain.WorkflowTask, error)
    RecoverStaleTasks(ctx context.Context) (int, error)
    DeleteByWorkflowID(ctx context.Context, workflowID uuid.UUID) error
}

type ActivityTaskRepository interface {
    Enqueue(ctx context.Context, task domain.ActivityTask) error
    // Poll はキューからタスクを1件取得する。タスクがない場合はdomain.ErrNoTaskAvailableを返す。
    Poll(ctx context.Context, queueName string, workerID string) (*domain.ActivityTask, error)
    Complete(ctx context.Context, taskID int64) error
    GetByID(ctx context.Context, taskID int64) (*domain.ActivityTask, error)
    GetTimedOut(ctx context.Context) ([]domain.ActivityTask, error)
    Requeue(ctx context.Context, taskID int64, scheduledAt time.Time) error
    RecoverStaleTasks(ctx context.Context) (int, error)
    DeleteByWorkflowID(ctx context.Context, workflowID uuid.UUID) error
}

type TimerRepository interface {
    Create(ctx context.Context, timer domain.Timer) error
    GetFired(ctx context.Context) ([]domain.Timer, error)
    MarkFired(ctx context.Context, timerID int64) error
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
    workflows      port.WorkflowRepository
    events         port.EventRepository
    workflowTasks  port.WorkflowTaskRepository
    activityTasks  port.ActivityTaskRepository
    timers         port.TimerRepository
    tx             port.TxManager
}

// コンパイル時にインターフェース実装を保証
var _ port.ClientService       = (*Engine)(nil)
var _ port.WorkflowTaskService = (*Engine)(nil)
var _ port.ActivityTaskService = (*Engine)(nil)

func New(
    workflows port.WorkflowRepository,
    events port.EventRepository,
    workflowTasks port.WorkflowTaskRepository,
    activityTasks port.ActivityTaskRepository,
    timers port.TimerRepository,
    tx port.TxManager,
) *Engine {
    return &Engine{
        workflows:     workflows,
        events:        events,
        workflowTasks: workflowTasks,
        activityTasks: activityTasks,
        timers:        timers,
        tx:            tx,
    }
}

// --- ClientService の実装 ---

func (e *Engine) StartWorkflow(ctx context.Context, params port.StartWorkflowParams) (*domain.WorkflowExecution, error) {
    if params.ID == uuid.Nil {
        params.ID = uuid.New()
    }

    var wf *domain.WorkflowExecution
    err := e.tx.RunInTx(ctx, func(ctx context.Context) error {
        // 冪等性チェック: 同一IDのワークフローが存在するか
        existing, err := e.workflows.Get(ctx, params.ID)
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

func (e *Engine) TerminateWorkflow(ctx context.Context, id uuid.UUID, reason string) error {
    return e.tx.RunInTx(ctx, func(ctx context.Context) error {
        wf, err := e.workflows.Get(ctx, id)
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

// --- WorkflowTaskService の実装 ---

func (e *Engine) PollWorkflowTask(ctx context.Context, queueName string, workerID string) (*port.WorkflowTaskResult, error) {
    // タスクがない場合は(nil, nil)を返す。gRPCハンドラ側で空レスポンスに変換する。
    task, err := e.workflowTasks.Poll(ctx, queueName, workerID)
    if errors.Is(err, domain.ErrNoTaskAvailable) {
        return nil, nil
    }
    if err != nil {
        return nil, err
    }
    wf, err := e.workflows.Get(ctx, task.WorkflowID)
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

        wf, err := e.workflows.Get(ctx, task.WorkflowID)
        if err != nil {
            return err
        }
        return e.processCommands(ctx, task.WorkflowID, wf.TaskQueue, commands)
    })
}

// --- ActivityTaskService の実装 ---

func (e *Engine) CompleteActivityTask(ctx context.Context, taskID int64, result json.RawMessage) error {
    return e.tx.RunInTx(ctx, func(ctx context.Context) error {
        task, err := e.activityTasks.GetByID(ctx, taskID)
        if err != nil {
            return err
        }

        wf, err := e.workflows.Get(ctx, task.WorkflowID)
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

        wf, err := e.workflows.Get(ctx, task.WorkflowID)
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
    tx            port.TxManager
}

func NewBackgroundWorker(
    workflows port.WorkflowRepository,
    events port.EventRepository,
    workflowTasks port.WorkflowTaskRepository,
    activityTasks port.ActivityTaskRepository,
    tx port.TxManager,
) *BackgroundWorker {
    return &BackgroundWorker{
        workflows: workflows, events: events,
        workflowTasks: workflowTasks,
        activityTasks: activityTasks, tx: tx,
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
func (e *Engine) processCommands(ctx context.Context, workflowID uuid.UUID, taskQueue string, commands []domain.Command) error {
    for _, cmd := range commands {
        switch cmd.Type {
        case domain.CommandScheduleActivityTask:
            if err := e.processScheduleActivity(ctx, workflowID, taskQueue, cmd.Attributes); err != nil {
                return err
            }
        case domain.CommandCompleteWorkflow:
            if err := e.processCompleteWorkflow(ctx, workflowID, cmd.Attributes); err != nil {
                return err
            }
        case domain.CommandFailWorkflow:
            if err := e.processFailWorkflow(ctx, workflowID, cmd.Attributes); err != nil {
                return err
            }
        default:
            return fmt.Errorf("unknown command type: %s", cmd.Type)
        }
    }
    return nil
}

func (e *Engine) processScheduleActivity(ctx context.Context, workflowID uuid.UUID, taskQueue string, attrs json.RawMessage) error {
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
        Attempt:             1,
        MaxAttempts:         maxAttempts,
        RetryPolicy:         a.RetryPolicy,
    }); err != nil {
        return err
    }

    eventData, _ := json.Marshal(a)
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
    result, err := h.wfTask.PollWorkflowTask(ctx, req.QueueName, req.WorkerId)
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

func (s *Store) Workflows() port.WorkflowRepository          { return &WorkflowStore{store: s} }
func (s *Store) Events() port.EventRepository                { return &EventStore{store: s} }
func (s *Store) WorkflowTasks() port.WorkflowTaskRepository  { return &WorkflowTaskStore{store: s} }
func (s *Store) ActivityTasks() port.ActivityTaskRepository   { return &ActivityTaskStore{store: s} }
func (s *Store) Timers() port.TimerRepository                { return &TimerStore{store: s} }
```

### 依存の組み立て（cmd/dandori/main.go）

```go
func main() {
    // 環境変数（DATABASE_URL, GRPC_PORT）
    databaseURL := envOrDefault("DATABASE_URL", "postgres://dandori:dandori@localhost:5432/dandori?sslmode=disable")
    grpcPort := envOrDefault("GRPC_PORT", "7233")

    // DB接続 + ping + プール設定
    db, _ := sql.Open("postgres", databaseURL)
    db.SetMaxOpenConns(25)
    db.SetMaxIdleConns(5)
    db.SetConnMaxLifetime(5 * time.Minute)

    // embed.FSマイグレーション（冪等: テーブル存在チェック）
    postgres.RunMigrations(context.Background(), db)

    // Outbound Adapter
    store := postgres.New(db)

    // Application Core
    eng := engine.New(
        store.Workflows(),
        store.Events(),
        store.WorkflowTasks(),
        store.ActivityTasks(),
        store.Timers(),
        store, // TxManager
    )

    // Background Worker（Engineとは別の構造体）
    bgWorker := engine.NewBackgroundWorker(
        store.Workflows(),
        store.Events(),
        store.WorkflowTasks(),
        store.ActivityTasks(),
        store,
    )

    // Inbound Adapter（役割別にインターフェースを渡す）
    handler := grpcadapter.NewHandler(eng, eng, eng)

    ctx, cancel := context.WithCancel(context.Background())

    // バックグラウンドプロセス起動（Engineとは独立したライフサイクル）
    go bgWorker.RunActivityTimeoutChecker(ctx, 5*time.Second)
    go bgWorker.RunTaskRecovery(ctx, 10*time.Second)

    // gRPCサーバー起動（reflection有効）
    srv := grpc.NewServer()
    apiv1.RegisterDandoriServiceServer(srv, handler)
    reflection.Register(srv)
    go srv.Serve(lis)

    // graceful shutdown（SIGINT/SIGTERM → cancel → GracefulStop → db.Close）
    sig := <-sigCh
    cancel()
    srv.GracefulStop()
    db.Close()
}
```

### トランザクション境界

以下の操作は1トランザクション内で実行し、原子性を保証する:

| 操作 | トランザクション内で行うこと |
|------|---------------------------|
| StartWorkflow | 冪等性チェック + (終了済み再作成時は旧関連データ削除) + WorkflowExecution作成(upsert) + イベント記録 + Workflow Task投入 |
| CompleteWorkflowTask | Advisory Lock取得 + タスク解決 + コマンド→イベント変換 + タスク生成 + ステータス更新 |
| FailWorkflowTask | タスク完了 + ワークフローFAILED更新 + WorkflowExecutionFailedイベント記録 |
| CompleteActivityTask | ワークフロー状態チェック + イベント記録 + Workflow Task投入 |
| FailActivityTask | ワークフロー状態チェック + リトライ判定 + イベント記録 + タスク再投入 or Workflow Task投入 |
| TerminateWorkflow | ワークフロー状態チェック + イベント記録 + ステータスをTERMINATEDに更新 |

### バックグラウンドプロセス

BackgroundWorkerが管理する:

- タスクタイムアウト回収（`RunTaskRecovery`）: locked_untilを超えたタスクをPENDINGに戻す（visibility timeout）
- Activityタイムアウト監視（`RunActivityTimeoutChecker`）: timeout_atを超えたActivity Taskを検知し、ActivityTaskTimedOutイベントを記録してWorkflow Taskを生成する

Phase 2で追加:

- タイマーポーラー: 発火時刻到達のtimerを検知し、TimerFiredイベントを記録してWorkflow Taskを生成する

各プロセスはcontext.Done()でグレースフルに停止する。

### エラーモデル

各操作で返しうるドメインエラーを定義する:

| 操作 | 正常 | エラー |
|------|------|--------|
| StartWorkflow | *WorkflowExecution | ErrWorkflowAlreadyExists（同一ID+RUNNING） |
| DescribeWorkflow | *WorkflowExecution | ErrWorkflowNotFound |
| GetWorkflowHistory | []HistoryEvent | ErrWorkflowNotFound |
| TerminateWorkflow | nil | ErrWorkflowNotFound, ErrWorkflowNotRunning |
| PollWorkflowTask | *WorkflowTaskResult (nil=タスクなし) | — |
| CompleteWorkflowTask | nil | ErrTaskNotFound, ErrTaskAlreadyCompleted |
| FailWorkflowTask | nil | ErrTaskNotFound |
| PollActivityTask | *ActivityTask (nil=タスクなし) | — |
| CompleteActivityTask | nil | ErrTaskNotFound, ErrTaskAlreadyCompleted |
| FailActivityTask | nil | ErrTaskNotFound, ErrTaskAlreadyCompleted |

gRPCハンドラはdomainErrorToGRPC()で一元的にgRPCステータスコードに変換する:

| ドメインエラー | gRPC ステータス |
|---|---|
| ErrWorkflowNotFound | NOT_FOUND |
| ErrWorkflowAlreadyExists | ALREADY_EXISTS |
| ErrWorkflowNotRunning | FAILED_PRECONDITION |
| ErrTaskNotFound | NOT_FOUND |
| ErrTaskAlreadyCompleted | FAILED_PRECONDITION |
| ErrNoTaskAvailable | （エラーではなく空レスポンス） |
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

Phase 2 で追加: StartTimer, CancelTimer

### コマンド → イベント変換

| コマンド | 生成されるイベント | 副作用 |
|---------|-------------------|--------|
| ScheduleActivityTask | ActivityTaskScheduled | Activity Taskをキューに投入 |
| CompleteWorkflow | WorkflowExecutionCompleted | ステータスをCOMPLETEDに更新 |
| FailWorkflow | WorkflowExecutionFailed | ステータスをFAILEDに更新 |

### 外部トリガー → イベント

| トリガー | 生成されるイベント | 副作用 |
|---------|-------------------|--------|
| StartWorkflow API | WorkflowExecutionStarted | Workflow Task生成 |
| Activity完了報告 | ActivityTaskCompleted | Workflow Task生成 |
| Activity失敗報告（リトライ不可） | ActivityTaskFailed | Workflow Task生成 |
| Activity失敗報告（リトライ可） | （イベントなし） | 同一Activity Taskを再キューイング |
| Activityタイムアウト検知 | ActivityTaskTimedOut | Workflow Task生成 |
| TerminateWorkflow API | WorkflowExecutionTerminated | ステータスをTERMINATEDに更新 |

### イベントタイプ一覧

MVP: WorkflowExecutionStarted, WorkflowExecutionCompleted, WorkflowExecutionFailed, WorkflowExecutionTerminated, ActivityTaskScheduled, ActivityTaskCompleted, ActivityTaskFailed, ActivityTaskTimedOut

Phase 2: WorkflowExecutionCancelRequested, WorkflowExecutionCanceled, TimerStarted, TimerFired, TimerCanceled, WorkflowSignaled

## 5. workflow.Contextの設計（Go SDKリポジトリで実装）

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

## 6. PostgreSQLスキーマ

### workflow_executions テーブル

```sql
CREATE TABLE workflow_executions (
    id              UUID PRIMARY KEY,
    workflow_type   TEXT NOT NULL,
    task_queue      TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'RUNNING',
    input           JSONB,
    result          JSONB,
    error_message   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at       TIMESTAMPTZ
);

CREATE INDEX idx_workflow_executions_status ON workflow_executions(status);
```

### workflow_events テーブル

```sql
CREATE TABLE workflow_events (
    id              BIGSERIAL PRIMARY KEY,
    workflow_id     UUID NOT NULL REFERENCES workflow_executions(id),
    sequence_num    INT NOT NULL,
    event_type      TEXT NOT NULL,
    event_data      JSONB NOT NULL,
    timestamp       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(workflow_id, sequence_num)
);

CREATE INDEX idx_workflow_events_workflow_id ON workflow_events(workflow_id);
```

### workflow_tasks テーブル

WorkflowTaskとActivityTaskはスキーマが異なるため、テーブルを分離する。

```sql
CREATE TABLE workflow_tasks (
    id              BIGSERIAL PRIMARY KEY,
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
    queue_name             TEXT NOT NULL,
    workflow_id            UUID NOT NULL REFERENCES workflow_executions(id),
    activity_type          TEXT NOT NULL,
    activity_input         JSONB,
    activity_seq_id        BIGINT NOT NULL,
    start_to_close_timeout INTERVAL,
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
    END
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
    workflow_id     UUID NOT NULL REFERENCES workflow_executions(id),
    seq_id          BIGINT NOT NULL,
    fire_at         TIMESTAMPTZ NOT NULL,
    status          TEXT NOT NULL DEFAULT 'PENDING',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_timers_pending ON timers(fire_at) WHERE status = 'PENDING';
```

### PostgreSQL活用ポイント

- SKIP LOCKED: タスクキューの排他制御
- JSONB: ワークフロー入出力、イベントデータ、RetryPolicyの柔軟な格納
- Advisory Lock: CompleteWorkflowTask時にworkflow_idハッシュでpg_advisory_xact_lockを取得し、同一ワークフローのWorkflow Task処理を直列化
- UNIQUE(workflow_id, sequence_num): イベントの重複挿入を防止
- タイムアウト検知: timeout_atカラムによるActivity StartToCloseTimeout監視
- LISTEN/NOTIFY: タスク投入の即時通知（Phase 2）

## 7. gRPCサービス定義

```protobuf
service DandoriService {
  // === クライアント向け API ===
  rpc StartWorkflow(StartWorkflowRequest) returns (StartWorkflowResponse);
  rpc DescribeWorkflow(DescribeWorkflowRequest) returns (DescribeWorkflowResponse);
  rpc GetWorkflowHistory(GetWorkflowHistoryRequest) returns (GetWorkflowHistoryResponse);
  rpc TerminateWorkflow(TerminateWorkflowRequest) returns (TerminateWorkflowResponse);

  // Phase 2:
  // rpc SignalWorkflow(...);
  // rpc CancelWorkflow(...);
  // rpc ListWorkflows(...);

  // === ワーカー向け API: Workflow Task ===
  rpc PollWorkflowTask(PollWorkflowTaskRequest) returns (PollWorkflowTaskResponse);
  rpc CompleteWorkflowTask(CompleteWorkflowTaskRequest) returns (CompleteWorkflowTaskResponse);
  rpc FailWorkflowTask(FailWorkflowTaskRequest) returns (FailWorkflowTaskResponse);

  // === ワーカー向け API: Activity Task ===
  rpc PollActivityTask(PollActivityTaskRequest) returns (PollActivityTaskResponse);
  rpc CompleteActivityTask(CompleteActivityTaskRequest) returns (CompleteActivityTaskResponse);
  rpc FailActivityTask(FailActivityTaskRequest) returns (FailActivityTaskResponse);

  // Phase 2:
  // rpc RecordActivityHeartbeat(...);
}
```

メッセージ定義は前回のものと同一（§7の主要メッセージ定義を参照）。省略。

## 8. Go SDKリポジトリ構成（dandori-sdk-go）

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
├── examples/
│   └── order/
└── go.mod
```

## 9. 実装の難所と対処方針

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

### StartWorkflowの冪等性

同一IDで非終了状態のワークフローが存在する場合はErrWorkflowAlreadyExistsを返す。終了済みの場合は新規作成を許可する。チェックと作成は同一トランザクション内で実行する。

### エラー定義の配置

ドメインエラーはdomain/に定義する。これによりadapter/grpc/がengine/に依存することを防ぎ、Hexagonal Architectureの依存方向を維持する。

### Poll操作の振る舞い

タスクがない場合、リポジトリはdomain.ErrNoTaskAvailableを返す。gRPCハンドラはこれを空レスポンスに変換する（エラーではない）。SDKのワーカーは空レスポンスを受け取ると一定間隔後に再度Pollする。

### goroutineの管理とリーク防止（Go SDK側）

Workflow Task処理完了時にcontextをcancelし、goroutineを適切に終了させる。

### WorkflowRun.Get()の実装（Go SDK側）

MVPではDescribeWorkflowのポーリングで実現する。

## 10. Phase 1 MVPの実装ステップ

### サーバー

1. プロジェクト骨格: go mod, docker-compose.yml, proto定義
2. domain/: 型定義 + エラー定義（ErrWorkflowNotFound等）
3. port/: 役割別Inbound Port（ClientService, WorkflowTaskService, ActivityTaskService）+ Outbound Port（WorkflowTaskRepository, ActivityTaskRepository を分離）
4. adapter/postgres/: PostgreSQL実装、テーブル分離（workflow_tasks, activity_tasks）
5. engine/: Engine（3つのInbound Port実装）、CommandProcessor、BackgroundWorker（別構造体）
6. adapter/grpc/: gRPCハンドラ（役割別インターフェース受取、domainErrorToGRPC）
7. cmd/dandori/: DI、サーバー起動、BackgroundWorker起動、graceful shutdown
8. テスト

### Go SDK

1. クライアントSDK: Dial, ExecuteWorkflow, GetWorkflow, WorkflowRun
2. ワーカーSDK - Activityサイド
3. ワーカーSDK - Workflowサイド（核心）
4. 非決定性検出 + FailWorkflowTask報告
5. サンプルワークフロー
6. テスト
