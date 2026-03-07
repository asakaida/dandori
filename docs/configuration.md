# 設定リファレンス

dandoriサーバーの設定は環境変数で行う。

## サーバー環境変数

| 変数 | デフォルト | 説明 |
| --- | --- | --- |
| `DATABASE_URL` | 下記参照 | PostgreSQL接続文字列 |
| `GRPC_PORT` | `7233` | gRPCリッスンポート |
| `HTTP_PORT` | `8080` | HTTPリッスンポート |
| `ENABLE_PPROF` | (無効) | `true`でpprof有効化 |

`DATABASE_URL`のデフォルト値:

```text
postgres://dandori:dandori@localhost:5432/dandori?sslmode=disable
```

## OpenTelemetry環境変数

dandoriはOpenTelemetryのGo SDKを使用するため、標準のOTLP環境変数が利用できる。

| 変数 | デフォルト | 説明 |
| --- | --- | --- |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | (未設定) | OTLPエクスポーターのエンドポイント |
| `OTEL_SERVICE_NAME` | `dandori` | サービス名 |
| `OTEL_SDK_DISABLED` | `false` | `true`でトレーシングを無効化 |

## データベース接続プール

サーバーコード内で以下のプール設定がハードコードされている。

| 設定 | 値 |
| --- | --- |
| MaxOpenConns | 25 |
| MaxIdleConns | 5 |
| ConnMaxLifetime | 5分 |

## バックグラウンドワーカー間隔

サーバー内部のバックグラウンドワーカーの実行間隔。

| ワーカー | 間隔 | 説明 |
| --- | --- | --- |
| ActivityTimeoutChecker | 5秒 | アクティビティタイムアウトの検知 |
| HeartbeatTimeoutChecker | 5秒 | ハートビートタイムアウトの検知 |
| TimerPoller | 1秒 | タイマー発火の検知 |
| TaskRecovery | 10秒 | 放棄されたタスクの回復 |

## HTTPエンドポイント

| パス | 説明 |
| --- | --- |
| `/v1/*` | REST API（gRPC-Gateway） |
| `/ui/` | Web UI |
| `/healthz` | ヘルスチェック |
| `/metrics` | Prometheusメトリクス |
| `/debug/pprof/*` | pprofプロファイリング（`ENABLE_PPROF=true`時のみ） |

## Docker Compose

`docker-compose.yml`のデフォルト設定。

| 設定 | 値 |
| --- | --- |
| PostgreSQLイメージ | `postgres:18-alpine3.23` |
| ユーザー | `dandori` |
| パスワード | `dandori` |
| データベース | `dandori` |
| ポート | `5432` |
| ボリューム | `pgdata` |

## マイグレーション

マイグレーションはサーバー起動時に自動実行される。手動実行は不要。

マイグレーションSQLは `internal/adapter/postgres/migration/` に配置されている。
