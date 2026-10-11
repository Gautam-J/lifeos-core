package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
)

// RecordTypes are the record types that Get, Update and Delete accept. A
// caller that knows the type when it is compiled, such as a REST route,
// calls the function of the type (GetItem, UpdateItem, DeleteItem). An MCP
// tool gets the type as input and calls the functions in this file.
var RecordTypes = []string{"item"}

func checkRecordType(recordType string) error {
	if !slices.Contains(RecordTypes, recordType) {
		return invalid("type", "must be one of: "+strings.Join(RecordTypes, ", "))
	}
	return nil
}

// Get returns one record. The value is the struct of the type: an Item for
// the type item.
func (s *Service) Get(ctx context.Context, recordType string, id int64) (any, error) {
	if err := checkRecordType(recordType); err != nil {
		return nil, err
	}
	item, err := s.GetItem(ctx, id)
	if err != nil {
		return nil, err
	}
	return item, nil
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
	// state.
	Inferred bool
}

// Update changes one record and returns it. It refuses a field that the
// type does not let a caller change, such as verified or source.
func (s *Service) Update(ctx context.Context, recordType string, id int64, p UpdateParams) (any, error) {
	if err := checkRecordType(recordType); err != nil {
		return nil, err
	}
	var params UpdateItemParams
	if err := decodeFields(p.Fields, &params, recordType); err != nil {
		return nil, err
	}
	params.Clear, params.Actor, params.Inferred = p.Clear, p.Actor, p.Inferred
	item, err := s.UpdateItem(ctx, id, params)
	if err != nil {
		return nil, err
	}
	return item, nil
}

// Delete marks one record as deleted.
func (s *Service) Delete(ctx context.Context, recordType string, id int64, p DeleteParams) error {
	if err := checkRecordType(recordType); err != nil {
		return err
	}
	return s.DeleteItem(ctx, id, p)
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
