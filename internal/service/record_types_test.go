package service

import "testing"

// A new record type is one entry in recordTypes (rule 9). An entry with a
// missing function would panic when a caller uses that type, so this test
// finds it first.
func TestEveryRecordTypeIsComplete(t *testing.T) {
	names := map[string]bool{}
	tables := map[string]bool{}
	prefixes := map[string]bool{}
	for _, rt := range recordTypes {
		if rt.name == "" || rt.table == "" || rt.word == "" || len(rt.prefixes) == 0 {
			t.Errorf("%q: name, table, word and prefixes must all be set", rt.name)
		}
		if rt.get == nil || rt.update == nil || rt.delete == nil || rt.purge == nil || rt.listDeleted == nil || rt.live == nil {
			t.Errorf("%q: get, update, delete, purge, listDeleted and live must all be set", rt.name)
		}
		if names[rt.name] || tables[rt.table] {
			t.Errorf("%q: the name or the table is used twice", rt.name)
		}
		names[rt.name], tables[rt.table] = true, true
		for _, prefix := range rt.prefixes {
			if prefixes[prefix] {
				t.Errorf("%q: the prefix %q is used twice", rt.name, prefix)
			}
			prefixes[prefix] = true
		}
	}
}
