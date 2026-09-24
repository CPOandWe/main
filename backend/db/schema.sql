CREATE TABLE
    IF NOT EXISTS roles (
        role_id SMALLSERIAL PRIMARY KEY,
        name VARCHAR NOT NULL
    );

CREATE UNIQUE INDEX IF NOT EXISTS roles_name_key ON roles (name);

INSERT INTO
    roles (role_id, name)
VALUES
    (1, 'user'),
    (2, 'moderator'),
    (3, 'admin') ON CONFLICT (name) DO NOTHING;

CREATE TABLE
    IF NOT EXISTS users (
        user_id UUID PRIMARY KEY DEFAULT uuidv4 (),
        email VARCHAR NOT NULL UNIQUE,
        password_hash VARCHAR NOT NULL,
        role_id SMALLINT NOT NULL REFERENCES roles (role_id)
    );

ALTER TABLE users
ADD COLUMN IF NOT EXISTS username VARCHAR NOT NULL DEFAULT '';

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- default admin (dev): admin@readness.local / admin12345 -- change the password after first login
INSERT INTO
    users (username, email, password_hash, role_id)
VALUES
    (
        'admin',
        'admin@readness.local',
        crypt ('admin12345', gen_salt ('bf')),
        (SELECT role_id FROM roles WHERE name = 'admin')
    ) ON CONFLICT (email) DO NOTHING;

CREATE TABLE
    IF NOT EXISTS refresh_tokens (
        token_id UUID PRIMARY KEY DEFAULT uuidv4 (),
        user_id UUID NOT NULL REFERENCES users (user_id),
        token_hash VARCHAR NOT NULL UNIQUE,
        expires_at TIMESTAMP NOT NULL,
        created_at TIMESTAMP NOT NULL DEFAULT now ()
    );

CREATE TABLE
    IF NOT EXISTS authors (
        author_id UUID PRIMARY KEY DEFAULT uuidv4 (),
        first_name VARCHAR NOT NULL,
        last_name VARCHAR NOT NULL
    );

CREATE TABLE
    IF NOT EXISTS languages (
        language_id UUID PRIMARY KEY DEFAULT uuidv4 (),
        name VARCHAR NOT NULL
    );

CREATE TABLE
    IF NOT EXISTS topics (
        topic_id UUID PRIMARY KEY DEFAULT uuidv4 (),
        name VARCHAR NOT NULL
    );

CREATE TABLE
    IF NOT EXISTS books (
        book_id UUID PRIMARY KEY DEFAULT uuidv4 (),
        title VARCHAR NOT NULL,
        description TEXT NOT NULL,
        author_id UUID NOT NULL REFERENCES authors (author_id),
        language_id UUID NOT NULL REFERENCES languages (language_id),
        published_at DATE NOT NULL,
        is_public BOOLEAN NOT NULL DEFAULT FALSE,
        file_path VARCHAR NOT NULL,
        file_size INTEGER,
        uploaded_at DATE NOT NULL,
        uploaded_by UUID NOT NULL REFERENCES users (user_id)
    );

CREATE TABLE
    IF NOT EXISTS book_topics (
        topic_id UUID NOT NULL REFERENCES topics (topic_id),
        book_id UUID NOT NULL REFERENCES books (book_id),
        PRIMARY KEY (book_id, topic_id)
    );

CREATE TABLE
    IF NOT EXISTS reviews (
        review_id UUID PRIMARY KEY DEFAULT uuidv4 (),
        book_id UUID NOT NULL REFERENCES books (book_id),
        user_id UUID NOT NULL REFERENCES users (user_id),
        rating SMALLINT NOT NULL,
        UNIQUE (user_id, book_id)
    );

CREATE TABLE
    IF NOT EXISTS book_status (
        book_id UUID NOT NULL REFERENCES books (book_id),
        user_id UUID NOT NULL REFERENCES users (user_id),
        status VARCHAR NOT NULL DEFAULT 'pending',
        PRIMARY KEY (book_id, user_id)
    );

CREATE TABLE
    IF NOT EXISTS book_add_requests (
        request_id UUID PRIMARY KEY DEFAULT uuidv4 (),
        user_id UUID NOT NULL REFERENCES users (user_id),
        book_id UUID NOT NULL REFERENCES books (book_id),
        created_at TIMESTAMP NOT NULL,
        status VARCHAR NOT NULL DEFAULT 'pending',
        reason TEXT,
        reviewed_by UUID REFERENCES users (user_id),
        reviewed_at TIMESTAMP
    );

CREATE TABLE
    IF NOT EXISTS user_library (
        user_id UUID NOT NULL REFERENCES users (user_id),
        book_id UUID NOT NULL REFERENCES books (book_id),
        added_at TIMESTAMP NOT NULL,
        PRIMARY KEY (user_id, book_id)
    );

ALTER TABLE books
ADD COLUMN IF NOT EXISTS cover_path VARCHAR NOT NULL DEFAULT '';
