# APIリファレンス

dandoriは gRPC API と HTTP API（grpc-gateway）の2つのインターフェースを提供する。
全APIに `namespace` パラメータがあり、省略時は `"default"` が使用される。

## エンドポイント

- gRPC: `localhost:7233`（デフォルト）
- HTTP: `localhost:8080`（デフォルト）
- OpenAPI仕様: `GET /swagger.json`

## Client API

### StartWorkflow

ワークフローの実行を開始する。`workflow_id` を省略するとUUIDが自動生成される。
同一IDで実行中のワークフローが存在する場合は冪等に既存のIDを返す。
終了済みのワークフローと同一IDの場合は、関連データを削除して再作成する。

- gRPC: `DandoriService/StartWorkflow`
- HTTP: `POST /v1/workflows`

リクエスト:

| フィールド | 型 | 必須 | 説明 |
| --- | --- | --- | --- |
| workflow_id | string | - | ワークフローID（省略時は自動生成） |
| workflow_type | string | 必須 | ワークフロータイプ名 |
| task_queue | string | 必須 | タスクキュー名 |
| input | bytes | - | ワークフローへの入力（JSON） |
| cron_schedule | string | - | Cronスケジュール式 |
| namespace | string | - | 名前空間（デフォルト: "default"） |

レスポンス:

| フィールド | 型 | 説明 |
| --- | --- | --- |
| workflow_id | string | 作成されたワークフローのID |

### DescribeWorkflow

ワークフローの詳細情報を取得する。

- gRPC: `DandoriService/DescribeWorkflow`
- HTTP: `GET /v1/workflows/{workflow_id}`

リクエスト:

| フィールド | 型 | 必須 | 説明 |
| --- | --- | --- | --- |
| workflow_id | string | 必須 | ワークフローID |
| namespace | string | - | 名前空間 |

レスポンス: `WorkflowExecution` オブジェクト

### GetWorkflowHistory

ワークフローのイベント履歴を取得する。

- gRPC: `DandoriService/GetWorkflowHistory`
- HTTP: `GET /v1/workflows/{workflow_id}/history`

リクエスト:

| フィールド | 型 | 必須 | 説明 |
| --- | --- | --- | --- |
| workflow_id | string | 必須 | ワークフローID |
| namespace | string | - | 名前空間 |

レスポンス:

| フィールド | 型 | 説明 |
| --- | --- | --- |
| events | HistoryEvent[] | イベント履歴の配列 |

### ListWorkflows

ワークフロー一覧を取得する。カーソルベースのページネーションをサポートする。

- gRPC: `DandoriService/ListWorkflows`
- HTTP: `GET /v1/workflows`

リクエスト:

| フィールド | 型 | 必須 | 説明 |
| --- | --- | --- | --- |
| page_size | int32 | - | ページサイズ（デフォルト: 20） |
| next_page_token | string | - | 次ページトークン |
| status_filter | string | - | ステータスフィルタ（RUNNING, COMPLETED等） |
| type_filter | string | - | ワークフロータイプフィルタ |
| queue_filter | string | - | タスクキューフィルタ |
| namespace | string | - | 名前空間 |
| search_attributes_filter | map | - | Search Attributesによるフィルタ |

レスポンス:

| フィールド | 型 | 説明 |
| --- | --- | --- |
| workflows | WorkflowExecution[] | ワークフロー配列 |
| next_page_token | string | 次ページトークン（最終ページでは空） |

### SignalWorkflow

実行中のワークフローにシグナルを送信する。

- gRPC: `DandoriService/SignalWorkflow`
- HTTP: `POST /v1/workflows/{workflow_id}/signals`

リクエスト:

| フィールド | 型 | 必須 | 説明 |
| --- | --- | --- | --- |
| workflow_id | string | 必須 | ワークフローID |
| signal_name | string | 必須 | シグナル名 |
| input | bytes | - | シグナルデータ（JSON） |
| namespace | string | - | 名前空間 |

### CancelWorkflow

ワークフローにキャンセルをリクエストする。ワークフローはキャンセルイベントを受け取り、自身でクリーンアップ処理を行える。

- gRPC: `DandoriService/CancelWorkflow`
- HTTP: `POST /v1/workflows/{workflow_id}/cancellation`

リクエスト:

| フィールド | 型 | 必須 | 説明 |
| --- | --- | --- | --- |
| workflow_id | string | 必須 | ワークフローID |
| namespace | string | - | 名前空間 |

### TerminateWorkflow

ワークフローを即座に強制終了する。キャンセルと異なり、ワークフローにクリーンアップの機会は与えられない。

- gRPC: `DandoriService/TerminateWorkflow`
- HTTP: `POST /v1/workflows/{workflow_id}/termination`

リクエスト:

| フィールド | 型 | 必須 | 説明 |
| --- | --- | --- | --- |
| workflow_id | string | 必須 | ワークフローID |
| reason | string | - | 終了理由 |
| namespace | string | - | 名前空間 |

### QueryWorkflow

実行中のワークフローの内部状態を読み取る。

- gRPC: `DandoriService/QueryWorkflow`
- HTTP: `POST /v1/workflows/{workflow_id}/queries`

リクエスト:

| フィールド | 型 | 必須 | 説明 |
| --- | --- | --- | --- |
| workflow_id | string | 必須 | ワークフローID |
| query_type | string | 必須 | クエリタイプ名 |
| input | bytes | - | クエリ入力（JSON） |
| namespace | string | - | 名前空間 |

レスポンス:

| フィールド | 型 | 説明 |
| --- | --- | --- |
| result | bytes | クエリ結果（JSON） |
| error_message | string | エラーメッセージ（エラー時） |

## Worker API: Workflow Task

### PollWorkflowTask

Workflow Taskをポーリングする。タスクがない場合は空レスポンスを返す。

- gRPC: `DandoriService/PollWorkflowTask`
- HTTP: `POST /v1/workflow-tasks/poll`

リクエスト:

| フィールド | 型 | 必須 | 説明 |
| --- | --- | --- | --- |
| queue_name | string | 必須 | タスクキュー名 |
| worker_id | string | 必須 | ワーカー識別子 |
| namespace | string | - | 名前空間 |

レスポンス:

| フィールド | 型 | 説明 |
| --- | --- | --- |
| task_id | int64 | タスクID（タスクなしの場合は0） |
| workflow_id | string | ワークフローID |
| workflow_type | string | ワークフロータイプ |
| events | HistoryEvent[] | 全イベント履歴 |
| pending_queries | PendingQuery[] | 未回答のクエリ |

### CompleteWorkflowTask

Workflow Taskを完了し、コマンドを送信する。

- gRPC: `DandoriService/CompleteWorkflowTask`
- HTTP: `POST /v1/workflow-tasks/{task_id}/completion`

リクエスト:

| フィールド | 型 | 必須 | 説明 |
| --- | --- | --- | --- |
| task_id | int64 | 必須 | タスクID |
| commands | Command[] | 必須 | 実行するコマンド配列 |
| metadata | map | - | メタデータ |

### FailWorkflowTask

Workflow Taskの失敗を報告する。

- gRPC: `DandoriService/FailWorkflowTask`
- HTTP: `POST /v1/workflow-tasks/{task_id}/failure`

リクエスト:

| フィールド | 型 | 必須 | 説明 |
| --- | --- | --- | --- |
| task_id | int64 | 必須 | タスクID |
| cause | string | - | 失敗原因 |
| message | string | - | エラーメッセージ |

### RespondQueryTask

クエリに対する応答を返す。

- gRPC: `DandoriService/RespondQueryTask`
- HTTP: `POST /v1/queries/{query_id}/response`

リクエスト:

| フィールド | 型 | 必須 | 説明 |
| --- | --- | --- | --- |
| query_id | int64 | 必須 | クエリID |
| result | bytes | - | クエリ結果（JSON） |
| error_message | string | - | エラーメッセージ |

## Worker API: Activity Task

### PollActivityTask

Activity Taskをポーリングする。タスクがない場合は空レスポンスを返す。

- gRPC: `DandoriService/PollActivityTask`
- HTTP: `POST /v1/activity-tasks/poll`

リクエスト:

| フィールド | 型 | 必須 | 説明 |
| --- | --- | --- | --- |
| queue_name | string | 必須 | タスクキュー名 |
| worker_id | string | 必須 | ワーカー識別子 |
| namespace | string | - | 名前空間 |

レスポンス:

| フィールド | 型 | 説明 |
| --- | --- | --- |
| task_id | int64 | タスクID（タスクなしの場合は0） |
| workflow_id | string | ワークフローID |
| activity_type | string | アクティビティタイプ |
| activity_input | bytes | アクティビティ入力 |
| attempt | int32 | 現在の試行回数 |
| scheduled_at | Timestamp | スケジュール時刻 |

### CompleteActivityTask

Activity Taskを正常完了する。

- gRPC: `DandoriService/CompleteActivityTask`
- HTTP: `POST /v1/activity-tasks/{task_id}/completion`

リクエスト:

| フィールド | 型 | 必須 | 説明 |
| --- | --- | --- | --- |
| task_id | int64 | 必須 | タスクID |
| result | bytes | - | アクティビティ結果（JSON） |

### FailActivityTask

Activity Taskの失敗を報告する。リトライポリシーに基づいて自動リトライされる場合がある。

- gRPC: `DandoriService/FailActivityTask`
- HTTP: `POST /v1/activity-tasks/{task_id}/failure`

リクエスト:

| フィールド | 型 | 必須 | 説明 |
| --- | --- | --- | --- |
| task_id | int64 | 必須 | タスクID |
| failure | ActivityFailure | 必須 | 失敗情報 |

ActivityFailure:

| フィールド | 型 | 説明 |
| --- | --- | --- |
| message | string | エラーメッセージ |
| type | string | エラータイプ |
| non_retryable | bool | trueの場合リトライしない |

### RecordActivityHeartbeat

Activity Taskのハートビートを記録する。長時間実行されるアクティビティの生存確認に使用する。

- gRPC: `DandoriService/RecordActivityHeartbeat`
- HTTP: `POST /v1/activity-tasks/{task_id}/heartbeats`

リクエスト:

| フィールド | 型 | 必須 | 説明 |
| --- | --- | --- | --- |
| task_id | int64 | 必須 | タスクID |
| details | bytes | - | ハートビート詳細データ |

## コマンドタイプ

CompleteWorkflowTaskで送信するコマンドの種類:

| コマンドタイプ | 説明 |
| --- | --- |
| SCHEDULE_ACTIVITY_TASK | アクティビティをスケジュール |
| COMPLETE_WORKFLOW | ワークフローを正常完了 |
| FAIL_WORKFLOW | ワークフローを失敗で完了 |
| START_TIMER | タイマーを開始 |
| CANCEL_TIMER | タイマーをキャンセル |
| START_CHILD_WORKFLOW | 子ワークフローを開始 |
| RECORD_SIDE_EFFECT | SideEffectを記録 |
| CONTINUE_AS_NEW | 新しい実行として継続 |
| UPSERT_SEARCH_ATTRIBUTES | Search Attributesを更新 |

## イベントタイプ

イベント履歴に記録されるイベントの種類:

| イベントタイプ | 説明 |
| --- | --- |
| WorkflowExecutionStarted | ワークフロー実行開始 |
| WorkflowExecutionCompleted | ワークフロー正常完了 |
| WorkflowExecutionFailed | ワークフロー失敗 |
| WorkflowExecutionTerminated | ワークフロー強制終了 |
| ActivityTaskScheduled | アクティビティスケジュール |
| ActivityTaskCompleted | アクティビティ正常完了 |
| ActivityTaskFailed | アクティビティ失敗 |
| ActivityTaskTimedOut | アクティビティタイムアウト |
| TimerStarted | タイマー開始 |
| TimerFired | タイマー発火 |
| TimerCanceled | タイマーキャンセル |
| WorkflowSignaled | シグナル受信 |
| WorkflowCancelRequested | キャンセルリクエスト受信 |
| ChildWorkflowExecutionStarted | 子ワークフロー開始 |
| ChildWorkflowExecutionCompleted | 子ワークフロー完了 |
| ChildWorkflowExecutionFailed | 子ワークフロー失敗 |
| SideEffectRecorded | SideEffect記録 |
| WorkflowExecutionContinuedAsNew | Continue-as-New実行 |
| SearchAttributesUpserted | Search Attributes更新 |

## ワークフローステータス

| ステータス | 説明 |
| --- | --- |
| RUNNING | 実行中 |
| COMPLETED | 正常完了 |
| FAILED | 失敗 |
| TERMINATED | 強制終了 |
| CONTINUED_AS_NEW | 新しい実行として継続 |

## 補助エンドポイント

| エンドポイント | メソッド | 説明 |
| --- | --- | --- |
| /healthz | GET | ヘルスチェック（DB接続確認含む） |
| /metrics | GET | Prometheusメトリクス |
| /swagger.json | GET | OpenAPI v2仕様 |
| /ui/ | GET | Web UI（SPA） |
| /v1/sse/workflows | GET | Server-Sent Events（リアルタイム更新） |
| /debug/pprof/ | GET | pprofプロファイリング（ENABLE_PPROF=true時のみ） |

## エラーコード

gRPCステータスコードのマッピング:

| ドメインエラー | gRPCコード | 説明 |
| --- | --- | --- |
| ErrWorkflowNotFound | NOT_FOUND | ワークフローが存在しない |
| ErrWorkflowAlreadyExists | ALREADY_EXISTS | ワークフローが既に存在する |
| ErrWorkflowNotRunning | FAILED_PRECONDITION | ワークフローが実行中でない |
| ErrTaskNotFound | NOT_FOUND | タスクが存在しない |
| ErrTaskAlreadyCompleted | FAILED_PRECONDITION | タスクが既に完了済み |
| ErrNoTaskAvailable | - | タスクなし（空レスポンスを返す） |
| ErrQueryNotFound | NOT_FOUND | クエリが存在しない |
| ErrQueryTimedOut | DEADLINE_EXCEEDED | クエリがタイムアウト |
| ErrNamespaceNotFound | NOT_FOUND | 名前空間が存在しない |
