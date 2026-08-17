CREATE TABLE IF NOT EXISTS subject_imports (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    idempotency_key TEXT NOT NULL,
    external_id TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    tags JSONB NOT NULL DEFAULT '[]'::jsonb,
    expected_chapters INTEGER NOT NULL,
    expected_assets INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'committed', 'aborted')),
    subject_id BIGINT REFERENCES subjects(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    committed_at TIMESTAMPTZ,
    UNIQUE (owner_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS subject_imports_owner_status_idx ON subject_imports(owner_id, status, updated_at DESC);

CREATE TABLE IF NOT EXISTS subject_import_batches (
    import_id BIGINT NOT NULL REFERENCES subject_imports(id) ON DELETE CASCADE,
    batch_key TEXT NOT NULL,
    digest CHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (import_id, batch_key)
);

CREATE TABLE IF NOT EXISTS subject_import_chapters (
    import_id BIGINT NOT NULL REFERENCES subject_imports(id) ON DELETE CASCADE,
    external_id TEXT NOT NULL,
    parent_external_id TEXT,
    position INTEGER NOT NULL,
    title TEXT NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    content_sha256 CHAR(64) NOT NULL,
    PRIMARY KEY (import_id, external_id),
    UNIQUE (import_id, position)
);

CREATE TABLE IF NOT EXISTS subject_import_assets (
    import_id BIGINT NOT NULL REFERENCES subject_imports(id) ON DELETE CASCADE,
    asset_key TEXT NOT NULL,
    asset_id BIGINT NOT NULL REFERENCES assets(id) ON DELETE RESTRICT,
    PRIMARY KEY (import_id, asset_key)
);
