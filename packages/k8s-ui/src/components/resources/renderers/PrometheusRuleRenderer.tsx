import { useState, useMemo } from 'react'
import { Bell, Search, ChevronRight } from 'lucide-react'
import { clsx } from 'clsx'
import { Section, PropertyList, Property, ConditionsSection } from '../../ui/drawer-components'
import {
  getPrometheusRuleGroups,
  getPrometheusRuleTotalRules,
  getPrometheusRuleGroupCount,
} from '../resource-utils-prometheus'
import type { PrometheusRuleGroup, PrometheusAlertRule, PrometheusRecordingRule } from '../resource-utils-prometheus'

interface PrometheusRuleRendererProps {
  data: any
}

const SEVERITY_BADGE: Record<string, string> = {
  critical: 'bg-red-100 text-red-700 border-red-300 dark:bg-red-950/50 dark:text-red-400 dark:border-red-700/40',
  warning: 'bg-amber-100 text-amber-800 border-amber-300 dark:bg-amber-950/50 dark:text-amber-400 dark:border-amber-700/40',
  info: 'bg-sky-100 text-sky-700 border-sky-300 dark:bg-sky-950/50 dark:text-sky-400 dark:border-sky-700/40',
}

const EXPR_TRUNCATE_LEN = 200
const SUMMARY_TRUNCATE_LEN = 100

function TruncatedExpr({ expr }: { expr: string }) {
  const [expanded, setExpanded] = useState(false)
  const needsTruncation = expr.length > EXPR_TRUNCATE_LEN

  return (
    <div className="mt-1">
      <pre className="text-xs font-mono text-theme-text-secondary bg-theme-elevated rounded px-2 py-1.5 whitespace-pre-wrap break-all">
        {expanded || !needsTruncation ? expr : expr.slice(0, EXPR_TRUNCATE_LEN) + '...'}
      </pre>
      {needsTruncation && (
        <button
          onClick={() => setExpanded(!expanded)}
          className="text-[10px] text-accent-text hover:underline mt-0.5"
        >
          {expanded ? 'Show less' : 'Show more'}
        </button>
      )}
    </div>
  )
}

function AlertRuleCard({ rule }: { rule: PrometheusAlertRule }) {
  const severityClass = rule.severity ? SEVERITY_BADGE[rule.severity] : undefined
  const summaryText = rule.summary || rule.description || ''
  const truncatedSummary = summaryText.length > SUMMARY_TRUNCATE_LEN
    ? summaryText.slice(0, SUMMARY_TRUNCATE_LEN) + '...'
    : summaryText

  return (
    <div className="card-inner text-sm">
      <div className="flex items-center gap-2 flex-wrap">
        <span className="text-theme-text-primary font-medium">{rule.alert}</span>
        {rule.severity && (
          <span className={clsx('badge-sm', severityClass || 'bg-theme-hover text-theme-text-secondary')}>
            {rule.severity}
          </span>
        )}
        {rule.for && (
          <span className="badge-sm bg-theme-hover text-theme-text-secondary">
            for: {rule.for}
          </span>
        )}
      </div>
      {truncatedSummary && (
        <div className="text-xs text-theme-text-tertiary mt-1">{truncatedSummary}</div>
      )}
      <TruncatedExpr expr={rule.expr} />
    </div>
  )
}

function RecordingRuleCard({ rule }: { rule: PrometheusRecordingRule }) {
  return (
    <div className="card-inner text-sm">
      <div className="flex items-center gap-2">
        <span className="text-theme-text-primary font-medium">{rule.record}</span>
        <span className="badge-sm bg-theme-hover text-theme-text-tertiary">recording</span>
      </div>
      <TruncatedExpr expr={rule.expr} />
    </div>
  )
}

function RuleGroupSection({ group, searchTerm }: { group: PrometheusRuleGroup; searchTerm: string }) {
  const defaultExpanded = group.ruleCount <= 10
  const [expanded, setExpanded] = useState(defaultExpanded)

  const filteredRules = useMemo(() => {
    if (!searchTerm) return group.rules
    const term = searchTerm.toLowerCase()
    return group.rules.filter((rule) => {
      if (rule.type === 'alert') {
        return (
          rule.alert.toLowerCase().includes(term) ||
          rule.expr.toLowerCase().includes(term) ||
          (rule.severity || '').toLowerCase().includes(term) ||
          (rule.summary || '').toLowerCase().includes(term) ||
          (rule.description || '').toLowerCase().includes(term)
        )
      }
      return (
        rule.record.toLowerCase().includes(term) ||
        rule.expr.toLowerCase().includes(term)
      )
    })
  }, [group.rules, searchTerm])

  // If searching and no matches, hide the group entirely
  if (searchTerm && filteredRules.length === 0) return null

  return (
    <div className="card-inner">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex items-center justify-between w-full text-left"
      >
        <div className="flex items-center gap-2">
          <ChevronRight className={clsx('w-3.5 h-3.5 text-theme-text-tertiary transition-transform duration-200', expanded && 'rotate-90')} />
          <span className="text-sm text-theme-text-primary font-medium">{group.name}</span>
        </div>
        <div className="flex items-center gap-2">
          {group.interval && (
            <span className="badge-sm bg-theme-hover text-theme-text-secondary">
              {group.interval}
            </span>
          )}
          <span className="text-xs text-theme-text-tertiary">
            {searchTerm ? `${filteredRules.length}/${group.ruleCount}` : group.ruleCount} rule{group.ruleCount !== 1 ? 's' : ''}
          </span>
        </div>
      </button>
      {!expanded && (
        <div className="text-xs text-theme-text-secondary mt-1 ml-5.5 flex gap-3">
          {group.alertCount > 0 && <span>{group.alertCount} alert{group.alertCount !== 1 ? 's' : ''}</span>}
          {group.recordCount > 0 && <span>{group.recordCount} recording</span>}
        </div>
      )}
      <div
        className="grid transition-[grid-template-rows] duration-200 ease-out"
        style={{ gridTemplateRows: expanded ? '1fr' : '0fr' }}
      >
        <div className="overflow-hidden">
          <div className="mt-2 space-y-2">
            {filteredRules.map((rule, i) => (
              rule.type === 'alert'
                ? <AlertRuleCard key={`alert-${rule.alert}-${i}`} rule={rule} />
                : <RecordingRuleCard key={`rec-${rule.record}-${i}`} rule={rule} />
            ))}
            {searchTerm && filteredRules.length === 0 && (
              <div className="text-xs text-theme-text-tertiary py-2 text-center">
                No rules match the current filter.
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

export function PrometheusRuleRenderer({ data }: PrometheusRuleRendererProps) {
  const groups = getPrometheusRuleGroups(data)
  const totalRules = getPrometheusRuleTotalRules(data)
  const totalAlerts = groups.reduce((sum, g) => sum + g.alertCount, 0)
  const totalRecords = groups.reduce((sum, g) => sum + g.recordCount, 0)
  const conditions = data.status?.conditions

  const [searchTerm, setSearchTerm] = useState('')

  return (
    <>
      <Section title="PrometheusRule" icon={Bell}>
        <PropertyList>
          <Property label="Groups" value={String(getPrometheusRuleGroupCount(data))} />
          <Property label="Total Rules" value={String(totalRules)} />
          <Property label="Alert Rules" value={String(totalAlerts)} />
          <Property label="Recording Rules" value={String(totalRecords)} />
        </PropertyList>
      </Section>

      {groups.length > 0 && (
        <Section title={`Rule Groups (${groups.length})`} defaultExpanded>
          {/* Search bar */}
          {totalRules > 5 && (
            <div className="relative mb-3">
              <Search className="absolute left-2 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-theme-text-tertiary" />
              <input
                type="text"
                placeholder="Filter rules by name or expression..."
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
                className="w-full pl-7 pr-2 py-1.5 text-xs bg-theme-elevated border border-theme-border rounded-md text-theme-text-primary placeholder:text-theme-text-tertiary focus:outline-none focus:ring-1 focus:ring-accent/50"
              />
            </div>
          )}
          <div className="space-y-2">
            {groups.map((group, i) => (
              <RuleGroupSection key={i} group={group} searchTerm={searchTerm} />
            ))}
            {searchTerm && groups.every(g => {
              const term = searchTerm.toLowerCase()
              return g.rules.every(rule => {
                if (rule.type === 'alert') {
                  return !(
                    rule.alert.toLowerCase().includes(term) ||
                    rule.expr.toLowerCase().includes(term) ||
                    (rule.severity || '').toLowerCase().includes(term) ||
                    (rule.summary || '').toLowerCase().includes(term) ||
                    (rule.description || '').toLowerCase().includes(term)
                  )
                }
                return !(
                  rule.record.toLowerCase().includes(term) ||
                  rule.expr.toLowerCase().includes(term)
                )
              })
            }) && (
              <div className="text-xs text-theme-text-tertiary py-3 text-center">
                No rules match "{searchTerm}".
              </div>
            )}
          </div>
        </Section>
      )}

      <ConditionsSection conditions={conditions} />
    </>
  )
}
