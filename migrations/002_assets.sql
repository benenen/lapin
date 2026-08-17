CREATE TABLE IF NOT EXISTS assets (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    sha256 CHAR(64) NOT NULL,
    mime_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    width INTEGER NOT NULL,
    height INTEGER NOT NULL,
    lease_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (owner_id, sha256)
);

CREATE INDEX IF NOT EXISTS assets_sha256_idx ON assets(sha256);

CREATE TABLE IF NOT EXISTS chapter_assets (
    chapter_id BIGINT NOT NULL REFERENCES chapters(id) ON DELETE CASCADE,
    asset_id BIGINT NOT NULL REFERENCES assets(id) ON DELETE RESTRICT,
    PRIMARY KEY (chapter_id, asset_id)
);

CREATE INDEX IF NOT EXISTS chapter_assets_asset_id_idx ON chapter_assets(asset_id);
