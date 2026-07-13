package store

import (
	"database/sql"
	"fmt"
)

// BrokerBacked exposes the local sqlite broker used for keyring and issuer metadata.
type BrokerBacked interface {
	BrokerDB() *sql.DB
}

// BrokerDB returns the underlying sqlite handle for keyring/admin routes.
func BrokerDB(s Store) (*sql.DB, error) {
	if x, ok := s.(*SQLite); ok {
		return x.DB(), nil
	}
	if b, ok := s.(BrokerBacked); ok {
		if db := b.BrokerDB(); db != nil {
			return db, nil
		}
	}
	return nil, fmt.Errorf("store has no broker database")
}