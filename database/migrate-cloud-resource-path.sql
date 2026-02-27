-- Migration: Rename GcpBatchJobName to CloudJobResourcePath
-- Description: Makes the database schema cloud-agnostic by renaming the column
--              that stores cloud provider-specific job resource identifiers.
--              This column will store:
--              - GCP: projects/{project}/locations/{region}/jobs/{job-id}
--              - AWS: arn:aws:batch:{region}:{account}:job/{job-id}
--              - Azure: /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.Batch/...
-- Date: 2026-02-16

-- Cloud Spanner doesn't support RENAME COLUMN directly
-- We need to: 1) Add new column 2) Copy data 3) Drop old column

-- Step 1: Add the new column
ALTER TABLE Jobs ADD COLUMN CloudJobResourcePath STRING(1024);

-- Step 2: Copy data from old column to new column (run this UPDATE separately after Step 1 completes)
-- UPDATE Jobs SET CloudJobResourcePath = GcpBatchJobName WHERE CloudJobResourcePath IS NULL;

-- Step 3: Drop the old column (run this ALTER separately after Step 2 completes)
-- ALTER TABLE Jobs DROP COLUMN GcpBatchJobName;
