package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// recordEvent appends one row to the events log. Call it in the same
// transaction as the write it describes. oldValue and newValue are encoded
// as JSON; pass nil when there is no value.
func recordEvent(ctx context.Context, q *store.Queries, action, recordType string, recordID int64, source Source, oldValue, newValue any) error {
	oldJSON, err := marshalOrNil(oldValue)
	if err != nil {
		return fmt.Errorf("event old value: %w", err)
	}
	newJSON, err := marshalOrNil(newValue)
	if err != nil {
		return fmt.Errorf("event new value: %w", err)
	}
	return q.InsertEvent(ctx, store.InsertEventParams{
		Action:     action,
		RecordType: recordType,
		RecordID:   &recordID,
		Source:     string(source),
		OldValue:   oldJSON,
		NewValue:   newJSON,
	})
}

func marshalOrNil(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}
