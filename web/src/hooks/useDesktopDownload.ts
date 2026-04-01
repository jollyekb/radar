import { useState, useEffect, useCallback } from 'react'
import { useToast } from '../components/ui/Toast'
import { isDesktopApp, desktopSaveFile } from '../utils/desktop-download'

/**
 * Returns a download override function when running in the desktop app,
 * or undefined when running in a browser (so the default blob URL approach is used).
 */
export function useDesktopDownload(): ((content: string, mime: string, filename: string) => void) | undefined {
  const [isDesktop, setIsDesktop] = useState(false)
  const { showSuccess, showError } = useToast()

  useEffect(() => {
    isDesktopApp().then(setIsDesktop)
  }, [])

  const download = useCallback((content: string, _mime: string, filename: string) => {
    desktopSaveFile(content, filename)
      .then((path) => showSuccess('File saved', path))
      .catch((err: Error) => {
        if (err.message !== 'cancelled') {
          showError('Save failed', err.message)
        }
      })
  }, [showSuccess, showError])

  if (!isDesktop) return undefined
  return download
}
