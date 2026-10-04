CREATE TABLE users (
    id UUID PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL
);

CREATE TABLE wallets (
    user_id UUID PRIMARY KEY REFERENCES users(id),
    available_amount BIGINT NOT NULL DEFAULT 0
     CHECK (available_amount >= 0)
);

CREATE TABLE items (
    id UUID PRIMARY KEY,
    type TEXT NOT NULL CHECK (length(trim(type)) > 0),
    name TEXT NOT NULL CHECK (length(trim(name)) > 0),
    description TEXT NOT NULL
);