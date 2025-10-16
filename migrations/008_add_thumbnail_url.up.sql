-- Add thumbnail_url column for caching Plex poster URLs
ALTER TABLE card_mappings ADD COLUMN thumbnail_url TEXT;

-- Add index for faster lookups when refreshing old thumbnails
CREATE INDEX idx_card_mappings_thumbnail_url ON card_mappings(thumbnail_url);
