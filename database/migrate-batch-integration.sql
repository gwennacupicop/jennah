-- Migration: Add GCP Batch integration fields
-- Run this to add MaxRetries and GcpBatchJobName columns

ALTER TABLE Jobs ADD COLUMN MaxRetries INT64 NOT NULL DEFAULT (3);
ALTER TABLE Jobs ADD COLUMN GcpBatchJobName STRING(1024);

-- Update existing jobs to have MaxRetries = 3
UPDATE Jobs SET MaxRetries = 3 WHERE MaxRetries IS NULL;
