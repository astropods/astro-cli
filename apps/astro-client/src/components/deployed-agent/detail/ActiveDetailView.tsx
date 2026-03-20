import { useState, useRef, useEffect, useMemo } from "react";
import { ResponsiveContainer, ComposedChart, Area, Line, XAxis, YAxis, CartesianGrid, Tooltip } from "recharts";
import { useNavigate } from "react-router";
import {
  ArrowLeft, Settings2, Play,
  ChevronRight, ChevronDown, MoreVertical, Copy, Check,
  Pencil, Trash2, X, Loader2, Activity, Rocket,
  Search, Eye, EyeOff, Calendar, RefreshCw,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { AgentIdentity } from "@/components/AgentIdentity";
import { mapDeploymentStatus, formatDate } from "@/lib/deployment-utils";
import { useObservabilityMetrics, useObservabilitySummary, useObservabilityTraces } from "@/api/queries/observability";
import { usePrefilledDeploymentTemplate } from "@/api/queries/agents";
import { useTriggerIngestion, useDeploymentLogs, useDeploymentHistory } from "@/api/queries/deployments";
import { useDeployForm, slugToTitle } from "@/components/deploy/useDeployForm";
import { DeployFormFields } from "@/components/deploy/DeployFormFields";
import { extractInitialValues } from "@/components/deploy/extractInitialValues";
import { useChangeTracking, type TrackedFormState } from "@/components/deploy/useChangeTracking";
import {
  selectPlaygroundBackendUrl,
  buildPlaygroundLaunchUrl,
  buildPortForwardCommand,
  isLocalEnv,
} from "@/lib/playground-url";
import type { AgentDeployment, ApiError, DeploymentHistoryRecord as ApiDeploymentHistoryRecord } from "@/lib/api";

// ─── color + font tokens (mirroring sketchbook exactly) ──────────────────────
const C = {
  bg:      '#ede7d9',
  bgAlt:   '#e5dece',
  bgDeep:  '#d8d0c0',
  panel:   '#f5f1e8',
  border:  '#c4b89e',
  teal:    '#073d3c',
  tealMid: '#15827d',
  tealLt:  '#57c4c1',
  text:    '#0d1f1e',
  muted:   '#4a5e5d',
  faint:   '#6b7e7c',
  stone:   '#9a8a72',
  amber:   '#D48F1E',
  amberBg: 'rgba(212,143,30,0.1)',
  amberBdr:'rgba(212,143,30,0.28)',
  coral:   '#F0816A',
  coralBg: 'rgba(240,129,106,0.1)',
  coralBdr:'rgba(240,129,106,0.28)',
  success: '#2d7a4f',
}
const S = {
  body: "'Geist', 'Inter', sans-serif",
  mono: "'Geist Mono', 'Space Mono', monospace",
}

type TraceStatus = 'success' | 'error' | 'timeout'
interface TraceRow {
  id: string; name: string; status: TraceStatus; latency: number
  time: string; tokens: number; input?: string; output?: string
}
const TRACE_STATUS_STYLE: Record<TraceStatus, { bg: string; color: string; label: string }> = {
  success: { bg: 'rgba(45,122,79,0.1)', color: C.success, label: 'success' },
  error:   { bg: C.coralBg,             color: C.coral,   label: 'error'   },
  timeout: { bg: C.amberBg,             color: C.amber,   label: 'timeout' },
}

function fmtTokens(n: number) {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000)     return `${(n / 1_000).toFixed(0)}k`
  return String(n)
}

// ─── css keyframes injection ──────────────────────────────────────────────────
function Styles() {
  return (
    <style>{`
      @keyframes dp-pulse { 0%,100% { opacity:1; } 50% { opacity:0.4; } }
      @keyframes dp-blink { 0%,100% { opacity:1; } 50% { opacity:0; } }
      @keyframes dp-fadein { from { opacity:0; transform:translateY(3px); } to { opacity:1; transform:translateY(0); } }
      @keyframes dp-spin { from { transform:rotate(0deg); } to { transform:rotate(360deg); } }
      @keyframes dp-slot-in { 0% { transform:translateY(110%); opacity:0.5; } 65% { transform:translateY(-6%); opacity:1; } 82% { transform:translateY(2%); } 100% { transform:translateY(0); } }
      .dp-slot-in { animation: dp-slot-in 0.32s cubic-bezier(0.34,1.56,0.64,1) forwards; }
      .dp-blink { animation: dp-blink 1.1s step-end infinite; }
      .dp-pulse { animation: dp-pulse 1.8s ease-in-out infinite; }
      .dp-log { animation: dp-fadein 0.2s ease forwards; }
      .dp-spin { animation: dp-spin 1.2s linear infinite; }
      .dp-scroll { scrollbar-width: thin; scrollbar-color: transparent transparent; scrollbar-gutter: stable; }
      .dp-scroll:hover { scrollbar-color: #c4b89e transparent; }
      .dp-scroll::-webkit-scrollbar { width: 6px; }
      .dp-scroll::-webkit-scrollbar-track { background: transparent; }
      .dp-scroll::-webkit-scrollbar-thumb { background: transparent; border-radius: 3px; }
      .dp-scroll:hover::-webkit-scrollbar-thumb { background: #c4b89e; }
      .dp-container-hdr:hover { background: ${C.panel} !important; }
    `}</style>
  )
}

// ─── log line color ───────────────────────────────────────────────────────────
function logLineColor(line: string): string {
  const l = line.toLowerCase()
  if (/✓|connected|ready|healthy|initialized|registered|success|loaded|complete/.test(l)) return C.success
  if (/error|failed|exception|fatal/.test(l)) return C.coral
  if (/warn|warning|retry|attempt/.test(l)) return C.amber
  return C.muted
}

// ─── kebab menu ───────────────────────────────────────────────────────────────
function KebabMenu({ deploymentId }: { deploymentId: string }) {
  const [open, setOpen] = useState(false)
  const [copied, setCopied] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const h = (e: MouseEvent) => { if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false) }
    document.addEventListener('mousedown', h)
    return () => document.removeEventListener('mousedown', h)
  }, [open])

  const copyId = () => {
    navigator.clipboard.writeText(deploymentId)
    setCopied(true)
    setTimeout(() => { setCopied(false); setOpen(false) }, 1600)
  }

  return (
    <div ref={ref} style={{ position: 'relative' }}>
      <button onClick={() => setOpen(o => !o)} style={{
        background: 'none', border: 'none', cursor: 'pointer', color: C.faint,
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        width: 28, height: 28, borderRadius: 6,
      }}
        onMouseEnter={e => (e.currentTarget.style.background = C.bgDeep)}
        onMouseLeave={e => (e.currentTarget.style.background = 'none')}
      >
        <MoreVertical size={15} />
      </button>
      {open && (
        <div style={{
          position: 'absolute', top: 'calc(100% + 4px)', left: 0, zIndex: 100,
          minWidth: 180, background: C.bgAlt, border: `1px solid ${C.border}`,
          borderRadius: 10, overflow: 'hidden', boxShadow: '0 8px 24px rgba(0,0,0,0.12)',
        }}>
          {[
            { icon: copied ? Check : Copy, label: copied ? 'Copied!' : 'Copy ID number', color: C.text, onClick: copyId, sep: false },
            { icon: Pencil, label: 'Rename', color: C.text, onClick: () => setOpen(false), sep: false },
            { icon: Trash2, label: 'Delete agent', color: C.coral, onClick: () => setOpen(false), sep: true },
          ].map(({ icon: Icon, label, color, onClick, sep }) => (
            <div key={label}>
              {sep && <div style={{ height: 1, background: C.border }} />}
              <button style={{
                width: '100%', display: 'flex', alignItems: 'center', gap: 10,
                padding: '10px 14px', background: 'none', border: 'none',
                cursor: 'pointer', fontFamily: S.body, fontSize: 13, color, textAlign: 'left' as const,
              }}
                onMouseEnter={e => (e.currentTarget.style.background = C.bgDeep)}
                onMouseLeave={e => (e.currentTarget.style.background = 'none')}
                onClick={onClick}
              >
                <Icon size={13} />{label}
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ─── multi-select dropdown ────────────────────────────────────────────────────
function MultiSelect({ options, selected, onChange, placeholder }: {
  options: { value: string; label: string; color?: string }[]
  selected: string[]
  onChange: (v: string[]) => void
  placeholder: string
}) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const h = (e: MouseEvent) => { if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false) }
    document.addEventListener('mousedown', h)
    return () => document.removeEventListener('mousedown', h)
  }, [open])

  const toggle = (v: string) =>
    onChange(selected.includes(v) ? selected.filter(s => s !== v) : [...selected, v])

  const allSelected = selected.length === 0 || selected.length === options.length
  const labelText = allSelected
    ? placeholder
    : selected.length === 1
      ? options.find(o => o.value === selected[0])?.label ?? selected[0]
      : `${selected.length} selected`

  return (
    <div ref={ref} style={{ position: 'relative' }}>
      <button onClick={() => setOpen(o => !o)} style={{
        display: 'inline-flex', alignItems: 'center', gap: 6,
        padding: '5px 12px', borderRadius: 7,
        border: `1px solid ${open ? C.tealMid : C.border}`,
        background: open ? C.bgDeep : C.bg, cursor: 'pointer',
        fontFamily: S.body, fontSize: 12, color: allSelected ? C.muted : C.teal,
        transition: 'all 0.12s', whiteSpace: 'nowrap' as const,
      }}>
        <span>{labelText}</span>
        <ChevronDown size={11} color={C.faint} style={{ transform: open ? 'rotate(180deg)' : 'none', transition: 'transform 0.15s' }} />
      </button>
      {open && (
        <div style={{
          position: 'absolute', top: 'calc(100% + 4px)', left: 0, zIndex: 200,
          minWidth: 160, background: C.bgAlt, border: `1px solid ${C.border}`,
          borderRadius: 8, overflow: 'hidden', boxShadow: '0 6px 20px rgba(0,0,0,0.1)',
        }}>
          <button onClick={() => onChange([])} style={{
            width: '100%', display: 'flex', alignItems: 'center', gap: 8,
            padding: '8px 12px', background: 'none', border: 'none', borderBottom: `1px solid ${C.border}`,
            cursor: 'pointer', fontFamily: S.body, fontSize: 12, color: C.faint, textAlign: 'left' as const,
          }}
            onMouseEnter={e => { e.currentTarget.style.background = C.bgDeep }}
            onMouseLeave={e => { e.currentTarget.style.background = 'none' }}
          >
            <div style={{ width: 14, height: 14, borderRadius: 3, flexShrink: 0, border: `1.5px solid ${allSelected ? C.tealMid : C.border}`, background: allSelected ? C.tealMid : 'transparent', display: 'flex', alignItems: 'center', justifyContent: 'center', transition: 'all 0.12s' }}>
              {allSelected && <svg width="8" height="8" viewBox="0 0 10 10"><path d="M2 5l2 2 4-4" stroke="#fff" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" fill="none" /></svg>}
            </div>
            All
          </button>
          {options.map(opt => {
            const checked = selected.includes(opt.value)
            return (
              <button key={opt.value} onClick={() => toggle(opt.value)} style={{
                width: '100%', display: 'flex', alignItems: 'center', gap: 8,
                padding: '8px 12px', background: 'none', border: 'none',
                cursor: 'pointer', fontFamily: S.body, fontSize: 12, color: C.text, textAlign: 'left' as const,
              }}
                onMouseEnter={e => { e.currentTarget.style.background = C.bgDeep }}
                onMouseLeave={e => { e.currentTarget.style.background = 'none' }}
              >
                <div style={{ width: 14, height: 14, borderRadius: 3, flexShrink: 0, border: `1.5px solid ${checked ? C.tealMid : C.border}`, background: checked ? C.tealMid : 'transparent', display: 'flex', alignItems: 'center', justifyContent: 'center', transition: 'all 0.12s' }}>
                  {checked && <svg width="8" height="8" viewBox="0 0 10 10"><path d="M2 5l2 2 4-4" stroke="#fff" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" fill="none" /></svg>}
                </div>
                {opt.color && <span style={{ width: 7, height: 7, borderRadius: '50%', background: opt.color, display: 'inline-block', flexShrink: 0 }} />}
                <span>{opt.label}</span>
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}

// ─── monitor tab ─────────────────────────────────────────────────────────────
const CHART_H = 130

interface ChartTooltipProps {
  active?: boolean
  payload?: { name: string; value: number; color: string }[]
  label?: string
  reqVisible: boolean
  avgLatVisible: boolean
}
function ChartTooltip({ active, payload, label, reqVisible, avgLatVisible }: ChartTooltipProps) {
  if (!active || !payload?.length) return null
  const req = payload.find(p => p.name === 'req')
  const avgLat = payload.find(p => p.name === 'avgLat')
  return (
    <div style={{
      background: C.panel, border: `1px solid ${C.border}`,
      borderRadius: 7, padding: '6px 10px',
      boxShadow: '0 2px 10px rgba(0,0,0,0.1)',
      pointerEvents: 'none',
    }}>
      <div style={{ fontFamily: S.mono, fontSize: 9, color: C.faint, marginBottom: 4, letterSpacing: '0.06em' }}>{label}</div>
      {reqVisible && req && (
        <div style={{ display: 'flex', alignItems: 'center', gap: 5, fontFamily: S.mono, fontSize: 11 }}>
          <span style={{ width: 6, height: 6, borderRadius: 1, background: C.tealMid, display: 'inline-block', flexShrink: 0 }} />
          <span style={{ color: C.tealMid, fontWeight: 600 }}>{req.value}</span>
          <span style={{ color: C.faint }}>req</span>
        </div>
      )}
      {avgLatVisible && avgLat && (
        <div style={{ display: 'flex', alignItems: 'center', gap: 5, fontFamily: S.mono, fontSize: 11, marginTop: 2 }}>
          <span style={{ width: 8, height: 1.5, background: C.amber, display: 'inline-block', flexShrink: 0 }} />
          <span style={{ color: C.amber, fontWeight: 600 }}>{avgLat.value}ms</span>
          <span style={{ color: C.faint }}>avg</span>
        </div>
      )}
    </div>
  )
}

function InlineChart({ data, reqVisible, avgLatVisible }: {
  data: { t: string; req: number; avgLatencyMs: number }[]
  reqVisible: boolean; avgLatVisible: boolean
}) {
  if (data.length < 2) return null
  const reqMax = Math.max(...data.map(d => d.req))
  const latMin = Math.min(...data.map(d => d.avgLatencyMs))
  const latMax = Math.max(...data.map(d => d.avgLatencyMs))
  const latPad = (latMax - latMin) * 0.15 || 50
  return (
    <ResponsiveContainer width="100%" height={CHART_H}>
      <ComposedChart data={data} margin={{ top: 8, right: 0, bottom: 0, left: 0 }}>
        <defs>
          <linearGradient id="req-grad-obs" x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor={C.tealMid} stopOpacity="0.18" />
            <stop offset="95%" stopColor={C.tealMid} stopOpacity="0.02" />
          </linearGradient>
        </defs>
        <CartesianGrid strokeDasharray="3 3" stroke={C.border} strokeOpacity={0.6} vertical={false} />
        <XAxis dataKey="t"
          tick={{ fontFamily: S.mono, fontSize: 8, fill: C.faint }}
          tickLine={false} axisLine={false} />
        <YAxis yAxisId="req" orientation="left"
          domain={[0, Math.ceil(reqMax * 1.15)]}
          tick={{ fontFamily: S.mono, fontSize: 8, fill: C.faint }}
          tickLine={false} axisLine={false} width={32}
          tickFormatter={(v: number) => v >= 1000 ? `${(v / 1000).toFixed(0)}k` : String(v)}
          tickCount={4} />
        <YAxis yAxisId="ms" orientation="right"
          domain={[Math.floor(latMin - latPad), Math.ceil(latMax + latPad)]}
          tick={{ fontFamily: S.mono, fontSize: 8, fill: C.faint }}
          tickLine={false} axisLine={false} width={38}
          tickFormatter={(v: number) => `${v}ms`}
          tickCount={4} />
        <Tooltip
          content={<ChartTooltip reqVisible={reqVisible} avgLatVisible={avgLatVisible} />}
          cursor={{ stroke: C.stone, strokeWidth: 0.8, strokeDasharray: '2 2' }} />
        {reqVisible && (
          <Area yAxisId="req" dataKey="req" name="req"
            stroke={C.tealMid} strokeWidth={1.5} fill="url(#req-grad-obs)"
            dot={false}
            activeDot={{ r: 4, fill: C.tealMid, stroke: C.panel, strokeWidth: 1.5 }}
            animationDuration={1000} animationEasing="ease-out" />
        )}
        {avgLatVisible && (
          <Line yAxisId="ms" dataKey="avgLatencyMs" name="avgLat"
            stroke={C.amber} strokeWidth={1.5} strokeDasharray="4 3" strokeOpacity={0.85}
            dot={false}
            activeDot={{ r: 4, fill: C.amber, stroke: C.panel, strokeWidth: 1.5 }}
            animationDuration={1000} animationEasing="ease-out" />
        )}
      </ComposedChart>
    </ResponsiveContainer>
  )
}

const WIN_HOURS: Record<string, number> = { '1h': 1, '24h': 24, '7d': 168 }

function buildTimeParams(win: string) {
  const hours = WIN_HOURS[win] ?? 24
  const end = new Date()
  const start = new Date(end.getTime() - hours * 60 * 60 * 1000)
  return { start_time: start.toISOString(), end_time: end.toISOString() }
}

function MonitorTab({ deployment, account }: { deployment: AgentDeployment; account: string }) {
  const [win, setWin] = useState<'1h' | '24h' | '7d'>('24h')
  const [traceSearch, setTraceSearch] = useState('')
  const [traceStatuses, setTraceStatuses] = useState<string[]>([])
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [series, setSeries] = useState({ req: true, avgLat: true })
  const [tokenView, setTokenView] = useState<'input' | 'output'>('input')

  const timeParams = buildTimeParams(win)

  const metricsQuery = useObservabilityMetrics(account, deployment.name, timeParams)
  const summaryQuery = useObservabilitySummary(account, deployment.name, timeParams)
  const tracesQuery = useObservabilityTraces(account, deployment.name, { ...timeParams, limit: '100' })
  const { data: metricsData } = metricsQuery
  const { data: summaryData } = summaryQuery
  const { data: tracesData } = tracesQuery
  const observabilityBackendError =
    metricsQuery.isError || summaryQuery.isError || tracesQuery.isError

  // Buckets expose avg_latency_ms per interval (not per-bucket p95); summary carries true p95.
  const tsData: { t: string; req: number; avgLatencyMs: number }[] = (metricsData?.buckets ?? []).map(b => ({
    t: new Date(b.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
    req: b.trace_count,
    avgLatencyMs: b.avg_latency_ms,
  }))

  const bucketTokenTotals = useMemo(() => {
    let input = 0
    let output = 0
    for (const b of metricsData?.buckets ?? []) {
      input += b.input_tokens ?? 0
      output += b.output_tokens ?? 0
    }
    return { input, output, sum: input + output }
  }, [metricsData])

  const traces: TraceRow[] = (tracesData?.traces ?? []).map(t => ({
    id: t.trace_id,
    name: t.name,
    status: t.status === 'error' || t.status === 'failed' ? 'error' : t.status === 'timeout' ? 'timeout' : 'success',
    latency: t.latency_ms,
    time: new Date(t.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
    tokens: 0,
    input: t.input,
    output: t.output,
  }))

  const visibleTraces = traces.filter(t => {
    if (traceStatuses.length > 0 && !traceStatuses.includes(t.status)) return false
    if (traceSearch && !t.name.toLowerCase().includes(traceSearch.toLowerCase())) return false
    return true
  })
  const toggleTrace = (id: string) =>
    setExpanded(prev => {
      const n = new Set(prev)
      if (n.has(id)) n.delete(id)
      else n.add(id)
      return n
    })

  const summary = summaryData?.metrics
  const tokenSplitFromBuckets = bucketTokenTotals.sum > 0
  const tokens = {
    input: tokenSplitFromBuckets ? bucketTokenTotals.input : 0,
    output: tokenSplitFromBuckets ? bucketTokenTotals.output : 0,
    total: summary?.total_tokens ?? bucketTokenTotals.sum,
    hasSplit: tokenSplitFromBuckets,
  }
  const activeToken = !tokens.hasSplit && tokens.total > 0
    ? { value: tokens.total, color: C.tealMid }
    : tokenView === 'input'
      ? { value: tokens.input, color: C.tealMid }
      : { value: tokens.output, color: C.amber }

  const isError = mapDeploymentStatus(deployment) === 'error'

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>

      {/* error banner */}
      {isError && (
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '10px 16px', borderRadius: 8, background: C.coralBg, border: `1px solid ${C.coralBdr}` }}>
          <span style={{ fontFamily: S.mono, fontSize: 10, fontWeight: 700, letterSpacing: '0.08em', color: C.coral }}>ERROR</span>
          <span style={{ fontFamily: S.body, fontSize: 12, color: C.coral, flex: 1 }}>This deployment is in an error state — no replicas are ready.</span>
        </div>
      )}

      {observabilityBackendError && !isError && (
        <div style={{ display: 'flex', alignItems: 'flex-start', gap: 10, padding: '10px 16px', borderRadius: 8, background: C.amberBg, border: `1px solid ${C.amberBdr}` }}>
          <span style={{ fontFamily: S.mono, fontSize: 10, fontWeight: 700, letterSpacing: '0.08em', color: C.amber }}>OBSERVABILITY</span>
          <span style={{ fontFamily: S.body, fontSize: 12, color: C.muted, flex: 1, lineHeight: 1.5 }}>
            Trace metrics couldn’t be loaded (backend returned an error). Local dev often needs valid Galileo credentials in <span style={{ fontFamily: S.mono, fontSize: 11 }}>astro-server</span> env.
            Pod logs on the <strong style={{ color: C.text }}>Deployments</strong> tab use Kubernetes/Loki and work independently when the cluster is reachable.
          </span>
        </div>
      )}

      {/* three-column layout: parking | content | parking */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr minmax(0, 900px) 1fr', gap: 12, alignItems: 'start' }}>

        {/* left parking space */}
        <div />

        {/* center content */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>

          {/* header + time picker */}
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <span style={{ fontFamily: S.body, fontSize: 17, fontWeight: 600, color: C.teal }}>Monitor</span>
            <select
              value={win}
              onChange={e => setWin(e.target.value as typeof win)}
              style={{
                padding: '6px 28px 6px 12px', borderRadius: 7, border: `1px solid ${C.border}`,
                background: C.bg, fontFamily: S.body, fontSize: 12, color: C.muted,
                cursor: 'pointer', outline: 'none', appearance: 'none' as const,
                backgroundImage: `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='10' viewBox='0 0 24 24' fill='none' stroke='%236b7e7c' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpolyline points='6 9 12 15 18 9'%3E%3C/polyline%3E%3C/svg%3E")`,
                backgroundRepeat: 'no-repeat', backgroundPosition: 'right 10px center',
              }}
            >
              <option value="1h">Last 1 hour</option>
              <option value="24h">Last 24 hours</option>
              <option value="7d">Last 7 days</option>
            </select>
          </div>

          {/* KPI cards */}
          {(() => {
            const kpis = [
              { label: 'TOTAL REQUESTS', value: summaryData ? String(summaryData.total_traces) : '—' },
              { label: 'ERROR RATE', value: summary ? `${(summary.error_rate * 100).toFixed(1)}%` : '—' },
              { label: 'AVG LATENCY', value: summary ? `${summary.avg_latency_ms.toFixed(0)}ms` : '—' },
              { label: 'P95 LATENCY', value: summary ? `${summary.p95_latency_ms.toFixed(0)}ms` : '—' },
            ]
            return (
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 10 }}>
                {kpis.map(({ label, value }) => (
                  <div key={label} style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, padding: '12px 14px' }}>
                    <span style={{ display: 'block', fontFamily: S.mono, fontSize: 9, letterSpacing: '0.07em', color: C.faint, marginBottom: 8 }}>{label}</span>
                    <span style={{ display: 'block', fontFamily: S.body, fontSize: 20, fontWeight: 700, color: C.teal }}>{value}</span>
                  </div>
                ))}
              </div>
            )
          })()}

          {/* charts row */}
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
            {/* Request volume — real SVG chart */}
            <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, padding: '14px 16px' }}>
              <div style={{ marginBottom: 8 }}>
                <span style={{ fontFamily: S.body, fontSize: 13, fontWeight: 700, color: C.teal }}>Request volume</span>
              </div>
              <div style={{ display: 'flex', gap: 10, marginBottom: 6 }}>
                {([
                  { key: 'req' as const, color: C.tealMid, label: 'Requests', dashed: false },
                  { key: 'avgLat' as const, color: C.amber, label: 'Avg latency', dashed: true },
                ]).map(s => (
                  <button key={s.key} onClick={() => setSeries(p => ({ ...p, [s.key]: !p[s.key] }))} style={{
                    display: 'flex', alignItems: 'center', gap: 4, background: 'none', border: 'none',
                    cursor: 'pointer', padding: 0, opacity: series[s.key] ? 1 : 0.35, transition: 'opacity 0.15s',
                  }}>
                    <span style={{ width: 18, height: 2, background: s.dashed ? 'none' : s.color, backgroundImage: s.dashed ? `repeating-linear-gradient(to right, ${s.color} 0, ${s.color} 4px, transparent 4px, transparent 7px)` : 'none', display: 'inline-block', borderRadius: 1, flexShrink: 0 }} />
                    <span style={{ fontFamily: S.mono, fontSize: 9, color: C.faint }}>{s.label}</span>
                  </button>
                ))}
              </div>
              {tsData.length === 0 ? (
                <div style={{ height: 130, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 10, textAlign: 'center' }}>
                  <div style={{ width: 32, height: 32, borderRadius: '50%', background: C.bgDeep, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                    <Activity size={15} color={C.stone} />
                  </div>
                  <div>
                    <p style={{ fontFamily: S.body, fontSize: 12, fontWeight: 600, color: C.text, margin: '0 0 3px' }}>No requests yet</p>
                    <p style={{ fontFamily: S.mono, fontSize: 10, color: C.faint, margin: 0, letterSpacing: '0.03em' }}>Volume will appear once traffic starts</p>
                  </div>
                </div>
              ) : (
                <InlineChart key={win} data={tsData} reqVisible={series.req} avgLatVisible={series.avgLat} />
              )}
            </div>

            {/* Token usage — real numbers */}
            <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, overflow: 'hidden' }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '12px 16px', borderBottom: `1px solid ${C.border}` }}>
                <span style={{ fontFamily: S.body, fontSize: 13, fontWeight: 700, color: C.teal }}>Token usage</span>
                {tokens.hasSplit ? (
                  <div style={{ display: 'flex', background: C.bgDeep, borderRadius: 6, padding: 2 }}>
                    {(['input', 'output'] as const).map(v => (
                      <button key={v} type="button" onClick={() => setTokenView(v)} style={{
                        padding: '3px 10px', borderRadius: 4, border: 'none', cursor: 'pointer',
                        background: tokenView === v ? C.panel : 'transparent',
                        fontFamily: S.body, fontSize: 12,
                        color: tokenView === v ? C.text : C.faint,
                        fontWeight: tokenView === v ? 600 : 400,
                        boxShadow: tokenView === v ? '0 1px 3px rgba(0,0,0,0.08)' : 'none',
                        textTransform: 'capitalize' as const, transition: 'all 0.12s',
                      }}>{v}</button>
                    ))}
                  </div>
                ) : (
                  <span style={{ fontFamily: S.mono, fontSize: 10, color: C.faint }}>from summary</span>
                )}
              </div>
              <div style={{ padding: '14px 16px 12px' }}>
                <div style={{ overflow: 'hidden', lineHeight: 1.2 }}>
                  <span key={`${win}-${tokenView}-${tokens.hasSplit}`} className="dp-slot-in" style={{ display: 'block', fontFamily: S.body, fontSize: 26, fontWeight: 700, color: activeToken.color, letterSpacing: '-0.02em' }}>{fmtTokens(activeToken.value)}</span>
                </div>
                {tokens.hasSplit ? (
                  <>
                    <div style={{ display: 'flex', height: 5, borderRadius: 3, overflow: 'hidden', margin: '12px 0 8px', background: C.bgDeep }}>
                      <div style={{ flex: tokens.input || 1, background: C.tealMid, opacity: tokenView === 'input' ? (tokens.total === 0 ? 0.15 : 1) : 0.25, transition: 'opacity 0.2s' }} />
                      <div style={{ flex: tokens.output || 1, background: C.amber, opacity: tokenView === 'output' ? (tokens.total === 0 ? 0.15 : 1) : 0.25, transition: 'opacity 0.2s' }} />
                    </div>
                    <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                      <div style={{ overflow: 'hidden', lineHeight: 1.3 }}>
                        <span key={win} className="dp-slot-in" style={{ display: 'block', fontFamily: S.body, fontSize: 11, color: C.faint }}>of {fmtTokens(tokens.total)} total</span>
                      </div>
                      {tokens.total > 0 && (
                        <div style={{ overflow: 'hidden', lineHeight: 1.3 }}>
                          <span key={`${win}-pct`} className="dp-slot-in" style={{ display: 'block', fontFamily: S.mono, fontSize: 10, color: C.faint }}>{Math.round((activeToken.value / tokens.total) * 100)}%</span>
                        </div>
                      )}
                    </div>
                  </>
                ) : (
                  <p style={{ fontFamily: S.mono, fontSize: 10, color: C.faint, margin: '10px 0 0', lineHeight: 1.5 }}>
                    Input/output split comes from metrics buckets when available.
                  </p>
                )}
              </div>
            </div>
          </div>

          {/* Traces table */}
          <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, overflow: 'hidden', display: 'flex', flexDirection: 'column', height: 360 }}>
            {/* header */}
            <div style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '12px 16px', borderBottom: `1px solid ${C.border}`, flexShrink: 0 }}>
              <span style={{ fontFamily: S.body, fontSize: 13, fontWeight: 700, color: C.teal, flex: 1 }}>Traces</span>
              <div style={{ display: 'flex', alignItems: 'center', gap: 5, padding: '4px 8px', borderRadius: 6, border: `1px solid ${C.border}`, background: C.bg }}>
                <Search size={11} color={C.faint} />
                <input type="text" placeholder="Search traces" value={traceSearch} onChange={e => setTraceSearch(e.target.value)}
                  style={{ background: 'none', border: 'none', outline: 'none', fontFamily: S.body, fontSize: 11, color: C.muted, width: 160, caretColor: C.tealMid }} />
              </div>
              <MultiSelect
                options={[
                  { value: 'success', label: 'Success', color: C.success },
                  { value: 'error',   label: 'Error',   color: C.coral   },
                  { value: 'timeout', label: 'Timeout', color: C.amber   },
                ]}
                selected={traceStatuses}
                onChange={setTraceStatuses}
                placeholder="All statuses"
              />
            </div>
            {/* table header */}
            <div style={{ display: 'grid', gridTemplateColumns: '16px 1fr 80px 72px 60px 72px', gap: 10, padding: '7px 16px', borderBottom: `1px solid ${C.border}`, background: C.bgDeep, flexShrink: 0 }}>
              {['', 'TRACE', 'STATUS', 'LATENCY', 'TOKENS', 'TIME'].map(h => (
                <span key={h} style={{ fontFamily: S.mono, fontSize: 9, letterSpacing: '0.07em', color: C.faint }}>{h}</span>
              ))}
            </div>
            {/* rows */}
            <div className="dp-scroll" style={{ flex: 1, overflowY: 'auto', minHeight: 0 }}>
              {traces.length === 0 && (
                <div style={{ flex: 1, minHeight: 300, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', textAlign: 'center' }}>
                  <div style={{ width: 40, height: 40, borderRadius: '50%', background: C.bgDeep, display: 'flex', alignItems: 'center', justifyContent: 'center', marginBottom: 14 }}>
                    <Activity size={18} color={C.stone} />
                  </div>
                  <p style={{ fontFamily: S.body, fontSize: 13, fontWeight: 600, color: C.text, margin: '0 0 6px' }}>Monitoring just started</p>
                  <p style={{ fontFamily: S.mono, fontSize: 11, color: C.faint, margin: 0, letterSpacing: '0.03em' }}>Traces will appear here on first request</p>
                </div>
              )}
              {traces.length > 0 && visibleTraces.length === 0 && (
                <div style={{ padding: '24px 16px', textAlign: 'center' }}>
                  <p style={{ fontFamily: S.body, fontSize: 12, color: C.success, margin: 0 }}>✓ All clear — no errors in this window</p>
                </div>
              )}
              {visibleTraces.map(trace => {
                const st = TRACE_STATUS_STYLE[trace.status]
                const isOpen = expanded.has(trace.id)
                const fmtK = (n: number) => n >= 1000 ? `${(n / 1000).toFixed(0)}k` : String(n)
                return (
                  <div key={trace.id} style={{ borderBottom: `1px solid ${C.border}` }}>
                    <div
                      onClick={() => toggleTrace(trace.id)}
                      style={{ display: 'grid', gridTemplateColumns: '16px 1fr 80px 72px 60px 72px', gap: 10, padding: '10px 16px', cursor: 'pointer', alignItems: 'center', transition: 'background 0.1s' }}
                      onMouseEnter={e => { (e.currentTarget as HTMLElement).style.background = C.bgDeep }}
                      onMouseLeave={e => { (e.currentTarget as HTMLElement).style.background = 'transparent' }}
                    >
                      <ChevronRight size={12} color={C.faint} style={{ transition: 'transform 0.15s', transform: isOpen ? 'rotate(90deg)' : 'none' }} />
                      <div style={{ minWidth: 0 }}>
                        <div style={{ fontFamily: S.body, fontSize: 12, color: C.text, whiteSpace: 'nowrap' as const, overflow: 'hidden', textOverflow: 'ellipsis' }}>{trace.name}</div>
                        <span style={{ fontFamily: S.mono, fontSize: 9, color: C.faint }}>{trace.id}</span>
                      </div>
                      <span style={{ fontFamily: S.mono, fontSize: 9, padding: '3px 7px', borderRadius: 20, background: st.bg, color: st.color, letterSpacing: '0.05em', justifySelf: 'start' as const }}>{st.label}</span>
                      <span style={{ fontFamily: S.mono, fontSize: 12, color: trace.latency > 2000 ? C.coral : C.muted }}>
                        {trace.latency >= 1000 ? `${(trace.latency / 1000).toFixed(1)}s` : `${trace.latency}ms`}
                      </span>
                      <span style={{ fontFamily: S.mono, fontSize: 11, color: trace.tokens > 0 ? C.muted : C.faint }}>
                        {trace.tokens > 0 ? fmtK(trace.tokens) : '—'}
                      </span>
                      <span style={{ fontFamily: S.mono, fontSize: 11, color: C.faint }}>{trace.time}</span>
                    </div>
                    {isOpen && (
                      <div style={{ background: C.panel, borderTop: `1px solid ${C.border}` }}>
                        <div style={{ padding: '10px 16px 11px', borderBottom: `1px solid ${C.border}` }}>
                          <span style={{ display: 'block', fontFamily: S.mono, fontSize: 9, letterSpacing: '0.09em', color: C.faint, marginBottom: 5 }}>INPUT</span>
                          <span style={{ fontFamily: S.mono, fontSize: 11, color: C.muted, lineHeight: 1.6 }}>{trace.input ?? '—'}</span>
                        </div>
                        <div style={{ padding: '10px 16px 12px' }}>
                          <span style={{ display: 'block', fontFamily: S.mono, fontSize: 9, letterSpacing: '0.09em', color: C.faint, marginBottom: 5 }}>OUTPUT</span>
                          {trace.output
                            ? <span style={{ fontFamily: S.mono, fontSize: 11, color: C.muted, lineHeight: 1.6 }}>{trace.output}</span>
                            : <span style={{ fontFamily: S.mono, fontSize: 11, color: C.coral }}>Trace did not complete — no output recorded</span>
                          }
                        </div>
                      </div>
                    )}
                  </div>
                )
              })}
            </div>
          </div>
        </div>

        {/* right parking space */}
        <div />

      </div>
    </div>
  )
}

// ─── container accordion (active deployment) ─────────────────────────────────
type LogTimeRange = '15m' | '1h' | '6h' | '24h' | '7d'

const LOG_TIME_RANGE_OPTIONS: { value: LogTimeRange; label: string }[] = [
  { value: '15m', label: 'Last 15 min' },
  { value: '1h', label: 'Last 1 hour' },
  { value: '6h', label: 'Last 6 hours' },
  { value: '24h', label: 'Last 24 hours' },
  { value: '7d', label: 'Last 7 days' },
]

interface ActiveContainerAccordionProps {
  name: string
  url?: string
  ready: string
  uptime: string
  liveLogs: { deploymentId: string; podName: string; containerName: string }
  vars: { key: string; value: string; secret: boolean; source: string }[]
  isOpen: boolean
  onToggle: () => void
}

function ActiveContainerAccordion({ name, url, ready, uptime, liveLogs, vars, isOpen, onToggle }: ActiveContainerAccordionProps) {
  const [view, setView] = useState<'logs' | 'vars'>('logs')
  const [revealed, setRevealed] = useState<Set<string>>(new Set())
  const [logSearch, setLogSearch] = useState('')
  const [logTimeRange, setLogTimeRange] = useState<LogTimeRange>('24h')
  const [activeFilters, setActiveFilters] = useState<Set<'errors' | 'warnings'>>(new Set())
  const [copiedUrl, setCopiedUrl] = useState(false)

  const { data: logsRaw, isLoading, isFetching, error, refetch } = useDeploymentLogs(
    liveLogs.deploymentId,
    liveLogs.podName,
    liveLogs.containerName,
    logTimeRange,
    // Fetch whenever the row is expanded so logs are ready when switching from Variables → Logs
    { enabled: isOpen },
  )

  const logs = useMemo(() => (logsRaw ?? '').split('\n'), [logsRaw])

  const logErrorMessage = error
    ? (error as unknown as ApiError & { details?: string }).details
      ?? (error as unknown as ApiError).error_description
      ?? (error as Error).message
      ?? 'Failed to fetch logs'
    : null

  const toggleReveal = (key: string) =>
    setRevealed(prev => {
      const n = new Set(prev)
      if (n.has(key)) n.delete(key)
      else n.add(key)
      return n
    })

  const toggleFilter = (f: 'errors' | 'warnings') =>
    setActiveFilters(prev => {
      const n = new Set(prev)
      if (n.has(f)) n.delete(f)
      else n.add(f)
      return n
    })

  const errCount  = logs.filter(l => /error|failed|fatal/i.test(l)).length
  const warnCount = logs.filter(l => /warn|warning|retry|attempt/i.test(l)).length
  const filtered  = logs.filter(l => {
    if (activeFilters.size > 0) {
      const isErr  = /error|failed|fatal/i.test(l)
      const isWarn = /warn|warning|retry|attempt/i.test(l)
      if (activeFilters.has('errors') && activeFilters.has('warnings') && !isErr && !isWarn) return false
      if (activeFilters.has('errors') && !activeFilters.has('warnings') && !isErr) return false
      if (activeFilters.has('warnings') && !activeFilters.has('errors') && !isWarn) return false
    }
    if (logSearch && !l.toLowerCase().includes(logSearch.toLowerCase())) return false
    return true
  })

  const handleCopyUrl = (e: React.MouseEvent) => {
    e.stopPropagation()
    if (url) {
      navigator.clipboard.writeText(url)
      setCopiedUrl(true)
      setTimeout(() => setCopiedUrl(false), 900)
    }
  }

  return (
    <div style={{ marginBottom: 6 }}>
      <button
        className="dp-container-hdr"
        onClick={onToggle}
        style={{
          display: 'flex', alignItems: 'center', gap: 8,
          width: '100%', padding: '10px 14px',
          borderRadius: isOpen ? '8px 8px 0 0' : 8,
          border: `1px solid ${C.border}`,
          borderBottom: isOpen ? `1px solid ${C.bgDeep}` : `1px solid ${C.border}`,
          background: isOpen ? C.bgAlt : C.bg,
          cursor: 'pointer', textAlign: 'left' as const, transition: 'background 0.15s',
        }}
        onMouseEnter={e => { if (!isOpen) e.currentTarget.style.background = C.panel }}
        onMouseLeave={e => { if (!isOpen) e.currentTarget.style.background = C.bg }}
      >
        <ChevronRight size={13} color={C.faint} style={{ flexShrink: 0, transform: isOpen ? 'rotate(90deg)' : 'none', transition: 'transform 0.18s' }} />
        <svg width="16" height="16" viewBox="0 0 24 24" style={{ flexShrink: 0 }}>
          <circle cx="12" cy="12" r="10" fill="rgba(21,130,125,0.12)" />
          <path d="M7.5 12l3 3 6-6" stroke={C.tealMid} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
        </svg>
        <span style={{ fontFamily: S.body, fontSize: 13, fontWeight: 500, color: C.text }}>{name}</span>
        <span style={{ flex: 1 }} />
        {url && (
          <button
            onClick={handleCopyUrl}
            style={{
              position: 'relative', padding: '3px 10px', borderRadius: 5,
              border: `1px solid ${C.border}`, background: 'transparent',
              cursor: 'pointer', flexShrink: 0,
              fontFamily: S.mono, fontSize: 10, color: C.stone,
              transition: 'background 0.15s', maxWidth: 280, overflow: 'hidden',
              textOverflow: 'ellipsis', whiteSpace: 'nowrap' as const,
            }}
            onMouseEnter={e => { e.currentTarget.style.background = C.bgDeep }}
            onMouseLeave={e => { e.currentTarget.style.background = 'transparent' }}
          >
            {url}
            {copiedUrl && (
              <span style={{
                position: 'absolute', right: 6, top: '50%', transform: 'translateY(-50%)',
                color: C.tealMid, display: 'flex', alignItems: 'center', gap: 4,
                fontFamily: S.body, fontSize: 11, whiteSpace: 'nowrap' as const,
                background: 'inherit', paddingLeft: 4,
              }}>
                <Check size={10} /> Copied
              </span>
            )}
          </button>
        )}
        <span style={{ fontFamily: S.mono, fontSize: 11, color: C.faint, flexShrink: 0, marginLeft: 8 }}>
          {ready} ready · {uptime}
        </span>
      </button>

      {isOpen && (
        <div style={{ border: `1px solid ${C.border}`, borderTop: 'none', borderRadius: '0 0 8px 8px', overflow: 'hidden' }}>
          {/* logs / vars toggle */}
          <div style={{ display: 'flex', background: C.bgAlt, borderBottom: `1px solid ${C.border}` }}>
            {(['logs', 'vars'] as const).map(v => (
              <button key={v} onClick={() => setView(v)} style={{
                padding: '7px 14px', background: 'none', border: 'none', cursor: 'pointer',
                fontFamily: S.body, fontSize: 12,
                fontWeight: view === v ? 600 : 400,
                color: view === v ? C.text : C.faint,
                borderBottom: view === v ? `2px solid ${C.tealMid}` : '2px solid transparent',
                transition: 'color 0.12s', textTransform: 'capitalize' as const,
              }}>
                {v === 'vars' ? 'Variables' : 'Logs'}
              </button>
            ))}
          </div>

          {/* variables view */}
          {view === 'vars' && (
            <div style={{ background: C.bg }}>
              {vars.length === 0 ? (
                <div style={{ padding: '16px', fontFamily: S.mono, fontSize: 11, color: C.faint }}>No variables</div>
              ) : vars.map((v, vi) => {
                const isRevealed = revealed.has(v.key)
                const isSecret = v.secret || v.value.startsWith('sk-') || v.value.startsWith('secret:') || v.value.includes('••')
                const srcStyle = v.source === 'input'
                  ? { bg: 'rgba(21,130,125,0.1)', color: C.tealMid, label: 'input' }
                  : v.source === 'injected'
                    ? { bg: 'rgba(212,143,30,0.1)', color: C.amber, label: 'injected' }
                    : { bg: C.bgDeep, color: C.stone, label: 'static' }
                return (
                  <div key={v.key} style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '9px 16px', borderBottom: vi < vars.length - 1 ? `1px solid ${C.border}` : 'none' }}>
                    <span style={{ fontFamily: S.mono, fontSize: 10, color: C.stone, flexShrink: 0, userSelect: 'none' as const }}>{'{}'}</span>
                    <span style={{ fontFamily: S.mono, fontSize: 12, color: C.text, minWidth: 160, flexShrink: 0 }}>{v.key}</span>
                    <div style={{ flex: 1, display: 'flex', alignItems: 'center', gap: 6, minWidth: 0 }}>
                      <span style={{ fontFamily: S.mono, fontSize: 12, color: C.faint, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' as const }}>
                        {isSecret && !isRevealed ? '•••••••••' : v.value}
                      </span>
                      {isSecret && (
                        <button onClick={() => toggleReveal(v.key)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: C.stone, display: 'flex', padding: 2, flexShrink: 0 }}>
                          {isRevealed ? <EyeOff size={13} /> : <Eye size={13} />}
                        </button>
                      )}
                    </div>
                    <span style={{ fontFamily: S.mono, fontSize: 9, letterSpacing: '0.08em', padding: '2px 6px', borderRadius: 4, background: srcStyle.bg, color: srcStyle.color, flexShrink: 0 }}>{srcStyle.label}</span>
                  </div>
                )
              })}
            </div>
          )}

          {/* logs view */}
          {view === 'logs' && (
            <div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '8px 14px', background: C.bgAlt, borderBottom: `1px solid ${C.border}` }}>
                {([
                  { key: 'errors'   as const, label: `Errors (${errCount})`,    accent: '#dc2626', activeBg: '#fef2f2', activeBdr: '#fca5a5' },
                  { key: 'warnings' as const, label: `Warnings (${warnCount})`, accent: '#d97706', activeBg: '#fffbeb', activeBdr: '#fcd34d' },
                ]).map(f => {
                  const active = activeFilters.has(f.key)
                  return (
                    <button key={f.key} onClick={() => toggleFilter(f.key)} style={{
                      display: 'flex', alignItems: 'center', gap: 5,
                      padding: '4px 8px', borderRadius: 6,
                      border: `1px solid ${active ? f.activeBdr : C.border}`,
                      cursor: 'pointer', fontFamily: S.body, fontSize: 11, transition: 'all 0.12s',
                      background: active ? f.activeBg : 'transparent',
                      color: active ? f.accent : C.muted,
                      fontWeight: active ? 500 : 400,
                      whiteSpace: 'nowrap' as const,
                    }}>
                      {f.label}
                      {active && <X size={9} style={{ marginLeft: 2, flexShrink: 0 }} />}
                    </button>
                  )
                })}
                <div style={{ flex: 1 }} />
                <select
                  value={logTimeRange}
                  onChange={e => setLogTimeRange(e.target.value as LogTimeRange)}
                  style={{
                    padding: '4px 24px 4px 10px', borderRadius: 6, border: `1px solid ${C.border}`,
                    background: C.bg, fontFamily: S.body, fontSize: 11, color: C.muted,
                    cursor: 'pointer', outline: 'none', appearance: 'none' as const,
                    backgroundImage: `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='10' viewBox='0 0 24 24' fill='none' stroke='%236b7e7c' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpolyline points='6 9 12 15 18 9'%3E%3C/polyline%3E%3C/svg%3E")`,
                    backgroundRepeat: 'no-repeat', backgroundPosition: 'right 8px center',
                  }}
                >
                  {LOG_TIME_RANGE_OPTIONS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
                </select>
                <div style={{ display: 'flex', alignItems: 'center', gap: 5, padding: '4px 8px', borderRadius: 5, border: `1px solid ${C.border}`, background: C.bg }}>
                  <Search size={11} color={C.faint} />
                  <input type="text" placeholder="Find in logs" value={logSearch} onChange={e => setLogSearch(e.target.value)}
                    style={{ background: 'none', border: 'none', outline: 'none', fontFamily: S.body, fontSize: 12, color: C.muted, width: 120, caretColor: C.tealMid }} />
                </div>
                <button
                  type="button"
                  title="Refresh logs"
                  onClick={() => void refetch()}
                  disabled={isFetching}
                  style={{
                    background: 'none', border: `1px solid ${C.border}`, cursor: isFetching ? 'wait' : 'pointer',
                    padding: '4px 6px', borderRadius: 5, color: C.faint, display: 'flex', opacity: isFetching ? 0.7 : 1,
                  }}
                >
                  <RefreshCw size={11} className={isFetching ? 'dp-spin' : undefined} />
                </button>
                <button
                  type="button"
                  onClick={() => navigator.clipboard.writeText(logs.join('\n'))}
                  style={{ background: 'none', border: `1px solid ${C.border}`, cursor: 'pointer', padding: '4px 6px', borderRadius: 5, color: C.faint, display: 'flex' }}
                >
                  <Copy size={11} />
                </button>
              </div>
              <div style={{ background: C.panel, padding: '10px 0 14px' }}>
                {isLoading ? (
                  <div style={{ padding: '12px 18px', display: 'flex', alignItems: 'center', gap: 8, fontFamily: S.mono, fontSize: 11, color: C.faint }}>
                    <Loader2 size={14} className="dp-spin" />
                    Loading logs…
                  </div>
                ) : logErrorMessage ? (
                  <div style={{ padding: '12px 18px', fontFamily: S.mono, fontSize: 11, color: C.coral, lineHeight: 1.5 }}>
                    {logErrorMessage}
                  </div>
                ) : filtered.length === 0 ? (
                  <div style={{ padding: '12px 18px', fontFamily: S.mono, fontSize: 11, color: C.faint }}>
                    {logs.length === 0 ? 'No log lines in this time window' : 'No matching lines'}
                  </div>
                ) : filtered.map((line, li) => (
                  <div key={li} className="dp-log" style={{ display: 'flex', alignItems: 'baseline', padding: '1px 0' }}>
                    <span style={{ fontFamily: S.mono, fontSize: 11, color: C.stone, minWidth: 56, textAlign: 'right' as const, paddingRight: 18, flexShrink: 0, userSelect: 'none' as const }}>{li + 1}</span>
                    <span style={{ fontFamily: S.mono, fontSize: 12, color: logLineColor(line), lineHeight: 1.75 }}>{line}</span>
                  </div>
                ))}
                {!isLoading && !logErrorMessage && filtered.length > 0 && (
                  <div style={{ display: 'flex', alignItems: 'baseline', padding: '1px 0', marginTop: 2 }}>
                    <span style={{ minWidth: 56, paddingRight: 18, flexShrink: 0 }} />
                    <span className="dp-blink" style={{ fontFamily: S.mono, fontSize: 12, color: C.tealMid }}>▊</span>
                  </div>
                )}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// ─── deployments tab ──────────────────────────────────────────────────────────
function formatDurationMs(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return '—'
  const sec = Math.max(0, Math.round(ms / 1000))
  if (sec < 60) return `${sec}s`
  const m = Math.floor(sec / 60)
  if (m < 60) return `${m}m`
  const h = Math.floor(m / 60)
  if (h < 48) return `${h}h`
  return `${Math.floor(h / 24)}d`
}

/** Prefer history `deployed_at`; for the live deployment row, fall back to `created_at` if the timestamp is missing/invalid. */
function resolveDeployedAtMs(h: ApiDeploymentHistoryRecord, live: AgentDeployment): number {
  const fromHist = new Date(h.deployed_at).getTime()
  if (h.id === live.id) {
    const fromLive = new Date(live.created_at).getTime()
    if (!Number.isFinite(fromHist) || Number.isNaN(fromHist)) return fromLive
    return fromHist
  }
  return fromHist
}

function deploymentHistoryDurationMs(
  h: ApiDeploymentHistoryRecord,
  idx: number,
  merged: ApiDeploymentHistoryRecord[],
  live: AgentDeployment,
  isCurrent: boolean,
): number | null {
  const start = resolveDeployedAtMs(h, live)
  if (!Number.isFinite(start) || Number.isNaN(start)) return null
  if (isCurrent) return Date.now() - start
  if (h.undeployed_at) {
    const end = new Date(h.undeployed_at).getTime()
    if (!Number.isFinite(end) || Number.isNaN(end)) return null
    return end - start
  }
  if (idx > 0) {
    const end = resolveDeployedAtMs(merged[idx - 1], live)
    if (!Number.isFinite(end) || Number.isNaN(end)) return null
    return end - start
  }
  return null
}

type DeployHistoryStatus = 'active' | 'ready' | 'failed' | 'undeployed'

function deploymentHistoryUiStatus(h: ApiDeploymentHistoryRecord, live: AgentDeployment): DeployHistoryStatus {
  if (h.undeployed_at) return 'undeployed'
  if (h.id === live.id) {
    const ds = mapDeploymentStatus(live)
    if (ds === 'error') return 'failed'
    if (ds === 'pending') return 'ready'
    return 'active'
  }
  const st = (h.status ?? '').toLowerCase()
  if (st === 'error' || st === 'failed') return 'failed'
  return 'ready'
}

interface DeploymentHistoryTableRow {
  id: string
  status: DeployHistoryStatus
  build: string
  duration: string
  time: string
  isCurrent: boolean
  rowLabel: string
  source: ApiDeploymentHistoryRecord
}

const DEPLOY_STATUS_STYLE: Record<DeployHistoryStatus, { color: string; label: string }> = {
  active: { color: C.success, label: 'Active' },
  ready: { color: C.success, label: 'Ready' },
  failed: { color: C.coral, label: 'Failed' },
  undeployed: { color: C.stone, label: 'Undeployed' },
}

function DeploymentsTab({
  deployment,
  account,
  onOpenConfigure,
}: {
  deployment: AgentDeployment
  account: string
  onOpenConfigure?: () => void
}) {
  const [deploySearch, setDeploySearch] = useState('')
  const [deployStatus, setDeployStatus] = useState<string[]>([])
  const [historyPreset, setHistoryPreset] = useState<'all' | '7d' | '30d'>('all')
  const [expandedDeploy, setExpandedDeploy] = useState<string | null>(deployment.id)
  const [openDeployMenu, setOpenDeployMenu] = useState<string | null>(null)
  const [openContainers, setOpenContainers] = useState<Set<string>>(new Set())

  const { data: historyData, isLoading: historyLoading, isError: historyError } = useDeploymentHistory(account, deployment.name)

  const toggleContainer = (id: string) =>
    setOpenContainers(prev => {
      const n = new Set(prev)
      if (n.has(id)) n.delete(id)
      else n.add(id)
      return n
    })

  const containers = (deployment.pods ?? []).flatMap(pod =>
    (pod.containers ?? []).map(c => ({
      id: `${pod.name}:${c.name}`,
      podName: pod.name,
      name: c.name,
      ready: c.ready ? '1/1' : '0/1',
      uptime: pod.age ?? '—',
      vars: (c.env ?? []).map(e => {
        const val = e.value ?? ''
        return {
          key: e.name,
          value: val,
          secret: val.startsWith('sk-') || val.startsWith('secret:') || val.includes('••'),
          source: e.from ?? 'static',
        }
      }),
      url: undefined as string | undefined,
    }))
  )

  const externalUrls = deployment.external_urls ?? []
  if (externalUrls.length > 0 && containers.length > 0) {
    const agentContainer = containers.find(c => c.name.includes('agent')) ?? containers[0]
    if (agentContainer) agentContainer.url = externalUrls[0]?.url
  }

  const allRows = useMemo((): DeploymentHistoryTableRow[] => {
    const fromApi = historyData?.deployments ?? []
    const seen = new Set(fromApi.map(h => h.id))
    const merged: ApiDeploymentHistoryRecord[] = [...fromApi]
    if (!seen.has(deployment.id)) {
      merged.unshift({
        id: deployment.id,
        agent_name: deployment.name,
        build_id: deployment.build_id,
        namespace: deployment.namespace,
        status: deployment.status,
        deployed_at: deployment.created_at,
        spec: {},
      })
    }
    merged.sort((a, b) => resolveDeployedAtMs(b, deployment) - resolveDeployedAtMs(a, deployment))

    const cutoff =
      historyPreset === 'all' ? 0
        : historyPreset === '7d' ? Date.now() - 7 * 86400000
          : Date.now() - 30 * 86400000

    let rows: DeploymentHistoryTableRow[] = merged.map((h, idx) => {
      const isCurrent = h.id === deployment.id
      const status = deploymentHistoryUiStatus(h, deployment)
      const build = h.build_id?.slice(0, 8) || '—'
      const rowLabel = isCurrent
        ? (deployment.display_name || deployment.name)
        : `${deployment.name} · ${build}`
      const durMs = deploymentHistoryDurationMs(h, idx, merged, deployment, isCurrent)
      const deployedAtIso = new Date(resolveDeployedAtMs(h, deployment)).toISOString()
      return {
        id: h.id,
        status,
        build,
        duration: durMs !== null ? formatDurationMs(durMs) : '—',
        time: formatDate(deployedAtIso),
        isCurrent,
        rowLabel,
        source: h,
      }
    })

    if (cutoff > 0) {
      rows = rows.filter((r) => resolveDeployedAtMs(r.source, deployment) >= cutoff)
    }

    const q = deploySearch.trim().toLowerCase()
    if (q) {
      rows = rows.filter(
        (r) =>
          r.id.toLowerCase().includes(q) ||
          r.build.toLowerCase().includes(q) ||
          deployment.name.toLowerCase().includes(q) ||
          (deployment.display_name?.toLowerCase().includes(q) ?? false),
      )
    }

    if (deployStatus.length > 0) {
      rows = rows.filter((r) => deployStatus.includes(r.status))
    }

    return rows
  }, [historyData, deployment, historyPreset, deploySearch, deployStatus])

  return (
    <div style={{ display: 'grid', gridTemplateColumns: '1fr minmax(0, 900px) 1fr', gap: 12, alignItems: 'start' }}>
      <div />
      <div style={{ display: 'flex', flexDirection: 'column', gap: 0 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 14 }}>
          <span style={{ fontFamily: S.body, fontSize: 17, fontWeight: 600, color: C.teal, flex: 1 }}>Deployments</span>
          <div style={{ display: 'flex', alignItems: 'center', gap: 5, padding: '5px 10px', borderRadius: 7, border: `1px solid ${C.border}`, background: C.bg }}>
            <Search size={12} color={C.faint} />
            <input type="text" placeholder="Search by name, build, id" value={deploySearch} onChange={e => setDeploySearch(e.target.value)}
              style={{ background: 'none', border: 'none', outline: 'none', fontFamily: S.body, fontSize: 12, color: C.muted, width: 200, caretColor: C.tealMid }} />
          </div>
          <MultiSelect
            options={[
              { value: 'active', label: 'Active', color: C.tealMid },
              { value: 'ready', label: 'Ready', color: C.success },
              { value: 'failed', label: 'Failed', color: C.coral },
              { value: 'undeployed', label: 'Undeployed', color: C.stone },
            ]}
            selected={deployStatus}
            onChange={setDeployStatus}
            placeholder="All statuses"
          />
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <Calendar size={12} color={C.faint} />
            <select
              value={historyPreset}
              onChange={e => setHistoryPreset(e.target.value as typeof historyPreset)}
              style={{
                padding: '5px 22px 5px 8px', borderRadius: 7, border: `1px solid ${C.border}`,
                background: C.bg, fontFamily: S.body, fontSize: 12, color: C.muted,
                cursor: 'pointer', outline: 'none', appearance: 'none' as const,
                backgroundImage: `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='10' viewBox='0 0 24 24' fill='none' stroke='%236b7e7c' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpolyline points='6 9 12 15 18 9'%3E%3C/polyline%3E%3C/svg%3E")`,
                backgroundRepeat: 'no-repeat', backgroundPosition: 'right 6px center',
              }}
            >
              <option value="all">All time</option>
              <option value="7d">Last 7 days</option>
              <option value="30d">Last 30 days</option>
            </select>
          </div>
        </div>

        {historyError && (
          <p style={{ fontFamily: S.mono, fontSize: 11, color: C.coral, margin: '0 0 10px' }}>
            Could not load deployment history from the server.
          </p>
        )}

        <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, overflow: 'hidden' }}>
          <div style={{ display: 'grid', gridTemplateColumns: '16px minmax(160px, 1fr) 80px 72px 110px 72px 32px', gap: 12, padding: '8px 16px', borderBottom: `1px solid ${C.border}`, background: C.bgDeep }}>
            {['', 'Deployment', 'Status', 'Duration', 'Build No.', 'Deployed on', ''].map(h => (
              <span key={h} style={{ fontFamily: S.mono, fontSize: 9, letterSpacing: '0.07em', color: C.faint }}>{h.toUpperCase()}</span>
            ))}
          </div>

          {historyLoading ? (
            <div style={{ padding: '20px 16px', display: 'flex', alignItems: 'center', gap: 10, fontFamily: S.mono, fontSize: 11, color: C.faint }}>
              <Loader2 size={14} className="dp-spin" />
              Loading deployment history…
            </div>
          ) : allRows.length === 0 ? (
            <div style={{ padding: '20px 16px', fontFamily: S.mono, fontSize: 11, color: C.faint }}>
              No deployments match your filters.
            </div>
          ) : allRows.map((d, i) => {
            const ds = DEPLOY_STATUS_STYLE[d.status]
            const isCurrent = d.isCurrent
            const isExpanded = expandedDeploy === d.id
            return (
              <div key={d.id} style={{ borderBottom: i < allRows.length - 1 ? `1px solid ${C.border}` : 'none' }}>
                <div
                  onClick={() => setExpandedDeploy(isExpanded ? null : d.id)}
                  style={{
                    display: 'grid', gridTemplateColumns: '16px minmax(160px, 1fr) 80px 72px 110px 72px 32px', gap: 12,
                    padding: '12px 16px', alignItems: 'center', cursor: 'pointer',
                    borderLeft: isCurrent ? `3px solid ${C.tealMid}` : '3px solid transparent',
                    background: isExpanded ? C.bgDeep : isCurrent ? 'rgba(21,130,125,0.02)' : 'transparent',
                    transition: 'background 0.12s',
                  }}
                >
                  <ChevronRight size={12} color={C.faint} style={{ transition: 'transform 0.15s', transform: isExpanded ? 'rotate(90deg)' : 'none' }} />
                  <div style={{ minWidth: 0 }}>
                    <div style={{ fontFamily: S.body, fontSize: 12, fontWeight: 500, color: C.text, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' as const }} title={d.rowLabel}>
                      {d.rowLabel}
                    </div>
                  </div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 7 }}>
                    <span style={{ width: 8, height: 8, borderRadius: '50%', background: ds.color, display: 'inline-block', flexShrink: 0 }} />
                    <span style={{ fontFamily: S.mono, fontSize: 10, letterSpacing: '0.06em', color: ds.color, fontWeight: 500 }}>{ds.label.toUpperCase()}</span>
                  </div>
                  <span style={{ fontFamily: S.mono, fontSize: 11, color: C.faint }}>{d.duration}</span>
                  <span style={{ fontFamily: S.mono, fontSize: 11, fontWeight: 600, color: C.muted }}>{d.build}</span>
                  <span style={{ fontFamily: S.mono, fontSize: 11, color: C.faint, whiteSpace: 'nowrap' as const }}>{d.time}</span>
                  <div style={{ position: 'relative' }} onClick={e => e.stopPropagation()}>
                    <button type="button" onClick={() => setOpenDeployMenu(openDeployMenu === d.id ? null : d.id)}
                      style={{ background: 'none', border: 'none', cursor: 'pointer', color: C.faint, display: 'flex', padding: 4, borderRadius: 4 }}
                      onMouseEnter={e => { e.currentTarget.style.background = C.bgDeep }}
                      onMouseLeave={e => { e.currentTarget.style.background = 'none' }}
                    ><MoreVertical size={13} /></button>
                    {openDeployMenu === d.id && (
                      <>
                        <div onClick={() => setOpenDeployMenu(null)} style={{ position: 'fixed', inset: 0, zIndex: 10 }} />
                        <div style={{ position: 'absolute', right: 0, top: 'calc(100% + 4px)', zIndex: 20, minWidth: 160, background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 8, overflow: 'hidden', boxShadow: '0 6px 20px rgba(0,0,0,0.1)' }}>
                          <button type="button" onClick={() => { setOpenDeployMenu(null); onOpenConfigure?.() }} style={{
                            width: '100%', display: 'flex', alignItems: 'center', gap: 8,
                            padding: '9px 14px', background: 'none', border: 'none',
                            cursor: 'pointer', fontFamily: S.body, fontSize: 12, color: C.text, textAlign: 'left' as const,
                          }}
                            onMouseEnter={e => { e.currentTarget.style.background = C.bgDeep }}
                            onMouseLeave={e => { e.currentTarget.style.background = 'none' }}
                          >Redeploy…</button>
                          <div style={{ height: 1, background: C.border }} />
                          <button
                            type="button"
                            disabled={!isCurrent || containers.length === 0}
                            title={!isCurrent ? 'Only the live deployment has pod logs here' : undefined}
                            onClick={() => {
                              setOpenDeployMenu(null)
                              if (isCurrent && containers.length > 0) {
                                setExpandedDeploy(d.id)
                                setOpenContainers(new Set([containers[0].id]))
                              }
                            }}
                            style={{
                              width: '100%', display: 'flex', alignItems: 'center', gap: 8,
                              padding: '9px 14px', background: 'none', border: 'none',
                              cursor: isCurrent && containers.length > 0 ? 'pointer' : 'not-allowed',
                              fontFamily: S.body, fontSize: 12, color: C.text, textAlign: 'left' as const,
                              opacity: isCurrent && containers.length > 0 ? 1 : 0.45,
                            }}
                            onMouseEnter={e => { if (isCurrent && containers.length > 0) e.currentTarget.style.background = C.bgDeep }}
                            onMouseLeave={e => { e.currentTarget.style.background = 'none' }}
                          >View pod logs</button>
                          <div style={{ height: 1, background: C.border }} />
                          <button type="button" disabled title="Rollback is not available yet" style={{
                            width: '100%', display: 'flex', alignItems: 'center', gap: 8,
                            padding: '9px 14px', background: 'none', border: 'none',
                            cursor: 'not-allowed', fontFamily: S.body, fontSize: 12, color: C.coral, textAlign: 'left' as const,
                            opacity: 0.45,
                          }}
                          >Rollback</button>
                        </div>
                      </>
                    )}
                  </div>
                </div>

                {isExpanded && (
                  <div style={{ padding: '8px 16px 16px', borderTop: `1px solid ${C.border}`, background: C.bg }}>
                    {isCurrent ? (
                      containers.length === 0 ? (
                        <p style={{ fontFamily: S.mono, fontSize: 11, color: C.faint, margin: 0 }}>No container data available</p>
                      ) : containers.map(c => (
                        <ActiveContainerAccordion
                          key={c.id}
                          name={c.name}
                          url={c.url}
                          ready={c.ready}
                          uptime={c.uptime}
                          liveLogs={{
                            deploymentId: deployment.id,
                            podName: c.podName,
                            containerName: c.name,
                          }}
                          vars={c.vars}
                          isOpen={openContainers.has(c.id)}
                          onToggle={() => toggleContainer(c.id)}
                        />
                      ))
                    ) : (
                      <p style={{ fontFamily: S.mono, fontSize: 11, color: C.faint, margin: 0 }}>
                        Pod logs are only available for the live deployment ({deployment.id.slice(0, 8)}…).
                      </p>
                    )}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      </div>
      <div />
    </div>
  )
}

// ─── configure panel ──────────────────────────────────────────────────────────
const PANEL_FORM_ID = "configure-side-panel-form"

function ConfigurePanelLoaded({ deployment, account, template, onClose, onRedeploy }: {
  deployment: AgentDeployment; account: string
  template: import("@/lib/api").DeploymentTemplate; onClose: () => void; onRedeploy?: () => void
}) {
  const initialValues = useMemo(() => extractInitialValues(template, account), [template, account])
  const form = useDeployForm(account, deployment.name, { initialTemplate: template, skipTemplateFetch: true, initialValues })

  const trackedState: TrackedFormState = {
    deployName: form.deployName,
    variableValues: form.variableValues,
    selectedAdapters: form.selectedAdapters,
    adapterCredentials: form.adapterCredentials,
    ingestionSchedules: form.ingestionSchedules,
  }
  const initialTrackedState: TrackedFormState = {
    deployName: initialValues.deployName ?? '',
    variableValues: initialValues.variableValues ?? {},
    selectedAdapters: initialValues.selectedAdapters ?? ['web'],
    adapterCredentials: initialValues.adapterCredentials ?? {},
    ingestionSchedules: initialValues.ingestionSchedules ?? {},
  }
  const changes = useChangeTracking(initialTrackedState, trackedState)

  const handleSubmit = async (e: React.SyntheticEvent<HTMLFormElement>) => {
    e.preventDefault()
    if (!form.trySubmit()) return
    try { await form.deploy(); onClose(); onRedeploy?.() } catch { /* captured in form.deployError */ }
  }

  const manualIngestions = deployment.manual_ingestions ?? []

  return (
    <div className="flex flex-col h-full w-[420px] bg-background border-l border-border">

      {/* header */}
      <div className="flex items-center gap-2 h-[63px] shrink-0 px-4 border-b border-border">
        <Settings2 className="size-3.5 text-primary shrink-0" />
        <span className="flex-1 text-sm font-semibold text-foreground">Configure</span>
        <Button variant="ghost" size="icon" className="size-7 shrink-0" onClick={onClose}>
          <X className="size-4" />
        </Button>
      </div>

      {/* scrollable body + footer */}
      <form id={PANEL_FORM_ID} onSubmit={handleSubmit} className="flex-1 min-h-0 flex flex-col">
        <div className="flex-1 overflow-y-auto px-5 py-6">
          <DeployFormFields
            form={form}
            hideAccountPicker
            ingestionExtra={
              manualIngestions.length > 0
                ? <ManualTriggers
                    deploymentId={deployment.id}
                    names={manualIngestions}
                    account={account}
                    hasBorderTop={form.scheduleIngestions.length > 0}
                  />
                : undefined
            }
          />
        </div>

        {changes.isDirty && (
          <div className="flex flex-col gap-2 p-4 border-t border-border bg-muted/40 shrink-0">
            <Button type="submit" disabled={form.isDeploying} className="w-full">
              {form.isDeploying ? <Loader2 className="size-3.5 animate-spin" /> : <Rocket className="size-3.5" />}
              {form.isDeploying ? 'Redeploying…' : changes.requiresRedeploy ? 'Save & Redeploy' : 'Save'}
            </Button>
            <Button type="button" variant="ghost" className="w-full" onClick={() => form.reset(initialValues)}>
              Cancel
            </Button>
          </div>
        )}
      </form>

    </div>
  )
}

function ManualTriggers({ deploymentId, names, account, hasBorderTop }: { deploymentId: string; names: string[]; account: string; hasBorderTop: boolean }) {
  const triggerMutation = useTriggerIngestion(account)
  const [triggeredName, setTriggeredName] = useState<string | null>(null)

  useEffect(() => {
    if (!triggeredName) return
    const timer = setTimeout(() => setTriggeredName(null), 2000)
    return () => clearTimeout(timer)
  }, [triggeredName])

  return (
    <div className={hasBorderTop ? "mt-6 pt-6 border-t border-border" : ""}>
      <p className="text-sm font-medium text-foreground mb-3">Manual Triggers</p>
      <div className="flex flex-wrap gap-2">
        {names.map((name) => {
          const isTriggering = triggerMutation.isPending && triggerMutation.variables?.ingestion === name
          const justTriggered = triggeredName === name
          return (
            <Button key={name} type="button" variant="outline" size="sm"
              disabled={isTriggering || justTriggered}
              onClick={() => triggerMutation.mutate({ deploymentId, ingestion: name }, { onSuccess: () => setTriggeredName(name) })}
            >
              {justTriggered ? <Check className="size-3.5 text-green-600" /> : isTriggering ? <Loader2 className="size-3.5 animate-spin" /> : <Play className="size-3.5" />}
              {slugToTitle(name)}
            </Button>
          )
        })}
      </div>
      {triggerMutation.isError && <p className="text-sm text-destructive mt-2">Failed to trigger ingestion. Please try again.</p>}
    </div>
  )
}

function ConfigurePanel({ deployment, account, onClose, onRedeploy }: { deployment: AgentDeployment; account: string; onClose: () => void; onRedeploy?: () => void }) {
  const { data: template, isLoading, isError } = usePrefilledDeploymentTemplate(account, deployment.name, deployment.id)

  const shell = (children: React.ReactNode) => (
    <div className="flex flex-col h-full w-[420px] bg-background border-l border-border">
      <div className="flex items-center gap-2 h-[63px] shrink-0 px-4 border-b border-border">
        <Settings2 className="size-3.5 text-primary shrink-0" />
        <span className="flex-1 text-sm font-semibold text-foreground">Configure</span>
        <Button variant="ghost" size="icon" className="size-7 shrink-0" onClick={onClose}>
          <X className="size-4" />
        </Button>
      </div>
      <div className="flex-1 flex items-center justify-center">{children}</div>
    </div>
  )

  if (isLoading) return shell(<Loader2 className="size-5 animate-spin text-muted-foreground" />)
  if (isError || !template) return shell(<p className="text-xs text-destructive">Failed to load configuration.</p>)
  return <ConfigurePanelLoaded deployment={deployment} account={account} template={template} onClose={onClose} onRedeploy={onRedeploy} />
}

// ─── main component ───────────────────────────────────────────────────────────
interface ActiveDetailViewProps {
  deployment: AgentDeployment;
  account: string;
  isPersonal: boolean;
  initialTab?: 'monitor' | 'deployments';
  onRedeploy?: () => void;
}

export function ActiveDetailView({ deployment, account, isPersonal, initialTab = 'monitor', onRedeploy }: ActiveDetailViewProps) {
  const navigate = useNavigate()
  const [tab, setTab] = useState<'monitor' | 'deployments'>(initialTab)
  const [configOpen, setConfigOpen] = useState(false)
  const displayName = deployment.display_name || deployment.name
  const backPath = isPersonal ? '/agents' : `/${account}`

  // playground logic
  const PLAYGROUND_LAUNCH_BASE_URL = typeof import.meta !== 'undefined' && import.meta.env
    ? (import.meta.env["VITE_PLAYGROUND_LAUNCH_URL"] as string | undefined)
    : undefined
  const urls = deployment.external_urls ?? []
  const backendUrl = selectPlaygroundBackendUrl(urls)
  const local = isLocalEnv()
  const [pfCopied, setPfCopied] = useState(false)
  const [pfOpen, setPfOpen] = useState(false)
  const pfRef = useRef<HTMLDivElement>(null)
  const portForwardCmd = buildPortForwardCommand(deployment.namespace, deployment.name)

  useEffect(() => {
    if (!pfOpen) return
    const handler = (e: MouseEvent) => {
      if (pfRef.current && !pfRef.current.contains(e.target as Node)) setPfOpen(false)
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [pfOpen])

  const handlePlaygroundClick = () => {
    if (local) {
      window.open('http://localhost:3737', '_blank', 'noopener,noreferrer')
    } else if (backendUrl) {
      window.open(buildPlaygroundLaunchUrl(backendUrl, PLAYGROUND_LAUNCH_BASE_URL), '_blank', 'noopener,noreferrer')
    }
  }

  const playgroundAvailable = !!backendUrl || local

  return (
    <div style={{ display: 'flex', flex: 1, flexDirection: 'column', background: C.bg, minHeight: 0 }}>
      <Styles />

      {/* ── TOP BAR ── */}
      <header style={{
        background: C.panel,
        borderBottom: `1px solid ${C.border}`,
        position: 'sticky', top: 0, zIndex: 40,
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        padding: '0 40px', height: 63, flexShrink: 0,
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <button
            onClick={() => navigate(backPath)}
            style={{ background: 'none', border: 'none', cursor: 'pointer', color: C.faint, display: 'flex', padding: 4 }}
          >
            <ArrowLeft size={15} />
          </button>
          <div style={{ borderRadius: 8, overflow: 'hidden', flexShrink: 0, lineHeight: 0 }}>
            <AgentIdentity account={account} name={deployment.name} size={26} className="rounded-sm" />
          </div>
          <span style={{ fontFamily: S.body, fontSize: 13, fontWeight: 600, color: C.text }}>{displayName}</span>
          {(() => {
            const ds = mapDeploymentStatus(deployment)
            const badge =
              ds === 'error'
                ? { bg: C.coralBg, bdr: C.coralBdr, dot: C.coral, label: 'Error' }
                : ds === 'pending'
                  ? { bg: C.amberBg, bdr: C.amberBdr, dot: C.amber, label: 'Deploying' }
                  : ds === 'inactive'
                    ? { bg: C.bgDeep, bdr: C.border, dot: C.faint, label: 'Inactive' }
                    : { bg: 'rgba(21,130,125,0.08)', bdr: 'rgba(21,130,125,0.22)', dot: C.tealMid, label: 'Live' }
            return (
              <span style={{
                display: 'inline-flex', alignItems: 'center', gap: 5,
                padding: '2px 10px', borderRadius: 99,
                background: badge.bg, border: `1px solid ${badge.bdr}`,
                fontFamily: S.mono, fontSize: 10, letterSpacing: '0.06em', color: badge.dot,
              }}>
                <span style={{ width: 5, height: 5, borderRadius: '50%', background: badge.dot, display: 'inline-block' }} />
                {badge.label}
              </span>
            )
          })()}
          <KebabMenu deploymentId={deployment.id} />
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <div ref={pfRef} style={{ position: 'relative' }}>
            <div style={{ display: 'flex' }}>
              <button
                onClick={playgroundAvailable ? handlePlaygroundClick : undefined}
                disabled={!playgroundAvailable}
                title={!playgroundAvailable ? 'No external URL available' : undefined}
                style={{
                  display: 'inline-flex', alignItems: 'center', gap: 6,
                  padding: '6px 12px', borderRadius: local ? '6px 0 0 6px' : 6,
                  cursor: playgroundAvailable ? 'pointer' : 'default',
                  background: 'transparent', border: `1px solid ${C.border}`,
                  borderRight: local ? 'none' : undefined,
                  fontFamily: S.body, fontSize: 13, color: playgroundAvailable ? C.muted : C.faint,
                  transition: 'background 0.12s', opacity: playgroundAvailable ? 1 : 0.5,
                }}
                onMouseEnter={e => { if (playgroundAvailable) e.currentTarget.style.background = C.bgDeep }}
                onMouseLeave={e => { e.currentTarget.style.background = 'transparent' }}
              >
                <Play size={13} /> Playground
              </button>
              {local && (
                <button
                  onClick={() => setPfOpen(o => !o)}
                  title="Port-forward setup"
                  style={{
                    display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
                    width: 28, borderRadius: '0 6px 6px 0',
                    cursor: 'pointer', background: pfOpen ? C.bgDeep : 'transparent',
                    border: `1px solid ${C.border}`, borderLeft: `1px solid ${C.border}`,
                    color: C.muted, transition: 'background 0.12s',
                  }}
                  onMouseEnter={e => { e.currentTarget.style.background = C.bgDeep }}
                  onMouseLeave={e => { if (!pfOpen) e.currentTarget.style.background = 'transparent' }}
                >
                  <ChevronDown size={11} />
                </button>
              )}
            </div>
            {pfOpen && (
              <div style={{
                position: 'absolute', top: 'calc(100% + 6px)', right: 0,
                background: C.panel, border: `1px solid ${C.border}`,
                borderRadius: 8, padding: '12px 14px', zIndex: 100,
                boxShadow: '0 4px 16px rgba(7,61,60,0.10)', minWidth: 340,
              }}>
                <div style={{ fontFamily: S.mono, fontSize: 10, letterSpacing: '0.08em', color: C.faint, marginBottom: 8 }}>
                  PORT-FORWARD SETUP
                </div>
                <div style={{
                  display: 'flex', alignItems: 'center', gap: 8,
                  background: C.bg, border: `1px solid ${C.border}`,
                  borderRadius: 6, padding: '6px 10px',
                }}>
                  <code style={{ flex: 1, fontFamily: S.mono, fontSize: 11, color: C.text, wordBreak: 'break-all' }}>
                    {portForwardCmd}
                  </code>
                  <button
                    onClick={() => {
                      navigator.clipboard.writeText(portForwardCmd)
                      setPfCopied(true)
                      setTimeout(() => setPfCopied(false), 2000)
                    }}
                    style={{
                      flexShrink: 0, background: 'none', border: 'none', cursor: 'pointer',
                      color: pfCopied ? C.tealMid : C.faint, padding: 2,
                    }}
                  >
                    {pfCopied ? <Check size={13} /> : <Copy size={13} />}
                  </button>
                </div>
                <p style={{ fontFamily: S.mono, fontSize: 10, color: C.faint, marginTop: 8, lineHeight: 1.6 }}>
                  Run this in your terminal, then click Playground.
                </p>
              </div>
            )}
          </div>
          <button
            onClick={() => setConfigOpen(o => !o)}
            style={{
              display: 'inline-flex', alignItems: 'center', gap: 6,
              padding: '6px 14px', borderRadius: 6, cursor: 'pointer',
              background: configOpen ? C.bgDeep : 'transparent',
              border: `1px solid ${configOpen ? C.tealMid : C.border}`,
              fontFamily: S.body, fontSize: 13, color: configOpen ? C.teal : C.muted,
              transition: 'all 0.12s',
            }}
            onMouseEnter={e => { if (!configOpen) e.currentTarget.style.background = C.bgDeep }}
            onMouseLeave={e => { if (!configOpen) e.currentTarget.style.background = 'transparent' }}
          >
            <Settings2 size={13} /> Configure
          </button>
        </div>
      </header>

      {/* ── MAIN AREA (tab bar + content + side panel) ── */}
      <div style={{ display: 'flex', flex: 1, minHeight: 0 }}>

        {/* left: tabs + content */}
        <div style={{ display: 'flex', flexDirection: 'column', flex: 1, minWidth: 0, minHeight: 0 }}>

          {/* tab bar */}
          <div style={{ display: 'flex', padding: '0 28px', background: C.bg, borderBottom: `1px solid ${C.border}`, flexShrink: 0 }}>
            {([
              { id: 'monitor' as const, label: 'Monitor', icon: (
                <svg style={{ width: 14, height: 14, flexShrink: 0 }} fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 3v11.25A2.25 2.25 0 006 16.5h2.25M3.75 3h-1.5m1.5 0h16.5m0 0h1.5m-1.5 0v11.25A2.25 2.25 0 0118 16.5h-2.25m-7.5 0h7.5m-7.5 0l-1 3m8.5-3l1 3m0 0l.5 1.5m-.5-1.5h-9.5m0 0l-.5 1.5M9 11.25v1.5M12 9v3.75m3-6v6" />
                </svg>
              )},
{ id: 'deployments' as const, label: 'Deployments', icon: (
                <svg style={{ width: 14, height: 14, flexShrink: 0 }} fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M15.59 14.37a6 6 0 01-5.84 7.38v-4.8m5.84-2.58a14.98 14.98 0 006.16-12.12A14.98 14.98 0 009.631 8.41m5.96 5.96a14.926 14.926 0 01-5.841 2.58m-.119-8.54a6 6 0 00-7.381 5.84h4.8m2.581-5.84a14.927 14.927 0 00-2.58 5.84m2.699 2.7c-.103.021-.207.041-.311.06a15.09 15.09 0 01-2.448-2.448 14.9 14.9 0 01.06-.312m-2.24 2.39a4.493 4.493 0 00-1.757 4.306 4.493 4.493 0 004.306-1.758M16.5 9a1.5 1.5 0 11-3 0 1.5 1.5 0 013 0z" />
                </svg>
              )},
            ]).map(({ id, label, icon }) => (
              <button
                key={id}
                onClick={() => setTab(id)}
                style={{
                  display: 'flex', alignItems: 'center', gap: 6,
                  background: 'none', border: 'none', cursor: 'pointer',
                  fontFamily: S.body, fontSize: 13,
                  fontWeight: tab === id ? 600 : 400,
                  color: tab === id ? C.text : C.faint,
                  padding: '11px 16px',
                  borderBottom: tab === id ? `2px solid ${C.tealMid}` : '2px solid transparent',
                  transition: 'color 0.15s',
                }}
              >
                {icon}
                {label}
              </button>
            ))}
          </div>

          {/* tab content */}
          <div className="dp-scroll" style={{ flex: 1, overflowY: 'auto', padding: '20px 28px 32px' }}>
            {tab === 'monitor' ? (
              <MonitorTab deployment={deployment} account={account} />
            ) : (
              <DeploymentsTab
                deployment={deployment}
                account={account}
                onOpenConfigure={() => setConfigOpen(true)}
              />
            )}
          </div>
        </div>

        {/* right: configure side panel (slides in, pushes content) */}
        <div style={{
          flexShrink: 0,
          width: configOpen ? 420 : 0,
          overflowX: 'hidden',
          transition: 'width 0.3s cubic-bezier(0.16, 1, 0.3, 1)',
          position: 'sticky',
          top: 0,
          height: 'calc(100vh - 63px)',
        }}>
          {configOpen && <ConfigurePanel deployment={deployment} account={account} onClose={() => setConfigOpen(false)} onRedeploy={onRedeploy} />}
        </div>

      </div>
    </div>
  )
}
