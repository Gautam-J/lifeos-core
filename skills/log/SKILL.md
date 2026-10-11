---
name: log
description: Use when Gautam reports something watched, read, played or visited, or a habit done ("watched Dune Part Two, 4 stars", "finished the book", "add X to my books", "log gym", "did my run today").
---

# Log

Add list items and habit check-ins. Ask when the kind is ambiguous; never guess it.

## Tools

`[planned]` is in the design doc but not built. `[missing]` is not in the design doc; the step that needs it cannot run yet.

| Tool | Status | Used for |
| --- | --- | --- |
| `add_item` | [planned] | Add a movie, book, place or game |
| `log_habit` | [planned] | Record a habit check-in |
| `search` | [planned] | Check whether the item already exists |
| `query` | [planned] | List habit definitions |
| `update` | [planned] | Correct a rating, date or kind |
| create a habit definition | [missing] | Add a habit that has no definition yet |

## List item

1. **Kind.** Movie, book, place or game. Take it from Gautam's words ("watched", "read", "played", "went to") or the title when only one kind is possible. "Add Dune" could be the book or the movie: ask.
2. **Fields.** Title as he said it, the date in IST ("last night" is yesterday), the rating if he gave one, a note if he gave one. Leave out anything he did not say.
3. **Duplicate check.** `search` for the title and kind. If the item exists, this is a correction or a repeat: say so and ask which.
4. **Write.** Call `add_item`. The row is dictated, so it is verified.
5. **Reply.** One line with the ID.

## Habit

1. **Match.** `query` the habit definitions and match his word to one ("gym", "run"). No match: ask whether he means an existing habit or a new one.
2. **Date.** Today in IST unless he names another day.
3. **Write.** Call `log_habit`. Reply in one line.

## Corrections

"That was 3 stars, not 4": find the row and call `update`. Wiki pages cite the row by ID and need no change.

## Rules

- Never guess a `kind`, a rating or a date.
- Ratings run 0 to 5. The service rejects anything else; relay the error and ask.
- A habit already logged for that day: say so, and do not log it twice.
- Several items in one message get one `add_item` call each.
