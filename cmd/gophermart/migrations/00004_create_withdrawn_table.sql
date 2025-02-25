-- +goose Up

CREATE TABLE IF NOT EXISTS withdrawals (
    user_id bigint,
    order_id bigint not null PRIMARY KEY,
    sum double precision,
    processed_at timestamptz not null DEFAULT NOW()
);


-- +goose Down
DROP TABLE withdrawals;
