-- +goose Up

CREATE TABLE IF NOT EXISTS withdrawals (
    user_id bigint,
    order_id bigint not null PRIMARY KEY,
    sum decimal(19,2),
    processed_at timestamptz not null DEFAULT NOW()
);


-- +goose Down
DROP TABLE withdrawals;
