-- Migration Step 3 (OPTIONAL): Drop old GcpBatchJobName column
-- Run this AFTER Step 2 completes and you've verified CloudJobResourcePath has all data
-- WARNING: This is irreversible! Make sure CloudJobResourcePath is working first.

ALTER TABLE Jobs DROP COLUMN GcpBatchJobName;
