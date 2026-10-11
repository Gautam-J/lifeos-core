# Backlog

Work that is not in a plan yet. Move an entry into a plan when its turn comes.

## List items (`catalog_items`)

- **One JSON schema for `attrs` for each item kind.** `attrs` accepts any JSON object now. Define the keys and their types for `movie`, `book`, `place` and `game`, and make the service check them.
- **Duplicate detection.** The same kind and title can be stored two times now. Define what a duplicate is, and make the service detect it.

## Todos (`todos`)

- **A description field.** A todo has only a title now. Add an optional `description` for the details of the task.
- **A projects table.** `todos.project` is free text now. Add a `projects` table, and make `todos.project` a foreign key to it.

## Money (`accounts`, `transactions`, `budgets`)

LifeOS is the one place where Gautam logs each record, money included. A bank statement is not the source of the transactions; it is a check on them. These entries replace "Plan 5. Money and importers" of the Phase 2 plans.

- **Money tables with entry by hand.** Add `accounts`, `transactions` and `budgets` as record tables, with entry through the admin pages, the REST API and the MCP tools. Give each table the right `GRANT SELECT ... TO lifeos_query` and its name in the description of the `query` tool (AGENTS.md rule 13).
- **Reconciliation with a bank statement.** A script that Gautam runs by hand on one statement CSV. It compares the statement to the stored transactions and reports each line that has no transaction in LifeOS. It needs the money tables first.

## One-time imports

An import is an individual script that runs one time for one export. It is not a permanent importer, so the command `lifeos import-csv` of the design doc is not necessary for it.

- **Letterboxd, Goodreads and other exports.** One script for each export. It adds rows to `catalog_items` through the service with `source = import`, so that provenance and events hold. A second run must not store a row two times (see "Duplicate detection" above).
