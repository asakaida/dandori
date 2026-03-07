# CLIリファレンス

dandori-cliはdandoriサーバーをコマンドラインから操作するためのツール。

## ビルド

```bash
go build -o dandori-cli ./cmd/dandori-cli
```

## グローバルフラグ

全サブコマンドで使用可能。

| フラグ | デフォルト | 説明 |
| --- | --- | --- |
| `--server` | `localhost:7233` | gRPCサーバーアドレス |
| `--namespace` | `default` | Namespace |

## サブコマンド

### start - ワークフローの開始

```bash
dandori-cli start --type MyWorkflow --queue my-queue --input '{"key":"value"}'
```

| フラグ | 必須 | デフォルト | 説明 |
| --- | --- | --- | --- |
| `--type` | はい | - | ワークフロータイプ |
| `--queue` | いいえ | `default` | タスクキュー |
| `--id` | いいえ | (自動生成) | ワークフローID |
| `--input` | いいえ | `{}` | 入力JSON |
| `--cron` | いいえ | - | cronスケジュール（5フィールド形式） |

```bash
# 基本的な使い方
dandori-cli start --type HelloWorkflow --queue hello-queue

# IDを指定
dandori-cli start --type HelloWorkflow --queue hello-queue --id my-workflow-1

# 入力を指定
dandori-cli start --type HelloWorkflow --queue hello-queue --input '{"name":"dandori"}'

# cronスケジュール
dandori-cli start --type CleanupWorkflow --queue ops-queue --cron "0 * * * *"
```

### describe - ワークフローの詳細表示

```bash
dandori-cli describe <workflow_id>
```

出力例:

```text
ID:      hello-world-1
Type:    HelloWorkflow
Queue:   hello-queue
Status:  COMPLETED
Created: 2026-03-07 10:00:00
Closed:  2026-03-07 10:00:01
Result:  {"message":"Hello, dandori!"}
```

### history - イベント履歴の表示

```bash
dandori-cli history <workflow_id>
```

出力例:

```text
SEQ  EVENT_TYPE                   TIMESTAMP            DATA
1    WorkflowExecutionStarted    2026-03-07 10:00:00  ...
2    ActivityTaskScheduled        2026-03-07 10:00:00  ...
3    ActivityTaskCompleted        2026-03-07 10:00:00  ...
4    WorkflowExecutionCompleted   2026-03-07 10:00:01  ...
```

### list - ワークフロー一覧の表示

```bash
dandori-cli list [flags]
```

| フラグ | デフォルト | 説明 |
| --- | --- | --- |
| `--status` | (全て) | ステータスでフィルタ |
| `--type` | (全て) | ワークフロータイプでフィルタ |
| `--queue` | (全て) | タスクキューでフィルタ |
| `--limit` | `20` | 1ページの表示件数 |
| `--token` | - | 次ページトークン |

```bash
# 全ワークフロー
dandori-cli list

# RUNNINGのみ
dandori-cli list --status RUNNING

# 特定タイプのみ
dandori-cli list --type HelloWorkflow
```

### signal - シグナルの送信

```bash
dandori-cli signal <workflow_id> --name <signal_name> [--input '<json>']
```

| フラグ | 必須 | デフォルト | 説明 |
| --- | --- | --- | --- |
| `--name` | はい | - | シグナル名 |
| `--input` | いいえ | `{}` | シグナル入力JSON |

```bash
dandori-cli signal my-workflow-1 --name approve --input '{"approved":true}'
```

### cancel - キャンセルの要求

```bash
dandori-cli cancel <workflow_id>
```

ワークフローにキャンセルを要求する。ワークフロー側でキャンセルを処理するかどうかはワークフローの実装次第。

### terminate - 強制終了

```bash
dandori-cli terminate <workflow_id> [--reason '<reason>']
```

| フラグ | 必須 | デフォルト | 説明 |
| --- | --- | --- | --- |
| `--reason` | いいえ | - | 終了理由 |

```bash
dandori-cli terminate my-workflow-1 --reason "手動停止"
```

キャンセルと異なり、ワークフローは即座に終了する。補償処理は実行されない。
