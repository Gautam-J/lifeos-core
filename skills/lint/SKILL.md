---
name: lint
description: Use when the wiki needs a health check ("lint the wiki", "check for contradictions", "any stale or orphan pages?"), or when run as "/lint" by the Sunday lint job.
---

# Lint

Find what is wrong in the wiki and report it. Lint never fixes a page.

## Tools

`[planned]` is in the design doc but not built. `[missing]` is not in the design doc; the step that needs it cannot run yet.

| Tool | Status | Used for |
| --- | --- | --- |
| `wiki_read` | [planned] | Read `index.md` and each page |
| `wiki_search` | [planned] | Find pages by subject |
| `get` | [planned] | Resolve a citation to its row |
| `wiki_write` | [planned] | Append the run to `log.md`, nothing else |
| list every wiki page | [missing] | Find pages that `index.md` does not list |
| file a finding in the review queue | [missing] | Deliver the findings |

## Checks

Run all six on every page except `index.md` and `log.md`.

| # | Finding | Test |
| --- | --- | --- |
| 1 | Uncited claim | A sentence states a fact and has no `(prefix:id)` |
| 2 | Dead citation | `get` finds no row, or the row has `deleted_at` set |
| 3 | Contradiction | Two pages, or a page and the row it cites, disagree, and the lines are not dated to show the change |
| 4 | Orphan | No page other than `index.md` links to it; or it is missing from `index.md`; or `index.md` lists a page that does not exist |
| 5 | Stale | `updated` is more than 90 days ago. Skip `review` pages and projects with `status: done` |
| 6 | Bad frontmatter | A required field is missing, or `type` does not match the folder |

## Steps

1. **List pages.** Read `index.md`, then get the full page list to catch unlisted pages.
2. **Check.** Read each page once and run the six checks. Resolve each distinct citation once.
3. **File.** One finding per problem: the page, the line, the check number, and what is wrong in one sentence. File each in the review queue.
4. **Log.** Append to `log.md`: `## [date] lint | <n> findings`, then the count per check.
5. **Report.** In conversation: the counts and the five worst findings. As a job: the log entry is the report.

Until the queue tool exists, step 3 cannot file anything: in conversation, list the findings in the reply; as a job, put them in the log entry.

## Rules

- The only write is the `log.md` entry. Never edit, merge, rename or delete a page, even for an obvious fix.
- Report each problem once. A line that fails two checks is two findings.
- A sentence that only connects cited facts is not an uncited claim.
- Do not judge whether a claim is true beyond its cited rows.
- No findings is a valid result. Log it as `0 findings`.
