-- +migrate Up
alter table game add column linux_allow_backups boolean not null default false;
alter table game add column windows_allow_backups boolean not null default false;

-- +migrate Down
alter table game drop column windows_allow_backups;
alter table game drop column linux_allow_backups;
