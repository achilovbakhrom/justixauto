CREATE SCHEMA IF NOT EXISTS documents;

-- Immutable uploaded files of a company. Bytes live in private storage under
-- storage_key; the key is never exposed.
CREATE TABLE documents.files (
    id          uuid        PRIMARY KEY,
    company_id  uuid        NOT NULL,
    purpose     text        NOT NULL,
    file_name   text        NOT NULL CHECK (length(file_name) BETWEEN 1 AND 255),
    mime        text        NOT NULL CHECK (mime IN ('application/pdf', 'image/jpeg', 'image/png')),
    byte_length bigint      NOT NULL CHECK (byte_length > 0),
    sha256      text        NOT NULL,
    storage_key text        NOT NULL UNIQUE,
    sensitive   boolean     NOT NULL,
    created_by  uuid        NOT NULL,
    created_at  timestamptz NOT NULL
);
CREATE INDEX files_company_idx ON documents.files (company_id, created_at DESC);

-- Read access for another company, granted by the module that uses the file
-- (e.g. a finance document submission shares it with the provider).
CREATE TABLE documents.shares (
    file_id       uuid        NOT NULL REFERENCES documents.files,
    company_id    uuid        NOT NULL,
    resource_type text        NOT NULL,
    resource_id   uuid        NOT NULL,
    created_at    timestamptz NOT NULL,
    PRIMARY KEY (file_id, company_id, resource_id)
);

-- Finance document exchange after agreement.
CREATE TABLE financing.document_requests (
    id             uuid        PRIMARY KEY,
    application_id uuid        NOT NULL REFERENCES financing.applications,
    title          text        NOT NULL,
    requirements   text        NOT NULL DEFAULT '',
    status         text        NOT NULL CHECK (status IN ('requested', 'review', 'changes', 'accepted', 'cancelled')),
    status_note    text        NOT NULL DEFAULT '',
    version        bigint      NOT NULL CHECK (version >= 1),
    created_by     uuid        NOT NULL,
    created_at     timestamptz NOT NULL,
    updated_at     timestamptz NOT NULL
);
CREATE INDEX document_requests_application_idx ON financing.document_requests (application_id);

CREATE TABLE financing.document_submissions (
    request_id uuid        NOT NULL REFERENCES financing.document_requests,
    number     integer     NOT NULL CHECK (number >= 1),
    file_id    uuid        NOT NULL,
    note       text        NOT NULL DEFAULT '',
    created_by uuid        NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (request_id, number)
);
