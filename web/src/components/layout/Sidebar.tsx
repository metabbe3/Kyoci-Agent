import { NavLink, useLocation } from "react-router-dom";
import { motion } from "motion/react";
import {
  LayoutDashboard,
  MessageSquare,
  Users,
  Plug,
  Sparkles,
  Boxes,
  Cpu,
  Activity,
  Settings,
  CircuitBoard,
  ChevronRight,
  Waypoints,
  Wand2,
} from "lucide-react";
import { useEffect, useState } from "react";
import { api, health } from "@/lib/api";
import { cn } from "@/lib/utils";
import { springs } from "@/lib/motion";

type BackendState = "checking" | "online" | "offline";

const navItems = [
  { to: "/overview", label: "Overview", icon: LayoutDashboard, hint: "Mission control" },
  { to: "/chat", label: "Chat", icon: MessageSquare, hint: "Talk to agents" },
  { to: "/activity", label: "Live Activity", icon: Waypoints, hint: "Agent runs in progress" },
  { to: "/agents", label: "Agents", icon: Users, hint: "Role browser" },
  { to: "/providers", label: "Providers", icon: Plug, hint: "LLM gateways" },
  { to: "/skills", label: "Skills", icon: Sparkles, hint: "Zero-AI shortcuts" },
  { to: "/skill-maker", label: "Skill Maker", icon: Wand2, hint: "Create custom skills" },
  { to: "/mcp", label: "MCP Manager", icon: Plug, hint: "Install tool servers" },
  { to: "/models", label: "Models", icon: Boxes, hint: "Catalog" },
  { to: "/hardware", label: "Hardware", icon: Cpu, hint: "Local fit" },
  { to: "/status", label: "Status", icon: Activity, hint: "Live telemetry" },
  { to: "/settings", label: "Settings", icon: Settings, hint: "Configure" },
];

export function Sidebar() {
  const location = useLocation();
  const [activeCount, setActiveCount] = useState<number | null>(null);
  const [total, setTotal] = useState(20);
  const [backend, setBackend] = useState<BackendState>("checking");
  const [mobileOpen, setMobileOpen] = useState(false);

  useEffect(() => {
    let cancelled = false;
    const pollHealth = () => {
      health()
        .then(() => !cancelled && setBackend("online"))
        .catch(() => !cancelled && setBackend("offline"));
    };
    pollHealth();
    const healthId = setInterval(pollHealth, 10000);

    const refreshProviders = () => {
      api.providers()
        .then((r) => {
          if (cancelled) return;
          setActiveCount(r.providers.filter((p) => p.available).length);
          setTotal(r.providers.length);
        })
        .catch(() => {});
    };
    refreshProviders();
    const provId = setInterval(refreshProviders, 15000);

    return () => {
      cancelled = true;
      clearInterval(healthId);
      clearInterval(provId);
    };
  }, []);

  // Close mobile drawer on route change
  useEffect(() => {
    setMobileOpen(false);
  }, [location.pathname]);

  const statusColor =
    backend === "online" ? "var(--color-lime)" :
    backend === "offline" ? "var(--color-coral)" :
    "var(--color-ink-faint)";
  const statusLabel =
    backend === "online" ? "All systems nominal" :
    backend === "offline" ? "Backend offline — run `go run ./cmd/server`" :
    "Checking backend…";

  return (
    <>
      {/* Mobile menu trigger — only visible < lg */}
      <button
        type="button"
        aria-label="Open navigation"
        onClick={() => setMobileOpen((v) => !v)}
        className="fixed left-4 top-4 z-[60] lg:hidden glass-strong h-11 w-11 rounded-full grid place-items-center ring-glow"
      >
        <ChevronRight
          className={cn("h-4 w-4 transition-transform", mobileOpen && "rotate-180")}
          style={{ color: "var(--color-lime)" }}
        />
      </button>

      {/* Mobile backdrop */}
      {mobileOpen && (
        <div
          aria-hidden
          onClick={() => setMobileOpen(false)}
          className="fixed inset-0 z-40 bg-black/50 backdrop-blur-sm lg:hidden"
        />
      )}

      <aside
        className={cn(
          "fixed left-0 top-0 z-50 h-screen w-[240px] glass-panel flex flex-col",
          "transition-transform duration-300 ease-out",
          mobileOpen ? "translate-x-0" : "-translate-x-full lg:translate-x-0"
        )}
      >
        {/* Brand */}
        <div className="px-5 pt-6 pb-5 border-b border-white/5">
          <div className="flex items-center gap-2.5">
            <div className="relative h-9 w-9 grid place-items-center rounded-xl glass-strong">
              <CircuitBoard className="h-4 w-4" style={{ color: "var(--color-lime)" }} />
              <span
                className="absolute -right-0.5 -top-0.5 h-2 w-2 rounded-full"
                style={{
                  background: statusColor,
                  animation: backend === "online" ? "pulse-dot 2s ease-in-out infinite" : undefined,
                }}
              />
            </div>
            <div className="flex flex-col leading-tight">
              <span
                className="text-[1.45rem] font-semibold tracking-tight"
                style={{ fontFamily: "var(--font-display)" }}
              >
                KYOCI
              </span>
              <span className="text-[10px] uppercase tracking-[0.18em] text-[var(--color-ink-faint)]">
                Agent v5
              </span>
            </div>
          </div>
        </div>

        {/* Nav */}
        <nav className="flex-1 px-3 py-4 space-y-0.5 overflow-y-auto">
          {navItems.map(({ to, label, icon: Icon }) => {
            const active = location.pathname === to;
            return (
              <NavLink key={to} to={to} className="block" data-cursor="hover">
                <div
                  className={cn(
                    "relative flex items-center gap-3 rounded-xl px-3 py-3 text-[15px] transition-colors",
                    active
                      ? "text-[var(--color-lime)]"
                      : "text-[var(--color-ink-muted)] hover:text-[var(--color-ink)]"
                  )}
                >
                  {active && (
                    <motion.span
                      layoutId="nav-active"
                      transition={springs.snappy}
                      className="absolute inset-0 rounded-xl"
                      style={{
                        background: "rgba(198, 244, 50, 0.10)",
                        border: "1px solid rgba(198, 244, 50, 0.25)",
                        boxShadow: "0 0 30px -8px rgba(198, 244, 50, 0.25)",
                      }}
                    />
                  )}
                  <Icon className="relative h-[18px] w-[18px] shrink-0" />
                  <span className="relative font-medium">{label}</span>
                  {active && (
                    <motion.span
                      layoutId="nav-dot"
                      transition={springs.snappy}
                      className="relative ml-auto h-1.5 w-1.5 rounded-full"
                      style={{ background: "var(--color-lime)" }}
                    />
                  )}
                </div>
              </NavLink>
            );
          })}
        </nav>

        {/* Footer — provider count + status */}
        <div className="px-4 py-4 border-t border-white/5 space-y-2">
          <div className="flex items-center justify-between text-xs">
            <span className="text-[var(--color-ink-muted)]">Providers</span>
            <span
              className="tabular font-mono"
              style={{ color: activeCount ? "var(--color-lime)" : "var(--color-ink)" }}
            >
              {activeCount ?? "—"} / {total}
            </span>
          </div>
          <div className="flex items-center gap-2 text-xs text-[var(--color-ink-faint)]">
            <span
              className="h-1.5 w-1.5 rounded-full"
              style={{ background: statusColor }}
            />
            <span className="truncate" title={statusLabel}>
              {statusLabel}
            </span>
          </div>
        </div>
      </aside>
    </>
  );
}
