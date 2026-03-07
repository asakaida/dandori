# SDK開発ガイド

dandoriサーバーと連携するSDKの利用方法と、カスタムSDKの開発方法を説明する。

## Go SDK

公式Go SDKは別リポジトリで管理されている: [dandori-sdk-go](https://github.com/asakaida/dandori-sdk-go)

Go SDKはワーカーとクライアントの両方の機能を提供する。

## SDKの役割

dandoriのSDKは以下の責務を持つ:

1. サーバーとのgRPC通信（タスクポーリング、結果送信）
2. ワークフローの決定論的リプレイエンジン
3. アクティビティの実行管理
4. リトライ、タイムアウト、ハートビートの制御
5. シグナル、クエリ、キャンセルの処理
6. Sagaパターンのサポート

## プロトコル仕様

カスタムSDKを開発するには、dandoriの通信プロトコルを理解する必要がある。

### ワーカーのライフサイクル

```text
1. PollWorkflowTask(queue, worker_id)
   → task_id, workflow_id, workflow_type, events[], pending_queries[]

2. ワークフロー関数を実行（イベント履歴でリプレイ）
   → コマンド配列を生成

3. CompleteWorkflowTask(task_id, commands[])
   → サーバーがコマンドを処理

4. PollActivityTask(queue, worker_id)
   → task_id, workflow_id, activity_type, activity_input, attempt

5. アクティビティ関数を実行

6. CompleteActivityTask(task_id, result)
   または FailActivityTask(task_id, failure)
```

### 決定論的リプレイの実装

SDKの最も重要な責務は決定論的リプレイの実装である。

リプレイの手順:

1. イベント履歴を受け取る
2. ワークフロー関数を先頭から実行する
3. `ScheduleActivity()` 等の呼び出し時に:
   - イベント履歴にマッチするイベントがあれば、記録済みの結果を返す
   - マッチするイベントがなければ、新しいコマンドとして記録する
4. シーケンス番号でイベントとコマンドの対応を管理する
5. 全ての新しいコマンドをCompleteWorkflowTaskで送信する

### シーケンス番号の管理

SDKは内部カウンタでシーケンス番号（seq_id）を管理する。
アクティビティ、タイマー、子ワークフロー、SideEffectの各操作にユニークなseq_idを割り当てる。

リプレイ時にseq_idが過去のイベントと一致しない場合、非決定論的エラー（Non-Determinism Error）が発生する。

### コマンドの構成

CompleteWorkflowTaskで送信するコマンドは以下の形式:

```json
{
  "type": "SCHEDULE_ACTIVITY_TASK",
  "attributes": "<JSON-encoded ScheduleActivityTaskAttributes>"
}
```

attributes フィールドはコマンドタイプごとに異なる構造のJSONをバイト列にエンコードしたもの。

### アクティビティのタイムアウト

SDKはアクティビティスケジュール時に以下のタイムアウトを指定できる:

- `start_to_close_timeout`: アクティビティ開始から完了まで
- `schedule_to_close_timeout`: スケジュールから完了まで
- `schedule_to_start_timeout`: スケジュールからワーカー取得まで
- `heartbeat_timeout`: ハートビート間隔

### ハートビート

長時間実行アクティビティでは、定期的にハートビートを送信する必要がある:

```text
RecordActivityHeartbeat(task_id, details)
```

`heartbeat_timeout` を超えてハートビートが来ない場合、サーバーはアクティビティをタイムアウトとして扱う。

### シグナル処理

ワークフローがシグナルを受信すると、イベント履歴に `WorkflowSignaled` イベントが追加される。
SDKはリプレイ中にこのイベントを検出し、登録されたシグナルハンドラを呼び出す。

### キャンセル処理

ワークフローにキャンセルがリクエストされると、`WorkflowCancelRequested` イベントが記録される。
SDKはこのイベントを検出し、ワークフローにキャンセルを通知する。
ワークフローはクリーンアップ処理を行った後、FailWorkflowコマンドで終了する。

### クエリ処理

PollWorkflowTaskのレスポンスに `pending_queries` が含まれる場合がある。
SDKはワークフローの現在の状態を使ってクエリに応答し、RespondQueryTaskで結果を返す。

### Continue-as-New

SDKがContinueAsNewコマンドを送信すると、現在のワークフローはCONTINUED_AS_NEWステータスで終了し、新しいワークフロー実行が自動作成される。

### SideEffect

SDKは `RecordSideEffect` コマンドで非決定論的な値を記録する。
リプレイ時には `SideEffectRecorded` イベントから値を復元し、関数を再実行しない。

## カスタムSDK開発の手順

1. gRPCクライアントコードを生成する（`api/v1/service.proto` から）
2. Workflow Workerを実装する（PollWorkflowTask → リプレイ → CompleteWorkflowTask）
3. Activity Workerを実装する（PollActivityTask → 実行 → Complete/FailActivityTask）
4. 決定論的リプレイエンジンを実装する（シーケンス番号管理、イベントマッチング）
5. ユーザー向けAPIを設計する（ワークフロー定義、アクティビティ定義の方法）
