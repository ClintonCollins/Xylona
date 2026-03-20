-- +migrate Up
CREATE TABLE federation_advisory (
    id                    TEXT PRIMARY KEY,
    type                  TEXT NOT NULL,
    title                 TEXT NOT NULL,
    message               TEXT NOT NULL,
    source_node_id        TEXT NOT NULL DEFAULT '',
    source_node_name      TEXT NOT NULL DEFAULT '',
    subject_node_id       TEXT NOT NULL DEFAULT '',
    subject_node_name     TEXT NOT NULL DEFAULT '',
    subject_node_base_url TEXT NOT NULL DEFAULT '',
    read                  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE node ADD COLUMN departed BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE node ADD COLUMN auto_paired BOOLEAN NOT NULL DEFAULT FALSE;

-- +migrate Down
DROP TABLE IF EXISTS federation_advisory;
ALTER TABLE node DROP COLUMN departed;
ALTER TABLE node DROP COLUMN auto_paired;
