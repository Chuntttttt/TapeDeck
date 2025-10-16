DROP INDEX IF EXISTS idx_card_mappings_thumbnail_url;
ALTER TABLE card_mappings DROP COLUMN thumbnail_url;
