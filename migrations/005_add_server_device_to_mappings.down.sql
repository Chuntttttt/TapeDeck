-- SQLite doesn't support DROP COLUMN directly, so we need to recreate the table
CREATE TABLE card_mappings_backup (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    tag_id TEXT NOT NULL,
    media_type TEXT NOT NULL,
    media_id TEXT NOT NULL,
    media_title TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE(user_id, tag_id)
);

INSERT INTO card_mappings_backup (id, user_id, tag_id, media_type, media_id, media_title, created_at, updated_at)
SELECT id, user_id, tag_id, media_type, media_id, media_title, created_at, updated_at
FROM card_mappings;

DROP TABLE card_mappings;
ALTER TABLE card_mappings_backup RENAME TO card_mappings;

CREATE INDEX idx_card_mappings_user_id ON card_mappings(user_id);
CREATE INDEX idx_card_mappings_tag_id ON card_mappings(tag_id);
