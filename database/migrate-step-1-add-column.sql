-- Migration Step 1: Add CloudJobResourcePath column
-- Run this first in Cloud Console

ALTER TABLE Jobs ADD COLUMN CloudJobResourcePath STRING(1024);
