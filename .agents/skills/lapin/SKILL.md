---
name: lapin
description: Import or update a Lapin course from local Markdown or PDF documents through the repository's lapin-cli. Use when a user asks to turn course notes, chapter Markdown files, PDFs, or related local documents into a Lapin course, preserve a recursive chapter tree, upload local images, or repeat an existing manifest-based course import with stable external IDs.
---

# Lapin Course Import

Use `lapin-cli` as the only data-operation boundary. Create or update the manifest and Markdown files, then invoke the CLI. Never reproduce its file-loading or HTTP logic with `curl`, ad-hoc scripts, or direct API calls.

## Workflow

1. Work from the Lapin repository root. Build the CLI if `bin/lapin-cli` is missing or stale:

   ```bash
   make build-cli
   ```

2. Confirm `LAPIN_ACCESS_TOKEN` exists without printing or copying its value. If it is absent, stop and ask the user to create a Token in Lapin and export it in the current environment. Never read it from a file unless the user explicitly owns and identifies that secret source.

3. Inspect the supplied documents and design one recursive course tree. Copy [course-manifest.example.json](assets/course-manifest.example.json) when a Markdown starting template helps. For a PDF, generate an inspectable bundle first:

   ```bash
   ./bin/lapin-cli course prepare-pdf --pdf /absolute/path/book.pdf --output /tmp/book-bundle --external-id stable-id --title 'Course title' --profile zh-technical-book
   ```

   This requires Poppler's `pdftohtml` and `pdftocairo`. Use `--profile generic-book` for books that do not follow Chinese “第 N 章 / 图 N-N” conventions. Review the generated Markdown and image crops before importing `/tmp/book-bundle/course.json`; PDF tables, columns, and code blocks may need correction.

   For a course that already exists, pass the last reviewed manifest with `--reuse-chapter-tree /absolute/path/to/previous/course.json`. This preserves stable external IDs and grouping for title-matched chapters. Review unmatched titles before importing so a renamed chapter is not appended as a duplicate.

4. Keep every course and chapter `external_id` stable across edits, moves, and title changes. Assign a new ID only to genuinely new content. Keep `content_file` paths relative to the manifest directory.

5. Run exactly one CLI import:

   ```bash
   ./bin/lapin-cli course import --manifest /absolute/path/to/course.json
   ```

   Set `LAPIN_BASE_URL` when the server is not `http://127.0.0.1:8080`. Non-loopback servers must use HTTPS. Standard `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` variables are honored by the default Go HTTP client.

6. Report the structured success summary or the sanitized CLI error. Do not expose request headers, the environment, or the Token.

## Manifest Rules

- Use JSON with `version: 1` for Markdown-only courses. Use `version: 2` when the bundle has local assets or needs staged requests.
- Provide non-empty `external_id` and `title` for the course and every chapter.
- Use `content_file` for Markdown; omit it only for an intentionally empty grouping chapter.
- Use recursive `children` arrays; their order is the imported chapter order.
- In version 2, declare each PNG/JPEG in `assets` with a stable unique `key` and relative `file`, then reference it from Markdown as `lapin-asset://<key>`.
- Do not add inline `content`, unknown fields, absolute paths, or paths outside the manifest directory.
- Keep at most 100 chapters and 300 assets. The CLI enforces the API's title, description, tag, content, request-size, image-size, UTF-8, and path-safety limits before sending. Large imports are staged and only become visible after the final commit.

Repeated imports update matching external IDs and preserve their database IDs, annotations, whiteboards, and discussions. An imported title or body can overwrite later Web edits. Omitting an old chapter does not delete it.

## Hard Boundaries

- Never pass a Token as a command argument or write it to a manifest, script, log, or answer.
- Never generate replacement external IDs from changed titles.
- Never bypass `lapin-cli` with direct HTTP requests.
- Never assume an omitted chapter will be deleted.
