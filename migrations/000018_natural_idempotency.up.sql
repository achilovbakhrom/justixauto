-- Writes are idempotent by their data: unique business keys, state checks and
-- If-Match. The Idempotency-Key request ledger is gone.
DROP TABLE platform.idempotency_keys;
DROP SCHEMA platform;

-- Append-only records that had no natural key: a repeated submission of the
-- same record is a duplicate, not a second record.
CREATE UNIQUE INDEX payment_evidence_reference_key
    ON commerce.payment_evidence (invoice_id, external_reference) WHERE status <> 'rejected';
CREATE UNIQUE INDEX retail_payment_evidence_reference_key
    ON retail.payment_evidence (invoice_id, external_reference) WHERE status <> 'rejected';
CREATE UNIQUE INDEX shipment_milestones_key
    ON commerce.shipment_milestones (shipment_id, milestone_type, occurred_at);
CREATE UNIQUE INDEX document_requests_open_title_key
    ON financing.document_requests (application_id, lower(title)) WHERE status <> 'cancelled';
-- The same content uploaded again for the same purpose is the same file.
CREATE UNIQUE INDEX files_content_key ON documents.files (company_id, purpose, sha256);
