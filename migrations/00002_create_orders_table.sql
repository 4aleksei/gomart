-- +goose Up

CREATE TYPE  order_status_type AS ENUM (
'NEW',
'PROCESSING',
'REGISTERED',
'INVALID',
'PROCESSED'
);

CREATE TABLE IF NOT EXISTS orders (
    order_id bigint not null PRIMARY KEY,
    user_id bigint not null,
    status order_status_type,
    accrual decimal(19,2),
    uploaded_at timestamptz not null DEFAULT NOW(),
    changed_at timestamptz not null DEFAULT NOW()
);


-- +goose Down
DROP TABLE orders;
DROP TYPE order_status_type;
