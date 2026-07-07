package repository

import "context"

// Store is the boundary used by services for database operations.
// sqlc-generated query implementations will satisfy narrower repositories as they are added.
type Store interface {
	Ping(ctx context.Context) error
}
