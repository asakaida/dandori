package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/asakaida/dandori/internal/domain"
)

type NamespaceStore struct {
	store *Store
}

func (s *NamespaceStore) GetByName(ctx context.Context, name string) (*domain.Namespace, error) {
	var ns domain.Namespace
	err := s.store.conn(ctx).QueryRowContext(ctx,
		`SELECT name, description, created_at FROM namespaces WHERE name = $1`, name,
	).Scan(&ns.Name, &ns.Description, &ns.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNamespaceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &ns, nil
}

func (s *NamespaceStore) Create(ctx context.Context, ns domain.Namespace) error {
	_, err := s.store.conn(ctx).ExecContext(ctx,
		`INSERT INTO namespaces (name, description) VALUES ($1, $2)`,
		ns.Name, ns.Description,
	)
	return err
}

func (s *NamespaceStore) List(ctx context.Context) ([]domain.Namespace, error) {
	rows, err := s.store.conn(ctx).QueryContext(ctx,
		`SELECT name, description, created_at FROM namespaces ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var namespaces []domain.Namespace
	for rows.Next() {
		var ns domain.Namespace
		if err := rows.Scan(&ns.Name, &ns.Description, &ns.CreatedAt); err != nil {
			return nil, err
		}
		namespaces = append(namespaces, ns)
	}
	return namespaces, rows.Err()
}
