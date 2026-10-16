-- +goose Up

-- #41: one list of the things that records are about. A record links to the
-- entities it names through record_entities.
CREATE TABLE entities (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind        text NOT NULL CHECK (kind IN ('person', 'place', 'project', 'topic')),
    name        text NOT NULL,
    aliases     text[] NOT NULL DEFAULT '{}',
    wiki_path   text,                -- the page of the entity, null until a page exists
    archived_at timestamptz,

    source      text NOT NULL CHECK (source IN ('manual', 'import', 'connector', 'agent')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz,

    -- The target of the project key of a todo: the key then refuses an
    -- entity that is not a project.
    UNIQUE (id, kind)
);

CREATE UNIQUE INDEX entities_kind_name_idx ON entities (kind, lower(name)) WHERE deleted_at IS NULL;

CREATE TRIGGER entities_set_updated_at
    BEFORE UPDATE ON entities
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- One link of an entity to one record. A column for each record type keeps
-- a real foreign key; exactly one of them is set.
CREATE TABLE record_entities (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    entity_id    bigint NOT NULL REFERENCES entities (id),
    journal_id   bigint REFERENCES journal_entries (id),
    todo_id      bigint REFERENCES todos (id),
    item_id      bigint REFERENCES catalog_items (id),
    habit_log_id bigint REFERENCES habit_logs (id),
    source_id    bigint REFERENCES sources (id),

    source       text NOT NULL CHECK (source IN ('manual', 'import', 'connector', 'agent')),
    created_at   timestamptz NOT NULL DEFAULT now(),

    CHECK (num_nonnulls(journal_id, todo_id, item_id, habit_log_id, source_id) = 1)
);

-- A link exists once. The index also finds the records of an entity.
CREATE UNIQUE INDEX record_entities_journal_idx   ON record_entities (entity_id, journal_id)   WHERE journal_id IS NOT NULL;
CREATE UNIQUE INDEX record_entities_todo_idx      ON record_entities (entity_id, todo_id)      WHERE todo_id IS NOT NULL;
CREATE UNIQUE INDEX record_entities_item_idx      ON record_entities (entity_id, item_id)      WHERE item_id IS NOT NULL;
CREATE UNIQUE INDEX record_entities_habit_log_idx ON record_entities (entity_id, habit_log_id) WHERE habit_log_id IS NOT NULL;
CREATE UNIQUE INDEX record_entities_source_idx    ON record_entities (entity_id, source_id)    WHERE source_id IS NOT NULL;
-- These find the links of a record, and Purge of a record uses them.
CREATE INDEX record_entities_journal_id_idx   ON record_entities (journal_id)   WHERE journal_id IS NOT NULL;
CREATE INDEX record_entities_todo_id_idx      ON record_entities (todo_id)      WHERE todo_id IS NOT NULL;
CREATE INDEX record_entities_item_id_idx      ON record_entities (item_id)      WHERE item_id IS NOT NULL;
CREATE INDEX record_entities_habit_log_id_idx ON record_entities (habit_log_id) WHERE habit_log_id IS NOT NULL;
CREATE INDEX record_entities_source_id_idx    ON record_entities (source_id)    WHERE source_id IS NOT NULL;

GRANT SELECT ON entities, record_entities TO lifeos_query;

-- The project of a todo is one project entity. The key on (id, kind) makes
-- the database refuse a person as the project.
ALTER TABLE todos ADD COLUMN project_id bigint;
ALTER TABLE todos ADD COLUMN project_kind text GENERATED ALWAYS AS ('project') STORED;
ALTER TABLE todos ADD FOREIGN KEY (project_id, project_kind) REFERENCES entities (id, kind);
CREATE INDEX todos_project_id_idx ON todos (project_id) WHERE project_id IS NOT NULL;

-- Each distinct old value becomes one project. Two spellings that differ
-- only in case become one project, with the spelling of the oldest todo.
INSERT INTO entities (kind, name, source)
SELECT DISTINCT ON (lower(project)) 'project', project, 'import'
FROM todos
WHERE project IS NOT NULL
ORDER BY lower(project), id;

INSERT INTO events (action, record_type, record_id, source, new_value)
SELECT 'create', 'entities', id, 'import', jsonb_build_object('kind', kind, 'name', name)
FROM entities;

UPDATE todos SET project_id = e.id
FROM entities e
WHERE e.kind = 'project' AND lower(e.name) = lower(todos.project);

ALTER TABLE todos DROP COLUMN project;

-- +goose Down
ALTER TABLE todos ADD COLUMN project text;
UPDATE todos SET project = e.name FROM entities e WHERE e.id = todos.project_id;
ALTER TABLE todos DROP COLUMN project_id CASCADE;
ALTER TABLE todos DROP COLUMN project_kind;
DROP TABLE record_entities;
DROP TABLE entities;
