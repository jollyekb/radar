// Desktop file-save utilities.
// In the desktop app (Wails), blob URL downloads are silently ignored by
// WKWebView / WebView2. These helpers route downloads through a backend
// endpoint that shows the native OS save dialog instead.

import { fetchJSON } from '../api/client'

let desktopCheck: Promise<boolean> | null = null

/** Returns true when running inside the desktop (Wails) app. Cached after first call. */
export function isDesktopApp(): Promise<boolean> {
  if (!desktopCheck) {
    desktopCheck = fetchJSON<{ isDesktop: boolean }>('/config')
      .then((d) => d.isDesktop ?? false)
      .catch(() => false)
  }
  return desktopCheck
}

/** Save text content via native save dialog. Returns the chosen file path. */
export async function desktopSaveFile(content: string, filename: string): Promise<string> {
  const res = await fetch('/api/desktop/save-file', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content, filename }),
  })
  if (res.status === 204) throw new Error('cancelled')
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: 'Save failed' }))
    throw new Error(body.error ?? 'Save failed')
  }
  const body = await res.json()
  return body.path
}

/** Save a Blob via native save dialog. Returns the chosen file path. */
export async function desktopSaveBlob(blob: Blob, filename: string): Promise<string> {
  const buf = await blob.arrayBuffer()
  const bytes = new Uint8Array(buf)
  let binary = ''
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i])
  }
  const contentBase64 = btoa(binary)

  const res = await fetch('/api/desktop/save-file', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ contentBase64, filename }),
  })
  if (res.status === 204) throw new Error('cancelled')
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: 'Save failed' }))
    throw new Error(body.error ?? 'Save failed')
  }
  const body = await res.json()
  return body.path
}
