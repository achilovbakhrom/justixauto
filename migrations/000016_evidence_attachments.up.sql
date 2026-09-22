-- Uploaded proof files attached to payment claims (documents.files IDs), and
-- a stable insertion order for claims recorded at the same instant.
ALTER TABLE commerce.payment_evidence
    ADD COLUMN attachment_ids jsonb NOT NULL DEFAULT '[]',
    ADD COLUMN seq bigint GENERATED ALWAYS AS IDENTITY UNIQUE;
ALTER TABLE retail.payment_evidence
    ADD COLUMN attachment_ids jsonb NOT NULL DEFAULT '[]',
    ADD COLUMN seq bigint GENERATED ALWAYS AS IDENTITY UNIQUE;
