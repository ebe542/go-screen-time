CREATE TABLE break_states (
    child_id TEXT PRIMARY KEY NOT NULL,
    used_nanoseconds INTEGER NOT NULL CHECK (used_nanoseconds >= 0),
    started_at INTEGER
);
