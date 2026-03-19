import { useState, useRef, useEffect, useMemo } from "react";
import { ResponsiveContainer, ComposedChart, Area, Line, XAxis, YAxis, CartesianGrid, Tooltip } from "recharts";
import { useNavigate } from "react-router";
import {
  ArrowLeft, Settings2, Play, Search, Calendar,
  ChevronRight, ChevronDown, MoreVertical, Copy, Check,
  Pencil, Trash2, Eye, EyeOff, X, Loader2, Activity,
} from "lucide-react";
import { AgentIdentity } from "@/components/AgentIdentity";
import { usePrefilledDeploymentTemplate } from "@/api/queries/agents";
import { useDeployForm } from "@/components/deploy/useDeployForm";
import { extractInitialValues } from "@/components/deploy/extractInitialValues";
import { useChangeTracking, type TrackedFormState } from "@/components/deploy/useChangeTracking";
import {
  selectPlaygroundBackendUrl,
  buildPlaygroundLaunchUrl,
  buildPortForwardCommand,
  isLocalEnv,
} from "@/lib/playground-url";
import type { AgentDeployment } from "@/lib/api";

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
  p95Visible: boolean
}
function ChartTooltip({ active, payload, label, reqVisible, p95Visible }: ChartTooltipProps) {
  if (!active || !payload?.length) return null
  const req = payload.find(p => p.name === 'req')
  const p95 = payload.find(p => p.name === 'p95')
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
      {p95Visible && p95 && (
        <div style={{ display: 'flex', alignItems: 'center', gap: 5, fontFamily: S.mono, fontSize: 11, marginTop: 2 }}>
          <span style={{ width: 8, height: 1.5, background: C.amber, display: 'inline-block', flexShrink: 0 }} />
          <span style={{ color: C.amber, fontWeight: 600 }}>{p95.value}ms</span>
          <span style={{ color: C.faint }}>p95</span>
        </div>
      )}
    </div>
  )
}

function InlineChart({ data, reqVisible, p95Visible }: {
  data: { t: string; req: number; p95: number }[]
  reqVisible: boolean; p95Visible: boolean
}) {
  if (data.length < 2) return null
  const reqMax = Math.max(...data.map(d => d.req))
  const p95Min = Math.min(...data.map(d => d.p95))
  const p95Max = Math.max(...data.map(d => d.p95))
  const p95Pad = (p95Max - p95Min) * 0.15 || 50
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
          domain={[Math.floor(p95Min - p95Pad), Math.ceil(p95Max + p95Pad)]}
          tick={{ fontFamily: S.mono, fontSize: 8, fill: C.faint }}
          tickLine={false} axisLine={false} width={38}
          tickFormatter={(v: number) => `${v}ms`}
          tickCount={4} />
        <Tooltip
          content={<ChartTooltip reqVisible={reqVisible} p95Visible={p95Visible} />}
          cursor={{ stroke: C.stone, strokeWidth: 0.8, strokeDasharray: '2 2' }} />
        {reqVisible && (
          <Area yAxisId="req" dataKey="req" name="req"
            stroke={C.tealMid} strokeWidth={1.5} fill="url(#req-grad-obs)"
            dot={false}
            activeDot={{ r: 4, fill: C.tealMid, stroke: C.panel, strokeWidth: 1.5 }}
            animationDuration={1000} animationEasing="ease-out" />
        )}
        {p95Visible && (
          <Line yAxisId="ms" dataKey="p95" name="p95"
            stroke={C.amber} strokeWidth={1.5} strokeDasharray="4 3" strokeOpacity={0.85}
            dot={false}
            activeDot={{ r: 4, fill: C.amber, stroke: C.panel, strokeWidth: 1.5 }}
            animationDuration={1000} animationEasing="ease-out" />
        )}
      </ComposedChart>
    </ResponsiveContainer>
  )
}

function MonitorTab() {
  const [win, setWin] = useState<'1h' | '24h' | '7d'>('24h')
  const [traceSearch, setTraceSearch] = useState('')
  const [traceStatuses, setTraceStatuses] = useState<string[]>([])
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [series, setSeries] = useState({ req: true, p95: true })
  const [tokenView, setTokenView] = useState<'input' | 'output'>('input')

  const tsData: { t: string; req: number; p95: number }[] = []
  const traces: TraceRow[] = []
  const alertTraces = traces.filter(t => t.status !== 'success')
  const visibleTraces = traces.filter(t => {
    if (traceStatuses.length > 0 && !traceStatuses.includes(t.status)) return false
    if (traceSearch && !t.name.toLowerCase().includes(traceSearch.toLowerCase())) return false
    return true
  })
  const toggleTrace = (id: string) =>
    setExpanded(prev => { const n = new Set(prev); n.has(id) ? n.delete(id) : n.add(id); return n })
  const tokens = { input: 0, output: 0, total: 0 }
  const activeToken = tokenView === 'input'
    ? { value: tokens.input, color: C.tealMid }
    : { value: tokens.output, color: C.amber }

  return (
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

      {/* two-column layout: left content + right needs attention */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 280px', gap: 12, alignItems: 'start' }}>

        {/* left column */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>

          {/* KPI cards */}
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 10 }}>
            {(['TOTAL REQUESTS', 'ERROR RATE', 'AVG LATENCY', 'P95 LATENCY'] as const).map((label) => (
              <div key={label} style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, padding: '12px 14px' }}>
                <span style={{ display: 'block', fontFamily: S.mono, fontSize: 9, letterSpacing: '0.07em', color: C.faint, marginBottom: 8 }}>{label}</span>
                <span style={{ display: 'block', fontFamily: S.body, fontSize: 20, fontWeight: 700, color: C.teal }}>—</span>
              </div>
            ))}
          </div>

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
                  { key: 'p95' as const, color: C.amber,   label: 'P95',      dashed: true  },
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
                <InlineChart key={win} data={tsData} reqVisible={series.req} p95Visible={series.p95} />
              )}
            </div>

            {/* Token usage — real numbers */}
            <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, overflow: 'hidden' }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '12px 16px', borderBottom: `1px solid ${C.border}` }}>
                <span style={{ fontFamily: S.body, fontSize: 13, fontWeight: 700, color: C.teal }}>Token usage</span>
                <div style={{ display: 'flex', background: C.bgDeep, borderRadius: 6, padding: 2 }}>
                  {(['input', 'output'] as const).map(v => (
                    <button key={v} onClick={() => setTokenView(v)} style={{
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
              </div>
              <div style={{ padding: '14px 16px 12px' }}>
                <div style={{ overflow: 'hidden', lineHeight: 1.2 }}>
                  <span key={`${win}-${tokenView}`} className="dp-slot-in" style={{ display: 'block', fontFamily: S.body, fontSize: 26, fontWeight: 700, color: activeToken.color, letterSpacing: '-0.02em' }}>{fmtTokens(activeToken.value)}</span>
                </div>
                <div style={{ display: 'flex', height: 5, borderRadius: 3, overflow: 'hidden', margin: '12px 0 8px', background: C.bgDeep }}>
                  <div style={{ flex: tokens.input || 1,  background: C.tealMid, opacity: tokenView === 'input'  ? (tokens.total === 0 ? 0.15 : 1) : 0.25, transition: 'opacity 0.2s' }} />
                  <div style={{ flex: tokens.output || 1, background: C.amber,   opacity: tokenView === 'output' ? (tokens.total === 0 ? 0.15 : 1) : 0.25, transition: 'opacity 0.2s' }} />
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

        {/* right column: needs attention */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, padding: '14px 16px' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 7, marginBottom: 10 }}>
              <span style={{ fontFamily: S.body, fontSize: 12, fontWeight: 700, color: C.text }}>Needs attention</span>
              {alertTraces.length > 0 ? (
                <span style={{ fontFamily: S.mono, fontSize: 10, fontWeight: 600, color: C.coral, background: C.coralBg, border: `1px solid ${C.coralBdr}`, borderRadius: 10, padding: '1px 6px' }}>{alertTraces.length}</span>
              ) : (
                <span style={{ fontFamily: S.mono, fontSize: 10, fontWeight: 600, color: C.faint, background: C.bgDeep, borderRadius: 10, padding: '1px 6px' }}>0</span>
              )}
            </div>
            {alertTraces.length === 0 ? (
              <div style={{ padding: '20px 0', textAlign: 'center' }}>
                <p style={{ fontFamily: S.body, fontSize: 12, color: C.success, margin: 0 }}>✓ All clear</p>
              </div>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 7 }}>
                {alertTraces.map(trace => {
                  const isTimeout = trace.status === 'timeout'
                  const color = isTimeout ? C.amber : C.coral
                  const bg    = isTimeout ? C.amberBg : C.coralBg
                  const bdr   = isTimeout ? C.amberBdr : C.coralBdr
                  return (
                    <div key={trace.id} style={{ borderRadius: 7, background: bg, border: `1px solid ${bdr}`, padding: '8px 10px' }}>
                      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 3 }}>
                        <span style={{ fontFamily: S.mono, fontSize: 9, fontWeight: 700, letterSpacing: '0.08em', color }}>{trace.status.toUpperCase()}</span>
                        <span style={{ fontFamily: S.mono, fontSize: 9, color: C.faint }}>{trace.time}</span>
                      </div>
                      <span style={{ fontFamily: S.body, fontSize: 11, color: C.text, lineHeight: 1.4 }}>
                        {trace.name}
                        {trace.latency >= 1000 && <span style={{ color: C.faint }}> ({(trace.latency / 1000).toFixed(1)}s)</span>}
                      </span>
                    </div>
                  )
                })}
              </div>
            )}
          </div>
        </div>

      </div>
    </div>
  )
}

// ─── container accordion (active deployment) ─────────────────────────────────
interface ActiveContainerAccordionProps {
  name: string
  url?: string
  ready: string
  uptime: string
  logs: string[]
  vars: { key: string; value: string; secret: boolean; source: string }[]
  isOpen: boolean
  onToggle: () => void
}

function ActiveContainerAccordion({ name, url, ready, uptime, logs, vars, isOpen, onToggle }: ActiveContainerAccordionProps) {
  const [view, setView] = useState<'logs' | 'vars'>('logs')
  const [revealed, setRevealed] = useState<Set<string>>(new Set())
  const [logSearch, setLogSearch] = useState('')
  const [logTimeframe, setLogTimeframe] = useState('Last 24 hours')
  const [activeFilters, setActiveFilters] = useState<Set<'errors' | 'warnings'>>(new Set())
  const [copiedUrl, setCopiedUrl] = useState(false)

  const toggleReveal = (key: string) =>
    setRevealed(prev => { const n = new Set(prev); n.has(key) ? n.delete(key) : n.add(key); return n })

  const toggleFilter = (f: 'errors' | 'warnings') =>
    setActiveFilters(prev => { const n = new Set(prev); n.has(f) ? n.delete(f) : n.add(f); return n })

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
                  value={logTimeframe}
                  onChange={e => setLogTimeframe(e.target.value)}
                  style={{
                    padding: '4px 24px 4px 10px', borderRadius: 6, border: `1px solid ${C.border}`,
                    background: C.bg, fontFamily: S.body, fontSize: 11, color: C.muted,
                    cursor: 'pointer', outline: 'none', appearance: 'none' as const,
                    backgroundImage: `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='10' viewBox='0 0 24 24' fill='none' stroke='%236b7e7c' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpolyline points='6 9 12 15 18 9'%3E%3C/polyline%3E%3C/svg%3E")`,
                    backgroundRepeat: 'no-repeat', backgroundPosition: 'right 8px center',
                  }}
                >
                  {['Last 30 min', 'Last 1 hour', 'Last 24 hours', 'Last 7 days'].map(t => <option key={t}>{t}</option>)}
                </select>
                <div style={{ display: 'flex', alignItems: 'center', gap: 5, padding: '4px 8px', borderRadius: 5, border: `1px solid ${C.border}`, background: C.bg }}>
                  <Search size={11} color={C.faint} />
                  <input type="text" placeholder="Find in logs" value={logSearch} onChange={e => setLogSearch(e.target.value)}
                    style={{ background: 'none', border: 'none', outline: 'none', fontFamily: S.body, fontSize: 12, color: C.muted, width: 120, caretColor: C.tealMid }} />
                </div>
                <button onClick={() => navigator.clipboard.writeText(logs.join('\n'))}
                  style={{ background: 'none', border: `1px solid ${C.border}`, cursor: 'pointer', padding: '4px 6px', borderRadius: 5, color: C.faint, display: 'flex' }}>
                  <Copy size={11} />
                </button>
              </div>
              <div style={{ background: C.panel, padding: '10px 0 14px' }}>
                {filtered.length === 0 ? (
                  <div style={{ padding: '12px 18px', fontFamily: S.mono, fontSize: 11, color: C.faint }}>
                    {logs.length === 0 ? 'Live logs not yet available' : 'No matching lines'}
                  </div>
                ) : filtered.map((line, li) => (
                  <div key={li} className="dp-log" style={{ display: 'flex', alignItems: 'baseline', padding: '1px 0' }}>
                    <span style={{ fontFamily: S.mono, fontSize: 11, color: C.stone, minWidth: 56, textAlign: 'right' as const, paddingRight: 18, flexShrink: 0, userSelect: 'none' as const }}>{li + 1}</span>
                    <span style={{ fontFamily: S.mono, fontSize: 12, color: logLineColor(line), lineHeight: 1.75 }}>{line}</span>
                  </div>
                ))}
                <div style={{ display: 'flex', alignItems: 'baseline', padding: '1px 0', marginTop: 2 }}>
                  <span style={{ minWidth: 56, paddingRight: 18, flexShrink: 0 }} />
                  <span className="dp-blink" style={{ fontFamily: S.mono, fontSize: 12, color: C.tealMid }}>▊</span>
                </div>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// ─── blueprints tab ───────────────────────────────────────────────────────────
function BlueprintsTab({ deployment }: { deployment: AgentDeployment }) {
  const components = deployment.components ?? []
  const urls = deployment.external_urls ?? []

  const COMPONENT_ICONS: Record<string, string> = {
    model: '🧠', knowledge: '📚', tool: '🔧', messaging: '💬',
    memory: '🗃️', ingestion: '📥', collector: '📊',
  }
  const getIcon = (c: string) => {
    for (const [key, icon] of Object.entries(COMPONENT_ICONS)) {
      if (c.toLowerCase().includes(key)) return icon
    }
    return '⬡'
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
      {/* header */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <span style={{ fontFamily: S.body, fontSize: 17, fontWeight: 600, color: C.teal }}>Blueprints</span>
        <span style={{ fontFamily: S.mono, fontSize: 10, letterSpacing: '0.1em', color: C.faint }}>BUILD {deployment.build_id.slice(0, 8).toUpperCase()}</span>
      </div>

      {/* components grid */}
      <div>
        <p style={{ fontFamily: S.mono, fontSize: 9, letterSpacing: '0.12em', color: C.stone, marginBottom: 12 }}>COMPONENTS</p>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(180px, 1fr))', gap: 10 }}>
          {(components.length > 0 ? components : ['model', 'knowledge', 'tool']).map((c, i) => (
            <div key={i} style={{
              display: 'flex', alignItems: 'center', gap: 10,
              padding: '12px 14px', borderRadius: 10,
              background: C.bgAlt, border: `1px solid ${C.border}`,
            }}>
              <span style={{ fontSize: 18, lineHeight: 1, flexShrink: 0 }}>{getIcon(c)}</span>
              <div style={{ minWidth: 0 }}>
                <div style={{ fontFamily: S.body, fontSize: 12, fontWeight: 600, color: C.text, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{c}</div>
                <div style={{ fontFamily: S.mono, fontSize: 10, color: C.faint, marginTop: 1 }}>active</div>
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* spec metadata */}
      <div>
        <p style={{ fontFamily: S.mono, fontSize: 9, letterSpacing: '0.12em', color: C.stone, marginBottom: 12 }}>SPEC</p>
        <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, overflow: 'hidden' }}>
          {[
            { label: 'Deployment ID', value: deployment.id },
            { label: 'Namespace',     value: deployment.namespace },
            { label: 'Build',         value: deployment.build_id },
            { label: 'Replicas',      value: `${deployment.ready} / ${deployment.replicas}` },
          ].map((row, i, arr) => (
            <div key={row.label} style={{
              display: 'flex', alignItems: 'center', justifyContent: 'space-between',
              padding: '10px 16px',
              borderBottom: i < arr.length - 1 ? `1px solid ${C.border}` : 'none',
            }}>
              <span style={{ fontFamily: S.mono, fontSize: 11, color: C.faint }}>{row.label}</span>
              <span style={{ fontFamily: S.mono, fontSize: 11, color: C.text, maxWidth: 260, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{row.value}</span>
            </div>
          ))}
        </div>
      </div>

      {/* endpoints */}
      {urls.length > 0 && (
        <div>
          <p style={{ fontFamily: S.mono, fontSize: 9, letterSpacing: '0.12em', color: C.stone, marginBottom: 12 }}>ENDPOINTS</p>
          <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, overflow: 'hidden' }}>
            {urls.map((u, i) => (
              <div key={i} style={{
                display: 'flex', alignItems: 'center', justifyContent: 'space-between',
                padding: '10px 16px',
                borderBottom: i < urls.length - 1 ? `1px solid ${C.border}` : 'none',
              }}>
                <span style={{ fontFamily: S.mono, fontSize: 11, color: C.faint }}>{u.name ?? `endpoint-${i + 1}`}</span>
                <a href={u.url} target="_blank" rel="noopener noreferrer" style={{ fontFamily: S.mono, fontSize: 11, color: C.tealMid, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: 320, textDecoration: 'none' }}
                  onClick={e => e.stopPropagation()}>{u.url}</a>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

// ─── deployments tab ──────────────────────────────────────────────────────────
type DeployHistoryStatus = 'active' | 'ready' | 'failed'
interface DeployHistoryRecord {
  id: string; status: DeployHistoryStatus; build: string; duration: string; time: string; user: string; isCurrent?: boolean
}
const DEPLOY_STATUS_STYLE: Record<DeployHistoryStatus, { color: string; label: string }> = {
  active: { color: C.success, label: 'Active' },
  ready:  { color: C.success, label: 'Ready'  },
  failed: { color: C.coral,   label: 'Failed' },
}

function DeploymentsTab({ deployment }: { deployment: AgentDeployment }) {
  const [deploySearch, setDeploySearch] = useState('')
  const [deployStatus, setDeployStatus] = useState<string[]>([])
  const [expandedDeploy, setExpandedDeploy] = useState<string | null>(deployment.id)
  const [openDeployMenu, setOpenDeployMenu] = useState<string | null>(null)
  const [openContainers, setOpenContainers] = useState<Set<string>>(new Set())

  const toggleContainer = (id: string) =>
    setOpenContainers(prev => { const n = new Set(prev); n.has(id) ? n.delete(id) : n.add(id); return n })

  // derive containers from pods, injecting mock logs by container name
  const containers = (deployment.pods ?? []).flatMap(pod =>
    (pod.containers ?? []).map(c => ({
      id: `${pod.name}:${c.name}`,
      name: c.name,
      ready: c.ready ? '1/1' : '0/1',
      uptime: pod.age ?? '—',
      logs: [] as string[],
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

  // assign external URL to the agent container (first one named "agent", or first container)
  const externalUrls = deployment.external_urls ?? []
  if (externalUrls.length > 0 && containers.length > 0) {
    const agentContainer = containers.find(c => c.name.includes('agent')) ?? containers[0]
    if (agentContainer) agentContainer.url = externalUrls[0]?.url
  }

  const allRows: DeployHistoryRecord[] = [
    { id: deployment.id, status: 'active', build: deployment.build_id.slice(0, 8), duration: '—', time: 'Just now', user: '—', isCurrent: true },
  ]

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 0 }}>
      {/* header row */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 14 }}>
        <span style={{ fontFamily: S.body, fontSize: 17, fontWeight: 600, color: C.teal, flex: 1 }}>Deployments</span>
        <div style={{ display: 'flex', alignItems: 'center', gap: 5, padding: '5px 10px', borderRadius: 7, border: `1px solid ${C.border}`, background: C.bg }}>
          <Search size={12} color={C.faint} />
          <input type="text" placeholder="Search deployments" value={deploySearch} onChange={e => setDeploySearch(e.target.value)}
            style={{ background: 'none', border: 'none', outline: 'none', fontFamily: S.body, fontSize: 12, color: C.muted, width: 200, caretColor: C.tealMid }} />
        </div>
        <MultiSelect
          options={[
            { value: 'active', label: 'Active',  color: C.tealMid },
            { value: 'ready',  label: 'Ready',   color: C.success  },
            { value: 'failed', label: 'Failed',  color: C.coral    },
          ]}
          selected={deployStatus}
          onChange={setDeployStatus}
          placeholder="All statuses"
        />
        <button style={{ display: 'flex', alignItems: 'center', gap: 5, padding: '5px 12px', borderRadius: 7, border: `1px solid ${C.border}`, background: C.bg, cursor: 'pointer', fontFamily: S.body, fontSize: 12, color: C.muted }}>
          <Calendar size={12} /> Date range
        </button>
      </div>

      {/* table */}
      <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, overflow: 'hidden' }}>
        {/* table header */}
        <div style={{ display: 'grid', gridTemplateColumns: '16px 160px 80px 72px 110px 1fr 72px 32px', gap: 12, padding: '8px 16px', borderBottom: `1px solid ${C.border}`, background: C.bgDeep }}>
          {['', 'Deployment', 'Status', 'Duration', 'Build No.', 'Deployed by', 'Deployed on', ''].map(h => (
            <span key={h} style={{ fontFamily: S.mono, fontSize: 9, letterSpacing: '0.07em', color: C.faint }}>{h.toUpperCase()}</span>
          ))}
        </div>

        {allRows.map((d, i) => {
          const ds = DEPLOY_STATUS_STYLE[d.status]
          const isCurrent = !!d.isCurrent
          const isExpanded = expandedDeploy === d.id
          const initials = d.user.split('.').map((p: string) => p[0].toUpperCase()).join('')
          const avatarColor = d.user === 't.green' ? C.teal : '#5c5047'
          return (
            <div key={d.id} style={{ borderBottom: i < allRows.length - 1 ? `1px solid ${C.border}` : 'none' }}>
              {/* Row */}
              <div
                onClick={() => setExpandedDeploy(isExpanded ? null : d.id)}
                style={{
                  display: 'grid', gridTemplateColumns: '16px 160px 80px 72px 110px 1fr 72px 32px', gap: 12,
                  padding: '12px 16px', alignItems: 'center', cursor: 'pointer',
                  borderLeft: isCurrent ? `3px solid ${C.tealMid}` : '3px solid transparent',
                  background: isExpanded ? C.bgDeep : isCurrent ? 'rgba(21,130,125,0.02)' : 'transparent',
                  transition: 'background 0.12s',
                }}
              >
                <ChevronRight size={12} color={C.faint} style={{ transition: 'transform 0.15s', transform: isExpanded ? 'rotate(90deg)' : 'none' }} />
                <div style={{ minWidth: 0 }}>
                  <div style={{ fontFamily: S.body, fontSize: 12, fontWeight: 500, color: C.text, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' as const }}>
                    {isCurrent ? (deployment.display_name || deployment.name) : deployment.name}
                  </div>
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 7 }}>
                  <span style={{ width: 8, height: 8, borderRadius: '50%', background: ds.color, display: 'inline-block', flexShrink: 0 }} />
                  <span style={{ fontFamily: S.mono, fontSize: 10, letterSpacing: '0.06em', color: ds.color, fontWeight: 500 }}>{ds.label.toUpperCase()}</span>
                </div>
                <span style={{ fontFamily: S.mono, fontSize: 11, color: C.faint }}>{d.duration}</span>
                <span style={{ fontFamily: S.mono, fontSize: 11, fontWeight: 600, color: C.muted }}>{d.build}</span>
                <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                  <div style={{ width: 20, height: 20, borderRadius: '50%', background: avatarColor, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                    <span style={{ fontFamily: S.mono, fontSize: 8, fontWeight: 700, color: '#fff' }}>{initials}</span>
                  </div>
                  <span style={{ fontFamily: S.mono, fontSize: 10, color: C.muted, whiteSpace: 'nowrap' as const }}>{d.user}</span>
                </div>
                <span style={{ fontFamily: S.mono, fontSize: 11, color: C.faint, whiteSpace: 'nowrap' as const }}>{d.time}</span>
                <div style={{ position: 'relative' }} onClick={e => e.stopPropagation()}>
                  <button onClick={() => setOpenDeployMenu(openDeployMenu === d.id ? null : d.id)}
                    style={{ background: 'none', border: 'none', cursor: 'pointer', color: C.faint, display: 'flex', padding: 4, borderRadius: 4 }}
                    onMouseEnter={e => { e.currentTarget.style.background = C.bgDeep }}
                    onMouseLeave={e => { e.currentTarget.style.background = 'none' }}
                  ><MoreVertical size={13} /></button>
                  {openDeployMenu === d.id && (
                    <>
                      <div onClick={() => setOpenDeployMenu(null)} style={{ position: 'fixed', inset: 0, zIndex: 10 }} />
                      <div style={{ position: 'absolute', right: 0, top: 'calc(100% + 4px)', zIndex: 20, minWidth: 140, background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 8, overflow: 'hidden', boxShadow: '0 6px 20px rgba(0,0,0,0.1)' }}>
                        {[
                          { label: 'Redeploy',  color: C.text,  sep: false },
                          { label: 'View logs', color: C.text,  sep: false },
                          { label: 'Rollback',  color: C.coral, sep: true  },
                        ].map(({ label, color, sep }) => (
                          <div key={label}>
                            {sep && <div style={{ height: 1, background: C.border }} />}
                            <button onClick={() => setOpenDeployMenu(null)} style={{
                              width: '100%', display: 'flex', alignItems: 'center', gap: 8,
                              padding: '9px 14px', background: 'none', border: 'none',
                              cursor: 'pointer', fontFamily: S.body, fontSize: 12, color, textAlign: 'left' as const,
                            }}
                              onMouseEnter={e => { e.currentTarget.style.background = C.bgDeep }}
                              onMouseLeave={e => { e.currentTarget.style.background = 'none' }}
                            >{label}</button>
                          </div>
                        ))}
                      </div>
                    </>
                  )}
                </div>
              </div>

              {/* Expanded: container accordions (current) or placeholder (historical) */}
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
                        logs={c.logs}
                        vars={c.vars}
                        isOpen={openContainers.has(c.id)}
                        onToggle={() => toggleContainer(c.id)}
                      />
                    ))
                  ) : (
                    <p style={{ fontFamily: S.mono, fontSize: 11, color: C.faint, margin: 0 }}>Historical deployment logs not available</p>
                  )}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

// ─── configure panel ──────────────────────────────────────────────────────────
const PANEL_FORM_ID = "configure-side-panel-form"

const TONES = [
  { id: 'analytical',    label: 'Analytical'    },
  { id: 'technical',     label: 'Technical'     },
  { id: 'concise',       label: 'Concise'       },
  { id: 'structured',    label: 'Structured'    },
  { id: 'collaborative', label: 'Collaborative' },
]

function formatVarKey(key: string): string {
  return key.replace(/_/g, ' ').toLowerCase().replace(/\b\w/g, c => c.toUpperCase())
}

function ConfigurePanelLoaded({ deployment, account, template, onClose, onRedeploy }: {
  deployment: AgentDeployment; account: string
  template: import("@/lib/api").DeploymentTemplate; onClose: () => void; onRedeploy?: () => void
}) {
  const initialValues = useMemo(() => extractInitialValues(template, account), [template, account])
  const form = useDeployForm(account, deployment.name, { initialTemplate: template, skipTemplateFetch: true, initialValues })

  const [tone, setTone] = useState('')
  const [trigger, setTrigger] = useState('event')
  const [visibility, setVisibility] = useState('private')
  const [revealed, setRevealed] = useState<Set<string>>(new Set())

  const toggleReveal = (key: string) =>
    setRevealed(prev => { const n = new Set(prev); n.has(key) ? n.delete(key) : n.add(key); return n })

  const trackedState: TrackedFormState = {
    deployName: form.deployName,
    variableValues: form.variableValues,
    selectedAdapters: form.selectedAdapters,
    adapterCredentials: form.adapterCredentials,
  }
  const initialTrackedState: TrackedFormState = {
    deployName: initialValues.deployName ?? '',
    variableValues: initialValues.variableValues ?? {},
    selectedAdapters: initialValues.selectedAdapters ?? ['web'],
    adapterCredentials: initialValues.adapterCredentials ?? {},
  }
  const changes = useChangeTracking(initialTrackedState, trackedState)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!form.trySubmit()) return
    try { await form.deploy(); onClose(); onRedeploy?.() } catch { /* captured in form.deployError */ }
  }

  const inputStyle: React.CSSProperties = {
    width: '100%', padding: '8px 12px', borderRadius: 8,
    border: `1px solid ${C.border}`, background: C.bg,
    fontFamily: S.body, fontSize: 13, color: C.text,
    boxSizing: 'border-box', outline: 'none',
  }

  const SectionHead = ({ title, desc }: { title: string; desc: string }) => (
    <div style={{ marginBottom: 16 }}>
      <div style={{ fontFamily: S.body, fontSize: 14, fontWeight: 700, color: C.teal, marginBottom: 3 }}>{title}</div>
      <div style={{ fontFamily: S.body, fontSize: 12, color: C.faint }}>{desc}</div>
      <div style={{ height: 1, background: C.border, marginTop: 12 }} />
    </div>
  )
  const FieldLabel = ({ children }: { children: React.ReactNode }) => (
    <span style={{ fontFamily: S.body, fontSize: 12, fontWeight: 600, color: C.teal, display: 'block', marginBottom: 6 }}>{children}</span>
  )

  return (
    <div style={{ width: 420, height: '100%', display: 'flex', flexDirection: 'column', background: C.panel, borderLeft: `1px solid ${C.border}` }}>

      {/* header */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, height: 63, flexShrink: 0, padding: '0 12px 0 16px', borderBottom: `1px solid ${C.border}` }}>
        <Settings2 size={14} color={C.teal} />
        <span style={{ flex: 1, fontFamily: S.body, fontSize: 13, fontWeight: 600, color: C.teal }}>Configure</span>
        <button
          onClick={onClose}
          style={{ background: 'none', border: 'none', cursor: 'pointer', color: C.faint, display: 'flex', padding: 6, borderRadius: 6 }}
          onMouseEnter={e => { e.currentTarget.style.background = C.bgDeep }}
          onMouseLeave={e => { e.currentTarget.style.background = 'none' }}
        >
          <X size={15} />
        </button>
      </div>

      {/* scrollable body */}
      <form id={PANEL_FORM_ID} onSubmit={handleSubmit} style={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
        <div className="dp-scroll" style={{ flex: 1, overflowY: 'auto', padding: '24px 20px', display: 'flex', flexDirection: 'column', gap: 32 }}>

          {/* Identity */}
          <div>
            <SectionHead title="Identity" desc="Define who this agent is." />
            <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
              <div>
                <FieldLabel>Name</FieldLabel>
                <input
                  value={form.deployName}
                  onChange={e => form.setDeployName(e.target.value)}
                  maxLength={64}
                  style={inputStyle}
                />
              </div>
              <div>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 6 }}>
                  <FieldLabel>Greeting message</FieldLabel>
                  <span style={{ fontFamily: S.mono, fontSize: 10, color: C.faint }}>{(form.variableValues['GREETING'] ?? '').length}/200</span>
                </div>
                <textarea
                  value={form.variableValues['GREETING'] ?? ''}
                  onChange={e => form.setVariableValues({ ...form.variableValues, GREETING: e.target.value.slice(0, 200) })}
                  rows={3}
                  style={{ ...inputStyle, resize: 'vertical' as const, lineHeight: 1.55 }}
                />
              </div>
              <div>
                <FieldLabel>Response tone <span style={{ fontWeight: 400, color: C.faint }}>(optional)</span></FieldLabel>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                  {TONES.map(t => {
                    const active = tone === t.id
                    return (
                      <button key={t.id} type="button" onClick={() => setTone(active ? '' : t.id)} style={{
                        display: 'inline-flex', alignItems: 'center', gap: 5,
                        padding: '5px 12px', borderRadius: 6,
                        border: `1px solid ${active ? C.tealMid : C.border}`,
                        background: active ? 'rgba(21,130,125,0.1)' : C.bg,
                        fontFamily: S.mono, fontSize: 10, letterSpacing: '0.04em',
                        color: active ? C.tealMid : C.muted,
                        cursor: 'pointer', transition: 'all 0.15s',
                      }}>
                        {t.label}{active && <Check size={10} />}
                      </button>
                    )
                  })}
                </div>
              </div>
            </div>
          </div>

          {/* Configuration — bespoke per agent: required vars */}
          {form.requiredVariables.length > 0 && (
            <div>
              <SectionHead title="Configuration" desc="Credentials and settings this agent needs." />
              <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
                {form.requiredVariables.map(([key, variable]) => {
                  const isSecret = variable.secret
                  const isRevealed = revealed.has(key)
                  const hasError = form.errors.credentials?.includes(key)
                  return (
                    <div key={key}>
                      <FieldLabel>{formatVarKey(key)}</FieldLabel>
                      {variable.description && (
                        <div style={{ fontFamily: S.body, fontSize: 11, color: C.faint, marginBottom: 6 }}>{variable.description}</div>
                      )}
                      <div style={{ position: 'relative' }}>
                        <input
                          type={isSecret && !isRevealed ? 'password' : 'text'}
                          value={form.variableValues[key] ?? ''}
                          onChange={e => form.setVariableValues({ ...form.variableValues, [key]: e.target.value })}
                          placeholder={variable.defaultValue ?? (isSecret ? '••••••••' : '')}
                          style={{
                            ...inputStyle,
                            paddingRight: isSecret ? 36 : undefined,
                            borderColor: hasError ? C.coral : C.border,
                          }}
                        />
                        {isSecret && (
                          <button type="button" onClick={() => toggleReveal(key)} style={{
                            position: 'absolute', right: 10, top: '50%', transform: 'translateY(-50%)',
                            background: 'none', border: 'none', cursor: 'pointer',
                            color: C.faint, display: 'flex', padding: 2,
                          }}>
                            {isRevealed ? <EyeOff size={13} /> : <Eye size={13} />}
                          </button>
                        )}
                      </div>
                      {hasError && (
                        <div style={{ fontFamily: S.body, fontSize: 11, color: C.coral, marginTop: 4 }}>Required</div>
                      )}
                    </div>
                  )
                })}
              </div>
            </div>
          )}

          {/* Optional vars */}
          {form.optionalVariables.length > 0 && (
            <div>
              <SectionHead title="Optional credentials" desc="Not required but enable extra functionality." />
              <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
                {form.optionalVariables.map(([key, variable]) => {
                  const isSecret = variable.secret
                  const isRevealed = revealed.has(key)
                  return (
                    <div key={key}>
                      <FieldLabel>{formatVarKey(key)}</FieldLabel>
                      {variable.description && (
                        <div style={{ fontFamily: S.body, fontSize: 11, color: C.faint, marginBottom: 6 }}>{variable.description}</div>
                      )}
                      <div style={{ position: 'relative' }}>
                        <input
                          type={isSecret && !isRevealed ? 'password' : 'text'}
                          value={form.variableValues[key] ?? ''}
                          onChange={e => form.setVariableValues({ ...form.variableValues, [key]: e.target.value })}
                          placeholder={variable.defaultValue ?? ''}
                          style={{ ...inputStyle, paddingRight: isSecret ? 36 : undefined }}
                        />
                        {isSecret && (
                          <button type="button" onClick={() => toggleReveal(key)} style={{
                            position: 'absolute', right: 10, top: '50%', transform: 'translateY(-50%)',
                            background: 'none', border: 'none', cursor: 'pointer', color: C.faint, display: 'flex', padding: 2,
                          }}>
                            {isRevealed ? <EyeOff size={13} /> : <Eye size={13} />}
                          </button>
                        )}
                      </div>
                    </div>
                  )
                })}
              </div>
            </div>
          )}

          {/* Ingestion Trigger */}
          <div>
            <SectionHead title="Ingestion Trigger" desc="Choose what kicks off your agent." />
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 8 }}>
              {([
                { id: 'event',    label: 'On Event', Icon: Calendar },
                { id: 'manual',   label: 'Manual',   Icon: Play     },
                { id: 'schedule', label: 'Schedule', Icon: Search   },
              ] as { id: string; label: string; Icon: React.ElementType }[]).map(({ id, label, Icon }) => {
                const active = trigger === id
                return (
                  <button key={id} type="button" onClick={() => setTrigger(id)} style={{
                    display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 8,
                    padding: '14px 10px', borderRadius: 10, cursor: 'pointer',
                    border: `1px solid ${active ? C.tealMid : C.border}`,
                    background: active ? 'rgba(21,130,125,0.08)' : C.bg,
                    transition: 'all 0.15s', position: 'relative',
                  }}>
                    {active && <span style={{ position: 'absolute', top: 7, right: 7, width: 14, height: 14, borderRadius: '50%', background: C.tealMid, display: 'flex', alignItems: 'center', justifyContent: 'center' }}><Check size={8} color="#fff" /></span>}
                    <Icon size={16} color={active ? C.tealMid : C.faint} />
                    <span style={{ fontFamily: S.mono, fontSize: 10, letterSpacing: '0.04em', color: active ? C.tealMid : C.muted }}>{label.toUpperCase()}</span>
                  </button>
                )
              })}
            </div>
          </div>

          {/* Visibility */}
          <div>
            <SectionHead title="Visibility" desc="Control who can discover and use this agent." />
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8 }}>
              {([
                { id: 'public',  label: 'Public',  icon: '🌐', desc: 'Anyone in your org'    },
                { id: 'private', label: 'Private', icon: '🔒', desc: 'Only invited members'  },
              ]).map(({ id, label, icon, desc }) => {
                const active = visibility === id
                return (
                  <button key={id} type="button" onClick={() => setVisibility(id)} style={{
                    display: 'flex', flexDirection: 'column', alignItems: 'flex-start', gap: 6,
                    padding: '14px', borderRadius: 10, cursor: 'pointer',
                    border: `1px solid ${active ? C.tealMid : C.border}`,
                    background: active ? 'rgba(21,130,125,0.08)' : C.bg,
                    transition: 'all 0.15s', textAlign: 'left', position: 'relative',
                  }}>
                    {active && <span style={{ position: 'absolute', top: 8, right: 8, width: 14, height: 14, borderRadius: '50%', background: C.tealMid, display: 'flex', alignItems: 'center', justifyContent: 'center' }}><Check size={8} color="#fff" /></span>}
                    <span style={{ fontSize: 15 }}>{icon}</span>
                    <div>
                      <div style={{ fontFamily: S.body, fontSize: 13, fontWeight: 600, color: active ? C.teal : C.text }}>{label}</div>
                      <div style={{ fontFamily: S.body, fontSize: 11, color: C.faint, marginTop: 2 }}>{desc}</div>
                    </div>
                  </button>
                )
              })}
            </div>
          </div>

          {form.deployError && (
            <div style={{ padding: '10px 14px', borderRadius: 8, border: `1px solid ${C.coralBdr}`, background: C.coralBg }}>
              <p style={{ fontFamily: S.mono, fontSize: 11, color: C.coral, margin: 0 }}>{form.deployError}</p>
            </div>
          )}

        </div>

        {/* footer — always visible, matches sketchbook exactly */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8, padding: '14px 20px', borderTop: `1px solid ${C.border}`, background: C.bgAlt, flexShrink: 0 }}>
          <button
            type="submit"
            form={PANEL_FORM_ID}
            disabled={form.isDeploying}
            style={{
              display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 8,
              padding: '11px 0', borderRadius: 9, border: 'none',
              background: C.teal, color: C.bg,
              fontFamily: S.body, fontSize: 13, fontWeight: 600,
              cursor: form.isDeploying ? 'not-allowed' : 'pointer',
              opacity: form.isDeploying ? 0.7 : 1, transition: 'opacity 0.15s',
            }}
          >
            {form.isDeploying && <Loader2 size={13} className="dp-spin" />}
            {form.isDeploying ? 'Redeploying…' : changes.requiresRedeploy ? 'Save & Redeploy' : 'Redeploy'}
          </button>
          <button
            type="button"
            onClick={() => { form.reset(initialValues); onClose() }}
            style={{ padding: '9px 0', borderRadius: 9, border: 'none', background: 'transparent', fontFamily: S.body, fontSize: 13, color: C.faint, cursor: 'pointer', transition: 'color 0.15s' }}
            onMouseEnter={e => { e.currentTarget.style.color = C.muted }}
            onMouseLeave={e => { e.currentTarget.style.color = C.faint }}
          >
            Discard
          </button>
        </div>
      </form>

    </div>
  )
}

function ConfigurePanel({ deployment, account, onClose, onRedeploy }: { deployment: AgentDeployment; account: string; onClose: () => void; onRedeploy?: () => void }) {
  const { data: template, isLoading, isError } = usePrefilledDeploymentTemplate(account, deployment.name, deployment.id)

  const shell = (children: React.ReactNode) => (
    <div style={{ width: 420, height: '100%', display: 'flex', flexDirection: 'column', background: C.panel, borderLeft: `1px solid ${C.border}` }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, height: 63, flexShrink: 0, padding: '0 12px 0 16px', borderBottom: `1px solid ${C.border}` }}>
        <Settings2 size={14} color={C.teal} />
        <span style={{ flex: 1, fontFamily: S.body, fontSize: 13, fontWeight: 600, color: C.teal }}>Configure</span>
        <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', color: C.faint, display: 'flex', padding: 6, borderRadius: 6 }}
          onMouseEnter={e => { e.currentTarget.style.background = C.bgDeep }}
          onMouseLeave={e => { e.currentTarget.style.background = 'none' }}>
          <X size={15} />
        </button>
      </div>
      <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>{children}</div>
    </div>
  )

  if (isLoading) return shell(<Loader2 size={20} className="dp-spin" color={C.faint} />)
  if (isError || !template) return shell(<p style={{ fontFamily: S.mono, fontSize: 11, color: C.coral, margin: 0 }}>Failed to load configuration.</p>)
  return <ConfigurePanelLoaded deployment={deployment} account={account} template={template} onClose={onClose} onRedeploy={onRedeploy} />
}

// ─── main component ───────────────────────────────────────────────────────────
interface ActiveDetailViewProps {
  deployment: AgentDeployment;
  account: string;
  isPersonal: boolean;
  onRedeploy?: () => void;
}

export function ActiveDetailView({ deployment, account, isPersonal, onRedeploy }: ActiveDetailViewProps) {
  const navigate = useNavigate()
  const [tab, setTab] = useState<'monitor' | 'deployments'>('monitor')
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
          {/* Active badge */}
          <span style={{
            display: 'inline-flex', alignItems: 'center', gap: 5,
            padding: '2px 10px', borderRadius: 99,
            background: 'rgba(21,130,125,0.08)', border: '1px solid rgba(21,130,125,0.22)',
            fontFamily: S.mono, fontSize: 10, letterSpacing: '0.06em', color: C.tealMid,
          }}>
            <span style={{ width: 5, height: 5, borderRadius: '50%', background: C.tealMid, display: 'inline-block' }} />
            Active
          </span>
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
              <MonitorTab />
            ) : (
              <DeploymentsTab deployment={deployment} />
            )}
          </div>
        </div>

        {/* right: configure side panel (slides in, pushes content) */}
        <div style={{
          flexShrink: 0,
          width: configOpen ? 420 : 0,
          overflow: 'hidden',
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
