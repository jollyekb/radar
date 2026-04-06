import { useState, useEffect } from 'react'
import { X, Plus, Trash2 } from 'lucide-react'
import { useAuditSettings, useUpdateAuditSettings, useAudit } from '../../api/client'
import type { CheckMeta } from '@skyhook-io/k8s-ui'

interface AuditSettingsDialogProps {
  namespaces: string[]
  onClose: () => void
}

export function AuditSettingsDialog({ namespaces, onClose }: AuditSettingsDialogProps) {
  const { data: settings } = useAuditSettings()
  const { data: auditData } = useAudit(namespaces)
  const updateSettings = useUpdateAuditSettings()
  const [ignoredNs, setIgnoredNs] = useState<string[]>([])
  const [disabledChecks, setDisabledChecks] = useState<string[]>([])
  const [newNs, setNewNs] = useState('')

  useEffect(() => {
    if (settings) {
      setIgnoredNs(settings.ignoredNamespaces || [])
      setDisabledChecks(settings.disabledChecks || [])
    }
  }, [settings])

  // Get all available checks from the audit response
  const allChecks: CheckMeta[] = auditData?.checks
    ? Object.values(auditData.checks).sort((a, b) => a.title.localeCompare(b.title))
    : []

  const addNamespace = () => {
    const ns = newNs.trim()
    if (ns && !ignoredNs.includes(ns)) {
      setIgnoredNs([...ignoredNs, ns])
      setNewNs('')
    }
  }

  const toggleCheck = (checkID: string) => {
    if (disabledChecks.includes(checkID)) {
      setDisabledChecks(disabledChecks.filter(c => c !== checkID))
    } else {
      setDisabledChecks([...disabledChecks, checkID])
    }
  }

  const handleSave = () => {
    updateSettings.mutate(
      { ignoredNamespaces: ignoredNs, disabledChecks },
      { onSuccess: () => onClose() },
    )
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={onClose}>
      <div className="bg-theme-surface rounded-xl shadow-xl w-full max-w-lg mx-4 max-h-[80vh] flex flex-col" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between px-5 py-4 border-b border-theme-border shrink-0">
          <h2 className="text-sm font-semibold text-theme-text-primary">Audit Settings</h2>
          <button onClick={onClose} className="p-1 rounded-lg hover:bg-theme-hover transition-colors">
            <X className="w-4 h-4 text-theme-text-tertiary" />
          </button>
        </div>

        <div className="px-5 py-4 overflow-y-auto flex-1">
          {/* Ignored Namespaces */}
          <div className="mb-6">
            <label className="text-xs font-medium text-theme-text-secondary uppercase tracking-wider">
              Ignored Namespaces
            </label>
            <p className="text-xs text-theme-text-tertiary mt-1 mb-3">
              Findings in these namespaces are hidden from all views.
            </p>

            <div className="flex flex-col gap-1.5 mb-3">
              {ignoredNs.map(ns => (
                <div key={ns} className="flex items-center justify-between px-3 py-1.5 bg-theme-elevated rounded-lg">
                  <span className="text-sm text-theme-text-primary">{ns}</span>
                  <button
                    onClick={() => setIgnoredNs(ignoredNs.filter(n => n !== ns))}
                    className="p-1 rounded hover:bg-theme-hover text-theme-text-tertiary hover:text-red-400 transition-colors"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </div>
              ))}
              {ignoredNs.length === 0 && (
                <div className="text-xs text-theme-text-tertiary py-2">No namespaces ignored.</div>
              )}
            </div>

            <div className="flex gap-2">
              <input
                type="text"
                value={newNs}
                onChange={e => setNewNs(e.target.value)}
                onKeyDown={e => { if (e.key === 'Enter') addNamespace() }}
                placeholder="Add namespace..."
                className="flex-1 px-3 py-1.5 bg-theme-elevated border border-theme-border-light rounded-lg text-sm text-theme-text-primary placeholder-theme-text-disabled focus:outline-none focus:ring-2 focus:ring-skyhook-500"
              />
              <button
                onClick={addNamespace}
                disabled={!newNs.trim()}
                className="px-3 py-1.5 text-sm btn-brand rounded-lg disabled:opacity-50 disabled:cursor-not-allowed"
              >
                <Plus className="w-4 h-4" />
              </button>
            </div>
          </div>

          {/* Disabled Checks */}
          <div>
            <label className="text-xs font-medium text-theme-text-secondary uppercase tracking-wider">
              Enabled Checks
            </label>
            <p className="text-xs text-theme-text-tertiary mt-1 mb-3">
              Uncheck to disable specific checks globally across all views.
            </p>

            <div className="flex flex-col gap-0.5">
              {allChecks.map(check => {
                const disabled = disabledChecks.includes(check.id)
                return (
                  <label
                    key={check.id}
                    className="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-theme-hover/50 cursor-pointer transition-colors"
                  >
                    <input
                      type="checkbox"
                      checked={!disabled}
                      onChange={() => toggleCheck(check.id)}
                      className="w-4 h-4 rounded border-theme-border text-skyhook-500 focus:ring-skyhook-500"
                    />
                    <div className="flex-1 min-w-0">
                      <span className="text-sm text-theme-text-primary">{check.title}</span>
                      <p className="text-xs text-theme-text-tertiary truncate">{check.description}</p>
                    </div>
                  </label>
                )
              })}
            </div>
          </div>
        </div>

        <div className="flex justify-end gap-2 px-5 py-3 border-t border-theme-border shrink-0">
          <button
            onClick={onClose}
            className="px-4 py-1.5 text-sm text-theme-text-secondary hover:text-theme-text-primary bg-theme-elevated hover:bg-theme-hover border border-theme-border rounded-lg transition-colors"
          >
            Cancel
          </button>
          <button
            onClick={handleSave}
            disabled={updateSettings.isPending}
            className="px-4 py-1.5 text-sm btn-brand rounded-lg disabled:opacity-50"
          >
            {updateSettings.isPending ? 'Saving...' : 'Save'}
          </button>
        </div>
      </div>
    </div>
  )
}
