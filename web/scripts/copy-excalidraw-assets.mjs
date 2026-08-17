import { cpSync, mkdirSync, statSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const source = fileURLToPath(new URL('../node_modules/@excalidraw/excalidraw/dist/prod/fonts/', import.meta.url))
const destination = fileURLToPath(new URL('../public/excalidraw-assets/fonts/', import.meta.url))

if (!statSync(source).isDirectory()) {
  throw new Error('Excalidraw font directory is unavailable; run npm ci first')
}

mkdirSync(destination, { recursive: true })
cpSync(source, destination, { recursive: true, force: true })
