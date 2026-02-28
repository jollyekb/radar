import type { FileNode } from '../../types'

/** Trigger a browser file download from a Blob. */
export function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

/** Recursively filter a FileNode tree by name substring match. */
export function filterTree(node: FileNode, query: string): FileNode | null {
  if (node.name.toLowerCase().includes(query)) {
    return node
  }

  if (node.type === 'dir' && node.children) {
    const filteredChildren = node.children
      .map((child) => filterTree(child, query))
      .filter((child): child is FileNode => child !== null)

    if (filteredChildren.length > 0) {
      return { ...node, children: filteredChildren }
    }
  }

  return null
}
