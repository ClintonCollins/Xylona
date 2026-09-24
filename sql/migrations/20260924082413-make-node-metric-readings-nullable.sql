-- +migrate Up
-- A host reading the node could not take is stored as NULL instead of a false 0.
-- SQLite cannot drop NOT NULL in place, so the table is rebuilt. Rows whose node
-- is gone are unreachable (the foreign key cascades) and would fail the copy.
create table node_metrics_history_new
(
    id                        text primary key not null,
    node_id                   text             not null references node (id) on delete cascade,
    cpu_percent               real,
    memory_percent            real,
    memory_used_bytes         integer,
    memory_total_bytes        integer,
    disk_percent              real,
    disk_used_bytes           integer,
    disk_total_bytes          integer,
    game_server_count         integer          not null default 0,
    running_game_server_count integer          not null default 0,
    user_count                integer          not null default 0,
    recorded_at               datetime         not null default current_timestamp
);

insert into node_metrics_history_new (id, node_id, cpu_percent, memory_percent, memory_used_bytes,
                                      memory_total_bytes, disk_percent, disk_used_bytes, disk_total_bytes,
                                      game_server_count, running_game_server_count, user_count, recorded_at)
select id, node_id, cpu_percent, memory_percent, memory_used_bytes,
       memory_total_bytes, disk_percent, disk_used_bytes, disk_total_bytes,
       game_server_count, running_game_server_count, user_count, recorded_at
from node_metrics_history
where node_id in (select id from node);

drop table node_metrics_history;
alter table node_metrics_history_new rename to node_metrics_history;

create index idx_node_metrics_node_time
    on node_metrics_history (node_id, recorded_at);

-- +migrate Down
-- Unavailable readings cannot stay NULL under NOT NULL, so they become 0 again.
create table node_metrics_history_old
(
    id                        text primary key not null,
    node_id                   text             not null references node (id) on delete cascade,
    cpu_percent               real             not null default 0,
    memory_percent            real             not null default 0,
    memory_used_bytes         integer          not null default 0,
    memory_total_bytes        integer          not null default 0,
    disk_percent              real             not null default 0,
    disk_used_bytes           integer          not null default 0,
    disk_total_bytes          integer          not null default 0,
    game_server_count         integer          not null default 0,
    running_game_server_count integer          not null default 0,
    user_count                integer          not null default 0,
    recorded_at               datetime         not null default current_timestamp
);

insert into node_metrics_history_old (id, node_id, cpu_percent, memory_percent, memory_used_bytes,
                                      memory_total_bytes, disk_percent, disk_used_bytes, disk_total_bytes,
                                      game_server_count, running_game_server_count, user_count, recorded_at)
select id, node_id, coalesce(cpu_percent, 0), coalesce(memory_percent, 0), coalesce(memory_used_bytes, 0),
       coalesce(memory_total_bytes, 0), coalesce(disk_percent, 0), coalesce(disk_used_bytes, 0),
       coalesce(disk_total_bytes, 0), game_server_count, running_game_server_count, user_count, recorded_at
from node_metrics_history;

drop table node_metrics_history;
alter table node_metrics_history_old rename to node_metrics_history;

create index idx_node_metrics_node_time
    on node_metrics_history (node_id, recorded_at);
