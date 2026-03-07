# コントリビューションガイド

dandoriへの貢献方法を説明する。

## 前提条件

- Go 1.24以上
- Docker / Docker Compose
- protoc（Protocol Buffersコンパイラ）
- protoc-gen-go, protoc-gen-go-grpc, protoc-gen-grpc-gateway（コード生成プラグイン）

## 開発環境の構築

### 1. リポジトリのクローン

```bash
git clone https://github.com/asakaida/dandori.git
cd dandori
```

### 2. PostgreSQLの起動

```bash
docker compose up -d
```

PostgreSQL 18が `localhost:5432` で起動する。

### 3. 依存パッケージの取得

```bash
go mod download
```

### 4. サーバーのビルドと起動

```bash
go build -o dandori ./cmd/dandori
go build -o dandori-cli ./cmd/dandori-cli
./dandori
```

マイグレーションはサーバー起動時に自動実行される。

## テストの実行

### ユニットテスト

```bash
go test ./internal/engine/...
```

### インテグレーションテスト（PostgreSQL必須、testcontainersで自動起動）

```bash
go test ./internal/adapter/postgres/...
```

### E2Eテスト

```bash
go test ./internal/test/e2e/...
```

### ベンチマークテスト

```bash
go test -bench=. ./internal/test/bench/...
```

### 全テスト

```bash
go test ./...
```

testcontainers-goを使用しているため、Docker Daemonが稼働している必要がある。
テスト用のPostgreSQLコンテナは自動的に作成・破棄される。

## Protocol Buffersの再生成

`api/v1/service.proto` または `api/v1/types.proto` を変更した場合:

```bash
protoc \
  --go_out=. --go_opt=paths=source_relative \
  --go-grpc_out=. --go-grpc_opt=paths=source_relative \
  --grpc-gateway_out=. --grpc-gateway_opt=paths=source_relative \
  --openapiv2_out=api/v1 \
  api/v1/service.proto api/v1/types.proto
```

## ディレクトリ構成とコーディング規約

### レイヤー間の依存ルール

- `internal/domain/`: 外部パッケージに依存しない（標準ライブラリとgoogle/uuidのみ）
- `internal/port/`: domain のみに依存する
- `internal/engine/`: domain と port のみに依存する
- `internal/adapter/`: domain, port に依存する（engine には依存しない）

adapter層がengine層に直接依存しないことで、テスト時のモック差し替えが容易になる。

### エラーハンドリング

- ドメインエラーは `internal/domain/errors.go` に定義する
- gRPCハンドラでドメインエラーをgRPCステータスコードに変換する
- `errors.Is()` でエラー判定を行う

### ログ

- `log/slog` を使用する（構造化JSON出力）
- サードパーティのロギングライブラリは使用しない

### データベースアクセス

- `database/sql` + `github.com/lib/pq` を使用する（ORMは不使用）
- トランザクションは `TxManager.RunInTx()` で管理する
- SQLは直接記述する

## PR手順

1. mainブランチから作業ブランチを作成する
2. 変更を実装する
3. 関連するテスト（ユニット、インテグレーション、E2E）を追加・更新する
4. `go test ./...` で全テストが通ることを確認する
5. `go vet ./...` でリンターエラーがないことを確認する
6. PRを作成する
