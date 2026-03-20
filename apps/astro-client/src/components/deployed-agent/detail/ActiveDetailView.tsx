import { useState } from "react";
import { useNavigate } from "react-router";
import { ArrowLeft, Settings2 } from "lucide-react";
import { AgentIdentity } from "@/components/AgentIdentity";
import { mapDeploymentStatus } from "@/lib/deployment-utils";
import type { AgentDeployment } from "@/lib/api";
import { C, S } from "./theme";
import { Styles } from "./styles";
import { KebabMenu } from "./shared/KebabMenu";
import { MonitorTab } from "./monitor/MonitorTab";
import { DeploymentsTab } from "./deployments/DeploymentsTab";
import { ConfigurePanel } from "./configure/ConfigurePanel";

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
