package repo

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

func (r *Repo) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}
