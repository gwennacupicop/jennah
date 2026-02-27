-- Migration Step 2: Copy data from GcpBatchJobName to CloudJobResourcePath
-- Run this AFTER Step 1 completes successfully
-- This is a DML statement, run in Query tab (not DDL tab)

UPDATE Jobs 
SET CloudJobResourcePath = GcpBatchJobName 
WHERE CloudJobResourcePath IS NULL AND GcpBatchJobName IS NOT NULL;
