package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const testCard = "4111 1111 1111 1111"

// wantSecretRefused checks that err is a ValidationError for field that
// names the kind of secret and does not repeat the secret.
func wantSecretRefused(t *testing.T, name string, err error, field, kind, secret string) {
	t.Helper()
	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Errorf("%s: error = %v, want a ValidationError", name, err)
		return
	}
	if invalid.Field != field {
		t.Errorf("%s: field = %q, want %q", name, invalid.Field, field)
	}
	if !strings.Contains(invalid.Message, "must not hold "+kind) {
		t.Errorf("%s: message = %q, want it to name %s", name, invalid.Message, kind)
	}
	if strings.Contains(invalid.Error(), secret) {
		t.Errorf("%s: the error repeats the secret: %q", name, invalid.Error())
	}
}

func TestAddItemRefusesASecret(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()

	cases := []struct {
		name        string
		change      func(*AddItemParams)
		field, kind string
		secret      string
	}{
		{"a card number in the title", func(p *AddItemParams) { p.Title = "card " + testCard }, "title", "a card number", testCard},
		{"a token in an attr", func(p *AddItemParams) { p.Attrs = map[string]any{"note": awsKey} }, "attrs", "an access key or a token", awsKey},
		{"a card number as a number in an attr", func(p *AddItemParams) { p.Attrs = map[string]any{"card": float64(4111111111111111)} }, "attrs", "a card number", "4111111111111111"},
		{"a PAN deep in the attrs", func(p *AddItemParams) {
			p.Attrs = map[string]any{"people": []any{map[string]any{"pan": "ABCPE1234F"}}}
		}, "attrs", "a PAN", "ABCPE1234F"},
		{"an Aadhaar number in the name of an attr", func(p *AddItemParams) { p.Attrs = map[string]any{"2345 6789 0124": true} }, "attrs", "an Aadhaar number", "2345 6789 0124"},
	}
	for _, c := range cases {
		p := dune()
		c.change(&p)
		_, err := s.AddItem(ctx, p)
		wantSecretRefused(t, c.name, err, c.field, c.kind, c.secret)
	}
	if n := countRows(t, pool, "catalog_items") + countRows(t, pool, "events"); n != 0 {
		t.Errorf("rows in catalog_items and events = %d, want 0", n)
	}
}

func TestAddItemRefusesTextThatPostgresCannotStore(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()

	for name, change := range map[string]func(*AddItemParams){
		"a NUL in the title":        func(p *AddItemParams) { p.Title = "Dune\x00" },
		"a NUL in an attr":          func(p *AddItemParams) { p.Attrs = map[string]any{"director": "Denis\x00"} },
		"a title that is not UTF-8": func(p *AddItemParams) { p.Title = "Dune\xff" },
	} {
		p := dune()
		change(&p)
		_, err := s.AddItem(ctx, p)
		var invalid *ValidationError
		if !errors.As(err, &invalid) {
			t.Errorf("%s: error = %v, want a ValidationError", name, err)
		}
	}
	if n := countRows(t, pool, "catalog_items"); n != 0 {
		t.Errorf("rows in catalog_items = %d, want 0", n)
	}
}

func TestUpdateItemRefusesASecret(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	item := addItem(t, s, "movie", "Dune")

	_, err := s.UpdateItem(ctx, item.ID, UpdateItemParams{Title: ptr("Dune " + testCard), Actor: SourceManual})
	wantSecretRefused(t, "title", err, "title", "a card number", testCard)
	_, err = s.UpdateItem(ctx, item.ID, UpdateItemParams{Attrs: map[string]any{"key": githubToken}, Actor: SourceManual})
	wantSecretRefused(t, "attrs", err, "attrs", "an access key or a token", githubToken)

	got, err := s.GetItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if got.Title != "Dune" || len(got.Attrs) != 0 {
		t.Errorf("item = %+v, want it as it was", got)
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d, want only the create event", n)
	}
}

func TestJournalRefusesASecret(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()

	_, err := s.AddJournalEntry(ctx, AddJournalEntryParams{
		Body:       "new card today:\n" + testCard + "\nexpires in 2030",
		Provenance: Provenance{Source: SourceManual},
	})
	wantSecretRefused(t, "add", err, "body", "a card number", testCard)
	if n := countRows(t, pool, "journal_entries"); n != 0 {
		t.Errorf("journal entries = %d, want 0", n)
	}

	entry := addJournal(t, s, "a quiet day", "")
	_, err = s.UpdateJournalEntry(ctx, entry.ID, UpdateJournalEntryParams{Body: ptr("the key is " + privateKey), Actor: SourceManual})
	wantSecretRefused(t, "update", err, "body", "a private key", "MIIEow")
	if got, _ := s.GetJournalEntry(ctx, entry.ID); got.Body != "a quiet day" {
		t.Errorf("body = %q, want it as it was", got.Body)
	}
}

func TestTodoRefusesASecret(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()

	p := buyDomain()
	p.Title = "pay with " + testCard
	_, err := s.AddTodo(ctx, p)
	wantSecretRefused(t, "title", err, "title", "a card number", testCard)

	p = buyDomain()
	p.Project = slackToken
	_, err = s.AddTodo(ctx, p)
	wantSecretRefused(t, "project", err, "project", "an access key or a token", slackToken)
	if n := countRows(t, pool, "todos"); n != 0 {
		t.Errorf("todos = %d, want 0", n)
	}

	todo := addTodo(t, s, "Buy a domain")
	_, err = s.UpdateTodo(ctx, todo.ID, UpdateTodoParams{Title: ptr("Aadhaar 2345 6789 0124"), Actor: SourceManual})
	wantSecretRefused(t, "update", err, "title", "an Aadhaar number", "2345 6789 0124")
}

func TestHabitNameRefusesASecret(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()

	_, err := s.AddHabit(ctx, AddHabitParams{Name: testCard, Provenance: Provenance{Source: SourceManual}})
	wantSecretRefused(t, "add", err, "name", "a card number", testCard)
	if n := countRows(t, pool, "habits"); n != 0 {
		t.Errorf("habits = %d, want 0", n)
	}

	habit := addHabit(t, s, "gym")
	_, err = s.UpdateHabit(ctx, habit.ID, UpdateHabitParams{Name: ptr(testCard), Actor: SourceManual})
	wantSecretRefused(t, "rename", err, "name", "a card number", testCard)
}

func TestTextWithoutASecretIsStored(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	// A phone number, a date, an amount and an ISBN are not secrets.
	body := "called +91 98765 43210 on 2026-10-11, paid 12,50,000, ISBN 9780441172719"
	entry, err := s.AddJournalEntry(ctx, AddJournalEntryParams{Body: body, Provenance: Provenance{Source: SourceManual}})
	if err != nil {
		t.Fatalf("AddJournalEntry: %v", err)
	}
	if entry.Body != body {
		t.Errorf("body = %q, want it as written", entry.Body)
	}
}
