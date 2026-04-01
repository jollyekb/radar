/** Reveal a file in the system file manager (Finder on macOS, Explorer on Windows). */
export function openFolder(path: string): void {
  fetch('/api/desktop/open-folder', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path }),
  }).catch((err) => console.warn('[desktop] Failed to open folder:', err))
}

/** Open a file with the system default application. */
export function openFile(path: string): void {
  fetch('/api/desktop/open-file', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path }),
  }).catch((err) => console.warn('[desktop] Failed to open file:', err))
}
