-- +migrate Up
create table game_server_diagnosis (
    game_server_id text primary key not null references game_server(id) on delete cascade,
    node_id text not null,
    execution_id text not null,
    attempt_started_at integer not null,
    occurred_at integer not null,
    stage text not null,
    category text not null,
    error text not null,
    evidence text not null,
    matched_evidence text not null,
    truncated boolean not null,
    evidence_available boolean not null,
    exit_code integer,
    quality integer not null
);

-- +migrate Down
drop table game_server_diagnosis;
