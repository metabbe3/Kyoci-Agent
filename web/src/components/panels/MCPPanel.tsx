import { useState, useEffect } from "react";
import { motion } from "motion/react";
import { Plug, Plus, Trash2, Power } from "lucide-react";
import { TopBar } from "@/components/layout/TopBar";
import { Badge } from "@/components/ui/badge";
import { springs } from "@/lib/motion";

type MCPServer = {
  name: string;
  enabled: boolean;
  command: string;
  args: string[];
  env?: Record<string, string>;
};

export function MCPPanel() {
  const [servers, setServers] = useState<MCPServer[]>([]);
  const [showAdd, setShowAdd] = useState(false);
  const [newName, setNewName] = useState("");
  const [newCommand, setNewCommand] = useState("npx");
  const [newArgs, setNewArgs] = useState("-y @modelcontextprotocol/server-github");
  const [newEnv, setNewEnv] = useState("");
  const [msg, setMsg] = useState("");

  useEffect(() => {
    fetch("/api/dashboard/mcp/servers")
      .then((r) => r.json())
      .then((d) => setServers(d.servers || []))
      .catch(() => {});
  }, []);

  const toggle = async (name: string, enabled: boolean) => {
    const srv = servers.find((s) => s.name === name);
    if (!srv) return;
    await fetch("/api/dashboard/mcp/server", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ...srv, enabled: !enabled }),
    });
    setServers((prev) => prev.map((s) => s.name === name ? { ...s, enabled: !enabled } : s));
    setMsg(`${name} ${!enabled ? "enabled" : "disabled"} — restart to apply`);
    setTimeout(() => setMsg(""), 4000);
  };

  const remove = async (name: string) => {
    await fetch(`/api/dashboard/mcp/server?name=${name}`, { method: "DELETE" });
    setServers((prev) => prev.filter((s) => s.name !== name));
    setMsg(`${name} deleted — restart to apply`);
    setTimeout(() => setMsg(""), 4000);
  };

  const add = async () => {
    if (!newName.trim()) return;
    const args = newArgs.split(" ").filter(Boolean);
    const env: Record<string, string> = {};
    if (newEnv.trim()) {
      newEnv.split("\n").forEach((line) => {
        const [k, v] = line.split("=");
        if (k && v) env[k.trim()] = v.trim();
      });
    }
    await fetch("/api/dashboard/mcp/server", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name: newName, command: newCommand, args, env, enabled: true }),
    });
    setServers((prev) => [...prev, { name: newName, enabled: true, command: newCommand, args, env }]);
    setNewName(""); setNewArgs("-y @modelcontextprotocol/server-github"); setNewEnv("");
    setShowAdd(false);
    setMsg(`${newName} added — restart to apply`);
    setTimeout(() => setMsg(""), 4000);
  };

  return (
    <>
      <TopBar eyebrow="MCP" title="Server Manager" subtitle="Install, toggle, and remove MCP tool servers" />
      <motion.div className="px-6 py-6 max-w-4xl">
        {msg && (
          <div className="mb-4 rounded-xl border border-[var(--color-lime)]/30 bg-[var(--color-lime)]/5 px-4 py-2 text-sm text-[var(--color-lime)]">
            {msg}
          </div>
        )}
        <div className="flex items-center justify-between mb-4">
          <div className="text-sm opacity-60">{servers.length} MCP server(s) configured</div>
          <button
            onClick={() => setShowAdd(!showAdd)}
            className="inline-flex items-center gap-2 rounded-lg border border-white/10 bg-white/5 px-3 py-1.5 text-sm hover:bg-white/10"
          >
            <Plus className="h-4 w-4" /> Add Server
          </button>
        </div>

        {showAdd && (
          <motion.div initial={{ opacity: 0, height: 0 }} animate={{ opacity: 1, height: "auto" }} className="mb-4 rounded-xl border border-white/10 bg-white/[0.02] p-4 space-y-3">
            <input className="w-full rounded-lg bg-white/5 border border-white/10 px-3 py-2 text-sm" placeholder="Server name (e.g. github)" value={newName} onChange={(e) => setNewName(e.target.value)} />
            <input className="w-full rounded-lg bg-white/5 border border-white/10 px-3 py-2 text-sm" placeholder="Command (e.g. npx)" value={newCommand} onChange={(e) => setNewCommand(e.target.value)} />
            <input className="w-full rounded-lg bg-white/5 border border-white/10 px-3 py-2 text-sm font-mono" placeholder="Args (space-separated)" value={newArgs} onChange={(e) => setNewArgs(e.target.value)} />
            <textarea className="w-full rounded-lg bg-white/5 border border-white/10 px-3 py-2 text-sm font-mono" placeholder="Env vars (one per line: KEY=value)" rows={2} value={newEnv} onChange={(e) => setNewEnv(e.target.value)} />
            <button onClick={add} className="rounded-lg bg-[var(--color-lime)]/20 px-4 py-2 text-sm font-medium text-[var(--color-lime)] hover:bg-[var(--color-lime)]/30">Save & Add</button>
          </motion.div>
        )}

        <div className="space-y-2">
          {servers.map((srv) => (
            <motion.div key={srv.name} initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="flex items-center gap-3 rounded-xl border border-white/10 bg-white/[0.02] p-4">
              <div className={`flex h-10 w-10 items-center justify-center rounded-lg ${srv.enabled ? "bg-[var(--color-lime)]/10 text-[var(--color-lime)]" : "bg-white/5 text-white/30"}`}>
                <Plug className="h-5 w-5" />
              </div>
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <span className="font-medium text-sm">{srv.name}</span>
                  <Badge variant={srv.enabled ? "default" : "secondary"} className="text-[10px]">{srv.enabled ? "ON" : "OFF"}</Badge>
                </div>
                <div className="text-xs opacity-50 font-mono truncate">{srv.command} {srv.args.join(" ")}</div>
              </div>
              <button onClick={() => toggle(srv.name, srv.enabled)} className="rounded-lg p-2 hover:bg-white/10" title="Toggle">
                <Power className={`h-4 w-4 ${srv.enabled ? "text-[var(--color-lime)]" : "text-white/30"}`} />
              </button>
              <button onClick={() => remove(srv.name)} className="rounded-lg p-2 hover:bg-red-500/10" title="Delete">
                <Trash2 className="h-4 w-4 text-red-400/60" />
              </button>
            </motion.div>
          ))}
          {servers.length === 0 && (
            <div className="rounded-xl border border-white/10 bg-white/[0.02] p-8 text-center text-sm opacity-50">
              No MCP servers configured. Click "Add Server" to install one.
            </div>
          )}
        </div>
      </motion.div>
    </>
  );
}
