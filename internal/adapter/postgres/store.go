package postgres

import (
	"context"
	"database/sql"

	"github.com/asakaida/dandori/internal/port"
)

type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

type txKey struct{}

func withTx(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

func txFromContext(ctx context.Context) *sql.Tx {
	tx, _ := ctx.Value(txKey{}).(*sql.Tx)
	return tx
}

func (s *Store) conn(ctx context.Context) DBTX {
	if tx := txFromContext(ctx); tx != nil {
		return tx
	}
	return s.db
}

func (s *Store) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if txFromContext(ctx) != nil {
		return fn(ctx)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := fn(withTx(ctx, tx)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Workflows() port.WorkflowRepository     { return &WorkflowStore{store: s} }
func (s *Store) Events() port.EventRepository           { return &EventStore{store: s} }
func (s *Store) WorkflowTasks() port.WorkflowTaskRepository { return &WorkflowTaskStore{store: s} }
func (s *Store) ActivityTasks() port.ActivityTaskRepository  { return &ActivityTaskStore{store: s} }
func (s *Store) Timers() port.TimerRepository            { return &TimerStore{store: s} }
