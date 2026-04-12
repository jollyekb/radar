// Azure CAPI Infrastructure Provider cell components for ResourcesView table

import { Tooltip } from '../../ui/Tooltip'
import { clsx } from 'clsx'
import {
  getAzureMCPStatus, getAzureMCPLocation, getAzureMCPVersion, getAzureMCPResourceGroup,
  getAzureMMPStatus, getAzureMMPSKU, getAzureMMPMode, getAzureMMPReplicas, getAzureMMPScaleSetPriority,
  getAzureMachineStatus, getAzureMachineVMSize,
  getAzureMTVMSize,
  getAzureManagedClusterStatus,
} from '../resource-utils-azure-capi'

function StatusBadge({ resource, getStatus }: { resource: any; getStatus: (r: any) => { text: string; color: string } }) {
  const status = getStatus(resource)
  return (
    <Tooltip content={status.text}>
      <span className={clsx('badge truncate max-w-[140px]', status.color)}>{status.text}</span>
    </Tooltip>
  )
}

function TextCell({ value }: { value: string }) {
  return <span className="text-sm text-theme-text-secondary">{value}</span>
}

export function AzureManagedControlPlaneCell({ resource, column }: { resource: any; column: string }) {
  switch (column) {
    case 'status': return <StatusBadge resource={resource} getStatus={getAzureMCPStatus} />
    case 'location': return <TextCell value={getAzureMCPLocation(resource)} />
    case 'resourceGroup': return <TextCell value={getAzureMCPResourceGroup(resource)} />
    case 'version': return <TextCell value={getAzureMCPVersion(resource)} />
    default: return <span className="text-sm text-theme-text-tertiary">-</span>
  }
}

export function AzureManagedMachinePoolCell({ resource, column }: { resource: any; column: string }) {
  switch (column) {
    case 'status': return <StatusBadge resource={resource} getStatus={getAzureMMPStatus} />
    case 'sku': return <TextCell value={getAzureMMPSKU(resource)} />
    case 'mode': {
      const mode = getAzureMMPMode(resource)
      return (
        <span className={clsx('badge badge-sm', mode === 'System'
          ? 'bg-purple-100 text-purple-800 border-purple-300 dark:bg-purple-950/50 dark:text-purple-400 dark:border-purple-700/40'
          : 'bg-sky-100 text-sky-700 border-sky-300 dark:bg-sky-950/50 dark:text-sky-400 dark:border-sky-700/40'
        )}>{mode}</span>
      )
    }
    case 'replicas': return <TextCell value={getAzureMMPReplicas(resource)} />
    case 'priority': {
      const p = getAzureMMPScaleSetPriority(resource)
      return (
        <span className={clsx('badge badge-sm', p === 'Spot'
          ? 'bg-amber-100 text-amber-800 border-amber-300 dark:bg-amber-950/50 dark:text-amber-400 dark:border-amber-700/40'
          : 'bg-sky-100 text-sky-700 border-sky-300 dark:bg-sky-950/50 dark:text-sky-400 dark:border-sky-700/40'
        )}>{p}</span>
      )
    }
    default: return <span className="text-sm text-theme-text-tertiary">-</span>
  }
}

export function AzureMachineCell({ resource, column }: { resource: any; column: string }) {
  switch (column) {
    case 'status': return <StatusBadge resource={resource} getStatus={getAzureMachineStatus} />
    case 'vmSize': return <TextCell value={getAzureMachineVMSize(resource)} />
    default: return <span className="text-sm text-theme-text-tertiary">-</span>
  }
}

export function AzureMachineTemplateCell({ resource, column }: { resource: any; column: string }) {
  switch (column) {
    case 'vmSize': return <TextCell value={getAzureMTVMSize(resource)} />
    default: return <span className="text-sm text-theme-text-tertiary">-</span>
  }
}

export function AzureManagedClusterCell({ resource, column }: { resource: any; column: string }) {
  switch (column) {
    case 'status': return <StatusBadge resource={resource} getStatus={getAzureManagedClusterStatus} />
    default: return <span className="text-sm text-theme-text-tertiary">-</span>
  }
}
