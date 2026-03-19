import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router";
import { ArrowRight } from "lucide-react";
import { AgentCard } from "@/components/AgentCard";
import { deploymentPath } from "@/lib/routes";
import type { AgentDeployment } from "@/lib/api";

interface LiveRevealOverlayProps {
  deployment: AgentDeployment;
  account: string;
  onComplete: () => void;
}

function LiveRevealConfetti({ canvasRef }: { canvasRef: React.RefObject<HTMLCanvasElement | null> }) {
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    canvas.width  = window.innerWidth;
    canvas.height = window.innerHeight;

    const COLORS = ['#15827d', '#57c4c1', '#D48F1E', '#F0816A', '#073d3c', '#c4b89e', '#2d7a4f'];
    const pieces: { x: number; y: number; vx: number; vy: number; rot: number; vr: number; w: number; h: number; color: string; shape: 'rect' | 'circle' }[] = [];

    for (let i = 0; i < 120; i++) {
      pieces.push({
        x: Math.random() * canvas.width,
        y: -10 - Math.random() * 200,
        vx: (Math.random() - 0.5) * 3,
        vy: 2 + Math.random() * 4,
        rot: Math.random() * Math.PI * 2,
        vr: (Math.random() - 0.5) * 0.15,
        w: 6 + Math.random() * 8,
        h: 4 + Math.random() * 6,
        color: COLORS[Math.floor(Math.random() * COLORS.length)],
        shape: Math.random() > 0.5 ? 'rect' : 'circle',
      });
    }

    let raf: number;
    const draw = () => {
      ctx.clearRect(0, 0, canvas.width, canvas.height);
      let alive = false;
      for (const p of pieces) {
        p.x   += p.vx;
        p.y   += p.vy;
        p.rot += p.vr;
        p.vy  += 0.05;
        if (p.y < canvas.height + 20) alive = true;
        ctx.save();
        ctx.translate(p.x, p.y);
        ctx.rotate(p.rot);
        ctx.fillStyle = p.color;
        ctx.globalAlpha = Math.max(0, 1 - p.y / canvas.height);
        if (p.shape === 'circle') {
          ctx.beginPath(); ctx.arc(0, 0, p.w / 2, 0, Math.PI * 2); ctx.fill();
        } else {
          ctx.fillRect(-p.w / 2, -p.h / 2, p.w, p.h);
        }
        ctx.restore();
      }
      if (alive) raf = requestAnimationFrame(draw);
    };

    const t = setTimeout(() => { raf = requestAnimationFrame(draw); }, 400);
    return () => { clearTimeout(t); cancelAnimationFrame(raf); };
  }, [canvasRef]);

  return null;
}

export function LiveRevealOverlay({ deployment, account, onComplete }: LiveRevealOverlayProps) {
  const navigate = useNavigate();
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [visible, setVisible] = useState(false);
  const [cardVisible, setCardVisible] = useState(false);
  const [textVisible, setTextVisible] = useState(false);

  useEffect(() => {
    setVisible(true);
    const cardTimer = setTimeout(() => setCardVisible(true), 120);
    const textTimer = setTimeout(() => setTextVisible(true), 600);
    return () => {
      clearTimeout(cardTimer);
      clearTimeout(textTimer);
    };
  }, []);

  const displayName = deployment.display_name || deployment.name;

  const handleViewMonitoring = () => {
    onComplete();
    navigate(deploymentPath(account, deployment.id));
  };

  const handleShareBadge = () => {
    onComplete();
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center overflow-hidden"
      style={{ opacity: visible ? 1 : 0, transition: "opacity 200ms ease", background: '#ede7d9' }}
    >
      <canvas ref={canvasRef} style={{ position: 'absolute', inset: 0, pointerEvents: 'none', zIndex: 0 }} />
      <LiveRevealConfetti canvasRef={canvasRef} />

      {/* Radial teal glow */}
      <div style={{ position: 'absolute', width: 600, height: 700, borderRadius: '50%', background: 'radial-gradient(ellipse, rgba(21,130,125,0.18) 0%, rgba(7,61,60,0.06) 50%, transparent 70%)', pointerEvents: 'none' }} />

      <div
        className="relative z-10 grid grid-cols-2 items-center w-full"
        style={{ maxWidth: 960, padding: '0 60px', gap: 80 }}
      >
        {/* Left: text + CTAs */}
        <div
          style={{
            opacity: textVisible ? 1 : 0,
            transform: textVisible ? 'none' : 'translateY(16px)',
            transition: 'opacity 0.7s cubic-bezier(0.16,1,0.3,1), transform 0.7s cubic-bezier(0.16,1,0.3,1)',
          }}
        >
          {/* Dot + label */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 20 }}>
            <span className="animate-pulse" style={{ width: 7, height: 7, borderRadius: '50%', background: '#15827d', display: 'inline-block', boxShadow: '0 0 0 4px rgba(21,130,125,0.15)' }} />
            <span style={{ fontFamily: "'Geist Mono', 'Space Mono', monospace", fontSize: 10, letterSpacing: '0.18em', color: '#15827d' }}>AGENT LIVE</span>
          </div>
          {/* Heading */}
          <div style={{ fontFamily: "'Geist', 'Inter', sans-serif", fontSize: 48, fontWeight: 700, letterSpacing: '-0.025em', color: '#073d3c', lineHeight: 1.05, marginBottom: 8 }}>
            <div>Your agent</div>
            <div>is ready.</div>
          </div>
          {/* Subtitle */}
          <p style={{ fontFamily: "'Geist Mono', 'Space Mono', monospace", fontSize: 12, letterSpacing: '0.04em', color: '#6b7e7c', lineHeight: 2, marginBottom: 40 }}>
            {displayName} is online.<br />Monitoring begins on first request.
          </p>
          {/* Buttons */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
            <button
              onClick={handleViewMonitoring}
              style={{
                display: 'inline-flex', alignItems: 'center', justifyContent: 'space-between',
                height: 40, padding: '0 20px', borderRadius: 8, border: 'none', cursor: 'pointer',
                background: '#073d3c', color: '#ede7d9',
                fontFamily: "'Geist', 'Inter', sans-serif", fontSize: 14, letterSpacing: '-0.01em', fontWeight: 600,
                transition: 'background 0.15s ease',
              }}
              onMouseEnter={e => { e.currentTarget.style.background = '#15827d'; }}
              onMouseLeave={e => { e.currentTarget.style.background = '#073d3c'; }}
            >
              <span>View Monitoring</span>
              <ArrowRight size={16} strokeWidth={2} />
            </button>
            <button
              onClick={handleShareBadge}
              style={{
                display: 'inline-flex', alignItems: 'center', justifyContent: 'space-between',
                height: 40, padding: '0 14px', borderRadius: 8, cursor: 'pointer',
                background: 'transparent', border: '1px solid #c4b89e', color: '#0d1f1e',
                fontFamily: "'Geist', 'Inter', sans-serif", fontSize: 14, letterSpacing: '-0.01em', fontWeight: 400,
                transition: 'background 0.15s ease, border-color 0.15s ease',
              }}
              onMouseEnter={e => { e.currentTarget.style.background = '#d8d0c0'; e.currentTarget.style.borderColor = '#9a8a72'; }}
              onMouseLeave={e => { e.currentTarget.style.background = 'transparent'; e.currentTarget.style.borderColor = '#c4b89e'; }}
            >
              <span>Share badge</span>
              <ArrowRight size={16} strokeWidth={2} />
            </button>
          </div>
        </div>

        {/* Right: agent card */}
        <div
          style={{
            display: 'flex', justifyContent: 'center',
            opacity: cardVisible ? 1 : 0,
            transform: cardVisible ? 'translateY(0) scale(1)' : 'translateY(24px) scale(0.96)',
            transition: 'opacity 0.8s cubic-bezier(0.16,1,0.3,1), transform 0.8s cubic-bezier(0.16,1,0.3,1)',
          }}
        >
          <AgentCard
            variant="user"
            account={account}
            name={deployment.name}
            displayName={displayName}
            tier="proven"
            deploymentId={deployment.id}
            capabilities={deployment.components}
          />
        </div>
      </div>
    </div>
  );
}
