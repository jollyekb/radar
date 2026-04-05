import { Shield, ArrowDownToLine, ArrowUpFromLine, Ban } from 'lucide-react'
import { Section, PropertyList, Property } from '../../ui/drawer-components'

interface CiliumNetworkPolicyRendererProps {
  data: any
}

export function CiliumNetworkPolicyRenderer({ data }: CiliumNetworkPolicyRendererProps) {
  const spec = data.spec || data.specs || {}
  const endpointSelector = spec.endpointSelector || {}
  const matchLabels = endpointSelector.matchLabels || {}
  const hasMatchLabels = Object.keys(matchLabels).length > 0
  const ingress: any[] | undefined = spec.ingress
  const ingressDeny: any[] | undefined = spec.ingressDeny
  const egress: any[] | undefined = spec.egress
  const egressDeny: any[] | undefined = spec.egressDeny
  const description = spec.description || ''

  return (
    <>
      <Section title="Target" icon={Shield}>
        {description && (
          <p className="text-xs text-theme-text-secondary mb-2">{description}</p>
        )}
        <PropertyList>
          <Property
            label="Endpoint Selector"
            value={hasMatchLabels ? undefined : 'All endpoints in namespace'}
          />
        </PropertyList>
        {hasMatchLabels && (
          <div className="mt-2">
            <div className="text-xs text-theme-text-tertiary mb-1">Endpoint Selector</div>
            <div className="flex flex-wrap gap-1">
              {Object.entries(matchLabels).map(([k, v]) => (
                <span key={k} className="badge bg-theme-elevated text-theme-text-secondary">
                  {k}={String(v)}
                </span>
              ))}
            </div>
          </div>
        )}
        {spec.nodeSelector && (
          <div className="mt-2">
            <div className="text-xs text-theme-text-tertiary mb-1">Node Selector</div>
            <div className="flex flex-wrap gap-1">
              {Object.entries(spec.nodeSelector.matchLabels || {}).map(([k, v]) => (
                <span key={k} className="badge bg-theme-elevated text-theme-text-secondary">
                  {k}={String(v)}
                </span>
              ))}
            </div>
          </div>
        )}
      </Section>

      {ingress && ingress.length > 0 && (
        <Section title="Ingress Allow" icon={ArrowDownToLine} defaultExpanded>
          <div className="space-y-3">
            {ingress.map((rule: any, i: number) => (
              <CiliumRuleCard key={i} rule={rule} />
            ))}
          </div>
        </Section>
      )}

      {ingressDeny && ingressDeny.length > 0 && (
        <Section title="Ingress Deny" icon={Ban} defaultExpanded>
          <div className="space-y-3">
            {ingressDeny.map((rule: any, i: number) => (
              <CiliumRuleCard key={i} rule={rule} />
            ))}
          </div>
        </Section>
      )}

      {egress && egress.length > 0 && (
        <Section title="Egress Allow" icon={ArrowUpFromLine} defaultExpanded>
          <div className="space-y-3">
            {egress.map((rule: any, i: number) => (
              <CiliumRuleCard key={i} rule={rule} />
            ))}
          </div>
        </Section>
      )}

      {egressDeny && egressDeny.length > 0 && (
        <Section title="Egress Deny" icon={Ban} defaultExpanded>
          <div className="space-y-3">
            {egressDeny.map((rule: any, i: number) => (
              <CiliumRuleCard key={i} rule={rule} />
            ))}
          </div>
        </Section>
      )}
    </>
  )
}

function CiliumRuleCard({ rule }: { rule: any }) {
  const fromEndpoints: any[] = rule.fromEndpoints || []
  const fromEntities: string[] = rule.fromEntities || []
  const fromCIDR: string[] = rule.fromCIDR || []
  const fromCIDRSet: any[] = rule.fromCIDRSet || []
  const toEndpoints: any[] = rule.toEndpoints || []
  const toEntities: string[] = rule.toEntities || []
  const toCIDR: string[] = rule.toCIDR || []
  const toCIDRSet: any[] = rule.toCIDRSet || []
  const toPorts: any[] = rule.toPorts || []

  return (
    <div className="card-inner-lg">
      {fromEndpoints.length > 0 && (
        <div className="mb-2">
          <div className="text-xs text-theme-text-tertiary mb-1">From Endpoints</div>
          <div className="space-y-1">
            {fromEndpoints.map((ep: any, j: number) => (
              <SelectorEntry key={j} selector={ep} />
            ))}
          </div>
        </div>
      )}

      {fromEntities.length > 0 && (
        <div className="mb-2">
          <div className="text-xs text-theme-text-tertiary mb-1">From Entities</div>
          <div className="flex flex-wrap gap-1">
            {fromEntities.map((e, j) => (
              <span key={j} className="badge bg-purple-500/20 text-purple-400 border-purple-500/30">{e}</span>
            ))}
          </div>
        </div>
      )}

      {(fromCIDR.length > 0 || fromCIDRSet.length > 0) && (
        <div className="mb-2">
          <div className="text-xs text-theme-text-tertiary mb-1">From CIDR</div>
          <div className="flex flex-wrap gap-1">
            {fromCIDR.map((c, j) => (
              <span key={j} className="badge bg-theme-elevated text-theme-text-secondary">{c}</span>
            ))}
            {fromCIDRSet.map((cs: any, j: number) => (
              <span key={`set-${j}`} className="badge bg-theme-elevated text-theme-text-secondary">{cs.cidr}</span>
            ))}
          </div>
        </div>
      )}

      {toEndpoints.length > 0 && (
        <div className="mb-2">
          <div className="text-xs text-theme-text-tertiary mb-1">To Endpoints</div>
          <div className="space-y-1">
            {toEndpoints.map((ep: any, j: number) => (
              <SelectorEntry key={j} selector={ep} />
            ))}
          </div>
        </div>
      )}

      {toEntities.length > 0 && (
        <div className="mb-2">
          <div className="text-xs text-theme-text-tertiary mb-1">To Entities</div>
          <div className="flex flex-wrap gap-1">
            {toEntities.map((e, j) => (
              <span key={j} className="badge bg-purple-500/20 text-purple-400 border-purple-500/30">{e}</span>
            ))}
          </div>
        </div>
      )}

      {(toCIDR.length > 0 || toCIDRSet.length > 0) && (
        <div className="mb-2">
          <div className="text-xs text-theme-text-tertiary mb-1">To CIDR</div>
          <div className="flex flex-wrap gap-1">
            {toCIDR.map((c, j) => (
              <span key={j} className="badge bg-theme-elevated text-theme-text-secondary">{c}</span>
            ))}
            {toCIDRSet.map((cs: any, j: number) => (
              <span key={`set-${j}`} className="badge bg-theme-elevated text-theme-text-secondary">{cs.cidr}</span>
            ))}
          </div>
        </div>
      )}

      {toPorts.length > 0 && (
        <div>
          <div className="text-xs text-theme-text-tertiary mb-1">Ports</div>
          <div className="flex flex-wrap gap-1">
            {toPorts.map((p: any, j: number) => {
              const ports = p.ports || []
              return ports.map((port: any, k: number) => (
                <span key={`${j}-${k}`} className="badge bg-theme-elevated text-theme-text-secondary">
                  {port.protocol || 'TCP'}/{port.port}
                </span>
              ))
            })}
          </div>
        </div>
      )}

      {fromEndpoints.length === 0 && fromEntities.length === 0 && fromCIDR.length === 0 && fromCIDRSet.length === 0 &&
       toEndpoints.length === 0 && toEntities.length === 0 && toCIDR.length === 0 && toCIDRSet.length === 0 &&
       toPorts.length === 0 && (
        <div className="text-xs text-theme-text-tertiary">Allow all</div>
      )}
    </div>
  )
}

function SelectorEntry({ selector }: { selector: any }) {
  const matchLabels = selector.matchLabels || {}
  const hasLabels = Object.keys(matchLabels).length > 0
  return (
    <div className="text-sm">
      {hasLabels ? (
        <span className="inline-flex flex-wrap gap-1 align-middle">
          {Object.entries(matchLabels).map(([k, v]) => (
            <span key={k} className="badge bg-theme-elevated text-theme-text-secondary">
              {k}={String(v)}
            </span>
          ))}
        </span>
      ) : (
        <span className="text-xs text-theme-text-tertiary">all endpoints</span>
      )}
    </div>
  )
}
