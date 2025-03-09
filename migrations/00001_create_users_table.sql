-- +goose Up
CREATE TABLE IF NOT EXISTS users (
    name varchar(128) not null PRIMARY KEY,
    password  varchar(128) not null,
    user_id BIGSERIAL,
    created_at timestamptz not null DEFAULT NOW()
);


-- +goose Down
DROP TABLE users;
