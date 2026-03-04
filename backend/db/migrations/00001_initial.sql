-- +goose Up
CREATE TABLE IF NOT EXISTS rooms (
    id         TEXT NOT NULL PRIMARY KEY,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS rooms;
