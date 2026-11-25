CREATE TABLE IF NOT EXISTS bonus_grants (
	child_id TEXT NOT NULL,
	app_id TEXT NOT NULL,
	granted_at INTEGER NOT NULL,
	duration INTEGER NOT NULL CHECK (duration > 0)
);
CREATE INDEX IF NOT EXISTS bonus_grants_by_child_time
	ON bonus_grants (child_id, granted_at);

CREATE TABLE IF NOT EXISTS bonus_codes (
	digest BLOB PRIMARY KEY NOT NULL CHECK (length(digest) = 32),
	child_id TEXT NOT NULL,
	app_id TEXT NOT NULL,
	issued_at INTEGER NOT NULL,
	expires_at INTEGER NOT NULL,
	duration INTEGER NOT NULL CHECK (duration > 0),
	redeemed INTEGER NOT NULL DEFAULT 0 CHECK (redeemed IN (0, 1))
);

CREATE TABLE IF NOT EXISTS bonus_redemption_attempts (
	child_id TEXT PRIMARY KEY NOT NULL,
	window_start INTEGER NOT NULL,
	failures INTEGER NOT NULL CHECK (failures > 0)
);

CREATE TABLE IF NOT EXISTS daily_usage (
	child_id TEXT NOT NULL,
	local_day TEXT NOT NULL,
	app_id TEXT NOT NULL,
	actual INTEGER NOT NULL,
	regular INTEGER NOT NULL,
	bonus INTEGER NOT NULL,
	PRIMARY KEY (child_id, local_day, app_id)
);
