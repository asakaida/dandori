# コンセプトガイド

dandoriの中核となる概念を解説する。

## ワークフロー (Workflow)

ワークフローは、dandoriにおける実行の最上位単位である。
一連のアクティビティ、タイマー、シグナル待受などのステップを定義し、サーバーが状態を永続化しながら進行を管理する。

ワークフローは以下の5つの状態を持つ:

- RUNNING: 実行中
- COMPLETED: 正常完了
- FAILED: エラーにより失敗
- TERMINATED: 外部からの強制終了
- CONTINUED_AS_NEW: 新しい実行として継続

ワークフローのロジックは決定論的 (Deterministic) でなければならない。
同じイベント履歴を再生 (Replay) したとき、同じコマンド列が生成されることをサーバーが検証する。

## アクティビティ (Activity)

アクティビティは、ワークフローから呼び出される副作用を伴う処理の単位である。
HTTP APIの呼び出し、データベース操作、ファイル処理など、外部システムとのやり取りをアクティビティとして実装する。

アクティビティには以下のタイムアウトを設定できる:

- StartToCloseTimeout: アクティビティ開始から完了までの制限時間
- ScheduleToCloseTimeout: スケジュールから完了までの制限時間
- ScheduleToStartTimeout: スケジュールからワーカー取得までの制限時間
- HeartbeatTimeout: ハートビート間隔の制限時間

## 決定論的リプレイ (Deterministic Replay)

dandoriの障害耐性の核心は決定論的リプレイにある。
ワーカーがクラッシュしても、サーバーに記録されたイベント履歴からワークフローの状態を復元できる。

仕組み:

1. ワーカーがWorkflow Taskをポーリングすると、全イベント履歴を受け取る
2. ワーカーはワークフローロジックを最初から再実行する
3. 過去に記録されたイベントと照合し、新しいコマンドのみをサーバーに送信する
4. サーバーはシーケンス番号で非決定論的な振る舞いを検出する

## イベントソーシング (Event Sourcing)

dandoriは全ての状態変更をイベントとして追記専用で記録する。
ワークフローの現在の状態はイベント履歴を再生することで導出される。

主なイベントタイプ:

- WorkflowExecutionStarted / Completed / Failed / Terminated
- ActivityTaskScheduled / Completed / Failed / TimedOut
- TimerStarted / Fired / Canceled
- WorkflowSignaled / WorkflowCancelRequested
- ChildWorkflowExecutionStarted / Completed / Failed
- SideEffectRecorded
- WorkflowExecutionContinuedAsNew
- SearchAttributesUpserted

## タスクキュー (Task Queue)

タスクキューは、サーバーとワーカー間の作業分配メカニズムである。
PostgreSQLの `SELECT FOR UPDATE SKIP LOCKED` により、複数ワーカーが同じキューから排他的にタスクを取得する。

タスクには2種類ある:

- Workflow Task: ワークフローロジックの実行判断を行うタスク。イベント履歴を含む。
- Activity Task: アクティビティの実行を指示するタスク。入力データとリトライ情報を含む。

## シグナル (Signal)

シグナルは、外部から実行中のワークフローにデータを送信する仕組みである。
ワークフローはシグナルを受信すると、次のWorkflow Taskでそのイベントを処理できる。

シグナルの送信はAPIまたはCLI経由で行う:

```bash
dandori-cli signal --workflow-id <id> --signal-name <name> --input '{"key":"value"}'
```

## タイマー (Timer)

タイマーは、ワークフロー内で指定した時間だけ待機する仕組みである。
SDKの `Sleep()` 関数として使用される。

タイマーはサーバー側で管理され、ワーカーのクラッシュに影響されない。
TimerPollerバックグラウンドワーカーが1秒間隔で発火済みタイマーを検出し、対応するイベントを記録する。

## 子ワークフロー (Child Workflow)

ワークフローから別のワークフローを起動できる。
親ワークフローは子ワークフローの完了・失敗を待つことができ、親が終了すると子も終了する。

用途:

- 大きなワークフローの分割
- 異なるタスクキューでの実行
- イベント履歴の分離

## Saga / 補償トランザクション

Sagaパターンは、分散システムにおけるトランザクションの整合性を保つ設計パターンである。
各ステップが成功した際に補償アクション（ロールバック処理）を登録し、後続のステップが失敗した場合に逆順で補償アクションを実行する。

dandoriではSDK側で実装され、アクティビティのリトライポリシーと組み合わせて使用する。

## SideEffect

SideEffectは、ワークフロー内で非決定論的な値（乱数、現在時刻など）を安全に使用する仕組みである。
初回実行時に値をイベントとして記録し、リプレイ時には記録済みの値を返す。

## Query

Queryは、実行中のワークフローの内部状態を読み取るための仕組みである。
ワークフローの状態を変更せず、読み取り専用でアクセスする。

## Continue-as-New

長時間実行されるワークフローのイベント履歴が大きくなりすぎることを防ぐ仕組みである。
現在のワークフローを完了し、同じロジックで新しいワークフロー実行を開始する。

主な用途:

- Cronスケジュールワークフローの各実行
- イベント履歴の肥大化を防ぐ定期的なリセット

## Cronスケジュール

ワークフローの定期実行をサポートする。
標準的なcron式（`* * * * *` 形式）で実行間隔を指定する。

Cronワークフローは内部的にContinue-as-Newを使用し、各実行が独立したイベント履歴を持つ。

## Namespace（名前空間）

Namespaceはマルチテナントを実現する論理的な分離単位である。
全てのAPIは `namespace` パラメータを受け取り、デフォルトは `"default"` である。

ワークフロー、タスク、タイマーなどの全リソースがNamespaceに紐付く。

## Search Attributes

Search Attributesは、ワークフローにビジネスメタデータを付与する仕組みである。
`map[string]string` 形式のキーバリューペアとして保存され、ListWorkflows APIでフィルタリングに使用できる。

PostgreSQLのJSONBカラムとGINインデックスにより高速な検索を実現する。

## リトライポリシー (Retry Policy)

アクティビティの自動リトライを制御するポリシーである。

設定項目:

- MaxAttempts: 最大試行回数
- InitialInterval: 初回リトライまでの間隔
- BackoffCoefficient: 指数バックオフの係数
- MaxInterval: リトライ間隔の上限

リトライ間隔の計算: `delay = InitialInterval * BackoffCoefficient^(attempt-1)`（MaxIntervalで上限制限）

アクティビティが `NonRetryable` フラグ付きのエラーを返した場合、リトライは行われない。
