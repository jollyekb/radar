import { Cpu, Server, Network } from 'lucide-react'
import { Section, PropertyList, Property, ConditionsSection, AlertBanner, ResourceLink } from '../../ui/drawer-components'
import { getMachineStatus, getMachineRole, getMachineClusterName, getMachineNodeRef, getMachineVersion, getMachineProviderID } from '../resource-utils-capi'

interface Props {
  data: any
  onNavigate?: (ref: { kind: string; namespace: string; name: string; group?: string }) => void
}

export function CAPIMachineRenderer({ data, onNavigate }: Props) {
  const status = data.status || {}
  const spec = data.spec || {}
  const conditions = status.v1beta2?.conditions || status.conditions || []

  const machineStatus = getMachineStatus(data)
  const isFailed = machineStatus.level === 'unhealthy'
  const readyCond = conditions.find((c: any) => c.type === 'Ready')

  const phase = status.phase || 'Unknown'
  const role = getMachineRole(data)
  const clusterName = getMachineClusterName(data)
  const nodeName = getMachineNodeRef(data)
  const version = getMachineVersion(data)
  const providerID = getMachineProviderID(data)
  const addresses = status.addresses || []
  const nodeInfo = status.nodeInfo || {}
  const nodeRef = status.nodeRef || {}
  const bootstrapRef = spec.bootstrap?.configRef || {}
  const infraRef = spec.infrastructureRef || {}

  return (
    <>
      {isFailed && (
        <AlertBanner
          variant="error"
          title="Machine Not Ready"
          message={readyCond?.message || `Machine is in ${phase} state.`}
        />
      )}

      {/* Overview */}
      <Section title="Overview" icon={Cpu}>
        <PropertyList>
          <Property label="Phase" value={phase} />
          <Property label="Role" value={role} />
          <Property label="Cluster" value={clusterName} />
          <Property label="Version" value={version} />
          {providerID !== '-' && <Property label="Provider ID" value={providerID} />}
          {spec.failureDomain && <Property label="Failure Domain" value={spec.failureDomain} />}
        </PropertyList>
      </Section>

      {/* Node Reference */}
      {nodeName !== '-' && (
        <Section title="Node" icon={Server}>
          <PropertyList>
            <Property
              label="Name"
              value={
                <ResourceLink
                  name={nodeName}
                  kind="nodes"
                  namespace=""
                  label={nodeName}
                  onNavigate={onNavigate}
                />
              }
            />
            {nodeRef.uid && <Property label="UID" value={nodeRef.uid} />}
          </PropertyList>
        </Section>
      )}

      {/* References */}
      {(bootstrapRef.kind || infraRef.kind) && (
        <Section title="References" icon={Network}>
          <PropertyList>
            {bootstrapRef.kind && (
              <Property label="Bootstrap" value={`${bootstrapRef.kind}/${bootstrapRef.name}`} />
            )}
            {infraRef.kind && (
              <Property label="Infrastructure" value={`${infraRef.kind}/${infraRef.name}`} />
            )}
          </PropertyList>
        </Section>
      )}

      {/* Addresses */}
      {addresses.length > 0 && (
        <Section title="Addresses" icon={Network}>
          <table className="w-full text-xs">
            <thead>
              <tr className="text-theme-text-tertiary">
                <th className="text-left font-medium py-1">Type</th>
                <th className="text-left font-medium py-1">Address</th>
              </tr>
            </thead>
            <tbody>
              {addresses.map((addr: any, i: number) => (
                <tr key={i} className="border-t border-theme-border">
                  <td className="py-1 text-theme-text-secondary">{addr.type}</td>
                  <td className="py-1 text-theme-text-secondary font-mono">{addr.address}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Section>
      )}

      {/* Node Info */}
      {nodeInfo.kubeletVersion && (
        <Section title="Node Info" icon={Server}>
          <PropertyList>
            {nodeInfo.osImage && <Property label="OS Image" value={nodeInfo.osImage} />}
            {nodeInfo.architecture && <Property label="Architecture" value={nodeInfo.architecture} />}
            {nodeInfo.kernelVersion && <Property label="Kernel" value={nodeInfo.kernelVersion} />}
            {nodeInfo.containerRuntimeVersion && <Property label="Container Runtime" value={nodeInfo.containerRuntimeVersion} />}
            {nodeInfo.kubeletVersion && <Property label="Kubelet" value={nodeInfo.kubeletVersion} />}
          </PropertyList>
        </Section>
      )}

      <ConditionsSection conditions={conditions} />
    </>
  )
}
