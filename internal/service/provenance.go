package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// Source says how a record came into LifeOS.
type Source string

const (
	SourceManual    Source = "manual"    // Gautam, through a form, a Shortcut or a CLI command
	SourceImport    Source = "import"    // a CSV import
	SourceConnector Source = "connector" // a connector that pulls or receives data
	SourceAgent     Source = "agent"     // an LLM agent, through an MCP tool
)

// Provenance is recorded on every row that the service writes.
type Provenance struct {
	Source Source
	// SourceRef is the ID of the sources row the record came from, if any.
	SourceRef *int64
	// Verified is false for a row that an agent inferred. It stays false
	// until Gautam approves the row. A row that Gautam dictated is verified.
	Verified bool
}

func (p Provenance) validate() error {
	switch p.Source {
	case SourceManual, SourceImport, SourceConnector, SourceAgent:
		return nil
	}
	return invalid("source", "must be one of manual, import, connector, agent")
}

// checkSourceRef refuses a SourceRef that names no live source. Call it in
// the transaction of the write. The foreign key refuses an unknown ID too,
// but its error does not name the field, and it accepts a deleted source.
func (p Provenance) checkSourceRef(ctx context.Context, q *store.Queries) error {
	if p.SourceRef == nil {
		return nil
	}
	_, err := q.GetSource(ctx, *p.SourceRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return invalid("source_ref", "must be the ID of a source that exists")
	}
	if err != nil {
		return fmt.Errorf("check source_ref: %w", err)
	}
	return nil
}
