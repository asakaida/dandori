# dandori - 設計書

## 1. アーキテクチャ概要

### 全体構成

```text
Client (Go SDK / CLI)
    │
    │ gRPC
    ▼
dandori-server (本リポジトリ)
    │
    │ PostgreSQL
    ▼
PostgreSQL
    │
    ▼
dandori-worker (Go SDK側で実装, 1..N instances)
```

### サーバーの責務

サーバーはワークフローのステップ定義を理解しない。以下に徹する:

1. イベントの永続化（append-only）
2. ワーカーから返されたコマンドの処理（コマンド→イベント変換）
3. タスクキューの管理（Workflow Task + Activity Taskの2種類）
4. タイマーの管理（発火時刻到達の検知）
5. 外部APIの提供（StartWorkflow, GetWorkflow等）

サーバーは「次に何をすべきか」を知らない。イベントが発生するたびにWorkflow Taskを生成し、ワーカーに判断を委ねる。

### ワーカーの責務（Go SDKリポジトリで実装）

2種類のタスクを処理する:

- Workflow Task: ワークフロー関数を最初からreplayし、イベント履歴と照合しながら実行。新しい副作用呼び出しに到達したらコマンドを生成してサーバーに返す
- Activity Task: Activity関数を実際に実行し、結果をサーバーに報告

gRPC経由でサーバーに結果を返す。理由:

- ワーカーとDBが疎結合
- サーバー側でイベント記録とタスクスケジューリングをアトミックに実行可能
- proto定義をAPI契約とすることで、将来的に他言語SDKも作成可能

## 2. サーバー内部アーキテクチャ

Hexagonal Architecture（Ports and Adapters）を採用する。

```text
       Inbound Adapter                     Core                        Outbound Adapter
    ┌──────────────────┐                                           ┌────────────────────────┐
    │  adapter/grpc/   │          ┌──────────────────────┐         │  adapter/postgres/     │
    │                  │──►       │                      │         │                        │──► PostgreSQL
    │  proto型 →       │   port/  │  engine/             │  port/  │  Outbound Port の      │
    │  domain型変換    │──►Inbound│                      │  Out-   │  インターフェースを    │
    └──────────────────┘   Port   │  ビジネスロジック    │  bound ◄│  暗黙的に実装         │
                           │      │  コマンド処理        │  Port   │                        │
    ┌──────────────────┐   │      │  トランザクション制御│   │     │  migration/ も含む    │
    │  adapter/http/   │──►│      │                      │   │     └────────────────────────┘
    │  (Phase 3)       │   │      └──────────────────────┘   │
    └──────────────────┘   │                │                │     ┌────────────────────────┐
                           │                ▼                │     │  adapter/telemetry/    │
    ┌──────────────────┐   │           domain/               │     │  (Phase 2-3)           │
    │adapter/telemetry/│──►│           純粋な型定義           │     │                        │
    │  Inbound Port の │   │                                 │     │  Outbound Port の      │
    │  デコレータ      │                                     └────►│  デコレータ            │
    └──────────────────┘                                           └────────────────────────┘
```

adapter/telemetry/はInbound PortとOutbound Portの両方のデコレータとして機能する。
Phase 1では不要だが、Phase 2-3でメトリクス・トレーシングを追加する際にこの位置に配置する。

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
│   │       ├── store.go                      # コネクションプール、TxManager
│   │       ├── event.go                      # EventRepository実装
│   │       ├── task.go                       # TaskRepository実装
│   │       ├── timer.go                      # TimerRepository実装
│   │       └── workflow.go                   # WorkflowRepository実装
│   ├── domain/                               # 純粋な型定義（依存なし）
│   │   ├── command.go
│   │   ├── event.go
│   │   ├── task.go
│   │   ├── timer.go
│   │   └── workflow.go
│   ├── engine/                               # アプリケーションコア（ビジネスロジック）
│   │   ├── engine.go                         # 全操作のエントリーポイント（port.WorkflowServiceを実装）
│   │   ├── command_processor.go              # コマンド→イベント変換パイプライン
│   │   └── retry.go                          # リトライポリシー
│   └── port/                                 # ポート定義（インターフェース）
│       ├── service.go                        # Inbound Port: WorkflowService
│       └── repository.go                     # Outbound Port: 各Repository, TxManager
├── docker-compose.yml
└── go.mod
```

### 依存関係の方向

```text
cmd/dandori/main.go（依存の組み立て）
    │
    ├── adapter/grpc/ ──► port/ (Inbound) ◄── engine/ ──► port/ (Outbound) ◄── adapter/postgres/
    │                       │                    │              │                      │
    │                       ▼                    ▼              ▼                      ▼
    └──────────────────► domain/ ◄───────────────┴──────────────┴──────────────────────┘
```

- `domain/`: 純粋な型定義のみ。他のパッケージに依存しない
- `port/`: Inbound Port（WorkflowService）とOutbound Port（各Repository, TxManager）の定義。domain/のみに依存
- `engine/`: ビジネスロジック。port.WorkflowServiceを実装し、port/のOutbound Portを通じてのみ外部と通信。port/とdomain/に依存
- `adapter/grpc/`: Inbound Adapter。port.WorkflowServiceインターフェースに依存。proto型とdomain型の変換を行い、インターフェース経由でengine/に委譲。port/とdomain/に依存（engine/には依存しない）
- `adapter/postgres/`: Outbound Adapter。port/のOutbound Portインターフェースを暗黙的に満たす。port/とdomain/に依存（engine/には依存しない）
- `cmd/dandori/`: 全パッケージをimportし、依存を組み立てるエントリーポイント。engine.EngineをWorkflowServiceとしてadapter/grpc/に渡す

### 各層の責務

#### domain/ - ドメインモデル

ビジネスロジックを持たない純粋な型定義。全パッケージから参照される共有の語彙。

```go
// domain/event.go
type EventType string

const (
    EventWorkflowExecutionStarted EventType = "WorkflowExecutionStarted"
    EventActivityTaskScheduled    EventType = "ActivityTaskScheduled"
    EventActivityTaskCompleted    EventType = "ActivityTaskCompleted"
    // ...
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
    // ...
)

type Command struct {
    Type       CommandType
    Attributes json.RawMessage
}
```

```go
// domain/workflow.go
type WorkflowStatus string

const (
    WorkflowStatusRunning   WorkflowStatus = "RUNNING"
    WorkflowStatusCompleted WorkflowStatus = "COMPLETED"
    WorkflowStatusFailed    WorkflowStatus = "FAILED"
)

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
type TaskType string

const (
    TaskTypeWorkflow TaskType = "WORKFLOW"
    TaskTypeActivity TaskType = "ACTIVITY"
)

type Task struct {
    ID            int64
    Type          TaskType
    QueueName     string
    WorkflowID    uuid.UUID
    ActivityType  string          // Activity Taskのみ
    ActivityInput json.RawMessage // Activity Taskのみ
    ActivitySeqID int64           // Activity Taskのみ
    Attempt       int
    MaxAttempts   int
    ScheduledAt   time.Time
}
```

#### port/ - ポート定義

Inbound PortとOutbound Portの両方のインターフェースを定義する。domain/のみに依存する独立したパッケージ。

```go
// port/service.go - Inbound Port
// Inbound Adapter（grpc/, http/等）はこのインターフェースに依存する。
// engine.Engine がこれを実装する。
// adapter/telemetry/ がデコレータとしてこれを実装する。
type WorkflowService interface {
    StartWorkflow(ctx context.Context, params StartWorkflowParams) (*domain.WorkflowExecution, error)
    GetWorkflow(ctx context.Context, id uuid.UUID) (*domain.WorkflowExecution, error)
    TerminateWorkflow(ctx context.Context, id uuid.UUID, reason string) error
    CompleteWorkflowTask(ctx context.Context, taskID int64, commands []domain.Command) error
    PollWorkflowTask(ctx context.Context, queueName string, workerID string) (*WorkflowTask, error)
    PollActivityTask(ctx context.Context, queueName string, workerID string) (*domain.Task, error)
    CompleteActivityTask(ctx context.Context, taskID int64, result json.RawMessage) error
    FailActivityTask(ctx context.Context, taskID int64, errMsg string) error
    GetWorkflowHistory(ctx context.Context, workflowID uuid.UUID) ([]domain.HistoryEvent, error)
}

// StartWorkflowParams, WorkflowTask 等のパラメータ型もport/に定義する
type StartWorkflowParams struct {
    ID           uuid.UUID
    WorkflowType string
    TaskQueue    string
    Input        json.RawMessage
}

type WorkflowTask struct {
    Task    domain.Task
    Events  []domain.HistoryEvent
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
    Append(ctx context.Context, events []domain.HistoryEvent) error
    GetByWorkflowID(ctx context.Context, workflowID uuid.UUID) ([]domain.HistoryEvent, error)
    GetNextSequenceNum(ctx context.Context, workflowID uuid.UUID) (int, error)
}

type TaskRepository interface {
    Enqueue(ctx context.Context, task domain.Task) error
    Poll(ctx context.Context, queueName string, taskType domain.TaskType, workerID string) (*domain.Task, error)
    Complete(ctx context.Context, taskID int64) error
}

type TimerRepository interface {
    Create(ctx context.Context, timer domain.Timer) error
    GetFired(ctx context.Context) ([]domain.Timer, error)
    MarkFired(ctx context.Context, timerID int64) error
}

type TxManager interface {
    RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}
```

#### engine/ - アプリケーションコア

ビジネスロジックとトランザクション制御を担う。port.WorkflowService（Inbound Port）を実装し、port/のOutbound Portを通じてのみ外部と通信する。

```go
// engine/engine.go
// Engine は port.WorkflowService を実装する
type Engine struct {
    workflows port.WorkflowRepository
    events    port.EventRepository
    tasks     port.TaskRepository
    timers    port.TimerRepository
    tx        port.TxManager
    cmdProc   *CommandProcessor
}

func New(
    workflows port.WorkflowRepository,
    events port.EventRepository,
    tasks port.TaskRepository,
    timers port.TimerRepository,
    tx port.TxManager,
) *Engine {
    e := &Engine{
        workflows: workflows,
        events:    events,
        tasks:     tasks,
        timers:    timers,
        tx:        tx,
    }
    e.cmdProc = &CommandProcessor{
        events:    events,
        tasks:     tasks,
        workflows: workflows,
        timers:    timers,
    }
    return e
}

// StartWorkflow: ワークフロー開始
// 1トランザクションで実行作成 + イベント記録 + タスク投入を行う
func (e *Engine) StartWorkflow(ctx context.Context, params port.StartWorkflowParams) (*domain.WorkflowExecution, error) {
    var wf domain.WorkflowExecution
    err := e.tx.RunInTx(ctx, func(ctx context.Context) error {
        wf = domain.WorkflowExecution{
            ID:           params.ID,
            WorkflowType: params.WorkflowType,
            TaskQueue:    params.TaskQueue,
            Status:       domain.WorkflowStatusRunning,
            Input:        params.Input,
        }
        if err := e.workflows.Create(ctx, wf); err != nil {
            return err
        }
        if err := e.events.Append(ctx, []domain.HistoryEvent{{
            WorkflowID: wf.ID,
            Type:       domain.EventWorkflowExecutionStarted,
            Data:       params.Input,
        }}); err != nil {
            return err
        }
        return e.tasks.Enqueue(ctx, domain.Task{
            Type:       domain.TaskTypeWorkflow,
            QueueName:  params.TaskQueue,
            WorkflowID: wf.ID,
        })
    })
    return &wf, err
}

// CompleteWorkflowTask: ワーカーからのコマンドリストを処理
func (e *Engine) CompleteWorkflowTask(ctx context.Context, taskID int64, commands []domain.Command) error {
    return e.tx.RunInTx(ctx, func(ctx context.Context) error {
        if err := e.tasks.Complete(ctx, taskID); err != nil {
            return err
        }
        return e.cmdProc.Process(ctx, commands)
    })
}
```

```go
// engine/command_processor.go
type CommandProcessor struct {
    events    port.EventRepository
    tasks     port.TaskRepository
    workflows port.WorkflowRepository
    timers    port.TimerRepository
}

// Process: コマンドリストを順次処理し、イベント記録とタスク生成を行う
// 呼び出し元のトランザクション内で実行される
func (p *CommandProcessor) Process(ctx context.Context, workflowID uuid.UUID, commands []domain.Command) error {
    for _, cmd := range commands {
        switch cmd.Type {
        case domain.CommandScheduleActivityTask:
            // → ActivityTaskScheduledイベント記録
            // → Activity Taskをキューに投入
        case domain.CommandCompleteWorkflow:
            // → WorkflowExecutionCompletedイベント記録
            // → ワークフローステータスをCOMPLETEDに更新
        case domain.CommandFailWorkflow:
            // → WorkflowExecutionFailedイベント記録
            // → ワークフローステータスをFAILEDに更新
        }
    }
    return nil
}
```

#### adapter/grpc/ - Inbound Adapter

proto型とdomain型の変換を行う薄い層。ビジネスロジックは一切持たない。port.WorkflowService（Inbound Port）のインターフェースに依存し、engine/には直接依存しない。

```go
// adapter/grpc/handler.go
type Handler struct {
    apiv1.UnimplementedDandoriServiceServer
    svc port.WorkflowService
}

func NewHandler(svc port.WorkflowService) *Handler {
    return &Handler{svc: svc}
}

func (h *Handler) StartWorkflow(ctx context.Context, req *apiv1.StartWorkflowRequest) (*apiv1.StartWorkflowResponse, error) {
    wf, err := h.svc.StartWorkflow(ctx, port.StartWorkflowParams{
        ID:           uuid.MustParse(req.WorkflowId),
        WorkflowType: req.WorkflowType,
        TaskQueue:    req.TaskQueue,
        Input:        req.Input,
    })
    if err != nil {
        return nil, status.Errorf(codes.Internal, "failed to start workflow: %v", err)
    }
    return &apiv1.StartWorkflowResponse{
        WorkflowId: wf.ID.String(),
    }, nil
}
```

#### adapter/postgres/ - Outbound Adapter

port/のインターフェースのPostgreSQL実装。マイグレーションもPostgreSQL実装の一部として管理する。

```go
// adapter/postgres/store.go
type Store struct {
    pool *pgxpool.Pool
}

// RunInTx: port.TxManager を満たす
func (s *Store) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
    tx, err := s.pool.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)

    ctx = withTx(ctx, tx)
    if err := fn(ctx); err != nil {
        return err
    }
    return tx.Commit(ctx)
}

func (s *Store) conn(ctx context.Context) pgxtype.Querier {
    if tx := txFromContext(ctx); tx != nil {
        return tx
    }
    return s.pool
}
```

```go
// adapter/postgres/event.go
type EventStore struct {
    store *Store
}

// port.EventRepository を暗黙的に満たす
func (s *EventStore) Append(ctx context.Context, events []domain.HistoryEvent) error {
    conn := s.store.conn(ctx)
    // INSERT INTO workflow_events ...
}

func (s *EventStore) GetByWorkflowID(ctx context.Context, workflowID uuid.UUID) ([]domain.HistoryEvent, error) {
    conn := s.store.conn(ctx)
    // SELECT FROM workflow_events WHERE workflow_id = $1 ORDER BY sequence_num
}
```

### 依存の組み立て（cmd/dandori/main.go）

```go
func main() {
    // Outbound Adapter
    store := postgres.New(pool)

    // Application Core（port.WorkflowServiceを実装）
    eng := engine.New(
        store.Workflows(),   // port.WorkflowRepository
        store.Events(),      // port.EventRepository
        store.Tasks(),       // port.TaskRepository
        store.Timers(),      // port.TimerRepository
        store,               // port.TxManager
    )

    // Inbound Adapter（port.WorkflowServiceインターフェースを受け取る）
    var svc port.WorkflowService = eng
    hdl := grpc.NewHandler(svc)

    // gRPCサーバー起動
    go runGRPCServer(hdl)

    // バックグラウンドプロセス起動
    go eng.RunTimerPoller(ctx)
    go eng.RunTaskTimeoutRecovery(ctx)

    // graceful shutdown
    waitForShutdown(ctx)
}
```

### トランザクション境界

以下の操作は1トランザクション内で実行し、原子性を保証する:

| 操作 | トランザクション内で行うこと |
|------|---------------------------|
| StartWorkflow | WorkflowExecution作成 + イベント記録 + Workflow Task投入 |
| CompleteWorkflowTask | コマンド→イベント変換 + タスク生成 + ステータス更新 |
| CompleteActivityTask | イベント記録 + Workflow Task投入 |
| FailActivityTask | リトライ判定 + イベント記録 + Activity Task再投入 or Workflow Task投入 |

engine/のメソッドがトランザクション境界を決定し、port.TxManagerを通じてトランザクションを開始する。adapter/postgres/の各リポジトリはcontext経由でトランザクションを受け取る。

### バックグラウンドプロセス

サーバーにはリクエスト駆動の処理に加えて、バックグラウンドで動作するプロセスがある:

- タイマーポーラー: 発火時刻到達のtimerを検知し、TimerFiredイベントを記録してWorkflow Taskを生成する
- タスクタイムアウト回収: locked_untilを超えたタスクをPENDINGに戻す

これらはengine/のメソッドとして実装し、cmd/dandori/main.goからgoroutineとして起動する。

## 3. Deterministic Replayの仕組み

### 処理シーケンス

1. クライアントがStartWorkflowを呼ぶ
2. サーバーがWorkflowExecutionStartedイベントを記録し、Workflow Taskをキューに投入
3. ワーカーがWorkflow Taskを取得し、イベント履歴とともにワークフロー関数を最初から実行
4. ワークフロー関数内でExecuteActivityが呼ばれると、SDKがイベント履歴を確認:
   - 完了イベントがあれば記録された結果を即座に返す（replay）
   - なければScheduleActivityTaskコマンドを生成し、ワークフロー関数をサスペンド
5. コマンドリストがサーバーに返される
6. サーバーがコマンドをイベントに変換し、Activity Taskをキューに投入
7. ワーカーがActivity Taskを取得し、Activity関数を実際に実行して結果を報告
8. サーバーがActivityTaskCompletedイベントを記録し、新しいWorkflow Taskを生成
9. ワーカーが再度ワークフロー関数を最初からreplay。既完了のActivityは履歴から結果が返り、新しいActivityに到達する
10. ワークフロー関数が最後まで実行されるとCompleteWorkflowコマンドが返される

### seqIDによるコマンドとイベントの対応付け

ワークフロー関数内でExecuteActivityやSleepが呼ばれるたびにseqIDがインクリメントされる。
replay時にこのseqIDをキーとしてイベント履歴を検索する。

例: 3つのActivityを順次呼ぶワークフロー

- 1回目のExecuteActivity → seqID=0
- 2回目のExecuteActivity → seqID=1
- 3回目のExecuteActivity → seqID=2

イベント履歴（2つ目のActivityまで完了した状態）:

```text
[0] WorkflowExecutionStarted
[1] ActivityTaskScheduled  {seqID: 0, activityType: "validate-order"}
[2] ActivityTaskCompleted  {seqID: 0, result: {...}}
[3] ActivityTaskScheduled  {seqID: 1, activityType: "charge-payment"}
[4] ActivityTaskCompleted  {seqID: 1, result: {...}}
```

replay時、seqID=0と1のExecuteActivityはキャッシュ結果を返し、seqID=2で新しいコマンドが生成される。

## 4. コマンドとイベントの関係

### コマンド一覧（ワーカー → サーバー）

| コマンド | 説明 |
|---------|------|
| ScheduleActivityTask | Activity実行を要求 |
| StartTimer | タイマー開始を要求 |
| CancelTimer | タイマーキャンセルを要求 |
| CompleteWorkflow | ワークフロー正常完了 |
| FailWorkflow | ワークフロー異常終了 |

### コマンド → イベント変換（CommandProcessorが処理）

| コマンド | 生成されるイベント | 副作用 |
|---------|-------------------|--------|
| ScheduleActivityTask | ActivityTaskScheduled | Activity Taskをキューに投入 |
| StartTimer | TimerStarted | timersテーブルにレコード挿入 |
| CancelTimer | TimerCanceled | timersのステータス更新 |
| CompleteWorkflow | WorkflowExecutionCompleted | ステータスをCOMPLETEDに更新 |
| FailWorkflow | WorkflowExecutionFailed | ステータスをFAILEDに更新 |

### 外部トリガー → イベント → Workflow Task生成

| トリガー | 生成されるイベント | 副作用 |
|---------|-------------------|--------|
| StartWorkflow API | WorkflowExecutionStarted | Workflow Task生成 |
| Activity完了報告 | ActivityTaskCompleted | Workflow Task生成 |
| Activity失敗報告 | ActivityTaskFailed | リトライ or Workflow Task生成 |
| タイマー発火 | TimerFired | Workflow Task生成 |
| SignalWorkflow API | WorkflowSignaled | Workflow Task生成 |

### イベントタイプ一覧

```go
// ワークフローライフサイクル
WorkflowExecutionStarted
WorkflowExecutionCompleted
WorkflowExecutionFailed
WorkflowExecutionCancelRequested
WorkflowExecutionCanceled
WorkflowExecutionTimedOut

// Activity
ActivityTaskScheduled
ActivityTaskStarted
ActivityTaskCompleted
ActivityTaskFailed
ActivityTaskTimedOut

// タイマー
TimerStarted
TimerFired
TimerCanceled

// シグナル
WorkflowSignaled

// Workflow Task（内部管理用）
WorkflowTaskScheduled
WorkflowTaskStarted
WorkflowTaskCompleted
```

## 5. workflow.Contextの設計（Go SDKリポジトリで実装）

### 内部構造

```go
type Context struct {
    env *workflowEnvironment
}

type workflowEnvironment struct {
    events       []HistoryEvent   // サーバーから受け取ったイベント履歴
    eventIndex   int              // 現在のreplay位置
    commands     []Command        // このWorkflow Task実行で生成したコマンド
    isReplaying  bool             // replay中かどうか
    scheduler    *coroutineScheduler
    nextSeqID    int64            // コマンドのシーケンスID
}
```

### coroutineScheduler（ブロッキングの実現）

ワークフロー関数は独立したgoroutine上で実行され、yieldでサスペンドされる。
協調スケジューラパターンでメインgoroutineとワークフローgoroutineの間の制御を切り替える。

```go
type coroutineScheduler struct {
    mainCh     chan struct{}  // ワークフロー→メインの通知
    workflowCh chan struct{} // メイン→ワークフローの通知
    ctx        context.Context
    completed  bool
    err        error
}
```

- `start(fn)`: ワークフローgoroutineを起動し、yieldまで待つ
- `yield()`: ワークフローgoroutineからメインに制御を返し、再開を待つ。contextがキャンセルされたらgoroutineを終了

### ExecuteActivityのreplayロジック

```go
func ExecuteActivity[I, O any](ctx Context, activityType string, input I, opts ...ActivityOption) (O, error) {
    env := ctx.env
    seqID := env.nextSeqID
    env.nextSeqID++

    // イベント履歴を確認
    if event := env.findCompletionEvent(seqID); event != nil {
        // replay: 記録された結果を返す
        var result O
        json.Unmarshal(event.Result, &result)
        return result, event.Error
    }

    // 新規: コマンドを生成してサスペンド
    env.commands = append(env.commands, Command{
        Type: CommandScheduleActivityTask,
        Attributes: &ScheduleActivityTaskAttributes{
            SeqID:        seqID,
            ActivityType: activityType,
            Input:        marshalJSON(input),
        },
    })
    env.scheduler.yield()

    var zero O
    return zero, nil
}
```

## 6. PostgreSQLスキーマ

以下のSQLはadapter/postgres/migration/に配置する。

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

### workflow_events テーブル（Source of Truth）

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

### task_queue テーブル

```sql
CREATE TABLE task_queue (
    id              BIGSERIAL PRIMARY KEY,
    task_type       TEXT NOT NULL,  -- 'WORKFLOW' or 'ACTIVITY'
    queue_name      TEXT NOT NULL,
    workflow_id     UUID NOT NULL REFERENCES workflow_executions(id),
    activity_type   TEXT,           -- Activity Task固有
    activity_input  JSONB,          -- Activity Task固有
    activity_seq_id BIGINT,         -- Activity Task固有
    attempt         INT NOT NULL DEFAULT 1,
    max_attempts    INT NOT NULL DEFAULT 3,
    status          TEXT NOT NULL DEFAULT 'PENDING',
    scheduled_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at      TIMESTAMPTZ,
    locked_by       TEXT,
    locked_until    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_task_queue_poll
    ON task_queue(queue_name, task_type, status, scheduled_at)
    WHERE status = 'PENDING';
```

タスク取得クエリ:

```sql
UPDATE task_queue
SET status = 'RUNNING',
    locked_by = $1,
    locked_until = NOW() + INTERVAL '30 seconds',
    started_at = NOW()
WHERE id = (
    SELECT id FROM task_queue
    WHERE queue_name = $2
      AND task_type = $3
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
- JSONB: ワークフロー入出力、イベントデータの柔軟な格納
- Advisory Lock: ワークフロー単位のロック（pg_advisory_xact_lock）で同一ワークフローのWorkflow Task直列化
- LISTEN/NOTIFY: タスク投入の即時通知（Phase 2）
- パーティショニング: workflow_eventsの肥大化対策（Phase 4）

## 7. gRPCサービス定義

```protobuf
service DandoriService {
  // クライアント向けAPI
  rpc StartWorkflow(StartWorkflowRequest) returns (StartWorkflowResponse);
  rpc GetWorkflowExecution(GetWorkflowExecutionRequest) returns (GetWorkflowExecutionResponse);
  rpc TerminateWorkflow(TerminateWorkflowRequest) returns (TerminateWorkflowResponse);

  // ワーカー向けAPI - Workflow Task
  rpc PollWorkflowTask(PollWorkflowTaskRequest) returns (PollWorkflowTaskResponse);
  rpc CompleteWorkflowTask(CompleteWorkflowTaskRequest) returns (CompleteWorkflowTaskResponse);

  // ワーカー向けAPI - Activity Task
  rpc PollActivityTask(PollActivityTaskRequest) returns (PollActivityTaskResponse);
  rpc CompleteActivityTask(CompleteActivityTaskRequest) returns (CompleteActivityTaskResponse);
  rpc FailActivityTask(FailActivityTaskRequest) returns (FailActivityTaskResponse);

  // イベント履歴
  rpc GetWorkflowHistory(GetWorkflowHistoryRequest) returns (GetWorkflowHistoryResponse);
}
```

## 8. Go SDKリポジトリ構成（dandori-sdk-go）

```text
dandori-sdk-go/
├── client/                          # クライアントSDK
│   └── client.go
├── worker/                          # ワーカー
│   ├── worker.go
│   ├── workflow_task_processor.go
│   └── activity_task_processor.go
├── workflow/                        # ワークフロー定義API
│   ├── context.go
│   ├── activity.go
│   ├── scheduler.go
│   └── env.go
├── internal/
│   └── ...
├── examples/
│   └── order/
│       ├── workflows.go
│       ├── activities.go
│       └── main.go
└── go.mod
```

## 9. 実装の難所と対処方針

### トランザクションの一貫性（サーバー側）

CompleteWorkflowTaskでは複数のイベント記録、タスク生成、ステータス更新を1トランザクションで行う。port.TxManagerとcontext伝搬でこれを実現する。

### Workflow Taskの直列化（サーバー側）

同一ワークフローのWorkflow Taskは1つだけが処理中であるよう、pg_advisory_xact_lockで制御する。

### ActivityリトライとseqIDの対応（サーバー側）

リトライはサーバー側で管理。同じseqIDのActivityが失敗→リトライされても、ワークフロー関数から見ると「まだ完了していないActivity」。全リトライ失敗時にActivityTaskFailedイベントが記録される。

### goroutineの管理とリーク防止（Go SDK側）

ワークフローgoroutineはyieldでサスペンドされる。Workflow Task処理完了時にcontextをcancelし、goroutineを適切に終了させる。

### replayの正確性（Go SDK側）

- seqIDの不一致でNonDeterministicErrorを発生させる
- workflow.Now(ctx)、workflow.NewUUID(ctx)等のAPI提供で非決定的操作を回避

### イベント履歴の肥大化

- Phase 1: 全履歴をWorkflow Taskに添付
- Phase 2: ワーカー側キャッシュ（sticky execution）
- Phase 4: Continue-as-New

## 10. Phase 1 MVPの実装ステップ

### サーバー（本リポジトリ）

1. プロジェクト骨格: go mod, docker-compose.yml, proto定義
2. domain/: ドメインモデル型定義
3. port/: リポジトリインターフェース定義
4. adapter/postgres/: PostgreSQL実装、TxManager、マイグレーション
5. engine/: Engine実装、CommandProcessor、リトライポリシー
6. adapter/grpc/: gRPCハンドラ
7. cmd/dandori/: DI、サーバー起動、graceful shutdown
8. テスト（adapter/postgres、engine、adapter/grpc）

### Go SDK（dandori-sdk-goリポジトリ）

1. ワーカーSDK - Activityサイド: ポーリング、Activity実行、結果報告
2. ワーカーSDK - Workflowサイド（核心）: workflow.Context, coroutineScheduler, ExecuteActivity（replay）
3. クライアントSDK: StartWorkflow, GetWorkflowExecution
4. 非決定性検出（seqID不一致でエラー）
5. サンプルワークフロー（順次実行の3ステップ）
6. テスト（replayロジックのユニットテスト、E2Eテスト）

## 11. 検証方法

- サンプルワークフロー（3ステップ順次Activity実行）のエンドツーエンド動作確認
- gRPCurlでStartWorkflow → GetWorkflowExecutionで最終的にCOMPLETEDになることを確認
- ワーカーを強制停止し再起動、ワークフローがreplayで途中から再開されることを確認
- 複数ワーカー起動、タスクが重複実行されないことを確認
- Activity失敗時のリトライが正しく動作することを確認
- replayの正確性テスト: 同じイベント履歴でワークフロー関数を再実行し、同じコマンドが生成されることを確認
- go test ./... で全ユニットテスト通過
- testcontainersでPostgreSQLに対するインテグレーションテスト
