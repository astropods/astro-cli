import { useState } from "react";
import { useNavigate } from "react-router";
import { useUndeployAgent } from "@/api/queries/deployments";
import {
  ArrowLeft, RotateCcw, Settings2, Search,
  ChevronRight, PanelLeftClose, PanelLeftOpen,
  Copy, Eye, EyeOff, X,
} from "lucide-react";
import { AgentIdentity } from "@/components/AgentIdentity";
import { deriveDeploymentStages } from "@/lib/deployment-utils";
import { deploymentConfigurePath } from "@/lib/routes";
import type { AgentDeployment } from "@/lib/api";

// ─── color + font tokens ──────────────────────────────────────────────────────
const C = {
  bg:      '#ede7d9',
  bgAlt:   '#e5dece',
  bgDeep:  '#d8d0c0',
  panel:   '#f5f1e8',
  border:  '#c4b89e',
  teal:    '#073d3c',
  tealMid: '#15827d',
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

// ─── css keyframes injection ──────────────────────────────────────────────────
function Styles() {
  return (
    <style>{`
      @keyframes dp-pulse { 0%,100% { opacity:1; } 50% { opacity:0.4; } }
      @keyframes dp-blink { 0%,100% { opacity:1; } 50% { opacity:0; } }
      @keyframes dp-fadein { from { opacity:0; transform:translateY(3px); } to { opacity:1; transform:translateY(0); } }
      @keyframes dp-spin { from { transform:rotate(0deg); } to { transform:rotate(360deg); } }
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

// ─── container accordion ─────────────────────────────────────────────────────
interface ContainerAccordionProps {
  name: string
  containerStatus: 'done' | 'running' | 'pending'
  stuckReason?: string
  logs: string[]
  vars: { key: string; value: string; secret: boolean; source: string }[]
  isOpen: boolean
  onToggle: () => void
}

function ContainerAccordion({ name, containerStatus, stuckReason, logs, vars, isOpen, onToggle }: ContainerAccordionProps) {
  const [view, setView] = useState<'logs' | 'vars'>('logs')
  const [revealed, setRevealed] = useState<Set<string>>(new Set())
  const [logSearch, setLogSearch] = useState('')
  const [logTimeframe, setLogTimeframe] = useState('Last 24 hours')
  const [activeFilters, setActiveFilters] = useState<Set<'errors' | 'warnings'>>(new Set())

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

  return (
    <div style={{ marginBottom: 8 }}>
      <button
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
        {/* status icon */}
        {containerStatus === 'done' ? (
          <svg width="16" height="16" viewBox="0 0 24 24" style={{ flexShrink: 0 }}>
            <circle cx="12" cy="12" r="10" fill="rgba(21,130,125,0.12)" />
            <path d="M7.5 12l3 3 6-6" stroke={C.tealMid} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
          </svg>
        ) : containerStatus === 'running' ? (
          <svg width="16" height="16" viewBox="0 0 24 24" style={{ flexShrink: 0 }}>
            <circle cx="12" cy="12" r="10" fill={stuckReason ? C.coralBg : C.amberBg} />
            <path d="M21 12a9 9 0 1 1-6.219-8.56" stroke={stuckReason ? C.coral : C.amber} strokeWidth="2" strokeLinecap="round" fill="none" style={{ transformOrigin: '12px 12px', animation: stuckReason ? 'none' : 'dp-spin 0.9s linear infinite' }} />
          </svg>
        ) : (
          <svg width="16" height="16" viewBox="0 0 24 24" style={{ flexShrink: 0, opacity: 0.4 }}>
            <circle cx="12" cy="12" r="10" stroke={C.stone} strokeWidth="1.5" fill="none" />
          </svg>
        )}
        <span style={{ fontFamily: S.body, fontSize: 13, fontWeight: 500, color: C.text }}>{name}</span>
        <span style={{ flex: 1 }} />
        <span style={{ fontFamily: S.mono, fontSize: 11, color: containerStatus === 'done' ? C.success : containerStatus === 'running' ? (stuckReason ? C.coral : C.amber) : C.faint }}>
          {containerStatus === 'done' ? 'ready' : stuckReason ?? (containerStatus === 'running' ? 'running…' : 'pending')}
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
                const srcStyle = v.source === 'input'
                  ? { bg: 'rgba(21,130,125,0.1)', color: '#15827d', label: 'input' }
                  : { bg: C.bgDeep, color: C.stone, label: 'auto' }
                return (
                  <div key={v.key} style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '9px 16px', borderBottom: vi < vars.length - 1 ? `1px solid ${C.border}` : 'none' }}>
                    <span style={{ fontFamily: S.mono, fontSize: 10, color: C.stone, flexShrink: 0, userSelect: 'none' as const }}>{'{}'}</span>
                    <span style={{ fontFamily: S.mono, fontSize: 12, color: C.text, minWidth: 160, flexShrink: 0 }}>{v.key}</span>
                    <div style={{ flex: 1, display: 'flex', alignItems: 'center', gap: 6, minWidth: 0 }}>
                      <span style={{ fontFamily: S.mono, fontSize: 12, color: C.faint, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {v.secret && !isRevealed ? '•••••••••' : v.value}
                      </span>
                      {v.secret && (
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
              {/* toolbar */}
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '8px 14px', background: C.bgAlt, borderBottom: `1px solid ${C.border}` }}>
                {([
                  { key: 'errors' as const,   label: `Errors (${errCount})`,    accent: '#dc2626', activeBg: '#fef2f2', activeBdr: '#fca5a5' },
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
              {/* log lines */}
              <div style={{ background: C.panel, padding: '10px 0 14px' }}>
                {filtered.length === 0 ? (
                  <div style={{ padding: '12px 18px', fontFamily: S.mono, fontSize: 11, color: C.faint }}>No matching lines</div>
                ) : filtered.map((line, li) => (
                  <div key={li} className="dp-log" style={{ display: 'flex', alignItems: 'baseline', padding: '1px 0' }}>
                    <span style={{ fontFamily: S.mono, fontSize: 11, color: C.stone, minWidth: 56, textAlign: 'right' as const, paddingRight: 18, flexShrink: 0, userSelect: 'none' as const }}>{li + 1}</span>
                    <span style={{ fontFamily: S.mono, fontSize: 12, color: logLineColor(line), lineHeight: 1.75 }}>{line}</span>
                  </div>
                ))}
                {/* blinking cursor */}
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

// ─── main component ───────────────────────────────────────────────────────────
interface DeployingDetailViewProps {
  deployment: AgentDeployment;
  account: string;
  isPersonal: boolean;
}

export function DeployingDetailView({ deployment, account, isPersonal }: DeployingDetailViewProps) {
  const navigate = useNavigate()
  const undeploy = useUndeployAgent(account)
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const [openContainers, setOpenContainers] = useState<Set<string>>(new Set())
  const [_annotationsOpen, _setAnnotationsOpen] = useState(false)

  const displayName = deployment.display_name || deployment.name
  const backPath = isPersonal ? '/agents' : `/${account}`
  const configurePath = deploymentConfigurePath(account, deployment.id)
  const stages = deriveDeploymentStages(deployment)

  const doneCount    = stages.filter(s => s.status === 'done').length
  const totalCount   = stages.length

  const toggleContainer = (id: string) =>
    setOpenContainers(prev => { const n = new Set(prev); n.has(id) ? n.delete(id) : n.add(id); return n })

  // map pods to containers with logs/vars derived from real data
  const pods = deployment.pods ?? []
  // surface any container-level stuck reason across all pods (e.g. ImagePullBackOff)
  const globalStuckReason = pods.flatMap(p => p.containers).find(c => c.reason)?.reason

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
          {/* Deploying badge */}
          <span style={{
            display: 'inline-flex', alignItems: 'center', gap: 5,
            padding: '2px 10px', borderRadius: 99,
            background: C.amberBg, border: `1px solid ${C.amberBdr}`,
            fontFamily: S.mono, fontSize: 10, letterSpacing: '0.06em', color: C.amber,
          }}>
            <span className="dp-pulse" style={{ width: 5, height: 5, borderRadius: '50%', background: C.amber, display: 'inline-block' }} />
            Deploying
          </span>
          <span style={{ fontFamily: S.mono, fontSize: 11, color: C.faint }}>{deployment.id}</span>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <button
            disabled={undeploy.isPending}
            onClick={() => undeploy.mutate({ deployment_id: deployment.id }, { onSuccess: () => navigate(backPath) })}
            style={{
              display: 'inline-flex', alignItems: 'center', gap: 6,
              padding: '6px 14px', borderRadius: 6, cursor: undeploy.isPending ? 'not-allowed' : 'pointer',
              background: 'transparent', border: `1px solid ${C.coralBdr}`,
              fontFamily: S.body, fontSize: 13, color: C.coral, transition: 'background 0.12s',
              opacity: undeploy.isPending ? 0.6 : 1,
            }}
            onMouseEnter={e => { if (!undeploy.isPending) e.currentTarget.style.background = C.coralBg }}
            onMouseLeave={e => { e.currentTarget.style.background = 'transparent' }}
          >
            <span style={{ width: 7, height: 7, borderRadius: 1, background: C.coral, display: 'inline-block' }} />
            {undeploy.isPending ? 'Stopping…' : 'Stop'}
          </button>
          <button style={{
            display: 'inline-flex', alignItems: 'center', gap: 6,
            padding: '6px 14px', borderRadius: 6, cursor: 'pointer',
            background: 'transparent', border: `1px solid ${C.border}`,
            fontFamily: S.body, fontSize: 13, color: C.muted, transition: 'background 0.12s',
          }}
            onMouseEnter={e => { e.currentTarget.style.background = C.bgDeep }}
            onMouseLeave={e => { e.currentTarget.style.background = 'transparent' }}
          >
            <RotateCcw size={13} /> Restart
          </button>
          <button
            onClick={() => navigate(configurePath)}
            style={{
              display: 'inline-flex', alignItems: 'center', gap: 6,
              padding: '6px 14px', borderRadius: 6, cursor: 'pointer',
              background: 'transparent', border: `1px solid ${C.border}`,
              fontFamily: S.body, fontSize: 13, color: C.muted, transition: 'background 0.12s',
            }}
            onMouseEnter={e => { e.currentTarget.style.background = C.bgDeep }}
            onMouseLeave={e => { e.currentTarget.style.background = 'transparent' }}
          >
            <Settings2 size={13} /> Configure
          </button>
        </div>
      </header>

      {/* ── BODY ── */}
      <div style={{ display: 'flex', flex: 1, minHeight: 0, overflow: 'hidden' }}>

        {/* ── LEFT: card assembly sidebar ── */}
        <div style={{
          width: sidebarCollapsed ? 48 : 300, flexShrink: 0,
          display: 'flex', flexDirection: 'column',
          background: C.bg, borderRight: `1px solid ${C.border}`,
          transition: 'width 0.3s cubic-bezier(0.16,1,0.3,1)',
          overflow: 'hidden',
        }}>
          {/* sidebar header */}
          <div style={{ display: 'flex', alignItems: 'center', padding: '10px 10px 10px 14px', flexShrink: 0, borderBottom: `1px solid ${C.border}` }}>
            {!sidebarCollapsed && (
              <div style={{ flex: 1, minWidth: 0, marginRight: 8 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 5 }}>
                  <span style={{ fontFamily: S.mono, fontSize: 9, letterSpacing: '0.12em', color: C.stone }}>ASSEMBLING AGENT BADGE</span>
                  <span style={{ fontFamily: S.mono, fontSize: 9, color: C.faint }}>{doneCount}/{totalCount}</span>
                </div>
                <div style={{ height: 3, borderRadius: 2, background: C.bgDeep, overflow: 'hidden' }}>
                  <div style={{ width: `${totalCount > 0 ? (doneCount / totalCount) * 100 : 0}%`, height: '100%', borderRadius: 2, background: C.tealMid, transition: 'width 0.6s ease' }} />
                </div>
              </div>
            )}
            <button
              onClick={() => setSidebarCollapsed(c => !c)}
              style={{ background: 'none', border: 'none', cursor: 'pointer', color: C.faint, display: 'flex', padding: 4, borderRadius: 4, flexShrink: 0 }}
              onMouseEnter={e => { e.currentTarget.style.background = C.bgDeep; e.currentTarget.style.color = C.muted }}
              onMouseLeave={e => { e.currentTarget.style.background = 'none'; e.currentTarget.style.color = C.faint }}
            >
              {sidebarCollapsed ? <PanelLeftOpen size={14} /> : <PanelLeftClose size={14} />}
            </button>
          </div>

          {/* sidebar content: expanded */}
          {!sidebarCollapsed && (
            <div className="dp-scroll" style={{ flex: 1, overflowY: 'auto', padding: '20px 16px', display: 'flex', flexDirection: 'column', gap: 12 }}>
              {/* agent identity card preview */}
              <div style={{ width: '100%', borderRadius: 14, overflow: 'hidden', background: '#0f1a19', border: '1px solid rgba(87,196,193,0.15)', boxShadow: '0 12px 40px rgba(0,0,0,0.3)' }}>
                {/* notch */}
                <div style={{ display: 'flex', justifyContent: 'center', paddingTop: 10, background: '#0a0e0d' }}>
                  <div style={{ width: 32, height: 10, background: '#0f1a19', borderRadius: '0 0 8px 8px', border: '1px solid rgba(87,196,193,0.15)', borderTop: 'none' }} />
                </div>
                {/* avatar */}
                <div style={{ background: '#0a0e0d', paddingBottom: 10, display: 'flex', justifyContent: 'center', paddingTop: 14 }}>
                  <div style={{ width: 80, height: 80, borderRadius: 18, overflow: 'hidden', lineHeight: 0, border: '1px solid rgba(87,196,193,0.15)' }}>
                    <AgentIdentity account={account} name={deployment.name} size={80} />
                  </div>
                </div>
                {/* name */}
                <div style={{ background: '#0a0e0d', paddingBottom: 14, textAlign: 'center' }}>
                  <p style={{ fontFamily: S.mono, fontSize: 13, fontWeight: 700, letterSpacing: '0.12em', color: '#e8e4dc', margin: 0 }}>
                    {displayName.toUpperCase()}
                  </p>
                </div>
                {/* build progress bar */}
                <div style={{ padding: '0 14px 14px' }}>
                  <div style={{ height: 3, borderRadius: 2, background: 'rgba(87,196,193,0.1)', overflow: 'hidden' }}>
                    <div style={{ width: `${totalCount > 0 ? (doneCount / totalCount) * 100 : 0}%`, height: '100%', borderRadius: 2, background: C.tealMid }} />
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 6 }}>
                    <span style={{ fontFamily: S.mono, fontSize: 9, color: 'rgba(232,228,220,0.45)' }}>Building identity...</span>
                    <span style={{ fontFamily: S.mono, fontSize: 9, color: 'rgba(232,228,220,0.45)' }}>{doneCount}/{totalCount}</span>
                  </div>
                </div>
              </div>

              {/* stages checklist */}
              <div style={{ paddingTop: 12, borderTop: `1px solid ${C.border}` }}>
                <p style={{ fontFamily: S.mono, fontSize: 9, letterSpacing: '0.1em', textTransform: 'uppercase' as const, color: C.stone, marginBottom: 8 }}>Elements assembled</p>
                {stages.map((stage, i) => {
                  const isStuck = stage.status === 'running' && !!globalStuckReason
                  return (
                    <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '3px 0', opacity: stage.status === 'pending' ? 0.35 : 1 }}>
                      {stage.status === 'done' ? (
                        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke={C.tealMid} strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" style={{ flexShrink: 0 }}>
                          <path d="M20 6L9 17l-5-5" />
                        </svg>
                      ) : stage.status === 'running' ? (
                        <svg className={isStuck ? '' : 'dp-spin'} width="12" height="12" viewBox="0 0 24 24" fill="none" stroke={isStuck ? C.coral : C.stone} strokeWidth="2" strokeLinecap="round" style={{ flexShrink: 0 }}>
                          <path d="M21 12a9 9 0 1 1-6.219-8.56" />
                        </svg>
                      ) : (
                        <div style={{ width: 12, height: 12, borderRadius: '50%', border: `1.5px solid ${C.stone}`, flexShrink: 0 }} />
                      )}
                      <span style={{ fontFamily: S.body, fontSize: 11, color: isStuck ? C.coral : stage.status === 'running' ? C.amber : C.muted }}>
                        {stage.label}{isStuck ? ` — ${globalStuckReason}` : ''}
                      </span>
                    </div>
                  )
                })}
              </div>
            </div>
          )}

          {/* sidebar content: collapsed */}
          {sidebarCollapsed && (
            <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 6, padding: '16px 0' }}>
              <div style={{ width: 24, height: 24, borderRadius: 6, overflow: 'hidden', lineHeight: 0 }}>
                <AgentIdentity account={account} name={deployment.name} size={24} />
              </div>
              <div style={{ height: 40, width: 3, borderRadius: 2, background: C.bgDeep, overflow: 'hidden', position: 'relative' }}>
                <div style={{ position: 'absolute', bottom: 0, left: 0, right: 0, height: `${totalCount > 0 ? (doneCount / totalCount) * 100 : 0}%`, background: C.tealMid, transition: 'height 0.6s ease' }} />
              </div>
            </div>
          )}
        </div>

        {/* ── CENTER: container accordions ── */}
        <div className="dp-scroll" style={{ flex: 1, minWidth: 0, minHeight: 0, display: 'flex', flexDirection: 'column', overflowY: 'auto', padding: '16px 28px 28px' }}>

          {/* section header */}
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 14 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
              <span style={{ fontFamily: S.body, fontSize: 14, fontWeight: 700, color: C.teal }}>Logs</span>
              <span style={{ fontFamily: S.mono, fontSize: 10, color: C.amber }}>
                {pods.filter(p => p.phase === 'Running').length} running
              </span>
            </div>
            <span style={{ fontFamily: S.mono, fontSize: 10, color: C.faint }}>
              {pods.filter(p => p.phase === 'Running' || p.phase === 'Succeeded').length}/{Math.max(pods.length, 1)} ready
            </span>
          </div>

          {/* no pods yet */}
          {pods.length === 0 && (
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '16px 14px', borderRadius: 8, background: C.bgAlt, border: `1px solid ${C.border}` }}>
              <svg className="dp-spin" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke={C.amber} strokeWidth="2" strokeLinecap="round" style={{ flexShrink: 0 }}>
                <path d="M21 12a9 9 0 1 1-6.219-8.56" />
              </svg>
              <span style={{ fontFamily: S.body, fontSize: 13, color: C.muted }}>Provisioning pods…</span>
            </div>
          )}

          {/* container accordions from real pods */}
          {pods.map(pod => {
            const isOpen = openContainers.has(pod.name)
            const podReady = pod.phase === 'Running' || pod.phase === 'Succeeded'
            const status: 'done' | 'running' | 'pending' = podReady ? 'done' : 'running'
            // surface container-level stuck reason (e.g. ImagePullBackOff, CrashLoopBackOff)
            const stuckReason = !podReady ? pod.containers.find(c => c.reason)?.reason : undefined

            // gather vars from all containers
            const allVars = pod.containers.flatMap(c =>
              (c.env ?? []).map(e => ({
                key: e.name,
                value: e.value ?? '',
                secret: (e.from ?? '').startsWith('secret:') || /key|secret|token|password/i.test(e.name),
                source: (e.from ?? 'static') as string,
              }))
            )

            return (
              <ContainerAccordion
                key={pod.name}
                name={pod.name}
                containerStatus={status}
                stuckReason={stuckReason}
                logs={[`Pod ${pod.name} — phase: ${pod.phase}`]}
                vars={allVars}
                isOpen={isOpen}
                onToggle={() => toggleContainer(pod.name)}
              />
            )
          })}
        </div>
      </div>
    </div>
  )
}
