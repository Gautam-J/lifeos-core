---
name: todo
description: Use when Gautam adds, changes, completes or lists tasks ("todo - ...", "remind me to ...", "I need to ...", "mark X done", "push that to Friday", "what's due this week").
---

# Todo

Turn a request into one todo per action, each with a project and, when Gautam gave one, a due date.

## Tools

`[planned]` is in the design doc but not built.

| Tool | Status | Used for |
| --- | --- | --- |
| `todo` | [planned] | Add a todo, change it, mark it done |
| `search` | [planned] | Find the todo a change refers to |
| `query` | [planned] | List todos; list existing project names |
| `delete` | [planned] | Remove a todo, after confirmation |

## Add

1. **Split.** One todo per action. "Buy cloudflare domain and set up tunnel" is two.
2. **Word it.** Start with a verb and keep Gautam's own nouns: "Buy Cloudflare domain".
3. **Project.** Use the project he names, or the one the conversation is plainly about. Match it to an existing project name (`query`) so one project is not spelled two ways. If none fits, leave it empty.
4. **Due date.** Set one only if he gave one. Resolve "tomorrow" and "Friday" in IST. No date given means no date; undated todos come back in the Monday morning brief.
5. **Write.** Call `todo` once per action. These rows are dictated, so they are verified.
6. **Reply.** One line per todo with its ID.

## Change, complete, list

- **Change or complete:** `search` for the todo. One match: call `todo` with the change. Several matches: list them and ask which.
- **Delete:** show the todo, ask, and call `delete` only after a yes. A finished task is marked done, not deleted.
- **List:** `query` open todos, grouped by project, overdue first.

## Rules

- Never invent a due date, a priority or a project.
- A todo you infer from a journal entry or a source belongs to that skill, and is unverified.
- If one request mixes a todo and something else ("todo: renew passport, also watched Dune"), handle the todo here and pass the rest to its skill.
- The service rejects an impossible date. Relay the error and ask.
