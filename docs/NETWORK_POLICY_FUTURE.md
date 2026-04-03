# NetworkPolicy Visualization — Future Ideas

Advanced visualization features explored during planning, deferred from the initial implementation. The foundation is in place: typed cache, topology nodes, dashboard coverage card, renderers for standard NetworkPolicy + CiliumNetworkPolicy + CiliumClusterwideNetworkPolicy + AdminNetworkPolicy + BaselineAdminNetworkPolicy.

## 1. Policy Effect Overlay on Traffic/Resource Topology

**Effort: ~5-7 hours MVP**

Color-code existing topology edges based on whether NetworkPolicy allows or blocks that traffic path:

- **Green**: explicitly allowed by a policy's ingress `from` rules
- **Red/dashed**: blocked (target has a selecting policy but source not in allow list)
- **Gray**: no policy on target (default-allow, unprotected)

Toggle in TopologyControls: "Show policy effect" checkbox. Best fit for the traffic topology view.

**Backend**: Extend `Edge` struct (`pkg/topology/types.go`) with optional `PolicyEffect` field. For each Service-to-Workload edge, evaluate target's selecting NetworkPolicies against source's labels/namespace using `LabelSelectorAsSelector()` (already available).

**MVP simplifications**: Ingress only (skip egress), skip port-specific rules, skip CIDR matching, binary verdict per edge.

**Key files**: `pkg/topology/types.go`, `pkg/topology/builder.go`, `packages/k8s-ui/src/components/topology/TopologyGraph.tsx` (`EDGE_COLORS`), `TopologyControls.tsx`.

## 2. Visual Policy Diagram in Renderer

**Effort: ~1-2 days**

Inside the NetworkPolicy detail drawer, render a mini-diagram using `@xyflow/react` (already a dependency):

```
[Sources] --ingress--> [Target Pods] --egress--> [Destinations]
```

Ingress sources on left, target pods center, egress destinations right. Clear AND/OR grouping. Reference UX: editor.networkpolicy.io but cluster-connected.

**Design-intensive**: AND/OR grouping, CIDR blocks, namespace selectors, port ranges all need thoughtful layout.

## 3. Dedicated "Security" Top-Level View

**Effort: larger scope**

A top-level nav tab combining:

- Network Policy coverage overview (pods/namespaces without policies)
- AdminNetworkPolicy (K8s 1.32+ GA) visualization with priority ordering
- CNI-specific policies (CiliumNetworkPolicy, Calico GlobalNetworkPolicy)
- RBAC visualization, PodSecurityAdmission status

Makes sense when there's enough security surface area to warrant a dedicated view.

## 4. Structured Policy Editor

**Effort: ~2-3 days**

Form-based create/edit for NetworkPolicies: select pods visually, add rules with dropdowns, preview affected pods before applying. Like editor.networkpolicy.io but connected to the live cluster.

Audience is narrow: most users writing NetworkPolicies are comfortable with YAML. The YAML editor already exists in the drawer.

## 5. Hubble Flow + Policy Correlation

**Effort: ~1-2 days**

Since Radar already ingests Hubble traffic flows, overlay observed traffic against declared policy rules: "Here's what your policy allows, and here's what's actually flowing." Highlight dropped traffic (Hubble reports verdict: FORWARDED/DROPPED).

This is what Calico Enterprise charges for. Only works when Hubble is available.

## Competitor Landscape (as of April 2026)

No free/OSS tool does live cluster-connected policy visualization:

| Tool | Visual Graph | Edit | Impact Analysis | Flow Overlay |
|------|-------------|------|-----------------|--------------|
| Headlamp | No | YAML only | No | No |
| Lens/Freelens | No | YAML only | No | No |
| K8s Dashboard | No | YAML only | No | No |
| K9s | No | $EDITOR | No | No |
| editor.networkpolicy.io | Yes (static) | Yes | No (not cluster-connected) | No |
| np-viewer/netfetch | No (CLI) | No | Coverage audit | No |
| Hubble UI | Traffic graph | No | Partial (dropped flows) | No |
| Calico Enterprise | Yes (Policy Board) | Yes | Yes | Yes |

**Gap**: No OSS tool combines live cluster data with network policy visualization. Radar's foundation work positions it to fill this gap.
