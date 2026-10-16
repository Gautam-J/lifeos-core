package service

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// recordType holds everything that the service knows about one record
// type. A new record type is one new entry in recordTypes (rule 9 of
// AGENTS.md). Get, Update, Delete, Purge, ListDeleted, Search and the
// check of a wiki citation look the type up here.
type recordType struct {
	// name is the type that a caller gives, such as habit_log.
	name string
	// table is the table of the type, which the events name.
	table string
	// word is the name of one record for a person, such as "habit log".
	word string
	// prefixes are the prefixes of a wiki citation that name this type.
	prefixes []string
	get      func(s *Service, ctx context.Context, id int64) (any, error)
	update   func(s *Service, ctx context.Context, id int64, p UpdateParams) (any, error)
	delete   func(s *Service, ctx context.Context, id int64, p DeleteParams) error
	// purge removes the row of a deleted record. It returns the path of
	// the file of the record, when the record has one.
	purge func(ctx context.Context, q *store.Queries, id int64) (file *string, err error)
	// listDeleted returns the last deleted records of the type.
	listDeleted func(ctx context.Context, q *store.Queries, limit int32) ([]DeletedRecord, error)
	// live reports whether a record that a citation with this prefix
	// names is there and not deleted.
	live func(ctx context.Context, q *store.Queries, prefix string, id int64) (bool, error)
	// link gives the column of record_entities that a record of the type
	// uses. It is nil for a type that no link names (an entity).
	link func(id int64) recordLink
	// purgeRefused is the error text when the database refuses a Purge
	// because a row still names the record. %s is the table of that row.
	purgeRefused string
	// search returns the newest hits of the type. It is nil for a type that
	// has no text.
	search func(ctx context.Context, q *store.Queries, patterns []string, maxRows int32) ([]SearchHit, error)
}

// recordTypes is set in init: an entry refers to functions that call
// lookupRecordType, and a package variable cannot refer to itself.
var recordTypes []recordType

func init() {
	recordTypes = []recordType{
		{
			name: "item", table: "catalog_items", word: "item", prefixes: ItemKinds,
			get: func(s *Service, ctx context.Context, id int64) (any, error) { return asRecord(s.GetItem(ctx, id)) },
			update: func(s *Service, ctx context.Context, id int64, p UpdateParams) (any, error) {
				var params UpdateItemParams
				if err := decodeFields(p.Fields, &params, "item"); err != nil {
					return nil, err
				}
				params.Clear, params.Actor = p.Clear, p.Actor
				return asRecord(s.UpdateItem(ctx, id, params))
			},
			link:   func(id int64) recordLink { return recordLink{item: &id} },
			delete: (*Service).DeleteItem,
			purge: func(ctx context.Context, q *store.Queries, id int64) (*string, error) {
				_, err := q.PurgeCatalogItem(ctx, id)
				return nil, err
			},
			listDeleted: deletedList("item", (*store.Queries).ListDeletedCatalogItems, func(_ context.Context, _ *store.Queries, row store.CatalogItem) (int64, string, *time.Time, error) {
				item, err := itemFromRow(row)
				return row.ID, item.summary(), row.DeletedAt, err
			}),
			live: func(ctx context.Context, q *store.Queries, kind string, id int64) (bool, error) {
				return q.LiveItemOfKindExists(ctx, store.LiveItemOfKindExistsParams{ID: id, Kind: kind})
			},
			search: func(ctx context.Context, q *store.Queries, patterns []string, maxRows int32) ([]SearchHit, error) {
				rows, err := q.SearchCatalogItems(ctx, store.SearchCatalogItemsParams{Patterns: patterns, MaxRows: maxRows})
				if err != nil {
					return nil, err
				}
				var hits []SearchHit
				for _, row := range rows {
					item, err := itemFromRow(row)
					if err != nil {
						return nil, err
					}
					hits = append(hits, SearchHit{"item", item.ID, item.summary(), item.CreatedAt})
				}
				return hits, nil
			},
		},
		{
			name: "journal", table: "journal_entries", word: "journal entry", prefixes: []string{"journal"},
			get: func(s *Service, ctx context.Context, id int64) (any, error) {
				return asRecord(s.GetJournalEntry(ctx, id))
			},
			update: func(s *Service, ctx context.Context, id int64, p UpdateParams) (any, error) {
				var params UpdateJournalEntryParams
				if err := decodeFields(p.Fields, &params, "journal"); err != nil {
					return nil, err
				}
				if len(p.Clear) > 0 {
					return nil, invalid("clear", "must be empty: a journal entry has no optional field")
				}
				params.Actor = p.Actor
				return asRecord(s.UpdateJournalEntry(ctx, id, params))
			},
			link:         func(id int64) recordLink { return recordLink{journal: &id} },
			delete:       (*Service).DeleteJournalEntry,
			purgeRefused: "a record in %s names this journal entry in its journal_ref: delete that record forever first",
			purge: func(ctx context.Context, q *store.Queries, id int64) (*string, error) {
				_, err := q.PurgeJournalEntry(ctx, id)
				return nil, err
			},
			listDeleted: deletedList("journal", (*store.Queries).ListDeletedJournalEntries, func(_ context.Context, _ *store.Queries, row store.JournalEntry) (int64, string, *time.Time, error) {
				return row.ID, journalEntryFromRow(row).summary(), row.DeletedAt, nil
			}),
			live: func(ctx context.Context, q *store.Queries, _ string, id int64) (bool, error) {
				return q.LiveJournalEntryExists(ctx, id)
			},
			search: func(ctx context.Context, q *store.Queries, patterns []string, maxRows int32) ([]SearchHit, error) {
				rows, err := q.SearchJournalEntries(ctx, store.SearchJournalEntriesParams{Patterns: patterns, MaxRows: maxRows})
				if err != nil {
					return nil, err
				}
				var hits []SearchHit
				for _, row := range rows {
					entry := journalEntryFromRow(row)
					hits = append(hits, SearchHit{"journal", entry.ID, entry.summary(), entry.CreatedAt})
				}
				return hits, nil
			},
		},
		{
			name: "todo", table: "todos", word: "todo", prefixes: []string{"todo"},
			get: func(s *Service, ctx context.Context, id int64) (any, error) { return asRecord(s.GetTodo(ctx, id)) },
			update: func(s *Service, ctx context.Context, id int64, p UpdateParams) (any, error) {
				var params UpdateTodoParams
				if err := decodeFields(p.Fields, &params, "todo"); err != nil {
					return nil, err
				}
				params.Clear, params.Actor = p.Clear, p.Actor
				return asRecord(s.UpdateTodo(ctx, id, params))
			},
			link:   func(id int64) recordLink { return recordLink{todo: &id} },
			delete: (*Service).DeleteTodo,
			purge: func(ctx context.Context, q *store.Queries, id int64) (*string, error) {
				_, err := q.PurgeTodo(ctx, id)
				return nil, err
			},
			listDeleted: deletedList("todo", (*store.Queries).ListDeletedTodos, func(ctx context.Context, q *store.Queries, row store.Todo) (int64, string, *time.Time, error) {
				todo, err := todoFromStore(ctx, q, row)
				return row.ID, todo.summary(), row.DeletedAt, err
			}),
			live: func(ctx context.Context, q *store.Queries, _ string, id int64) (bool, error) {
				return q.LiveTodoExists(ctx, id)
			},
			search: func(ctx context.Context, q *store.Queries, patterns []string, maxRows int32) ([]SearchHit, error) {
				rows, err := q.SearchTodos(ctx, store.SearchTodosParams{Patterns: patterns, MaxRows: maxRows})
				if err != nil {
					return nil, err
				}
				todos, err := todosFromStore(ctx, q, rows)
				if err != nil {
					return nil, err
				}
				var hits []SearchHit
				for _, todo := range todos {
					hits = append(hits, SearchHit{"todo", todo.ID, todo.summary(), todo.CreatedAt})
				}
				return hits, nil
			},
		},
		{
			name: "habit_log", table: "habit_logs", word: "habit log", prefixes: []string{"habitlog"},
			get: func(s *Service, ctx context.Context, id int64) (any, error) { return asRecord(s.GetHabitLog(ctx, id)) },
			update: func(*Service, context.Context, int64, UpdateParams) (any, error) {
				// The habit and the day are the log. A correction is a new log.
				return nil, invalid("type", "a habit_log has no field that you can change: delete the log, then log the habit for the correct day")
			},
			link:   func(id int64) recordLink { return recordLink{habitLog: &id} },
			delete: (*Service).DeleteHabitLog,
			purge: func(ctx context.Context, q *store.Queries, id int64) (*string, error) {
				_, err := q.PurgeHabitLog(ctx, id)
				return nil, err
			},
			listDeleted: deletedList("habit_log", (*store.Queries).ListDeletedHabitLogs, func(_ context.Context, _ *store.Queries, row store.ListDeletedHabitLogsRow) (int64, string, *time.Time, error) {
				log := habitLogFromRow(row.HabitLog, row.Habit)
				return log.ID, log.summary(), row.HabitLog.DeletedAt, nil
			}),
			live: func(ctx context.Context, q *store.Queries, _ string, id int64) (bool, error) {
				return q.LiveHabitLogExists(ctx, id)
			},
		},
		{
			name: "source", table: "sources", word: "source", prefixes: []string{"src"},
			get: func(s *Service, ctx context.Context, id int64) (any, error) { return asRecord(s.GetCapture(ctx, id)) },
			update: func(*Service, context.Context, int64, UpdateParams) (any, error) {
				// A source holds what came in. A correction is a new capture.
				return nil, invalid("type", "a source has no field that you can change: delete the source, then capture the correct text, link or file")
			},
			link:   func(id int64) recordLink { return recordLink{source: &id} },
			delete: (*Service).DeleteCapture,
			purge: func(ctx context.Context, q *store.Queries, id int64) (*string, error) {
				row, err := q.PurgeSource(ctx, id)
				return row.FilePath, err
			},
			listDeleted: deletedList("source", (*store.Queries).ListDeletedSources, func(_ context.Context, _ *store.Queries, row store.Source) (int64, string, *time.Time, error) {
				return row.ID, captureFromRow(row).summary(), row.DeletedAt, nil
			}),
			live: func(ctx context.Context, q *store.Queries, _ string, id int64) (bool, error) {
				return q.LiveSourceExists(ctx, id)
			},
			search: func(ctx context.Context, q *store.Queries, patterns []string, maxRows int32) ([]SearchHit, error) {
				rows, err := q.SearchSources(ctx, store.SearchSourcesParams{Patterns: patterns, MaxRows: maxRows})
				if err != nil {
					return nil, err
				}
				var hits []SearchHit
				for _, row := range rows {
					capture := captureFromRow(row)
					hits = append(hits, SearchHit{"source", capture.ID, capture.summary(), capture.CreatedAt})
				}
				return hits, nil
			},
		},
		{
			name: "entity", table: "entities", word: "entity", prefixes: []string{"entity"},
			get: func(s *Service, ctx context.Context, id int64) (any, error) {
				return asRecord(s.GetEntityDetail(ctx, id))
			},
			update: func(s *Service, ctx context.Context, id int64, p UpdateParams) (any, error) {
				var params UpdateEntityParams
				if err := decodeFields(p.Fields, &params, "entity"); err != nil {
					return nil, err
				}
				params.Clear, params.Actor = p.Clear, p.Actor
				return asRecord(s.UpdateEntity(ctx, id, params))
			},
			delete:       (*Service).DeleteEntity,
			purgeRefused: "a record in %s still names this entity: remove the link or delete that record forever first",
			purge: func(ctx context.Context, q *store.Queries, id int64) (*string, error) {
				_, err := q.PurgeEntity(ctx, id)
				return nil, err
			},
			listDeleted: deletedList("entity", (*store.Queries).ListDeletedEntities, func(_ context.Context, _ *store.Queries, row store.Entity) (int64, string, *time.Time, error) {
				return row.ID, entityFromRow(row).summary(), row.DeletedAt, nil
			}),
			live: func(ctx context.Context, q *store.Queries, _ string, id int64) (bool, error) {
				return q.LiveEntityExists(ctx, id)
			},
			search: func(ctx context.Context, q *store.Queries, patterns []string, maxRows int32) ([]SearchHit, error) {
				rows, err := q.SearchEntities(ctx, store.SearchEntitiesParams{Patterns: patterns, MaxRows: maxRows})
				if err != nil {
					return nil, err
				}
				var hits []SearchHit
				for _, row := range rows {
					entity := entityFromRow(row)
					hits = append(hits, SearchHit{"entity", entity.ID, entity.summary(), entity.CreatedAt})
				}
				return hits, nil
			},
		},
	}
	RecordTypes = recordTypeNames()
	SearchTypes = searchTypeNames()
	WikiCitationPrefixes = citationPrefixes()
}

// RecordTypes are the record types that Get, Update and Delete accept. A
// caller that knows the type when it is compiled, such as a REST route,
// calls the function of the type (GetItem, UpdateItem, DeleteItem). An MCP
// tool gets the type as input and calls the functions in records.go.
var RecordTypes []string

func recordTypeNames() []string {
	names := make([]string, len(recordTypes))
	for i, t := range recordTypes {
		names[i] = t.name
	}
	return names
}

// lookupRecordType returns the entry of a type, or a ValidationError that
// lists the types.
func lookupRecordType(name string) (*recordType, error) {
	for i := range recordTypes {
		if recordTypes[i].name == name {
			return &recordTypes[i], nil
		}
	}
	return nil, invalid("type", "must be one of: "+strings.Join(RecordTypes, ", "))
}

// RecordWord returns the name of one record of a table for a person, such
// as "habit log" for habit_logs. ok is false for a table that holds no
// record type.
func RecordWord(table string) (word string, ok bool) {
	for _, t := range recordTypes {
		if t.table == table {
			return t.word, true
		}
	}
	return "", false
}

// SearchTypes are the record types that Search looks in. A habit log has no
// text; the query tool answers a question about habits.
var SearchTypes []string

func searchTypeNames() []string {
	var names []string
	for _, t := range recordTypes {
		if t.search != nil {
			names = append(names, t.name)
		}
	}
	return names
}

// WikiCitationPrefixes are the prefixes that a citation can have. The
// description of the wiki_write tool lists them.
var WikiCitationPrefixes []string

func citationPrefixes() []string {
	var prefixes []string
	for _, t := range recordTypes {
		prefixes = append(prefixes, t.prefixes...)
	}
	return prefixes
}

// recordTypeOfPrefix returns the type that a citation prefix names.
func recordTypeOfPrefix(prefix string) (*recordType, bool) {
	for i := range recordTypes {
		if slices.Contains(recordTypes[i].prefixes, prefix) {
			return &recordTypes[i], true
		}
	}
	return nil, false
}

// deletedList makes the listDeleted function of a type. list is the query,
// and describe gives the ID, the summary and the time of delete of one row.
func deletedList[R any](
	name string,
	list func(*store.Queries, context.Context, int32) ([]R, error),
	describe func(context.Context, *store.Queries, R) (id int64, summary string, deletedAt *time.Time, err error),
) func(context.Context, *store.Queries, int32) ([]DeletedRecord, error) {
	return func(ctx context.Context, q *store.Queries, limit int32) ([]DeletedRecord, error) {
		rows, err := list(q, ctx, limit)
		if err != nil {
			return nil, err
		}
		records := make([]DeletedRecord, 0, len(rows))
		for _, row := range rows {
			id, summary, deletedAt, err := describe(ctx, q, row)
			if err != nil {
				return nil, err
			}
			records = append(records, DeletedRecord{Type: name, ID: id, Summary: summary, DeletedAt: *deletedAt})
		}
		return records, nil
	}
}
