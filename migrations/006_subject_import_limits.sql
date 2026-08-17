CREATE UNIQUE INDEX IF NOT EXISTS subject_imports_owner_external_draft_idx
    ON subject_imports(owner_id, external_id)
    WHERE status = 'draft';
