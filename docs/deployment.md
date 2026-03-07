# デプロイメントガイド

dandoriサーバーのデプロイ方法を説明する。

## 前提条件

- PostgreSQL 18以上
- dandoriサーバーバイナリ
- ネットワークポート: gRPC（デフォルト 7233）、HTTP（デフォルト 8080）

## バイナリ直接デプロイ

### ビルド

```bash
go build -o dandori ./cmd/dandori
go build -o dandori-cli ./cmd/dandori-cli
```

### 起動

```bash
export DATABASE_URL="postgres://user:pass@db-host:5432/dandori?sslmode=require"
export GRPC_PORT=7233
export HTTP_PORT=8080
./dandori
```

マイグレーションはサーバー起動時に自動実行される。

## Docker Composeデプロイ

開発・小規模環境向け。

```yaml
services:
  postgres:
    image: postgres:18-alpine3.23
    environment:
      POSTGRES_USER: dandori
      POSTGRES_PASSWORD: dandori
      POSTGRES_DB: dandori
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data

  dandori:
    build: .
    environment:
      DATABASE_URL: postgres://dandori:dandori@postgres:5432/dandori?sslmode=disable
      GRPC_PORT: "7233"
      HTTP_PORT: "8080"
    ports:
      - "7233:7233"
      - "8080:8080"
    depends_on:
      - postgres

volumes:
  pgdata:
```

## Kubernetesデプロイ

### Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dandori
spec:
  replicas: 2
  selector:
    matchLabels:
      app: dandori
  template:
    metadata:
      labels:
        app: dandori
    spec:
      containers:
        - name: dandori
          image: dandori:latest
          ports:
            - containerPort: 7233
              name: grpc
            - containerPort: 8080
              name: http
          env:
            - name: DATABASE_URL
              valueFrom:
                secretKeyRef:
                  name: dandori-db
                  key: url
            - name: GRPC_PORT
              value: "7233"
            - name: HTTP_PORT
              value: "8080"
          readinessProbe:
            httpGet:
              path: /healthz
              port: http
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:
            httpGet:
              path: /healthz
              port: http
            initialDelaySeconds: 15
            periodSeconds: 20
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
            limits:
              cpu: 500m
              memory: 512Mi
```

### Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: dandori
spec:
  selector:
    app: dandori
  ports:
    - name: grpc
      port: 7233
      targetPort: grpc
    - name: http
      port: 8080
      targetPort: http
```

### Secret（データベース接続）

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: dandori-db
type: Opaque
stringData:
  url: postgres://user:pass@db-host:5432/dandori?sslmode=require
```

## 環境変数

サーバー設定の詳細は [設定リファレンス](configuration.md) を参照。

主要な環境変数:

| 変数名 | デフォルト値 | 説明 |
| --- | --- | --- |
| DATABASE_URL | (下記参照) | PostgreSQL接続文字列 |
| GRPC_PORT | 7233 | gRPCリッスンポート |
| HTTP_PORT | 8080 | HTTPリッスンポート |
| ENABLE_PPROF | false | pprofエンドポイントの有効化 |
| OTEL_EXPORTER_OTLP_ENDPOINT | - | OTLPエクスポーター先 |
| OTEL_SERVICE_NAME | dandori | OTelサービス名 |
| OTEL_SDK_DISABLED | false | OTelトレーシングの無効化 |

DATABASE_URLのデフォルト値:
`postgres://dandori:dandori@localhost:5432/dandori?sslmode=disable`

## PostgreSQLの推奨設定

dandoriはPostgreSQLに大きく依存しているため、適切な設定が重要である。

### 接続プール

dandoriサーバーのデフォルト接続プール設定:

- MaxOpenConns: 25
- MaxIdleConns: 5
- ConnMaxLifetime: 5分

複数のdandoriインスタンスを実行する場合、PostgreSQLの `max_connections` を調整する。

### パフォーマンス

- `shared_buffers`: RAMの25%
- `work_mem`: 4MB以上
- `effective_cache_size`: RAMの75%
- WAL設定: ワークフロースループットに応じて調整

### バックアップ

dandoriはイベントソーシングを採用しているため、データベースのバックアップはシステムの完全なバックアップとなる。
定期的なpg_dumpまたはWALアーカイブによるバックアップを推奨する。

## ヘルスチェック

- HTTP: `GET /healthz`（PostgreSQL接続確認を含む）
- gRPC: `grpc.health.v1.Health/Check`

## 水平スケーリング

dandoriサーバーは複数インスタンスで水平スケーリングできる。
PostgreSQLの `SKIP LOCKED` によりタスクの排他制御が保証される。

考慮事項:

- 全インスタンスが同一のPostgreSQLデータベースに接続する
- バックグラウンドワーカー（タイムアウト検出、タイマーポーリング、タスクリカバリ）は各インスタンスで動作するが、冪等に設計されているため問題ない
- ロードバランサーでgRPC/HTTPトラフィックを分散する

## グレースフルシャットダウン

dandoriはSIGINT/SIGTERMを受信すると以下の順序でシャットダウンする:

1. コンテキストキャンセル（バックグラウンドワーカー停止）
2. HTTPサーバーのグレースフルシャットダウン（5秒タイムアウト）
3. gRPCサーバーのGracefulStop
4. データベース接続のクローズ
