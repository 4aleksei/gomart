-- +goose Up

CREATE TABLE IF NOT EXISTS balances (
    user_id bigint not null PRIMARY KEY,
    current double precision ,
    withdrawn double precision ,
    changed_at timestamptz not null DEFAULT NOW()
);


-- +goose Down
DROP TABLE balances;
