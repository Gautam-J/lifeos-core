## v0.7.0 (2026-10-16)

### Feat

- **mcp**: the instructions and the journal flow give entities their rule
- **admin**: pages for the entities
- **service**: the entities of the day in context
- **service**: merge two entities
- **mcp**: tag records with entities from every write tool
- **service**: the entity record type, links and tag
- **db**: entities and the project of a todo

## v0.6.1 (2026-10-16)

### Refactor

- **service**: one registry for the record types

## v0.6.0 (2026-10-16)

### Feat

- **admin**: the Write button puts the cursor in the journal text box

## v0.5.2 (2026-10-16)

### Fix

- **admin**: show a whole source on its own page

## v0.5.1 (2026-10-16)

### Fix

- **admin**: a day is a text field that reads DD-MM-YYYY

## v0.5.0 (2026-10-16)

### Feat

- **mcp**: context returns the journal and the todos of today and yesterday

## v0.4.0 (2026-10-16)

### Feat

- **mcp**: the context tool, and me.md in the instructions of a session
- **wiki**: me.md, a short page about the user with a cap of 2000 characters

## v0.3.1 (2026-10-16)

### Fix

- **service**: an agent does not change the body of a journal entry

## v0.3.0 (2026-10-16)

### BREAKING CHANGE

- the column source_ref and the MCP input source_ref are
gone.
- the column verified and the MCP input dictated are gone.
- the routes /api/review and /admin/review and the MCP
tools review_queue and verify are gone.

### Feat

- **admin**: link each record to the journal entry that reports it
- **mcp**: a journal entry logs its habits and closes its todos
- link a habit log, a todo and an item to the journal entry that reports it
- drop source_ref; no record comes from a capture
- write each record at once, with no approval flag
- remove the review queue and the verify step

### Fix

- **mcp**: update tells the agent to keep the words of a journal entry
- a habit log from a journal entry takes the day of the entry

## v0.2.0 (2026-10-16)

### Feat

- **cli**: add lifeos instructions, which prints the MCP server instructions
- **mcp**: act on a plain message about the user's life, with no keyword

### Fix

- name the release of the binary in the OpenAPI document and the MCP server

## v0.1.0 (2026-10-12)

### Feat

- show the release in the binary and in the footer
- **admin**: add a dark theme with a choice in the browser
- **admin**: style the admin pages and add a summary page
- **service**: add Overview, the state of today and yesterday
- **deploy**: push the wiki to GitHub from a mirror that the server cannot write
- **deploy**: check that Cloudflare Access guards the MCP hostname
- **mcp**: answer only a request that has a token of Cloudflare Access
- **mcp**: add the check of the Cloudflare Access token
- **mcp**: send server instructions and name the next step after journal and capture
- **mcp**: add the wiki tools and mark_processed
- **service**: mark a source as processed
- **service**: lint the wiki
- **service**: search the wiki pages
- **service**: append to a wiki page and delete a wiki page
- **service**: refuse a wiki citation that names no live record
- **service**: write a wiki page with a hash check, the index, a commit and an event
- **service**: read a wiki page
- **config**: add the wiki folder to the config and check it at the start
- **deploy**: run lifeos serve under systemd, with a deploy script and an Access check
- **serve**: refuse a request from another site that changes data
- **admin**: a page that removes a deleted record for good
- **service**: list the deleted records of every type
- **service**: remove a deleted source and its file for good
- **service**: remove a deleted record for good
- **deploy**: back up the file store
- **mcp**: add the query tool
- **config**: connect the query pool as the read-only user
- **service**: run a read-only query
- **db**: let the read-only user read every table
- **deploy**: add the command that makes the read-only Postgres user
- **mcp**: add the search tool
- **api**: add the search route
- **service**: search the records by their words
- **admin**: add the sources page
- **mcp**: add the capture tool and the source_ref argument
- **api**: add the ingest and source routes
- **service**: check the source_ref of a new record
- **service**: make source a record type
- **config**: add the file store and the inbox to the config
- **service**: capture a file into the file store
- **service**: capture a text or a link as a source
- **db**: add the sources table and the source_ref foreign keys
- **service**: refuse a secret in each text that a caller gives
- **service**: find card numbers, ID numbers and secrets in text
- **admin**: add the habits page
- **mcp**: add the log_habit tool
- **api**: add the habit routes
- **service**: delete a habit log, and add habit logs to the record functions and the review
- **service**: log a habit, and read the logs and the checklist of today
- **service**: add, list and change habits
- **admin**: add the todos page
- **mcp**: add the todo tool
- **api**: add the todo routes
- **service**: add todos to the record functions and the review
- **service**: correct and delete a todo
- **service**: add, read and list todos
- **admin**: add the journal page
- **mcp**: add the journal tool
- **api**: add the journal routes
- **service**: add journal entries to the record functions and the review
- **service**: correct and delete a journal entry
- **service**: add, read and list journal entries
- **db**: add the journal, todo and habit tables
- **admin**: add the review page
- **admin**: edit and delete an item
- **mcp**: add the update, delete, review_queue and verify tools
- **api**: add the review queue and verify endpoints
- **api**: add PATCH and DELETE for items
- **service**: add Get, Update and Delete by record type
- **service**: add the review queue and Verify
- **service**: delete a catalog item with a soft delete
- **service**: correct a catalog item with UpdateItem
- serve the API, admin pages and MCP server
- **admin**: add the items page with templ and htmx
- **mcp**: add the add_item and get tools
- **api**: add REST endpoints for catalog items
- **service**: read and list catalog items
- **service**: add catalog items with provenance and events
- **db**: apply goose migrations with lifeos migrate
- **config**: load config/lifeos.yaml and add the Makefile
- **db**: add initial migration for events and catalog_items
- scaffold lifeos binary with stub subcommands

### Fix

- **admin**: keep the label of a card in one line on a phone
- **deploy**: give the push of the wiki a time limit
- **service**: do not fail a wiki write when git has nothing to commit, and keep the files of a commit
- **service**: refuse a wiki title or summary that the page file cannot keep
- **service**: keep the status and the contested mark of a wiki page on an update
- **deploy**: do not print the connection errors of the wait loop
- **deploy**: reload systemd before the restart
- **deploy**: take only an answer of Access as proof in the Access check
- **service**: remove a deleted source when the folder of its file is gone
- **service**: take 12 digits as an Aadhaar number only next to its name
- **service**: match a card number to the length of its network
- **service**: limit the size of one row of a query
- **service**: keep a captured file through a power cut and a double request
- **mcp**: tell the model that a stated habit log approves a proposed one
- **service**: refuse an inferred change to a verified record
- **service**: refuse a null or an empty update, and store nothing when no value changes
- **admin**: show a server failure of the form post in the list
- **mcp**: serve MCP without sessions so that shutdown is clean

### Refactor

- **admin**: rename itemID to pathID
- **api**: share the PATCH steps between the record types
- **service**: share the delete steps between the record types
- **deploy**: send all ntfy messages through one script
