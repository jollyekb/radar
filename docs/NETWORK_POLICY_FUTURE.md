# NetworkPolicy — Future Ideas

Features explored during planning but not yet implemented. Current implementation covers: typed cache, topology nodes/edges, dashboard coverage card, renderers (NetworkPolicy, CiliumNetworkPolicy, CiliumClusterwideNetworkPolicy, ClusterNetworkPolicy), policy coverage overlay on topology, visual flow diagram in renderer, and drop correlation with policy evaluation in traffic view.

## 1. Highlight Unprotected Flows in Traffic View

Mark traffic edges between workloads where neither side has a NetworkPolicy. Answers "which traffic paths are unprotected?" — the visual version of the dashboard coverage card applied to actual observed flows.

## 2. Dedicated "Security" Top-Level View

A top-level nav tab combining:

- Network Policy coverage overview (pods/namespaces without policies)
- ClusterNetworkPolicy (policy.networking.k8s.io) visualization with priority/tier ordering
- CNI-specific policies (CiliumNetworkPolicy, Calico GlobalNetworkPolicy)
- RBAC visualization, PodSecurityAdmission status

Makes sense when there's enough security surface area to warrant a dedicated view.

## 3. Structured Policy Editor

Form-based create/edit for NetworkPolicies: select pods visually, add rules with dropdowns, preview affected pods before applying. Like editor.networkpolicy.io but connected to the live cluster.

Audience is narrow: most users writing NetworkPolicies are comfortable with YAML. The YAML editor already exists in the drawer.

## 4. Port-Specific Policy Evaluation

The current policy evaluation endpoint checks whether ingress/egress rules match the source/destination by labels and namespace, but does not evaluate port-specific rules. Adding port matching would give more precise verdicts (e.g., "allowed on TCP/9898 but this flow is on TCP/5432").

## 5. Detailed Cilium Policy Rule Evaluation

CiliumNetworkPolicy evaluation currently shows "selects this endpoint" without evaluating individual rules (entities, fromEndpoints, toPorts). Full rule evaluation would provide the same level of detail as standard NetworkPolicy correlation.

## Competitor Landscape (as of April 2026)

No free/OSS tool combines live cluster data with network policy visualization and drop correlation:

| Tool | Visual Graph | Edit | Drop Correlation | Coverage |
|------|-------------|------|-----------------|----------|
| Headlamp | No | YAML only | No | No |
| Lens/Freelens | No | YAML only | No | No |
| K8s Dashboard | No | YAML only | No | No |
| K9s | No | $EDITOR | No | No |
| editor.networkpolicy.io | Yes (static) | Yes | No | No |
| Hubble UI | Traffic graph | No | No | No |
| Calico Enterprise | Yes (Policy Board) | Yes | Yes | Yes |
| **Radar** | **Yes (topology + flow diagram)** | **YAML** | **Yes (per-flow)** | **Yes (dashboard + overlay)** |
