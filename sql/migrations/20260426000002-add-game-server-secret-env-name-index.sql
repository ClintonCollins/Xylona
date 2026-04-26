-- +migrate Up
delete from game_server_secret
where rowid not in (
	select rowid
	from (
		select
			rowid,
			row_number() over (
				partition by game_server_id, kind, lower(name)
				order by updated_at desc, rowid desc
			) as duplicate_rank
		from game_server_secret
	)
	where duplicate_rank = 1
);

create unique index idx_game_server_secret_server_kind_lower_name
	on game_server_secret (game_server_id, kind, lower(name));

-- +migrate Down
drop index idx_game_server_secret_server_kind_lower_name;
