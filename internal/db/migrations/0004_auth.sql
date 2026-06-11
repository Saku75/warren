-- 0004: users, sessions, API tokens (design doc 0002).
--
-- A user row exists for every person regardless of auth provider;
-- external providers JIT-create one on first login. Credentials are
-- never stored in plaintext: passwords are argon2id hashes, session and
-- API tokens are SHA-256 digests of random secrets.

CREATE TABLE users (
    id            uuid PRIMARY KEY,
    username      text        NOT NULL UNIQUE,
    display_name  text        NOT NULL DEFAULT '',
    email         text        NOT NULL DEFAULT '',
    provider      text        NOT NULL DEFAULT 'local'
                  CHECK (provider IN ('local', 'ldap', 'oidc')),
    external_id   text        NOT NULL DEFAULT '',
    password_hash text,
    is_admin      boolean     NOT NULL DEFAULT false,
    disabled      boolean     NOT NULL DEFAULT false,
    last_login_at timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- External identities are unique per provider; empty external_id (local
-- users) is exempt.
CREATE UNIQUE INDEX users_external_idx ON users (provider, external_id)
    WHERE external_id <> '';

CREATE TABLE sessions (
    id           uuid PRIMARY KEY,
    user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash   bytea       NOT NULL UNIQUE,
    csrf_token   text        NOT NULL,
    ip           text        NOT NULL DEFAULT '',
    user_agent   text        NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL
);

CREATE INDEX sessions_user_idx ON sessions (user_id);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

CREATE TABLE api_tokens (
    id           uuid PRIMARY KEY,
    user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name         text        NOT NULL,
    token_hash   bytea       NOT NULL UNIQUE,
    created_at   timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz,
    last_used_at timestamptz
);

CREATE INDEX api_tokens_user_idx ON api_tokens (user_id);
