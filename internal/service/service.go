// Package service holds all LifeOS logic: validation, provenance and events.
// API handlers, MCP tools and CLI subcommands are thin adapters that call it.
package service

import (
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// IST is the zone Gautam lives in. Times are stored in UTC; "today" for a
// date-only field is the IST date. India has no daylight saving time.
var IST = time.FixedZone("IST", 5*60*60+30*60)

// Service is the single write path to the database.
type Service struct {
	pool  *pgxpool.Pool
	now   func() time.Time
	files Files
	wiki  Wiki
	// wikiMu makes the writes of the wiki go one after the other.
	wikiMu sync.Mutex
	// queryPool connects as the read-only user. Only Query uses it.
	queryPool    *pgxpool.Pool
	queryTimeout time.Duration
}

// New returns a Service that uses pool.
func New(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, now: time.Now, queryTimeout: defaultQueryTimeout}
}
