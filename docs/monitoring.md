# 監視ガイド

dandoriの監視とObservabilityの設定方法を説明する。

## 概要

dandoriは以下のObservability機能を提供する:

- Prometheusメトリクス（`/metrics`エンドポイント）
- OpenTelemetryトレーシング（OTLP gRPCエクスポート）
- 構造化ログ（slog JSON形式）
- ヘルスチェック（HTTP `/healthz`、gRPC Health v1）
- pprofプロファイリング（`/debug/pprof/`、要 `ENABLE_PPROF=true`）

## Prometheusメトリクス

### エンドポイント

```text
GET http://localhost:8080/metrics
```

### dandori固有メトリクス

カウンター:

| メトリクス名 | 説明 |
| --- | --- |
| dandori_workflow_started_total | 開始されたワークフローの総数 |
| dandori_workflow_completed_total | 完了したワークフローの総数 |
| dandori_workflow_failed_total | 失敗したワークフローの総数 |
| dandori_workflow_terminated_total | 強制終了されたワークフローの総数 |
| dandori_workflow_canceled_total | キャンセルされたワークフローの総数 |
| dandori_workflow_task_poll_total | Workflow Taskポーリングの総数 |
| dandori_workflow_task_complete_total | Workflow Task完了の総数 |
| dandori_workflow_task_fail_total | Workflow Task失敗の総数 |
| dandori_activity_task_poll_total | Activity Taskポーリングの総数 |
| dandori_activity_task_complete_total | Activity Task完了の総数 |
| dandori_activity_task_fail_total | Activity Task失敗の総数 |

ヒストグラム:

| メトリクス名 | ラベル | 説明 |
| --- | --- | --- |
| dandori_operation_duration_seconds | operation | サービス操作のレイテンシ |

ゲージ:

| メトリクス名 | 説明 |
| --- | --- |
| dandori_active_workflows | 現在実行中のワークフロー数（概算） |

Goランタイムメトリクス（`go_*`）とプロセスメトリクス（`process_*`）も公開される。

### Prometheus設定例

```yaml
scrape_configs:
  - job_name: dandori
    scrape_interval: 15s
    static_configs:
      - targets:
          - "dandori:8080"
    metrics_path: /metrics
```

### Grafanaダッシュボード例

推奨するパネル構成:

ワークフロー概要:

- ワークフロー開始レート: `rate(dandori_workflow_started_total[5m])`
- ワークフロー完了レート: `rate(dandori_workflow_completed_total[5m])`
- ワークフロー失敗レート: `rate(dandori_workflow_failed_total[5m])`
- アクティブワークフロー数: `dandori_active_workflows`

タスク処理:

- Workflow Taskポーリングレート: `rate(dandori_workflow_task_poll_total[5m])`
- Activity Taskポーリングレート: `rate(dandori_activity_task_poll_total[5m])`
- タスク完了レート: `rate(dandori_workflow_task_complete_total[5m]) + rate(dandori_activity_task_complete_total[5m])`

レイテンシ:

- 操作レイテンシ (p50): `histogram_quantile(0.5, rate(dandori_operation_duration_seconds_bucket[5m]))`
- 操作レイテンシ (p99): `histogram_quantile(0.99, rate(dandori_operation_duration_seconds_bucket[5m]))`

### アラート設定例

```yaml
groups:
  - name: dandori
    rules:
      - alert: DandoriHighWorkflowFailureRate
        expr: >-
          rate(dandori_workflow_failed_total[5m])
          / rate(dandori_workflow_started_total[5m])
          > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: ワークフロー失敗率が10%を超過
          description: 過去5分間のワークフロー失敗率が{{ $value | humanizePercentage }}

      - alert: DandoriHighTaskFailureRate
        expr: rate(dandori_workflow_task_fail_total[5m]) > 0
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: Workflow Task失敗が継続
          description: 10分以上Workflow Task失敗が発生

      - alert: DandoriHealthCheckFailed
        expr: up{job="dandori"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: dandoriサーバーがダウン
          description: dandoriサーバーへのメトリクス取得が1分以上失敗
```

## OpenTelemetryトレーシング

### 環境変数

| 変数名 | デフォルト値 | 説明 |
| --- | --- | --- |
| OTEL_EXPORTER_OTLP_ENDPOINT | - | OTLPエクスポーター先（例: `http://jaeger:4317`） |
| OTEL_SERVICE_NAME | dandori | サービス名 |
| OTEL_SDK_DISABLED | false | トレーシングの無効化 |

### Jaegerとの連携例

```yaml
services:
  jaeger:
    image: jaegertracing/all-in-one:latest
    ports:
      - "4317:4317"
      - "16686:16686"

  dandori:
    environment:
      OTEL_EXPORTER_OTLP_ENDPOINT: http://jaeger:4317
      OTEL_SERVICE_NAME: dandori
```

Jaeger UIは `http://localhost:16686` でアクセスできる。

### トレーシング対象

dandoriは以下の操作にスパンを生成する:

- 全gRPC RPC呼び出し（gRPC interceptor経由）
- ClientServiceの全メソッド（StartWorkflow, DescribeWorkflow等）
- WorkflowTaskServiceの全メソッド（PollWorkflowTask, CompleteWorkflowTask等）
- ActivityTaskServiceの全メソッド（PollActivityTask, CompleteActivityTask等）

Decorator パターンにより、Engine層の全操作がトレーシング対象となる。

## 構造化ログ

dandoriは `log/slog` でJSON形式の構造化ログを標準エラー出力に出力する。

ログ出力例:

```json
{"time":"2026-03-07T10:00:00Z","level":"INFO","msg":"dandori gRPC server listening","port":"7233"}
{"time":"2026-03-07T10:00:00Z","level":"INFO","msg":"dandori HTTP server listening","port":"8080"}
```

ログは外部のログ収集ツール（Fluentd, Filebeat等）で収集し、Elasticsearch/Lokiなどに集約できる。

## pprofプロファイリング

`ENABLE_PPROF=true` で有効化すると、以下のエンドポイントが利用可能になる:

| エンドポイント | 説明 |
| --- | --- |
| /debug/pprof/ | プロファイルインデックス |
| /debug/pprof/profile | CPUプロファイル（30秒間のサンプリング） |
| /debug/pprof/heap | ヒーププロファイル |
| /debug/pprof/goroutine | ゴルーチンスタック |
| /debug/pprof/trace | 実行トレース |

使用例:

```bash
go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30
go tool pprof http://localhost:8080/debug/pprof/heap
```

本番環境では必要なときのみ有効化することを推奨する。
