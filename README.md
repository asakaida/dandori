# dandori

Go + PostgreSQLで実現する軽量 Durable Workflow エンジン。

## 特徴

- **Deterministic Replay**: ワークフロー関数を最初から再実行し、イベント履歴と照合して状態を復元
- **Event Sourcing**: 全状態変化をイベントとして記録。イベント履歴がSource of Truth
- **PostgreSQL**: 外部依存を最小限に。タスクキューは `SELECT FOR UPDATE SKIP LOCKED` で実現
- **gRPC API**: proto定義をAPI契約とし、将来的に他言語SDKにも対応可能
- **Hexagonal Architecture**: domain/ -> port/ -> adapter/ の依存方向でテスタビリティを確保

## 使い方: 旅行予約ワークフロー

航空券・宿泊・レンタカーを順番に予約し、結果をまとめて返すワークフローの例です。途中で失敗した場合は **Saga パターン** で予約済みのステップを **逆順にキャンセル（補償）** します。サーバーやワーカーが途中でクラッシュしても Deterministic Replay により中断地点から再開します。

### Activity の定義

実際の外部 API 呼び出しを行う関数です。dandori はこれらを自動でリトライ・タイムアウト管理します。予約用と、補償（キャンセル）用のペアを用意します。

```go
// activities.go
package travel

import "context"

// --- 予約用 Activity ---

type BookFlightInput struct {
	From string `json:"from"`
	To   string `json:"to"`
	Date string `json:"date"`
}

type BookFlightOutput struct {
	ConfirmationCode string `json:"confirmation_code"`
}

func BookFlight(ctx context.Context, input BookFlightInput) (BookFlightOutput, error) {
	code, err := callFlightAPI(input)
	if err != nil {
		return BookFlightOutput{}, err
	}
	return BookFlightOutput{ConfirmationCode: code}, nil
}

type BookHotelInput struct {
	City     string `json:"city"`
	CheckIn  string `json:"check_in"`
	CheckOut string `json:"check_out"`
}

type BookHotelOutput struct {
	ReservationID string `json:"reservation_id"`
}

func BookHotel(ctx context.Context, input BookHotelInput) (BookHotelOutput, error) {
	id, err := callHotelAPI(input)
	if err != nil {
		return BookHotelOutput{}, err
	}
	return BookHotelOutput{ReservationID: id}, nil
}

type BookCarInput struct {
	City     string `json:"city"`
	PickUp   string `json:"pick_up"`
	DropOff  string `json:"drop_off"`
}

type BookCarOutput struct {
	ReservationID string `json:"reservation_id"`
}

func BookCar(ctx context.Context, input BookCarInput) (BookCarOutput, error) {
	id, err := callCarAPI(input)
	if err != nil {
		return BookCarOutput{}, err
	}
	return BookCarOutput{ReservationID: id}, nil
}

// --- 補償（キャンセル）用 Activity ---

type CancelFlightInput struct {
	ConfirmationCode string `json:"confirmation_code"`
}

func CancelFlight(ctx context.Context, input CancelFlightInput) (struct{}, error) {
	return struct{}{}, cancelFlightAPI(input.ConfirmationCode)
}

type CancelHotelInput struct {
	ReservationID string `json:"reservation_id"`
}

func CancelHotel(ctx context.Context, input CancelHotelInput) (struct{}, error) {
	return struct{}{}, cancelHotelAPI(input.ReservationID)
}

type CancelCarInput struct {
	ReservationID string `json:"reservation_id"`
}

func CancelCar(ctx context.Context, input CancelCarInput) (struct{}, error) {
	return struct{}{}, cancelCarAPI(input.ReservationID)
}
```

### Workflow の定義

Activity をどの順序で実行するかを記述します。この関数は決定的（deterministic）でなければなりません。

**Saga パターンの仕組み:** `saga.New()` は補償スタックを作ります。各ステップの成功後にキャンセル用 Activity を `AddCompensation` でスタックに積みます。途中で失敗した場合、`Compensate` がスタックを逆順に実行して予約済みのステップをキャンセルします。

```go
// workflow.go
package travel

import (
	"time"

	"github.com/asakaida/dandori-sdk-go/saga"
	"github.com/asakaida/dandori-sdk-go/workflow"
)

type TripResult struct {
	FlightConfirmation string `json:"flight_confirmation"`
	HotelReservation   string `json:"hotel_reservation"`
	CarReservation     string `json:"car_reservation"`
}

func BookTripWorkflow(ctx workflow.Context, input BookTripInput) (TripResult, error) {
	actOpts := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &workflow.RetryPolicy{
			MaxAttempts:        3,
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, actOpts)

	// Saga: 補償スタックを作成（失敗時に逆順でキャンセルするための仕組み）
	s := saga.New(saga.Options{})

	// ステップ 1: 航空券を予約
	var flight BookFlightOutput
	err := workflow.ExecuteActivity(ctx, BookFlight, BookFlightInput{
		From: input.From,
		To:   input.To,
		Date: input.Date,
	}).Get(&flight)
	if err != nil {
		// まだ何も予約していないので補償不要
		return TripResult{}, err
	}
	// 成功 → 航空券キャンセルを補償スタックに積む
	//   スタック: [CancelFlight]
	s.AddCompensation(ctx, CancelFlight, CancelFlightInput{
		ConfirmationCode: flight.ConfirmationCode,
	})

	// ステップ 2: 宿泊を予約
	var hotel BookHotelOutput
	err = workflow.ExecuteActivity(ctx, BookHotel, BookHotelInput{
		City:     input.To,
		CheckIn:  input.Date,
		CheckOut: input.CheckOutDate,
	}).Get(&hotel)
	if err != nil {
		// 宿泊予約失敗 → 補償スタックを逆順実行: CancelFlight
		return TripResult{}, s.Compensate(ctx, err)
	}
	// 成功 → 宿泊キャンセルを補償スタックに積む
	//   スタック: [CancelFlight, CancelHotel]
	s.AddCompensation(ctx, CancelHotel, CancelHotelInput{
		ReservationID: hotel.ReservationID,
	})

	// ステップ 3: レンタカーを予約
	var car BookCarOutput
	err = workflow.ExecuteActivity(ctx, BookCar, BookCarInput{
		City:    input.To,
		PickUp:  input.Date,
		DropOff: input.CheckOutDate,
	}).Get(&car)
	if err != nil {
		// レンタカー予約失敗 → 補償スタックを逆順実行: CancelHotel → CancelFlight
		return TripResult{}, s.Compensate(ctx, err)
	}

	// 全予約成功
	return TripResult{
		FlightConfirmation: flight.ConfirmationCode,
		HotelReservation:   hotel.ReservationID,
		CarReservation:     car.ReservationID,
	}, nil
}
```

**ポイント:**
- Saga オブジェクト自体は予約処理を実行しない。**「失敗時に何を巻き戻すか」を管理する補償スタック**
- 各ステップの成功後に `AddCompensation` でキャンセル用 Activity をスタックに積む
- `Compensate` はスタックを**逆順**に実行する。ステップ 3 で失敗した場合、CancelHotel → CancelFlight の順にキャンセルが走る
- サーバーにとって補償 Activity（CancelHotel 等）は通常の Activity と同じ。Saga の逆順実行ロジックは SDK 側で完結する

### Worker の起動

```go
// worker/main.go
package main

import (
	"log"

	"github.com/asakaida/dandori-sdk-go/client"
	"github.com/asakaida/dandori-sdk-go/worker"
	"example.com/travel"
)

func main() {
	c, err := client.Dial("localhost:7233")
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	w := worker.New(c, "travel-queue")
	w.RegisterWorkflow(travel.BookTripWorkflow)
	w.RegisterActivity(travel.BookFlight)
	w.RegisterActivity(travel.BookHotel)
	w.RegisterActivity(travel.BookCar)
	w.RegisterActivity(travel.CancelFlight)
	w.RegisterActivity(travel.CancelHotel)
	w.RegisterActivity(travel.CancelCar)

	if err := w.Run(); err != nil {
		log.Fatal(err)
	}
}
```

### Client からワークフローを開始

```go
// client/main.go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/asakaida/dandori-sdk-go/client"
	"example.com/travel"
)

func main() {
	c, err := client.Dial("localhost:7233")
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	run, err := c.ExecuteWorkflow(context.Background(), client.StartWorkflowOptions{
		ID:        "trip-okinawa-2026-04",
		TaskQueue: "travel-queue",
	}, travel.BookTripWorkflow, travel.BookTripInput{
		From:         "NRT",
		To:           "OKA",
		Date:         "2026-04-01",
		CheckOutDate: "2026-04-05",
	})
	if err != nil {
		log.Fatal(err)
	}

	var result travel.TripResult
	if err := run.Get(context.Background(), &result); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Flight: %s\n", result.FlightConfirmation)
	fmt.Printf("Hotel:  %s\n", result.HotelReservation)
	fmt.Printf("Car:    %s\n", result.CarReservation)
}
```

### 処理の流れ（正常系）

```text
Client                     dandori-server                Worker
  |                             |                          |
  |-- StartWorkflow ----------->|                          |
  |                             |-- Workflow Task -------->|
  |                             |                          |-- BookTripWorkflow()
  |                             |                          |   "航空券を予約して"
  |                             |<-- ScheduleActivity ----|
  |                             |-- Activity Task -------->|
  |                             |                          |-- BookFlight() ✓
  |                             |<-- Complete (結果) ------|
  |                             |-- Workflow Task -------->|
  |                             |                          |-- replay + 続行
  |                             |                          |   "宿泊を予約して"
  |                             |<-- ScheduleActivity ----|
  |                             |-- Activity Task -------->|
  |                             |                          |-- BookHotel() ✓
  |                             |<-- Complete (結果) ------|
  |                             |-- Workflow Task -------->|
  |                             |                          |-- replay + 続行
  |                             |                          |   "レンタカーを予約して"
  |                             |<-- ScheduleActivity ----|
  |                             |-- Activity Task -------->|
  |                             |                          |-- BookCar() ✓
  |                             |<-- Complete (結果) ------|
  |                             |-- Workflow Task -------->|
  |                             |                          |-- replay + 完了
  |                             |<-- CompleteWorkflow -----|
  |<-- Result ------------------|                          |
```

### 処理の流れ（レンタカー予約失敗 → Saga 補償）

ステップ 3 のレンタカー予約が失敗した場合、Saga は補償スタックを逆順に実行して CancelHotel → CancelFlight の順にキャンセルします。

```text
Client                     dandori-server                Worker
  |                             |                          |
  |-- StartWorkflow ----------->|                          |
  |                             |-- Workflow Task -------->|
  |                             |                          |-- BookTripWorkflow()
  |                             |                          |   "航空券を予約して"
  |                             |<-- ScheduleActivity ----|
  |                             |-- Activity Task -------->|
  |                             |                          |-- BookFlight() ✓
  |                             |<-- Complete (結果) ------|
  |                             |-- Workflow Task -------->|
  |                             |                          |-- replay + 続行
  |                             |                          |   "宿泊を予約して"
  |                             |<-- ScheduleActivity ----|
  |                             |-- Activity Task -------->|
  |                             |                          |-- BookHotel() ✓
  |                             |<-- Complete (結果) ------|
  |                             |-- Workflow Task -------->|
  |                             |                          |-- replay + 続行
  |                             |                          |   "レンタカーを予約して"
  |                             |<-- ScheduleActivity ----|
  |                             |-- Activity Task -------->|
  |                             |                          |-- BookCar() ✗
  |                             |<-- Failed ---------------|
  |                             |-- Workflow Task -------->|
  |                             |                          |-- replay + Compensate()
  |                             |                          |   補償スタック逆順実行:
  |                             |                          |   1. "宿泊をキャンセルして"
  |                             |<-- ScheduleActivity ----|
  |                             |-- Activity Task -------->|
  |                             |                          |-- CancelHotel() ✓
  |                             |<-- Complete -------------|
  |                             |-- Workflow Task -------->|
  |                             |                          |   2. "航空券をキャンセルして"
  |                             |<-- ScheduleActivity ----|
  |                             |-- Activity Task -------->|
  |                             |                          |-- CancelFlight() ✓
  |                             |<-- Complete -------------|
  |                             |-- Workflow Task -------->|
  |                             |                          |-- replay + FailWorkflow
  |                             |<-- FailWorkflow ---------|
  |<-- Error ------------------|                          |
```

サーバーにとって補償 Activity（CancelHotel, CancelFlight）は通常の Activity と全く同じです。Saga の逆順実行ロジックは SDK 側で完結します。

### 耐障害性

もしワーカーが Activity 実行中にクラッシュしても、dandori-server がタイムアウトを検知してタスクを再キューイングします。ワークフロー関数はイベント履歴から状態を復元（replay）するため、完了済みのステップをスキップして中断地点から再開できます。

- **正常処理中のクラッシュ**: 例えば BookHotel 実行中にクラッシュ → replay で BookFlight の結果を復元し、BookHotel から再開
- **補償処理中のクラッシュ**: 例えば CancelFlight 実行中にクラッシュ → replay で CancelHotel の完了を確認し、CancelFlight から再開

## アーキテクチャ

```text
Client (Go SDK / CLI)
    |
    | gRPC
    v
dandori-server (本リポジトリ)
    |
    | PostgreSQL
    v
PostgreSQL
    |
    v
dandori-worker (Go SDK側で実装, 1..N instances)
```

サーバーはワークフローのステップ定義を理解しない。イベントの永続化、コマンド処理、タスクキュー管理に徹し、「次に何をすべきか」の判断はワーカーに委ねる。

## プロジェクト構成

```text
dandori/
├── api/v1/               # proto定義 + 生成コード
├── cmd/dandori/           # サーバーバイナリ
├── internal/
│   ├── domain/            # ドメインモデル（型定義、エラー定義）
│   ├── port/              # ポート（インターフェース定義）
│   ├── adapter/
│   │   ├── grpc/          # Inbound Adapter: gRPCハンドラ
│   │   └── postgres/      # Outbound Adapter: PostgreSQL実装
│   │       └── migration/ # SQLマイグレーション
│   └── engine/            # ビジネスロジック、コマンドプロセッサ
├── test/e2e/              # E2Eテスト
└── docs/                  # 設計書、スプリント管理
```

## 必要要件

- Go 1.24+
- Docker / Docker Compose（PostgreSQL用）
- protoc + protoc-gen-go + protoc-gen-go-grpc（proto再生成時のみ）

## セットアップ

### PostgreSQL起動

```bash
docker compose up -d
```

デフォルト接続情報: `postgres://dandori:dandori@localhost:5432/dandori?sslmode=disable`

### ビルド

```bash
go build ./cmd/dandori
```

### サーバー起動

```bash
./dandori
```

環境変数:

| 変数 | デフォルト | 説明 |
|------|-----------|------|
| `DATABASE_URL` | `postgres://dandori:dandori@localhost:5432/dandori?sslmode=disable` | PostgreSQL接続文字列 |
| `GRPC_PORT` | `7233` | gRPCリッスンポート |

マイグレーションはサーバー起動時に自動実行される。

## gRPC API

| カテゴリ | RPC | 説明 |
|---------|-----|------|
| Client | `StartWorkflow` | ワークフロー開始（冪等） |
| Client | `DescribeWorkflow` | ワークフロー状態取得 |
| Client | `GetWorkflowHistory` | イベント履歴取得 |
| Client | `TerminateWorkflow` | ワークフロー強制終了 |
| Worker | `PollWorkflowTask` | Workflow Taskの取得 |
| Worker | `CompleteWorkflowTask` | Workflow Task完了（コマンド返却） |
| Worker | `FailWorkflowTask` | Workflow Task失敗報告 |
| Worker | `PollActivityTask` | Activity Taskの取得 |
| Worker | `CompleteActivityTask` | Activity Task完了（結果返却） |
| Worker | `FailActivityTask` | Activity Task失敗報告 |

gRPC reflectionが有効なため、grpcurlで直接操作可能:

```bash
grpcurl -plaintext localhost:7233 list
grpcurl -plaintext -d '{"workflow_type": "MyWorkflow", "task_queue": "default"}' \
  localhost:7233 dandori.api.v1.DandoriService/StartWorkflow
```

## テスト

```bash
# 全テスト実行（PostgreSQL testcontainersが自動起動）
go test -v -race ./...

# 個別テスト
go test -v -race ./internal/engine/...         # Engine ユニットテスト
go test -v -race ./internal/adapter/postgres/... # PostgreSQL インテグレーションテスト
go test -v -race ./internal/adapter/grpc/...     # gRPC ハンドラテスト
go test -v -race ./test/e2e/...                  # E2Eテスト
```

テストにはDockerが必要（testcontainers-goがPostgreSQLコンテナを自動起動）。

## 関連リポジトリ

| リポジトリ | 役割 |
|-----------|------|
| dandori（本リポジトリ） | サーバー、proto定義（API契約のSource of Truth） |
| dandori-sdk-go | Go SDK（クライアント、ワーカー、deterministic replay） |

## ドキュメント

- [設計書](docs/design.md) - アーキテクチャ、データモデル、API仕様の詳細
- [プロダクト要求仕様書](docs/prd.md) - コアコンセプト、機能要件、フェーズ計画
- [スプリント管理](docs/sprints.md) - Sprint 1-21の詳細タスクと進捗

## 開発状況

- **Phase 1 (MVP)**: 完了 - Sprint 1-5 + E2Eテスト（104テスト通過）
- **Phase 2 (信頼性と機能拡張)**: Sprint 6-11（Timer, Signal, Cancel, Heartbeat, LISTEN/NOTIFY, CLI）
- **Phase 3 (高度な機能)**: Sprint 12-17（Saga, Child Workflow, SideEffect, Cron, HTTP API, Observability）
- **Phase 4 (運用性と最適化)**: Sprint 18-21（Namespace, Web UI, パフォーマンス, ドキュメント）

## ライセンス

TBD
