/**
 * Triggers a file download in the browser or desktop webview.
 * When `overrideFn` is provided (e.g. for native desktop save dialogs),
 * it is called instead of the default blob URL approach.
 */
export function triggerDownload(
  content: string,
  mime: string,
  filename: string,
  overrideFn?: (content: string, mime: string, filename: string) => void,
): void {
  if (overrideFn) {
    overrideFn(content, mime, filename)
    return
  }

  const blob = new Blob([content], { type: mime })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')

  a.href = url
  a.download = filename
  a.style.display = 'none'

  document.body.appendChild(a)
  a.click()

  // Delay cleanup so embedded webviews still have time to consume the blob URL.
  window.setTimeout(() => {
    URL.revokeObjectURL(url)
    a.remove()
  }, 1000)
}
