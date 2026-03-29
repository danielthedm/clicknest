-- Per-webhook JSON payload template. When non-empty, the push payload is built
-- by substituting {{lead.*}}, {{webhook.*}}, and {{timestamp}} placeholders.
-- {{leads}} is replaced with the full JSON array of lead objects.
ALTER TABLE crm_webhooks ADD COLUMN payload_template TEXT NOT NULL DEFAULT '';
