---
name: ask
description: Use when Gautam asks a question about his own life, records, notes or knowledge ("when did I last ...", "how much did I spend on ...", "what have I learned about ...", "prep me for tomorrow's call", "what do I know about X").
---

# Ask

Answer from the wiki first, then the database, with citations. Write nothing unless Gautam says to save.

## Tools

`[planned]` is in the design doc but not built.

| Tool | Status | Used for |
| --- | --- | --- |
| `wiki_search`, `wiki_read` | [planned] | Find and read pages |
| `search` | [planned] | Find records and sources |
| `get` | [planned] | Follow a citation to its row |
| `query` | [planned] | Counts, totals, date ranges (SELECT only) |
| `wiki_write` | [planned] | Save an answer as a page, on request |

## Steps

1. **Pick the starting point.**

   | Question | Start with |
   | --- | --- |
   | What do I think or know about X | `index.md`, then `wiki_search` and `wiki_read` |
   | When, where, which one | `search` |
   | How much, how many, how often | `query` |
   | Prep for a meeting or person | `query` the calendar event, then the people and project pages, then `search` recent emails |

2. **Check what matters.** When the answer rests on one claim, `get` the cited row and confirm the page still matches it.
3. **Answer.** Lead with the answer. Put the citation after each fact: `(visit:311)`. For numbers, state the period and say that only verified rows are counted.
4. **Say what is missing.** If the wiki and the database have nothing, say so. General knowledge may be added only when labelled as not from his records.
5. **Offer to save.** If the answer combines several pages or rows and would be useful again, offer to save it as `topics/<slug>.md`. On a yes: `wiki_write` the page with its citations, add it to `index.md`, append `## [date] ask | <title>` to `log.md`.

## Rules

- `query` runs as a read-only user. Write SELECT statements only, and filter totals to verified, non-deleted rows.
- Never present an inference as a recorded fact. "No visit is recorded after March" is not "you have not been since March".
- For a chart, point to the matching Grafana dashboard.
- A question that turns into a correction ("no, that was 3 stars") goes to `update`, not into the answer.
