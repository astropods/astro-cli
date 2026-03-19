import { useState, useMemo, useRef, useEffect } from "react";
import { createPortal } from "react-dom";
import { Link, useNavigate } from "react-router";
import {
  Search, Pin, MoreVertical, Trash2, CirclePause,
  AlertTriangle, ArrowRight, X, Grid2x2, List,
  ChevronDown, Check, Building2, Share2, MoreHorizontal,
} from "lucide-react";
import { ProtectedRoute } from "../components/ProtectedRoute";
import { AgentIdentity } from "../components/AgentIdentity";
import { DeleteDeploymentDialog } from "../components/DeleteDeploymentDialog";
import { useDeployments } from "../api/queries/deployments";
import { useAccountAgents } from "../api/queries/agents";
import { useAuth } from "../lib/auth";
import { mapDeploymentStatus, deploymentStatusLabel } from "../lib/deployment-utils";
import { deploymentPath } from "../lib/routes";
import type { AgentDeployment } from "../lib/api";

// ─── design tokens (maps to astro-client CSS vars) ────────────────────────────
const C = {
  canvas:    'var(--color-stone-200)',
  surface:   'var(--color-stone-25)',
  border:    'var(--color-stone-400)',
  borderMd:  'var(--color-stone-300)',
  teal:      'var(--color-teal-800)',
  tealMid:   'var(--color-teal-500)',
  tealSubtle:'color-mix(in oklch, var(--color-teal-500) 6%, transparent)',
  tealHover: 'color-mix(in oklch, var(--color-teal-500) 10%, transparent)',
  text:      'var(--foreground)',
  textMuted: 'var(--muted-foreground)',
  textFaint: 'var(--faint-foreground)',
  coral:     'var(--color-coral-500)',
  coralBg:   'color-mix(in oklch, var(--color-coral-500) 8%, transparent)',
  coralBdr:  'color-mix(in oklch, var(--color-coral-500) 30%, transparent)',
  amber:     'var(--color-amber-700)',
  amberBg:   'color-mix(in oklch, var(--color-amber-700) 8%, transparent)',
  amberBdr:  'color-mix(in oklch, var(--color-amber-700) 25%, transparent)',
  success:   'var(--color-teal-500)',
  stone200:  'var(--color-stone-200)',
  stone300:  'var(--color-stone-300)',
  stone400:  'var(--color-stone-400)',
} as const;

const F = {
  sans: 'var(--font-sans)',
  mono: 'var(--font-mono)',
} as const;

// ─── status style ─────────────────────────────────────────────────────────────
type DeployedAgentStatus = 'active' | 'inactive' | 'pending' | 'error';

const STATUS_STYLE: Record<DeployedAgentStatus, { dot: string; text: string }> = {
  active:   { dot: C.success,   text: C.success   },
  inactive: { dot: C.textFaint, text: C.textFaint },
  pending:  { dot: C.amber,     text: C.amber     },
  error:    { dot: C.coral,     text: C.coral     },
};

// ─── helpers ──────────────────────────────────────────────────────────────────
function greeting() {
  const h = new Date().getHours();
  if (h < 12) return 'Good morning';
  if (h < 17) return 'Good afternoon';
  return 'Good evening';
}

function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return 'Just now';
  if (mins < 60) return `${mins} min ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs} hr ago`;
  const days = Math.floor(hrs / 24);
  if (days === 1) return 'Yesterday';
  return `${days} days ago`;
}

// ─── types ────────────────────────────────────────────────────────────────────
type Tab = 'Deployed Agents' | 'Agent Blueprints';
type SortOrder = 'recent' | 'oldest';
type ViewMode = 'list' | 'grid';

// ─── agent row ────────────────────────────────────────────────────────────────
function AgentRow({
  deployment, account, pinned, onTogglePin,
}: {
  deployment: AgentDeployment; account: string;
  pinned: boolean; onTogglePin: () => void;
}) {
  const navigate = useNavigate();
  const [hovered, setHovered] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [menuPos, setMenuPos] = useState({ top: 0, right: 0 });
  const kebabRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (menuOpen && kebabRef.current) {
      const rect = kebabRef.current.getBoundingClientRect();
      setMenuPos({ top: rect.bottom + 4, right: window.innerWidth - rect.right });
    }
  }, [menuOpen]);
  const status = mapDeploymentStatus(deployment) as DeployedAgentStatus;
  const s = STATUS_STYLE[status];
  const displayName = deployment.display_name || deployment.name;

  return (
    <>
      <div
        onClick={() => navigate(deploymentPath(account, deployment.id))}
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
        style={{
          display: 'grid',
          gridTemplateColumns: '1fr 100px 100px 120px 40px 32px',
          alignItems: 'center', gap: 16, padding: '12px 20px',
          cursor: 'pointer',
          borderBottom: `1px solid ${C.stone200}`,
          background: pinned
            ? (hovered ? C.tealHover : C.tealSubtle)
            : (hovered ? C.canvas : 'transparent'),
          transition: 'background 0.1s',
        }}
      >
        {/* name */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, minWidth: 0 }}>
          <div style={{ flexShrink: 0, borderRadius: 6, overflow: 'hidden', lineHeight: 0 }}>
            <AgentIdentity account={account} name={deployment.name} size={28} />
          </div>
          <span style={{ fontFamily: F.sans, fontSize: 13, fontWeight: 500, color: C.text, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {displayName}
          </span>
          {pinned && <Pin size={11} color={C.tealMid} fill={C.tealMid} style={{ flexShrink: 0 }} />}
        </div>

        {/* status */}
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 5, fontFamily: F.mono, fontSize: 10 }}>
          <span style={{ width: 5, height: 5, borderRadius: '50%', background: s.dot, flexShrink: 0 }} />
          <span style={{ color: s.text }}>{deploymentStatusLabel[status].toUpperCase()}</span>
        </span>

        {/* requests */}
        <span style={{ fontFamily: F.mono, fontSize: 12, color: C.textMuted }}>—</span>

        {/* last active */}
        <span style={{ fontFamily: F.mono, fontSize: 12, color: C.textMuted }}>{timeAgo(deployment.created_at)}</span>

        {/* pin */}
        <button
          onClick={e => { e.stopPropagation(); onTogglePin(); }}
          title={pinned ? 'Unpin' : 'Pin'}
          style={{ background: 'none', border: 'none', cursor: 'pointer', color: pinned ? C.tealMid : C.textFaint, lineHeight: 0, padding: 4, opacity: pinned ? 1 : 0.45, transition: 'opacity 0.12s' }}
          onMouseEnter={e => { e.currentTarget.style.opacity = '1'; }}
          onMouseLeave={e => { e.currentTarget.style.opacity = pinned ? '1' : '0.45'; }}
        >
          <Pin size={13} fill={pinned ? C.tealMid : 'none'} />
        </button>

        {/* kebab */}
        <div onClick={e => e.stopPropagation()}>
          {menuOpen && createPortal(
            <>
              <div onClick={() => setMenuOpen(false)} style={{ position: 'fixed', inset: 0, zIndex: 1000 }} />
              <div style={{ position: 'fixed', top: menuPos.top, right: menuPos.right, background: C.surface, border: `1px solid ${C.border}`, borderRadius: 8, overflow: 'hidden', boxShadow: '0 6px 20px rgba(7,61,60,0.12)', zIndex: 1001, minWidth: 172, padding: '4px 0' }}>
                {[
                  { icon: <CirclePause size={13} />, label: 'Pause agent', color: C.text, onClick: () => setMenuOpen(false) },
                ].map(item => (
                  <button key={item.label} onClick={item.onClick} style={{ width: '100%', display: 'flex', alignItems: 'center', gap: 8, padding: '8px 14px', background: 'none', border: 'none', fontFamily: F.sans, fontSize: 13, color: item.color, cursor: 'pointer', textAlign: 'left', transition: 'background 0.1s' }}
                    onMouseEnter={e => { e.currentTarget.style.background = C.canvas; }}
                    onMouseLeave={e => { e.currentTarget.style.background = 'none'; }}
                  >
                    {item.icon}{item.label}
                  </button>
                ))}
                <div style={{ height: 1, background: C.stone200, margin: '4px 0' }} />
                <button onClick={() => { setMenuOpen(false); setDeleteOpen(true); }} style={{ width: '100%', display: 'flex', alignItems: 'center', gap: 8, padding: '8px 14px', background: 'none', border: 'none', fontFamily: F.sans, fontSize: 13, color: C.coral, cursor: 'pointer', textAlign: 'left', transition: 'background 0.1s' }}
                  onMouseEnter={e => { e.currentTarget.style.background = C.canvas; }}
                  onMouseLeave={e => { e.currentTarget.style.background = 'none'; }}
                >
                  <Trash2 size={13} />Delete
                </button>
              </div>
            </>,
            document.body
          )}
          <button
            ref={kebabRef}
            onClick={() => setMenuOpen(v => !v)}
            style={{ background: 'none', border: 'none', cursor: 'pointer', color: C.textFaint, lineHeight: 0, padding: 4, opacity: 0.45, transition: 'opacity 0.12s' }}
            onMouseEnter={e => { e.currentTarget.style.opacity = '1'; }}
            onMouseLeave={e => { e.currentTarget.style.opacity = '0.45'; }}
          >
            <MoreVertical size={13} />
          </button>
        </div>
      </div>

      <DeleteDeploymentDialog
        open={deleteOpen} onOpenChange={setDeleteOpen}
        deploymentId={deployment.id} deploymentName={deployment.name}
        displayName={deployment.display_name} account={account}
      />
    </>
  );
}

// ─── agent card (grid view) ───────────────────────────────────────────────────
function AgentCard({ deployment, account }: { deployment: AgentDeployment; account: string }) {
  const [hovered, setHovered] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const status = mapDeploymentStatus(deployment) as DeployedAgentStatus;
  const s = STATUS_STYLE[status];
  const displayName = deployment.display_name || deployment.name;

  return (
    <>
      <Link
        to={deploymentPath(account, deployment.id)}
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
        style={{
          display: 'flex', flexDirection: 'column', gap: 12,
          borderRadius: 8, border: `1px solid ${hovered ? C.tealMid : C.border}`,
          background: C.surface, padding: '14px 16px',
          transition: 'all 0.15s', boxShadow: hovered ? '0 4px 16px rgba(7,61,60,0.1)' : 'none',
          textDecoration: 'none',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <div style={{ flexShrink: 0, borderRadius: 6, overflow: 'hidden', lineHeight: 0 }}>
            <AgentIdentity account={account} name={deployment.name} size={36} />
          </div>
          <div style={{ minWidth: 0, flex: 1 }}>
            <p style={{ fontFamily: F.sans, fontSize: 13, fontWeight: 600, color: C.text, margin: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{displayName}</p>
            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 5, fontFamily: F.mono, fontSize: 10, marginTop: 4 }}>
              <span style={{ width: 5, height: 5, borderRadius: '50%', background: s.dot }} />
              <span style={{ color: s.text }}>{deploymentStatusLabel[status].toUpperCase()}</span>
            </span>
          </div>
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '8px 16px' }}>
          {[
            { label: 'Last active', value: timeAgo(deployment.created_at) },
            { label: 'Requests',    value: '—' },
          ].map(stat => (
            <div key={stat.label}>
              <p style={{ fontFamily: F.mono, fontSize: 9, letterSpacing: '0.1em', color: C.textFaint, textTransform: 'uppercase', marginBottom: 3 }}>{stat.label}</p>
              <p style={{ fontFamily: F.mono, fontSize: 12, color: C.textMuted, margin: 0 }}>{stat.value}</p>
            </div>
          ))}
        </div>
      </Link>

      <DeleteDeploymentDialog
        open={deleteOpen} onOpenChange={setDeleteOpen}
        deploymentId={deployment.id} deploymentName={deployment.name}
        displayName={deployment.display_name} account={account}
      />
    </>
  );
}

// ─── page ─────────────────────────────────────────────────────────────────────
function YourAgentsContent() {
  const navigate = useNavigate();
  const { user, personalAccount, accounts, isAuthenticated } = useAuth();
  const account = personalAccount?.name ?? '';

  const { data } = useDeployments(account, isAuthenticated);
  const deployments = data?.deployments ?? [];

  const { data: agentsData } = useAccountAgents(account, isAuthenticated);
  const blueprints = agentsData?.agents ?? [];

  const [activeTab, setActiveTab]       = useState<Tab>('Deployed Agents');
  const [search, setSearch]             = useState('');
  const [statusFilter, setStatusFilter] = useState<'All' | 'Active' | 'Error' | 'Deploying'>('All');
  const [sortOrder, setSortOrder]       = useState<SortOrder>('recent');
  const [viewMode, setViewMode]         = useState<ViewMode>('list');
  const [pinned, setPinned]             = useState<Set<string>>(new Set());
  const [dismissedErrors, setDismissedErrors] = useState<Set<string>>(new Set());
  const [orgDropdownOpen, setOrgDropdownOpen] = useState(false);
  const [activeOrg, setActiveOrg]       = useState<string | null>(null);
  const [blueprintMenuOpen, setBlueprintMenuOpen] = useState<string | null>(null);

  const orgs = (accounts ?? []).filter(a => a.type !== 'personal');
  const currentLabel = activeOrg ?? account;
  const firstName = user?.first_name || account || 'there';

  const togglePin = (id: string) =>
    setPinned(prev => { const n = new Set(prev); n.has(id) ? n.delete(id) : n.add(id); return n; });

  const filtered = useMemo(() => {
    let list = deployments.filter(d => {
      const status = mapDeploymentStatus(d);
      const name = (d.display_name || d.name).toLowerCase();
      if (search && !name.includes(search.toLowerCase())) return false;
      if (statusFilter === 'Active'   && status !== 'active')  return false;
      if (statusFilter === 'Error'    && status !== 'error')   return false;
      if (statusFilter === 'Deploying'&& status !== 'pending') return false;
      return true;
    });
    list = list.sort((a, b) => {
      const d = new Date(b.created_at).getTime() - new Date(a.created_at).getTime();
      return sortOrder === 'recent' ? d : -d;
    });
    return [
      ...list.filter(d => pinned.has(d.id)),
      ...list.filter(d => !pinned.has(d.id)),
    ];
  }, [deployments, search, statusFilter, sortOrder, pinned]);

  const errorDeployments = deployments.filter(
    d => mapDeploymentStatus(d) === 'error' && !dismissedErrors.has(d.id)
  );
  const activeCount = deployments.filter(d => mapDeploymentStatus(d) === 'active').length;

  return (
    <div style={{ minHeight: 'calc(100vh - 56px)', background: C.canvas }}>

      {/* ── BREADCRUMB HEADER ── */}
      <header style={{ background: C.stone200, borderBottom: `1px solid ${C.border}`, position: 'sticky', top: 56, zIndex: 40 }}>
        <div style={{ padding: '0 48px', display: 'flex', alignItems: 'center', justifyContent: 'space-between', height: 40 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span style={{ fontFamily: F.mono, fontSize: 12, color: C.textFaint }}>Dashboard</span>
            <span style={{ color: C.textFaint, fontSize: 14 }}>/</span>

            {/* org switcher */}
            <div style={{ position: 'relative' }}>
              {orgDropdownOpen && (
                <>
                  <div onClick={() => setOrgDropdownOpen(false)} style={{ position: 'fixed', inset: 0, zIndex: 10 }} />
                  <div style={{ position: 'absolute', top: 'calc(100% + 8px)', left: 0, background: C.surface, borderRadius: 10, border: `1px solid ${C.border}`, boxShadow: '0 8px 28px rgba(7,61,60,0.14)', zIndex: 50, minWidth: 220, overflow: 'hidden' }}>
                    {/* personal */}
                    <button
                      onClick={() => { setActiveOrg(null); setOrgDropdownOpen(false); }}
                      style={{ width: '100%', display: 'flex', alignItems: 'center', gap: 10, padding: '10px 14px', background: 'none', border: 'none', cursor: 'pointer', transition: 'background 0.1s' }}
                      onMouseEnter={e => { e.currentTarget.style.background = C.canvas; }}
                      onMouseLeave={e => { e.currentTarget.style.background = 'none'; }}
                    >
                      {user?.profile_picture_url
                        ? <img src={user.profile_picture_url} style={{ width: 22, height: 22, borderRadius: '50%', objectFit: 'cover', flexShrink: 0 }} />
                        : <div style={{ width: 22, height: 22, borderRadius: '50%', background: C.teal, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}><span style={{ fontSize: 9, fontWeight: 700, color: '#fff' }}>{(user?.first_name?.[0] ?? account[0] ?? '?').toUpperCase()}</span></div>
                      }
                      <span style={{ fontFamily: F.sans, fontSize: 13, color: C.text, flex: 1, textAlign: 'left' }}>{account}</span>
                      {activeOrg === null && <Check size={14} color={C.tealMid} />}
                    </button>

                    {orgs.length > 0 && <div style={{ height: 1, background: C.stone300, margin: '2px 0' }} />}

                    {orgs.map(org => (
                      <button key={org.name}
                        onClick={() => { setActiveOrg(org.name); setOrgDropdownOpen(false); }}
                        style={{ width: '100%', display: 'flex', alignItems: 'center', gap: 10, padding: '10px 14px', background: 'none', border: 'none', cursor: 'pointer', transition: 'background 0.1s' }}
                        onMouseEnter={e => { e.currentTarget.style.background = C.canvas; }}
                        onMouseLeave={e => { e.currentTarget.style.background = 'none'; }}
                      >
                        <div style={{ width: 22, height: 22, borderRadius: 6, background: C.teal, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                          <Building2 size={11} color="#fff" />
                        </div>
                        <span style={{ fontFamily: F.sans, fontSize: 13, color: C.text, flex: 1, textAlign: 'left' }}>{org.name}</span>
                        {activeOrg === org.name && <Check size={14} color={C.tealMid} />}
                      </button>
                    ))}

                    <div style={{ height: 1, background: C.stone300, margin: '2px 0' }} />
                    {['Manage organizations', 'Create organization'].map(label => (
                      <button key={label} onClick={() => setOrgDropdownOpen(false)} style={{ width: '100%', display: 'flex', alignItems: 'center', gap: 8, padding: '9px 14px', background: 'none', border: 'none', cursor: 'pointer', fontFamily: F.sans, fontSize: 12, color: C.textFaint, textAlign: 'left', transition: 'background 0.1s' }}
                        onMouseEnter={e => { e.currentTarget.style.background = C.canvas; }}
                        onMouseLeave={e => { e.currentTarget.style.background = 'none'; }}
                      >{label}</button>
                    ))}
                  </div>
                </>
              )}

              <button
                onClick={() => setOrgDropdownOpen(v => !v)}
                style={{ display: 'inline-flex', alignItems: 'center', gap: 5, background: 'none', border: 'none', cursor: 'pointer', fontFamily: F.mono, fontSize: 12, fontWeight: 600, color: orgDropdownOpen ? C.tealMid : C.text, padding: '4px 6px', borderRadius: 6, transition: 'all 0.12s' }}
                onMouseEnter={e => { e.currentTarget.style.background = C.stone300; }}
                onMouseLeave={e => { e.currentTarget.style.background = 'none'; }}
              >
                {user?.profile_picture_url
                  ? <img src={user.profile_picture_url} style={{ width: 20, height: 20, borderRadius: '50%', objectFit: 'cover' }} />
                  : <div style={{ width: 20, height: 20, borderRadius: '50%', background: C.teal, display: 'flex', alignItems: 'center', justifyContent: 'center' }}><span style={{ fontSize: 8, fontWeight: 700, color: '#fff' }}>{(user?.first_name?.[0] ?? account[0] ?? '?').toUpperCase()}</span></div>
                }
                {currentLabel}
                <ChevronDown size={13} style={{ transform: orgDropdownOpen ? 'rotate(180deg)' : 'none', transition: 'transform 0.15s' }} />
              </button>
            </div>
          </div>

          <button
            onClick={() => navigate('/browse')}
            style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '6px 14px', borderRadius: 7, cursor: 'pointer', background: 'transparent', border: `1px solid ${C.border}`, fontFamily: F.sans, fontSize: 12, fontWeight: 500, color: C.textMuted, transition: 'all 0.12s' }}
            onMouseEnter={e => { e.currentTarget.style.borderColor = C.tealMid; e.currentTarget.style.color = C.tealMid; }}
            onMouseLeave={e => { e.currentTarget.style.borderColor = C.border; e.currentTarget.style.color = C.textMuted; }}
          >
            Explore agents
          </button>
        </div>
      </header>

      {/* ── MAIN ── */}
      <div style={{ display: 'flex' }}>
        <main style={{ flex: 1, minWidth: 0, padding: '36px 48px 64px', display: 'flex', gap: 32, alignItems: 'flex-start' }}>
          <div style={{ flex: 1, minWidth: 0 }}>

            {/* greeting + stats */}
            <div style={{ marginBottom: 32 }}>
              <h1 style={{ fontFamily: F.sans, fontSize: 26, fontWeight: 700, color: C.text, letterSpacing: '-0.02em', marginBottom: 6 }}>
                {greeting()}, {firstName}
              </h1>
              <p style={{ fontFamily: F.sans, fontSize: 13, color: C.textFaint, marginBottom: 20 }}>
                Here's what's running across your workspace.
              </p>
              <div style={{ display: 'flex', gap: 12 }}>
                {[
                  { label: 'Active agents',  value: String(activeCount) },
                  { label: 'Requests today', value: '—' },
                  { label: 'Compute hrs',    value: '—' },
                ].map(stat => (
                  <div key={stat.label} style={{ flex: 1, background: C.surface, border: `1px solid ${C.borderMd}`, borderRadius: 10, padding: '14px 16px' }}>
                    <p style={{ fontFamily: F.mono, fontSize: 10, letterSpacing: '0.1em', color: C.textFaint, marginBottom: 10, textTransform: 'uppercase' }}>{stat.label}</p>
                    <span style={{ fontFamily: F.sans, fontSize: 22, fontWeight: 700, color: C.text, letterSpacing: '-0.02em', lineHeight: 1 }}>{stat.value}</span>
                  </div>
                ))}
              </div>
            </div>

            {/* tab nav */}
            <nav style={{ display: 'flex', borderBottom: `1px solid ${C.stone300}`, marginBottom: 16, padding: '0 4px' }}>
              {(['Deployed Agents', 'Agent Blueprints'] as Tab[]).map(tab => {
                const isActive = activeTab === tab;
                const count = tab === 'Deployed Agents' ? deployments.length : blueprints.length;
                return (
                  <button
                    key={tab}
                    onClick={() => setActiveTab(tab)}
                    style={{ display: 'flex', alignItems: 'center', gap: 6, background: 'none', border: 'none', cursor: 'pointer', fontFamily: F.sans, fontSize: 13, fontWeight: isActive ? 600 : 400, color: isActive ? C.text : C.textFaint, padding: '14px 16px', borderBottom: isActive ? `2px solid ${C.tealMid}` : '2px solid transparent', marginBottom: -1, transition: 'color 0.15s' }}
                  >
                    {tab}
                    <span style={{ fontFamily: F.mono, fontSize: 10, color: C.textFaint, background: C.stone200, borderRadius: 10, padding: '1px 6px', marginLeft: 4 }}>{count}</span>
                  </button>
                );
              })}
            </nav>

            {/* deployed agents tab */}
            {activeTab === 'Deployed Agents' && (
              <div>
                {/* toolbar */}
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12, flexWrap: 'wrap' as const }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '6px 12px', borderRadius: 6, border: `1px solid ${C.border}`, background: C.surface, flexShrink: 0 }}>
                    <Search size={13} color={C.textFaint} />
                    <input
                      type="text" placeholder="Search agents" value={search}
                      onChange={e => setSearch(e.target.value)}
                      style={{ width: 148, background: 'transparent', border: 'none', outline: 'none', fontFamily: F.sans, fontSize: 12, color: C.text }}
                    />
                  </div>

                  {(['All', 'Active', 'Error', 'Deploying'] as const).map(f => (
                    <button key={f} onClick={() => setStatusFilter(f)} style={{ padding: '6px 14px', borderRadius: 6, cursor: 'pointer', fontFamily: F.sans, fontSize: 12, fontWeight: statusFilter === f ? 600 : 400, background: statusFilter === f ? C.tealSubtle : 'transparent', border: `1px solid ${statusFilter === f ? C.tealMid : C.border}`, color: statusFilter === f ? C.tealMid : C.textMuted, transition: 'all 0.12s' }}>{f}</button>
                  ))}

                  <div style={{ flex: 1 }} />

                  <select
                    value={sortOrder} onChange={e => setSortOrder(e.target.value as SortOrder)}
                    style={{ padding: '6px 30px 6px 10px', borderRadius: 8, cursor: 'pointer', fontFamily: F.sans, fontSize: 12, color: C.textMuted, background: C.surface, border: `1px solid ${C.border}`, outline: 'none', appearance: 'none', WebkitAppearance: 'none', backgroundImage: `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%236b7e7c' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpolyline points='6 9 12 15 18 9'%3E%3C/polyline%3E%3C/svg%3E")`, backgroundRepeat: 'no-repeat', backgroundPosition: 'right 10px center' }}
                  >
                    <option value="recent">Most recent</option>
                    <option value="oldest">Least recent</option>
                  </select>

                  <div style={{ display: 'flex', background: C.stone300, borderRadius: 8, padding: 2, gap: 2 }}>
                    {([{ mode: 'list' as ViewMode, icon: <List size={14} /> }, { mode: 'grid' as ViewMode, icon: <Grid2x2 size={14} /> }]).map(({ mode, icon }) => (
                      <button key={mode} onClick={() => setViewMode(mode)} style={{ width: 28, height: 28, borderRadius: 6, border: 'none', cursor: 'pointer', display: 'flex', alignItems: 'center', justifyContent: 'center', background: viewMode === mode ? C.surface : 'transparent', color: viewMode === mode ? C.teal : C.textFaint, boxShadow: viewMode === mode ? '0 1px 3px rgba(7,61,60,0.1)' : 'none', transition: 'all 0.12s' }}>{icon}</button>
                    ))}
                  </div>
                </div>

                {/* list view */}
                {viewMode === 'list' && (
                  <div style={{ background: C.surface, border: `1px solid ${C.border}`, borderRadius: 6, overflow: 'hidden' }}>
                    <div style={{ display: 'grid', gridTemplateColumns: '1fr 100px 100px 120px 40px 32px', gap: 16, padding: '10px 20px', borderBottom: `1px solid ${C.stone300}`, background: C.stone200 }}>
                      {['Agent', 'Status', 'Requests', 'Last active', '', ''].map((h, i) => (
                        <span key={i} style={{ fontFamily: F.mono, fontSize: 10, letterSpacing: '0.07em', color: C.textFaint }}>{h.toUpperCase()}</span>
                      ))}
                    </div>
                    {filtered.length === 0
                      ? <div style={{ padding: '48px 20px', textAlign: 'center' }}><p style={{ fontFamily: F.sans, fontSize: 13, color: C.textFaint }}>No agents match your filters.</p></div>
                      : filtered.map(d => (
                          <AgentRow key={d.id} deployment={d} account={account} pinned={pinned.has(d.id)} onTogglePin={() => togglePin(d.id)} />
                        ))
                    }
                  </div>
                )}

                {/* grid view */}
                {viewMode === 'grid' && (
                  <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(260px, 1fr))', gap: 12 }}>
                    {filtered.length === 0
                      ? <div style={{ gridColumn: '1/-1', padding: '48px 0', textAlign: 'center' }}><p style={{ fontFamily: F.sans, fontSize: 13, color: C.textFaint }}>No agents match your filters.</p></div>
                      : filtered.map(d => <AgentCard key={d.id} deployment={d} account={account} />)
                    }
                  </div>
                )}
              </div>
            )}

            {/* blueprints tab */}
            {activeTab === 'Agent Blueprints' && (
              <div style={{ padding: '4px 0' }}>
                {blueprints.length === 0 ? (
                  <div style={{ padding: '48px 20px', textAlign: 'center' }}>
                    <p style={{ fontFamily: F.sans, fontSize: 13, color: C.textFaint }}>No published blueprints yet.</p>
                  </div>
                ) : blueprints.map((agent, i) => {
                  const latestVersion = agent.versions?.[agent.versions.length - 1];
                  const spec = latestVersion?.spec as Record<string, unknown> | undefined;
                  const meta = spec?.meta as Record<string, unknown> | undefined;
                  const displayName = (spec?.name as string | undefined) ?? agent.name;
                  const description = (meta?.description as string | undefined) ?? '';
                  const version = (meta?.version as string | undefined) ?? latestVersion?.build_id?.slice(0, 8) ?? '—';
                  const isMenuOpen = blueprintMenuOpen === agent.name;
                  return (
                    <div key={agent.name} style={{ display: 'flex', alignItems: 'center', gap: 16, padding: '14px 20px', borderBottom: i < blueprints.length - 1 ? `1px solid ${C.stone300}` : 'none' }}>
                      <div style={{ flexShrink: 0, borderRadius: 8, overflow: 'hidden', lineHeight: 0 }}>
                        <AgentIdentity account={account} name={agent.name} size={40} />
                      </div>
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <Link to={`/${account}/${agent.name}`} style={{ fontFamily: F.sans, fontSize: 14, fontWeight: 600, color: C.text, margin: '0 0 3px', display: 'block', textDecoration: 'none' }} onMouseEnter={e => { e.currentTarget.style.color = C.tealMid; }} onMouseLeave={e => { e.currentTarget.style.color = C.text; }}>{displayName}</Link>
                        <p style={{ fontFamily: F.sans, fontSize: 12, color: C.textMuted, margin: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{description || agent.name}</p>
                      </div>
                      <div style={{ display: 'flex', gap: 24, flexShrink: 0 }}>
                        {[
                          { label: 'deploys', value: `${agent.versions?.length ?? 0}` },
                          { label: 'hearts',  value: '—' },
                          { label: 'version',  value: version },
                        ].map(stat => (
                          <div key={stat.label} style={{ textAlign: 'right' }}>
                            <p style={{ fontFamily: F.mono, fontSize: 13, fontWeight: 600, color: C.text, margin: '0 0 2px' }}>{stat.value}</p>
                            <p style={{ fontFamily: F.sans, fontSize: 10, color: C.textFaint, margin: 0 }}>{stat.label}</p>
                          </div>
                        ))}
                      </div>
                      <div style={{ display: 'flex', gap: 8, flexShrink: 0 }}>
                        <button
                          style={{
                            display: 'inline-flex', alignItems: 'center', gap: 6,
                            padding: '6px 14px', borderRadius: 7, cursor: 'pointer',
                            background: 'transparent', border: `1px solid ${C.border}`,
                            fontFamily: F.sans, fontSize: 12, fontWeight: 500, color: C.textMuted,
                            transition: 'all 0.12s',
                          }}
                          onMouseEnter={e => { e.currentTarget.style.borderColor = C.tealMid; e.currentTarget.style.color = C.tealMid; }}
                          onMouseLeave={e => { e.currentTarget.style.borderColor = C.border; e.currentTarget.style.color = C.textMuted; }}
                        >
                          <Share2 size={12} />Share
                        </button>
                        <div style={{ position: 'relative' }} onClick={e => e.stopPropagation()}>
                          {isMenuOpen && (
                            <>
                              <div onClick={() => setBlueprintMenuOpen(null)} style={{ position: 'fixed', inset: 0, zIndex: 10 }} />
                              <div style={{
                                position: 'absolute', right: 0, top: 'calc(100% + 4px)',
                                background: C.surface, border: `1px solid ${C.border}`,
                                borderRadius: 8, overflow: 'hidden',
                                boxShadow: '0 6px 20px rgba(7,61,60,0.12)', zIndex: 20, minWidth: 160,
                              }}>
                                <button
                                  onClick={() => { setBlueprintMenuOpen(null); navigate(`/deploy/${account}/${agent.name}`); }}
                                  style={{ width: '100%', display: 'flex', alignItems: 'center', gap: 8, padding: '9px 14px', background: 'none', border: 'none', fontFamily: F.sans, fontSize: 13, color: C.text, cursor: 'pointer', textAlign: 'left', transition: 'background 0.1s' }}
                                  onMouseEnter={e => { e.currentTarget.style.background = C.stone200; }}
                                  onMouseLeave={e => { e.currentTarget.style.background = 'none'; }}
                                >
                                  <ArrowRight size={13} />Deploy
                                </button>
                              </div>
                            </>
                          )}
                          <button
                            onClick={() => setBlueprintMenuOpen(isMenuOpen ? null : agent.name)}
                            style={{ background: 'none', border: 'none', cursor: 'pointer', color: C.textFaint, lineHeight: 0, padding: 4, opacity: 0.45, transition: 'opacity 0.12s' }}
                            onMouseEnter={e => { e.currentTarget.style.opacity = '1'; }}
                            onMouseLeave={e => { e.currentTarget.style.opacity = '0.45'; }}
                          >
                            <MoreHorizontal size={15} />
                          </button>
                        </div>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>

          {/* ── RIGHT SIDEBAR ── */}
          <div style={{ width: 260, flexShrink: 0, display: 'flex', flexDirection: 'column', gap: 12, alignSelf: 'flex-start', position: 'sticky', top: 96 }}>

            {/* needs attention */}
            <div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 7, marginBottom: 10, padding: '0 2px' }}>
                <AlertTriangle size={13} color={C.coral} />
                <span style={{ fontFamily: F.sans, fontSize: 13, fontWeight: 600, color: C.text }}>Needs attention</span>
                {errorDeployments.length > 0 && (
                  <span style={{ fontFamily: F.mono, fontSize: 10, fontWeight: 600, color: C.coral, background: C.coralBg, border: `1px solid ${C.coralBdr}`, borderRadius: 10, padding: '1px 6px', marginLeft: 'auto' }}>
                    {errorDeployments.length}
                  </span>
                )}
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                {errorDeployments.length === 0 ? (
                  <div style={{ padding: 16, textAlign: 'center', background: C.surface, border: `1px solid ${C.borderMd}`, borderRadius: 10 }}>
                    <p style={{ fontFamily: F.sans, fontSize: 12, color: C.textFaint, margin: 0 }}>All clear</p>
                  </div>
                ) : (
                  errorDeployments.map(d => (
                    <div key={d.id} style={{ position: 'relative', background: C.coralBg, border: `1px solid ${C.coralBdr}`, borderLeft: `3px solid ${C.coral}`, borderRadius: 10, padding: '13px 14px 12px' }}>
                      <div style={{ display: 'flex', alignItems: 'flex-start', gap: 7, marginBottom: 6 }}>
                        <span style={{ width: 7, height: 7, borderRadius: '50%', background: C.coral, flexShrink: 0, marginTop: 4 }} />
                        <span style={{ fontFamily: F.sans, fontSize: 13, fontWeight: 600, color: C.text, lineHeight: 1.35, flex: 1 }}>{d.display_name || d.name}</span>
                      </div>
                      <p style={{ fontFamily: F.sans, fontSize: 12, color: C.textMuted, margin: '0 0 10px', lineHeight: 1.5, paddingLeft: 14 }}>Agent is in an error state. Check logs for details.</p>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 6, paddingLeft: 14 }}>
                        <button onClick={() => navigate(deploymentPath(account, d.id))} style={{ display: 'inline-flex', alignItems: 'center', gap: 4, padding: '4px 10px', borderRadius: 6, cursor: 'pointer', background: 'transparent', border: `1px solid ${C.coralBdr}`, fontFamily: F.sans, fontSize: 11, fontWeight: 500, color: C.coral, transition: 'background 0.12s' }}
                          onMouseEnter={e => { e.currentTarget.style.background = C.coralBg; }}
                          onMouseLeave={e => { e.currentTarget.style.background = 'transparent'; }}
                        >View details <ArrowRight size={10} /></button>
                        <button onClick={() => setDismissedErrors(prev => new Set([...prev, d.id]))} style={{ display: 'inline-flex', alignItems: 'center', gap: 4, padding: '4px 8px', borderRadius: 6, cursor: 'pointer', background: 'transparent', border: '1px solid transparent', fontFamily: F.sans, fontSize: 11, color: C.textFaint, transition: 'color 0.12s' }}
                          onMouseEnter={e => { e.currentTarget.style.color = C.textMuted; }}
                          onMouseLeave={e => { e.currentTarget.style.color = C.textFaint; }}
                        ><X size={10} /> Dismiss</button>
                      </div>
                      <span style={{ position: 'absolute', top: 13, right: 12, fontFamily: F.mono, fontSize: 10, color: C.textFaint }}>{timeAgo(d.created_at)}</span>
                    </div>
                  ))
                )}
              </div>
            </div>

            {/* recent deployments timeline */}
            {deployments.length > 0 && (
              <div style={{ background: C.surface, border: `1px solid ${C.borderMd}`, borderRadius: 12, overflow: 'hidden' }}>
                <div style={{ padding: '10px 16px', borderBottom: `1px solid ${C.borderMd}` }}>
                  <span style={{ fontFamily: F.mono, fontSize: 10, letterSpacing: '0.08em', color: C.textFaint, textTransform: 'uppercase' }}>Recent deployments</span>
                </div>
                <div style={{ padding: '16px 20px 12px' }}>
                  {deployments
                    .slice().sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
                    .slice(0, 4)
                    .map((d, i, arr) => {
                      const isLast = i === arr.length - 1;
                      const dStatus = mapDeploymentStatus(d) as DeployedAgentStatus;
                      const dStyle = STATUS_STYLE[dStatus];
                      return (
                        <div key={d.id} onClick={() => navigate(deploymentPath(account, d.id))}
                          style={{ display: 'flex', gap: 14, position: 'relative', cursor: 'pointer', borderRadius: 8, margin: '0 -8px', padding: '0 8px', transition: 'background 0.12s' }}
                          onMouseEnter={e => { e.currentTarget.style.background = C.canvas; }}
                          onMouseLeave={e => { e.currentTarget.style.background = 'transparent'; }}
                        >
                          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', flexShrink: 0, paddingTop: 3 }}>
                            <div style={{ width: 8, height: 8, borderRadius: '50%', background: dStyle.dot, flexShrink: 0 }} />
                            {!isLast && <div style={{ width: 1, flex: 1, background: C.stone300, marginTop: 4, minHeight: 24 }} />}
                          </div>
                          <div style={{ paddingBottom: isLast ? 0 : 20, minWidth: 0 }}>
                            <p style={{ fontFamily: F.mono, fontSize: 10, color: C.textFaint, marginBottom: 3 }}>{timeAgo(d.created_at)}</p>
                            <p style={{ fontFamily: F.sans, fontSize: 13, fontWeight: 600, color: C.text, lineHeight: 1.4, marginBottom: 2, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{d.display_name || d.name}</p>
                            <p style={{ fontFamily: F.sans, fontSize: 12, color: dStyle.text, lineHeight: 1.45 }}>{deploymentStatusLabel[dStatus]}</p>
                          </div>
                        </div>
                      );
                    })}
                </div>
              </div>
            )}

            {/* discover agents */}
            <div style={{ background: 'linear-gradient(160deg, var(--color-teal-50) 0%, var(--color-stone-100) 60%)', border: `1px solid ${C.borderMd}`, borderRadius: 14, padding: '24px 20px 20px', display: 'flex', flexDirection: 'column', alignItems: 'center', textAlign: 'center' }}>
              <div style={{ width: 44, height: 44, borderRadius: '50%', background: C.surface, border: `1px solid ${C.borderMd}`, display: 'flex', alignItems: 'center', justifyContent: 'center', marginBottom: 14 }}>
                <span style={{ fontSize: 22, lineHeight: 1, color: C.text, fontWeight: 300 }}>+</span>
              </div>
              <p style={{ fontFamily: F.sans, fontSize: 14, fontWeight: 700, color: C.text, marginBottom: 6 }}>Discover more agents</p>
              <p style={{ fontFamily: F.sans, fontSize: 12, color: C.textMuted, lineHeight: 1.55, marginBottom: 16 }}>
                Explore and install agents built by the community.
              </p>
              <button onClick={() => navigate('/browse')} style={{ width: '100%', padding: '10px 0', borderRadius: 10, background: C.teal, border: 'none', fontFamily: F.sans, fontSize: 13, fontWeight: 600, color: 'var(--color-stone-100)', cursor: 'pointer', transition: 'opacity 0.15s' }}
                onMouseEnter={e => { e.currentTarget.style.opacity = '0.85'; }}
                onMouseLeave={e => { e.currentTarget.style.opacity = '1'; }}
              >
                Explore agents
              </button>
            </div>

          </div>
        </main>
      </div>
    </div>
  );
}

export default function YourAgents() {
  return (
    <ProtectedRoute>
      <YourAgentsContent />
    </ProtectedRoute>
  );
}
