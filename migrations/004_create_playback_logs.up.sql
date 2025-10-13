-- Create playback_logs table for tracking media playback events
CREATE TABLE IF NOT EXISTS playback_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    tag_id TEXT NOT NULL,
    media_id TEXT NOT NULL,
    media_title TEXT NOT NULL,
    played_at DATETIME NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Create index for efficient queries by user_id
CREATE INDEX IF NOT EXISTS idx_playback_logs_user_id ON playback_logs(user_id);

-- Create index for efficient queries by played_at
CREATE INDEX IF NOT EXISTS idx_playback_logs_played_at ON playback_logs(played_at);
