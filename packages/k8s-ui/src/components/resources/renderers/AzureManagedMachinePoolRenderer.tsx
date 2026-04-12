import { Server, Settings } from 'lucide-react'
import { clsx } from 'clsx'
import { Section, PropertyList, Property, ConditionsSection, AlertBanner } from '../../ui/drawer-components'
import { getCAPIConditions } from '../resource-utils-capi'
import { getAzureMMPStatus, getAzureMMPSKU, getAzureMMPMode, getAzureMMPOSDiskInfo, getAzureMMPScaling, getAzureMMPScaleSetPriority, getAzureMMPOSType } from '../resource-utils-azure-capi'

interface Props {
  data: any
  onNavigate?: (ref: { kind: string; namespace: string; name: string; group?: string }) => void
}

export function AzureManagedMachinePoolRenderer({ data }: Props) {
  const spec = data.spec || {}
  const status = data.status || {}
  const conditions = getCAPIConditions(data)
  const mmpStatus = getAzureMMPStatus(data)
  const isFailed = mmpStatus.level === 'unhealthy'
  const readyCond = conditions.find((c: any) => c.type === 'Ready')
  const scaling = getAzureMMPScaling(data)
  const mode = getAzureMMPMode(data)
  const priority = getAzureMMPScaleSetPriority(data)
  const labels = spec.nodeLabels || {}
  const taints = spec.taints || []
  const zones = spec.availabilityZones || []

  return (
    <>
      {isFailed && (
        <AlertBanner variant="error" title="AKS Node Pool Not Ready" message={readyCond?.message || 'AzureManagedMachinePool is not ready.'} />
      )}

      <Section title="Overview" icon={Server}>
        <PropertyList>
          {spec.name && <Property label="Pool Name" value={spec.name} />}
          <Property label="VM Size" value={getAzureMMPSKU(data)} />
          <Property label="Mode" value={
            <span className={clsx('badge badge-sm', mode === 'System'
              ? 'bg-purple-100 text-purple-800 border-purple-300 dark:bg-purple-950/50 dark:text-purple-400 dark:border-purple-700/40'
              : 'bg-sky-100 text-sky-700 border-sky-300 dark:bg-sky-950/50 dark:text-sky-400 dark:border-sky-700/40'
            )}>{mode}</span>
          } />
          <Property label="OS" value={getAzureMMPOSType(data)} />
          <Property label="OS Disk" value={getAzureMMPOSDiskInfo(data)} />
          <Property label="Priority" value={
            <span className={clsx('badge badge-sm', priority === 'Spot'
              ? 'bg-amber-100 text-amber-800 border-amber-300 dark:bg-amber-950/50 dark:text-amber-400 dark:border-amber-700/40'
              : 'bg-sky-100 text-sky-700 border-sky-300 dark:bg-sky-950/50 dark:text-sky-400 dark:border-sky-700/40'
            )}>{priority}</span>
          } />
          {spec.maxPods != null && <Property label="Max Pods" value={String(spec.maxPods)} />}
        </PropertyList>
      </Section>

      <Section title="Scaling" icon={Settings}>
        <PropertyList>
          <Property label="Min Size" value={String(scaling.min)} />
          <Property label="Max Size" value={String(scaling.max)} />
          <Property label="Current Replicas" value={String(status.replicas ?? 0)} />
          {spec.scaleDownMode && <Property label="Scale Down Mode" value={spec.scaleDownMode} />}
        </PropertyList>
      </Section>

      {/* Availability Zones */}
      {zones.length > 0 && (
        <Section title="Availability Zones" icon={Server}>
          <div className="flex flex-wrap gap-1">
            {zones.map((z: string) => (
              <span key={z} className="badge badge-sm bg-theme-elevated text-theme-text-secondary border-theme-border">{z}</span>
            ))}
          </div>
        </Section>
      )}

      {/* Labels */}
      {Object.keys(labels).length > 0 && (
        <Section title="Node Labels" icon={Settings}>
          <div className="flex flex-wrap gap-1">
            {Object.entries(labels).map(([k, v]) => (
              <span key={k} className="badge badge-sm bg-theme-elevated text-theme-text-secondary border-theme-border text-[10px]">{k}={v as string}</span>
            ))}
          </div>
        </Section>
      )}

      {/* Taints */}
      {taints.length > 0 && (
        <Section title="Taints" icon={Settings}>
          <table className="w-full text-xs">
            <thead><tr className="text-theme-text-tertiary"><th className="text-left font-medium py-1">Key</th><th className="text-left font-medium py-1">Value</th><th className="text-left font-medium py-1">Effect</th></tr></thead>
            <tbody>
              {taints.map((t: any, i: number) => (
                <tr key={i} className="border-t border-theme-border">
                  <td className="py-1 text-theme-text-secondary">{t.key}</td>
                  <td className="py-1 text-theme-text-secondary">{t.value}</td>
                  <td className="py-1 text-theme-text-secondary">{t.effect}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Section>
      )}

      <ConditionsSection conditions={conditions} />
    </>
  )
}
