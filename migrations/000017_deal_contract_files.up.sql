-- Uploaded contract scans (documents.files IDs) of a retail sale.
ALTER TABLE retail.deals ADD COLUMN contract_file_ids jsonb NOT NULL DEFAULT '[]';
