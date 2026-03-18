
-- +migrate Up
-- SQLite does not support ALTER TABLE ... ADD CONSTRAINT, so we recreate the
-- table with ON DELETE CASCADE on user_id to ensure user sessions are
-- automatically cleaned up when a user is deleted.
PRAGMA foreign_keys = OFF;

CREATE TABLE user_session_new
(
    id         text primary key not null,
    user_id    text references user (id) on delete cascade not null,
    token      text             not null,
    created_at datetime         not null default current_timestamp,
    updated_at datetime         not null default current_timestamp,
    expires_at datetime         not null
);

INSERT INTO user_session_new SELECT * FROM user_session;
DROP TABLE user_session;
ALTER TABLE user_session_new RENAME TO user_session;

PRAGMA foreign_keys = ON;

-- +migrate Down
PRAGMA foreign_keys = OFF;

CREATE TABLE user_session_old
(
    id         text primary key          not null,
    user_id    text references user (id) not null,
    token      text                      not null,
    created_at datetime                  not null default current_timestamp,
    updated_at datetime                  not null default current_timestamp,
    expires_at datetime                  not null
);

INSERT INTO user_session_old SELECT * FROM user_session;
DROP TABLE user_session;
ALTER TABLE user_session_old RENAME TO user_session;

PRAGMA foreign_keys = ON;
