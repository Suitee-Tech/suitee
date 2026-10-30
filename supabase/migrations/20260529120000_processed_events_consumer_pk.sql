ALTER TABLE IF EXISTS events.processed_events
ADD COLUMN IF NOT EXISTS consumer TEXT NOT NULL DEFAULT 'legacy';

ALTER TABLE IF EXISTS events.processed_events
DROP CONSTRAINT IF EXISTS processed_events_pkey;

ALTER TABLE IF EXISTS events.processed_events
ADD CONSTRAINT processed_events_pkey PRIMARY KEY (consumer, message_id);

