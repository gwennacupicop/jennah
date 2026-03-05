-- migrate-remove-medium-tier.sql
-- Merges the MEDIUM service tier into SIMPLE.
-- Cloud Tasks was removed; Cloud Run Jobs now covers all non-COMPLEX workloads.
-- Safe to run multiple times (idempotent WHERE clause).

UPDATE Jobs
SET ServiceTier = 'SIMPLE'
WHERE ServiceTier = 'MEDIUM';
