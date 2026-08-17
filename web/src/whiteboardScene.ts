export function loadExcalidrawScene<T>(load: () => T, onError?: (error: Error) => void): T | null {
  try {
    return load()
  } catch (caught) {
    onError?.(caught instanceof Error ? caught : new Error('白板场景无法读取'))
    return null
  }
}
