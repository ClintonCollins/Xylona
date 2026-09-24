-- +migrate Up
-- Failed webhook deliveries used to store error text that could quote the
-- full webhook URL and its secret token (Go's url.Error, or a response body
-- that echoes the request path). Users who may only view alert history could
-- read it, so keep just the HTTP status of rows already written.
update alert_history
set delivery_error = case
        when delivery_error like 'webhooks: delivery failed (status %'
            then substr(delivery_error, 1, instr(delivery_error, ')')) || ' (details removed)'
        else 'webhooks: delivery failed (details removed)'
    end
where channel_type in (
        'NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD',
        'NOTIFICATION_CHANNEL_TYPE_WEBHOOK_SLACK',
        'NOTIFICATION_CHANNEL_TYPE_WEBHOOK_GENERIC'
    )
  and delivery_error is not null
  and delivery_error != '';

-- +migrate Down
-- No-op: the removed text cannot be restored.
select 1;
