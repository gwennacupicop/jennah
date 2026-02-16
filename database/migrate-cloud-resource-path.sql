-- Migration: Rename GcpBatchJobName to CloudJobResourcePath
-- Description: Makes the database schema cloud-agnostic by renaming the column
--              that stores cloud provider-specific job resource identifiers.
--              This column will store:
--              - GCP: projects/{project}/locations/{region}/jobs/{job-id}
--              - AWS: arn:aws:batch:{region}:{account}:job/{job-id}
--              - Azure: /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.Batch/...
-- Date: 2026-02-16

-- Rename the column
ALTER TABLE Jobs RENAME COLUMN GcpBatchJobName TO CloudJobResourcePath;

-- Note: This is a DDL change in Cloud Spanner and does not require data migration.
-- The column contents remain the same, only the name changes for better clarity.
-- All existing GCP resource paths will continue to work without modification.
