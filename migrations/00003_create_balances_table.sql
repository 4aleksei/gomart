-- +goose Up

CREATE TABLE IF NOT EXISTS balances (
    user_id bigint not null PRIMARY KEY,
    current decimal(19,2) ,
    withdrawn decimal(19,2) ,
    changed_at timestamptz not null DEFAULT NOW()
);


-- +goose Down
DROP TABLE balances;
