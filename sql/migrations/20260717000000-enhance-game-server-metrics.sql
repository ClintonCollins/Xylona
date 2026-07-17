-- +migrate Up
alter table game_server_metrics_history add column node_id text;
alter table game_server_metrics_history add column granularity_seconds integer not null default 60;
alter table game_server_metrics_history add column sample_count integer not null default 1;
alter table game_server_metrics_history add column available_sample_count integer not null default 0;
alter table game_server_metrics_history add column cpu_valid_sample_count integer not null default 0;
alter table game_server_metrics_history add column query_successful_sample_count integer not null default 0;
alter table game_server_metrics_history add column query_duration_valid_sample_count integer not null default 0;
alter table game_server_metrics_history add column server_fps_valid_sample_count integer not null default 0;
alter table game_server_metrics_history add column server_frame_time_valid_sample_count integer not null default 0;
alter table game_server_metrics_history add column volume_valid_sample_count integer not null default 0;
alter table game_server_metrics_history add column io_valid_sample_count integer not null default 0;
alter table game_server_metrics_history add column connection_valid_sample_count integer not null default 0;
alter table game_server_metrics_history add column availability_ratio real not null default 0;
alter table game_server_metrics_history add column collection_status text not null default 'unknown';
alter table game_server_metrics_history add column rollup_hour datetime;
alter table game_server_metrics_history add column process_collected_at datetime;
alter table game_server_metrics_history add column cpu_valid boolean not null default false;
alter table game_server_metrics_history add column cpu_percent_min real;
alter table game_server_metrics_history add column cpu_percent_max real;
alter table game_server_metrics_history add column node_cpu_cores integer;
alter table game_server_metrics_history add column memory_bytes_min integer;
alter table game_server_metrics_history add column memory_bytes_max integer;
alter table game_server_metrics_history add column memory_percent_min real;
alter table game_server_metrics_history add column memory_percent_max real;
alter table game_server_metrics_history add column node_memory_used_bytes integer;
alter table game_server_metrics_history add column node_memory_total_bytes integer;
alter table game_server_metrics_history add column configured_memory_bytes integer;
alter table game_server_metrics_history add column disk_usage_bytes_min integer;
alter table game_server_metrics_history add column disk_usage_bytes_max integer;
alter table game_server_metrics_history add column volume_total_bytes integer;
alter table game_server_metrics_history add column volume_free_bytes integer;
alter table game_server_metrics_history add column volume_percent real;
alter table game_server_metrics_history add column volume_valid boolean not null default false;
alter table game_server_metrics_history add column disk_measured_at datetime;
alter table game_server_metrics_history add column io_read_rate_min real;
alter table game_server_metrics_history add column io_read_rate_max real;
alter table game_server_metrics_history add column io_write_rate_min real;
alter table game_server_metrics_history add column io_write_rate_max real;
alter table game_server_metrics_history add column connection_count_min integer;
alter table game_server_metrics_history add column connection_count_max integer;
alter table game_server_metrics_history add column player_count_min integer;
alter table game_server_metrics_history add column player_count_max integer;
alter table game_server_metrics_history add column player_capacity integer;
alter table game_server_metrics_history add column query_supported boolean;
alter table game_server_metrics_history add column query_success boolean;
alter table game_server_metrics_history add column query_duration_ms real;
alter table game_server_metrics_history add column query_duration_ms_min real;
alter table game_server_metrics_history add column query_duration_ms_max real;
alter table game_server_metrics_history add column query_checked_at datetime;
alter table game_server_metrics_history add column server_fps real;
alter table game_server_metrics_history add column server_fps_min real;
alter table game_server_metrics_history add column server_fps_max real;
alter table game_server_metrics_history add column server_frame_time_ms real;
alter table game_server_metrics_history add column server_frame_time_ms_min real;
alter table game_server_metrics_history add column server_frame_time_ms_max real;
alter table game_server_metrics_history add column server_uptime_seconds integer;
alter table game_server_metrics_history add column process_status text;
alter table game_server_metrics_history add column execution_id text;

update game_server_metrics_history set
    cpu_percent_min = cpu_percent,
    cpu_percent_max = cpu_percent,
    memory_bytes_min = memory_bytes,
    memory_bytes_max = memory_bytes,
    memory_percent_min = memory_percent,
    memory_percent_max = memory_percent,
    disk_usage_bytes_min = disk_usage_bytes,
    disk_usage_bytes_max = disk_usage_bytes,
    io_read_rate_min = io_read_rate,
    io_read_rate_max = io_read_rate,
    io_write_rate_min = io_write_rate,
    io_write_rate_max = io_write_rate,
    connection_count_min = connection_count,
    connection_count_max = connection_count,
    player_count_min = player_count,
    player_count_max = player_count,
    available_sample_count = 0,
    cpu_valid_sample_count = 0,
    query_successful_sample_count = 0,
    query_duration_valid_sample_count = 0,
    server_fps_valid_sample_count = 0,
    server_frame_time_valid_sample_count = 0,
    volume_valid_sample_count = 0,
    io_valid_sample_count = 0,
    connection_valid_sample_count = 0,
    availability_ratio = 0,
    collection_status = 'unknown',
    cpu_valid = false,
    volume_valid = false,
    disk_measured_at = recorded_at;

delete from game_server_metrics_history
where substr(replace(recorded_at, 'T', ' '), 15, 5) = '00:00'
  and id not in (
    select min(id)
    from game_server_metrics_history
    where substr(replace(recorded_at, 'T', ' '), 15, 5) = '00:00'
    group by game_server_id, replace(substr(recorded_at, 1, 13), 'T', ' ')
  );

update game_server_metrics_history
set granularity_seconds = 3600,
    sample_count = 1,
    available_sample_count = 0,
    availability_ratio = 0,
    rollup_hour = replace(substr(recorded_at, 1, 13), 'T', ' ') || ':00:00'
where substr(replace(recorded_at, 'T', ' '), 15, 5) = '00:00';

create unique index game_server_metrics_history_rollup_hour_idx
    on game_server_metrics_history (game_server_id, rollup_hour)
    where rollup_hour is not null;

create table game_server_lifecycle_event (
    id text primary key not null,
    game_server_id text not null,
    node_id text not null,
    execution_id text not null,
    transition_sequence integer not null,
    previous_status text not null,
    status text not null,
    intentional_stop boolean not null default false,
    exit_code integer,
    observed_at datetime not null,
    foreign key (game_server_id) references game_server (id) on delete cascade,
    unique (node_id, game_server_id, execution_id, transition_sequence)
);

create index game_server_lifecycle_event_server_time_idx
    on game_server_lifecycle_event (game_server_id, observed_at);

create index game_server_lifecycle_event_observed_at_idx
    on game_server_lifecycle_event (observed_at);

create table game_server_operation_event (
    id text primary key not null,
    game_server_id text not null,
    operation text not null,
    phase text,
    outcome text not null,
    started_at datetime not null,
    completed_at datetime,
    duration_ms integer,
    bytes_processed integer,
    source text not null,
    foreign key (game_server_id) references game_server (id) on delete cascade
);

create index game_server_operation_event_server_time_idx
    on game_server_operation_event (game_server_id, started_at);

create index game_server_operation_event_retention_idx
    on game_server_operation_event (coalesce(completed_at, started_at));

-- +migrate Down
drop index game_server_operation_event_retention_idx;
drop index game_server_operation_event_server_time_idx;
drop table game_server_operation_event;
drop index game_server_lifecycle_event_observed_at_idx;
drop index game_server_lifecycle_event_server_time_idx;
drop table game_server_lifecycle_event;
drop index game_server_metrics_history_rollup_hour_idx;
alter table game_server_metrics_history drop column execution_id;
alter table game_server_metrics_history drop column process_status;
alter table game_server_metrics_history drop column server_uptime_seconds;
alter table game_server_metrics_history drop column server_frame_time_ms_max;
alter table game_server_metrics_history drop column server_frame_time_ms_min;
alter table game_server_metrics_history drop column server_frame_time_ms;
alter table game_server_metrics_history drop column server_fps_max;
alter table game_server_metrics_history drop column server_fps_min;
alter table game_server_metrics_history drop column server_fps;
alter table game_server_metrics_history drop column query_checked_at;
alter table game_server_metrics_history drop column query_duration_ms_max;
alter table game_server_metrics_history drop column query_duration_ms_min;
alter table game_server_metrics_history drop column query_duration_ms;
alter table game_server_metrics_history drop column query_success;
alter table game_server_metrics_history drop column query_supported;
alter table game_server_metrics_history drop column player_capacity;
alter table game_server_metrics_history drop column player_count_max;
alter table game_server_metrics_history drop column player_count_min;
alter table game_server_metrics_history drop column connection_count_max;
alter table game_server_metrics_history drop column connection_count_min;
alter table game_server_metrics_history drop column io_write_rate_max;
alter table game_server_metrics_history drop column io_write_rate_min;
alter table game_server_metrics_history drop column io_read_rate_max;
alter table game_server_metrics_history drop column io_read_rate_min;
alter table game_server_metrics_history drop column disk_measured_at;
alter table game_server_metrics_history drop column volume_valid;
alter table game_server_metrics_history drop column volume_percent;
alter table game_server_metrics_history drop column volume_free_bytes;
alter table game_server_metrics_history drop column volume_total_bytes;
alter table game_server_metrics_history drop column disk_usage_bytes_max;
alter table game_server_metrics_history drop column disk_usage_bytes_min;
alter table game_server_metrics_history drop column configured_memory_bytes;
alter table game_server_metrics_history drop column node_memory_total_bytes;
alter table game_server_metrics_history drop column node_memory_used_bytes;
alter table game_server_metrics_history drop column memory_percent_max;
alter table game_server_metrics_history drop column memory_percent_min;
alter table game_server_metrics_history drop column memory_bytes_max;
alter table game_server_metrics_history drop column memory_bytes_min;
alter table game_server_metrics_history drop column node_cpu_cores;
alter table game_server_metrics_history drop column cpu_percent_max;
alter table game_server_metrics_history drop column cpu_percent_min;
alter table game_server_metrics_history drop column cpu_valid;
alter table game_server_metrics_history drop column process_collected_at;
alter table game_server_metrics_history drop column collection_status;
alter table game_server_metrics_history drop column availability_ratio;
alter table game_server_metrics_history drop column rollup_hour;
alter table game_server_metrics_history drop column connection_valid_sample_count;
alter table game_server_metrics_history drop column io_valid_sample_count;
alter table game_server_metrics_history drop column volume_valid_sample_count;
alter table game_server_metrics_history drop column server_frame_time_valid_sample_count;
alter table game_server_metrics_history drop column server_fps_valid_sample_count;
alter table game_server_metrics_history drop column query_duration_valid_sample_count;
alter table game_server_metrics_history drop column query_successful_sample_count;
alter table game_server_metrics_history drop column cpu_valid_sample_count;
alter table game_server_metrics_history drop column available_sample_count;
alter table game_server_metrics_history drop column sample_count;
alter table game_server_metrics_history drop column granularity_seconds;
alter table game_server_metrics_history drop column node_id;
