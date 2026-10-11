# Backlog

Work that is not in a plan yet. Move an entry into a plan when its turn comes.

## List items (`catalog_items`)

- **One JSON schema for `attrs` for each item kind.** `attrs` accepts any JSON object now. Define the keys and their types for `movie`, `book`, `place` and `game`, and make the service check them.
- **Duplicate detection.** The same kind and title can be stored two times now. Define what a duplicate is, and make the service detect it.
