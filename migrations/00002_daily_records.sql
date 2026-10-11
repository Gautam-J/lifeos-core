-- +goose Up

-- Journal text exactly as Gautam wrote it.
CREATE TABLE journal_entries (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    body        text NOT NULL,       -- stored as written; the service never rewords it
    entry_date  date NOT NULL,       -- the IST day that the entry is about

    source      text NOT NULL CHECK (source IN ('manual', 'import', 'connector', 'agent')),
    source_ref  bigint,
    verified    boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE TRIGGER journal_entries_set_updated_at
    BEFORE UPDATE ON journal_entries
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Tasks.
CREATE TABLE todos (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title       text NOT NULL,
    status      text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'done', 'dropped')),
    due_on      date,
    project     text,
    done_at     timestamptz,         -- set when the status becomes done

    source      text NOT NULL CHECK (source IN ('manual', 'import', 'connector', 'agent')),
    source_ref  bigint,
    verified    boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE TRIGGER todos_set_updated_at
    BEFORE UPDATE ON todos
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Habit definitions.
CREATE TABLE habits (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name        text NOT NULL,       -- in lower case
    active      boolean NOT NULL DEFAULT true,

    source      text NOT NULL CHECK (source IN ('manual', 'import', 'connector', 'agent')),
    source_ref  bigint,
    verified    boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE UNIQUE INDEX habits_name_idx ON habits (name) WHERE deleted_at IS NULL;

CREATE TRIGGER habits_set_updated_at
    BEFORE UPDATE ON habits
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Daily check-ins: one live log for each habit and day.
CREATE TABLE habit_logs (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    habit_id    bigint NOT NULL REFERENCES habits (id),
    logged_on   date NOT NULL,       -- the IST day

    source      text NOT NULL CHECK (source IN ('manual', 'import', 'connector', 'agent')),
    source_ref  bigint,
    verified    boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE UNIQUE INDEX habit_logs_habit_day_idx ON habit_logs (habit_id, logged_on) WHERE deleted_at IS NULL;

CREATE TRIGGER habit_logs_set_updated_at
    BEFORE UPDATE ON habit_logs
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE habit_logs;
DROP TABLE habits;
DROP TABLE todos;
DROP TABLE journal_entries;
