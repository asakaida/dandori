# トラブルシューティング

dandoriの運用でよく遭遇する問題と解決方法をまとめる。

## サーバー起動

### データベース接続エラー

症状:

```text
failed to ping database
```

原因と対処:

- PostgreSQLが起動していない → `docker compose up -d` で起動する
- DATABASE_URLが間違っている → 接続文字列を確認する（デフォルト: `postgres://dandori:dandori@localhost:5432/dandori?sslmode=disable`）
- ポートが競合している → `docker compose ps` でPostgreSQLのポートを確認する

### マイグレーションエラー

症状:

```text
failed to run migrations
```

原因と対処:

- データベースが存在しない → `createdb dandori` で作成する（Docker Compose使用時は自動作成される）
- マイグレーションが中途半端に適用された → `schema_migrations` テーブルの状態を確認し、手動で修正する

## ワークフロー実行

### ワークフローが進行しない

考えられる原因:

1. ワーカーが起動していない → SDK側のワーカープロセスが稼働しているか確認する
2. タスクキュー名が一致していない → StartWorkflowで指定した `task_queue` とワーカーのポーリング先が同じか確認する
3. Namespace が一致していない → サーバーとワーカーで同じNamespaceを使用しているか確認する

CLIで確認:

```bash
dandori-cli describe --workflow-id <id>
dandori-cli history --workflow-id <id>
```

### NonDeterminismError（非決定論的エラー）

症状: Workflow Taskが繰り返し失敗する

原因:

- ワークフローコードを変更した後、実行中のワークフローをリプレイした
- ワークフロー内で非決定論的な処理（乱数、現在時刻、外部API呼び出し等）を直接使用した

対処:

- 非決定論的な値はSideEffectを使用する
- ワークフローコードの変更はバージョニングで対応する
- 進行不能なワークフローは `dandori-cli terminate` で強制終了する

### アクティビティがタイムアウトする

考えられる原因:

1. StartToCloseTimeoutが短すぎる → タイムアウト値を調整する
2. アクティビティワーカーがクラッシュした → ワーカーのログを確認する
3. HeartbeatTimeoutが設定されているがハートビートを送信していない → アクティビティ内で定期的にRecordActivityHeartbeatを呼び出す

イベント履歴で確認:

```bash
dandori-cli history --workflow-id <id>
```

`ActivityTaskTimedOut` イベントのタイミングとアクティビティのスケジュール時刻を比較する。

### リトライが期待通りに動作しない

確認ポイント:

- RetryPolicyが設定されているか（未設定の場合リトライは行われない）
- MaxAttemptsに達していないか
- エラーが `non_retryable: true` で返されていないか

## タスクキュー

### タスクが溜まり続ける

原因:

- ワーカー数が不足している → ワーカーのインスタンス数を増やす
- 個々のアクティビティの処理時間が長い → タイムアウト設定の見直し、処理の最適化

確認方法:

```sql
SELECT status, count(*) FROM workflow_tasks GROUP BY status;
SELECT status, count(*) FROM activity_tasks GROUP BY status;
```

### PENDING状態のタスクが残り続ける

原因:

- ワーカーがタスクを取得後にクラッシュし、タスクがRUNNING状態のまま残った

対処:

- dandoriのTaskRecoveryバックグラウンドワーカーが10秒間隔でstaleタスクを自動回収する
- 即座に回収したい場合はサーバーを再起動する

## データベース

### コネクションプール枯渇

症状: リクエストがタイムアウトする、レスポンスが極端に遅い

原因:

- dandoriの接続数（デフォルト25）がPostgreSQLの `max_connections` を超えている
- 複数のdandoriインスタンスが同じDBに接続している

対処:

- PostgreSQLの `max_connections` を増やす
- dandoriインスタンス数に応じて接続プール設定を調整する

### イベントテーブルの肥大化

長期運用でworkflow_eventsテーブルが大きくなる場合:

- dandoriはハッシュパーティショニング（16分割）を使用しており、一定のスケーラビリティがある
- 完了済みワークフローの古いイベントを定期的にアーカイブ・削除することを検討する

## Web UI

### Web UIにアクセスできない

確認ポイント:

- HTTPサーバーが起動しているか（デフォルトポート: 8080）
- URL: `http://localhost:8080/ui/`（末尾スラッシュが必要）

### リアルタイム更新が動作しない

SSE（Server-Sent Events）の問題:

- ブラウザの開発者ツールでネットワークタブを確認し、`/v1/sse/workflows` への接続状態を確認する
- プロキシやロードバランサーがSSE接続をタイムアウトしていないか確認する

## ネットワーク

### gRPC接続エラー

症状: `connection refused` または `deadline exceeded`

確認ポイント:

- サーバーが起動しているか
- ポート（デフォルト: 7233）が正しいか
- ファイアウォールでgRPCポートが開放されているか
- CLIの `--server` フラグが正しいか（デフォルト: `localhost:7233`）

### HTTP APIが404を返す

確認ポイント:

- HTTPポート（デフォルト: 8080）を使用しているか（gRPCポート 7233 ではない）
- URLプレフィックスが `/v1/` であるか
- HTTPメソッドが正しいか（GET/POST）

## パフォーマンス

### ワークフロースループットが低い

調査手順:

1. `ENABLE_PPROF=true` でpprofを有効化する
2. CPUプロファイルを取得: `go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30`
3. PostgreSQLのスロークエリログを確認する
4. `/metrics` でレイテンシ分布を確認する

一般的な対策:

- PostgreSQLの `shared_buffers`, `work_mem` を調整する
- dandoriサーバーのインスタンス数を増やす（水平スケーリング）
- ワーカー数を増やしてタスク処理を並列化する
