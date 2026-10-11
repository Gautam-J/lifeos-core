package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// RecordTypes are the record types that Get, Update and Delete accept. A
// caller that knows the type when it is compiled, such as a REST route,
// calls the function of the type (GetItem, UpdateItem, DeleteItem). An MCP
// tool gets the type as input and calls the functions in this file.
var RecordTypes = []string{"item", "journal", "todo", "habit_log", "source"}

func checkRecordType(recordType string) error {
	if !slices.Contains(RecordTypes, recordType) {
		return invalid("type", "must be one of: "+strings.Join(RecordTypes, ", "))
	}
	return nil
}

// errNoCode means that a type is in RecordTypes and one of the functions
// that take a type has no case for it.
func errNoCode(recordType string) error {
	return fmt.Errorf("no code for the record type %q", recordType)
}

// asRecord returns nil, not an empty struct, when there is an error.
func asRecord[T any](record T, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return record, nil
}

// Get returns one record. The value is the struct of the type: an Item for
// the type item, a JournalEntry for the type journal, a Todo for the type
// todo, a HabitLog for the type habit_log, a Capture for the type source.
func (s *Service) Get(ctx context.Context, recordType string, id int64) (any, error) {
	if err := checkRecordType(recordType); err != nil {
		return nil, err
	}
	switch recordType {
	case "item":
		return asRecord(s.GetItem(ctx, id))
	case "journal":
		return asRecord(s.GetJournalEntry(ctx, id))
	case "todo":
		return asRecord(s.GetTodo(ctx, id))
	case "habit_log":
		return asRecord(s.GetHabitLog(ctx, id))
	case "source":
		return asRecord(s.GetCapture(ctx, id))
	}
	return nil, errNoCode(recordType)
}

// UpdateParams is the input of Update.
type UpdateParams struct {
	// Fields holds the fields to change, by their JSON names, with the
	// new values. A field that is not in Fields keeps its value.
	Fields map[string]any
	// Clear names the optional fields to empty.
	Clear []string
	// Actor is who makes the change.
	Actor Source
	// Inferred is true when an agent makes a change that Gautam did not
	// state. Update refuses such a change to a verified record.
	Inferred bool
}

// Update changes one record and returns it. It refuses a field that the
// type does not let a caller change, such as verified or source.
func (s *Service) Update(ctx context.Context, recordType string, id int64, p UpdateParams) (any, error) {
	if err := checkRecordType(recordType); err != nil {
		return nil, err
	}
	switch recordType {
	case "item":
		var params UpdateItemParams
		if err := decodeFields(p.Fields, &params, recordType); err != nil {
			return nil, err
		}
		params.Clear, params.Actor, params.Inferred = p.Clear, p.Actor, p.Inferred
		return asRecord(s.UpdateItem(ctx, id, params))
	case "journal":
		var params UpdateJournalEntryParams
		if err := decodeFields(p.Fields, &params, recordType); err != nil {
			return nil, err
		}
		if len(p.Clear) > 0 {
			return nil, invalid("clear", "must be empty: a journal entry has no optional field")
		}
		params.Actor, params.Inferred = p.Actor, p.Inferred
		return asRecord(s.UpdateJournalEntry(ctx, id, params))
	case "todo":
		var params UpdateTodoParams
		if err := decodeFields(p.Fields, &params, recordType); err != nil {
			return nil, err
		}
		params.Clear, params.Actor, params.Inferred = p.Clear, p.Actor, p.Inferred
		return asRecord(s.UpdateTodo(ctx, id, params))
	case "habit_log":
		// The habit and the day are the log. A correction is a new log.
		return nil, invalid("type", "a habit_log has no field that you can change: delete the log, then log the habit for the correct day")
	case "source":
		// A source holds what came in. A correction is a new capture.
		return nil, invalid("type", "a source has no field that you can change: delete the source, then capture the correct text, link or file")
	}
	return nil, errNoCode(recordType)
}

// Delete marks one record as deleted.
func (s *Service) Delete(ctx context.Context, recordType string, id int64, p DeleteParams) error {
	if err := checkRecordType(recordType); err != nil {
		return err
	}
	switch recordType {
	case "item":
		return s.DeleteItem(ctx, id, p)
	case "journal":
		return s.DeleteJournalEntry(ctx, id, p)
	case "todo":
		return s.DeleteTodo(ctx, id, p)
	case "habit_log":
		return s.DeleteHabitLog(ctx, id, p)
	case "source":
		return s.DeleteCapture(ctx, id, p)
	}
	return errNoCode(recordType)
}

// decodeFields copies fields into target. target is a pointer to a struct
// that has a json tag for each field that a caller can change. A name that
// target does not have, a value of the wrong type, or a null gives a
// ValidationError that names the field. A null is refused because a caller
// can mean "empty this field" by it; that is what Clear is for.
func decodeFields(fields map[string]any, target any, recordType string) error {
	for name, value := range fields {
		if value == nil {
			return invalid(name, "must not be null; to empty an optional field, put its name in clear")
		}
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return invalid("fields", "must be a JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(target)
	if err == nil {
		return nil
	}
	var wrongType *json.UnmarshalTypeError
	if errors.As(err, &wrongType) {
		return invalid(wrongType.Field, "must be "+jsonTypeName(wrongType.Type))
	}
	if name, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
		return invalid(strings.Trim(name, `"`), "is not a field of "+recordType+" that you can change")
	}
	return invalid("fields", "must be a JSON object")
}

func jsonTypeName(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "text"
	case reflect.Float64, reflect.Int64:
		return "a number"
	case reflect.Bool:
		return "true or false"
	case reflect.Map:
		return "an object"
	}
	return "a different type"
}
