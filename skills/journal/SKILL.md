---
name: journal
description: Use when Gautam writes or dictates a journal entry ("journal - ...", "diary: ...", a reflection on his day, or a reply to the evening brief's journal question).
---

# Journal

Store the entry exactly as written, then connect it to the wiki. The entry is Gautam's words; everything you add goes somewhere else.

## Tools

`[planned]` is in the design doc but not built. `[missing]` is not in the design doc; the step that needs it cannot run yet.

| Tool | Status | Used for |
| --- | --- | --- |
| `journal` | [planned] | Store the entry, get its ID |
| `wiki_search`, `wiki_read`, `wiki_write` | [planned] | Add dated lines to pages the entry mentions |
| `todo` | [planned] | Propose todos found in the entry |
| inferred flag on `todo` | [missing] | Mark a proposed todo unverified |

## Steps

1. **Store.** Pass the text to `journal` character for character. Remove only the trigger ("journal -"). Keep typos, casing, punctuation, line breaks and language as they are. The entry date is today in IST unless Gautam names another day.
2. **Find pages.** `wiki_search` for each person, project and topic the entry names.
3. **Update pages.** For each page found: `wiki_read`, then `wiki_write` one dated line saying what the entry adds, citing `(journal:<id>)`. Write the line in your words; do not paste the entry into the wiki.
4. **Propose.** Do not write these as facts:
   - A task in the entry ("need to call the bank"): propose it as an unverified todo.
   - A person or project with no page: name it in your reply and create the page only after a yes.
5. **Log.** If any page changed, append to `log.md`: `## [date] journal | (journal:<id>)`, then the pages touched.
6. **Reply.** The entry ID, the pages updated, the proposals. Two or three lines.

## Rules

- Never reword, shorten, correct or translate the entry, and never store a summary in its place.
- Never add to an existing entry. More text the same day is a new entry.
- A correction to an old entry happens only when Gautam asks, through `update`.
- No comment on the entry's content unless Gautam asks for one.
- An entry that mentions nothing with a page is stored and acknowledged; do not create pages to have something to update.
