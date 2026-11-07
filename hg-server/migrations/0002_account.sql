-- Accounts: staff (H-Global Studio) and customers (storefront).

CREATE TABLE staff (
    id            uuid        PRIMARY KEY DEFAULT uuidv7(),
    email         text        NOT NULL,
    name          text        NOT NULL,
    role          text        NOT NULL CHECK (role IN ('owner', 'manager', 'cs')),
    password_hash text        NOT NULL,
    active        boolean     NOT NULL DEFAULT true,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX staff_email ON staff (lower(email));

CREATE TABLE customers (
    id            uuid        PRIMARY KEY DEFAULT uuidv7(),
    email         text        NOT NULL,
    name          text        NOT NULL DEFAULT '',
    password_hash text        NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX customers_email ON customers (lower(email));
