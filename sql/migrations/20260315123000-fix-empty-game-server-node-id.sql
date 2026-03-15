-- +migrate Up

update game_server
set node_id = (select node_id from local_settings where id = 1)
where (
    node_id is null
    or trim(node_id) = ''
    or not exists (
        select 1
        from node
        where node.id = game_server.node_id
    )
)
and exists (select 1 from local_settings where id = 1);

-- +migrate Down

select 1;
