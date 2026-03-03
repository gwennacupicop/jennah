-- Migration: Add ServiceTier column to Jobs table.
-- ServiceTier records which GCP service executed the job:
--   SIMPLE  → Cloud Tasks
--   MEDIUM  → Cloud Run Jobs
--   COMPLEX → Cloud Batch
ALTER TABLE Jobs ADD COLUMN ServiceTier STRING(20);
