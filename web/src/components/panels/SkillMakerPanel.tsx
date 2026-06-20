import { useState, useEffect } from "react";
import { motion } from "motion/react";
import { Wand2, Save, Trash2, FileText } from "lucide-react";
import { TopBar } from "@/components/layout/TopBar";
import { Badge } from "@/components/ui/badge";

type CustomSkill = { name: string; description: string; category: string; source_path: string };

export function SkillMakerPanel() {
  const [skills, setSkills] = useState<CustomSkill[]>([]);
  const [name, setName] = useState("");
  const [desc, setDesc] = useState("");
  const [category, setCategory] = useState("custom");
  const [keywords, setKeywords] = useState("");
  const [body, setBody] = useState("");
  const [msg, setMsg] = useState("");

  const load = () => {
    fetch("/api/dashboard/skills/custom").then((r) => r.json()).then((d) => setSkills(d.skills || [])).catch(() => {});
  };
  useEffect(() => { load(); }, []);

  const preview = () => {
    const kw = keywords.split(",").map((k) => k.trim()).filter(Boolean);
    const kwLines = kw.map((k) => `      - ${k}`).join("\n");
    return `---\nname: ${name || "my-skill"}\ndescription: ${desc}\ncategory: ${category}\ntriggers:\n    keywords:\n${kwLines || "      []"}\n    regex: []\npriority: normal\n---\n\n${body}`;
  };

  const save = async () => {
    if (!name.trim() || !body.trim()) { setMsg("Name and body are required"); setTimeout(() => setMsg(""), 3000); return; }
    const res = await fetch("/api/dashboard/skills/create", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name, description: desc, category, keywords, body }),
    });
    if (res.ok) {
      setMsg(`✓ Skill "${name}" created! It'll be active after restart.`);
      setName(""); setDesc(""); setKeywords(""); setBody("");
      load();
    } else { setMsg("Failed to create skill"); }
    setTimeout(() => setMsg(""), 4000);
  };

  const del = async (skillName: string) => {
    await fetch(`/api/dashboard/skills/delete?name=${skillName}`, { method: "DELETE" });
    setSkills((prev) => prev.filter((s) => s.name !== skillName));
    setMsg(`✓ "${skillName}" deleted`);
    setTimeout(() => setMsg(""), 3000);
  };

  return (
    <>
      <TopBar eyebrow="Create" title="Skill Maker" subtitle="Build custom prompt-skills the agent auto-loads" />
      <motion.div className="px-6 py-6 max-w-4xl">
        {msg && <div className="mb-4 rounded-xl border border-[var(--color-lime)]/30 bg-[var(--color-lime)]/5 px-4 py-2 text-sm text-[var(--color-lime)]">{msg}</div>}

        {/* Existing skills */}
        <div className="mb-6">
          <h3 className="text-sm font-medium mb-2 opacity-70">Custom Skills ({skills.length})</h3>
          <div className="space-y-1">
            {skills.map((s) => (
              <div key={s.name} className="flex items-center gap-3 rounded-lg border border-white/5 bg-white/[0.02] px-3 py-2">
                <FileText className="h-4 w-4 opacity-40" />
                <div className="flex-1 min-w-0">
                  <span className="text-sm font-medium">{s.name}</span>
                  {s.description && <span className="text-xs opacity-50 ml-2">{s.description}</span>}
                </div>
                <span className="text-[10px]">{s.category}</span>
                <button onClick={() => del(s.name)} className="rounded p-1 hover:bg-red-500/10"><Trash2 className="h-3.5 w-3.5 text-red-400/60" /></button>
              </div>
            ))}
            {skills.length === 0 && <div className="text-sm opacity-40 py-4 text-center">No custom skills yet</div>}
          </div>
        </div>

        {/* Create form */}
        <div className="rounded-xl border border-white/10 bg-white/[0.02] p-5 space-y-4">
          <h3 className="text-sm font-medium flex items-center gap-2"><Wand2 className="h-4 w-4 text-[var(--color-lime)]" /> Create New Skill</h3>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-xs opacity-60 mb-1 block">Name (slug)</label>
              <input className="w-full rounded-lg bg-white/5 border border-white/10 px-3 py-2 text-sm" placeholder="my-workflow" value={name} onChange={(e) => setName(e.target.value)} />
            </div>
            <div>
              <label className="text-xs opacity-60 mb-1 block">Category</label>
              <input className="w-full rounded-lg bg-white/5 border border-white/10 px-3 py-2 text-sm" placeholder="custom" value={category} onChange={(e) => setCategory(e.target.value)} />
            </div>
          </div>
          <div>
            <label className="text-xs opacity-60 mb-1 block">Description</label>
            <input className="w-full rounded-lg bg-white/5 border border-white/10 px-3 py-2 text-sm" placeholder="What this skill does" value={desc} onChange={(e) => setDesc(e.target.value)} />
          </div>
          <div>
            <label className="text-xs opacity-60 mb-1 block">Trigger Keywords (comma-separated)</label>
            <input className="w-full rounded-lg bg-white/5 border border-white/10 px-3 py-2 text-sm" placeholder="deploy, ship, release" value={keywords} onChange={(e) => setKeywords(e.target.value)} />
          </div>
          <div>
            <label className="text-xs opacity-60 mb-1 block">Skill Body (Markdown)</label>
            <textarea className="w-full rounded-lg bg-white/5 border border-white/10 px-3 py-2 text-sm font-mono" rows={8} placeholder={"# My Workflow\n\nStep 1: ...\nStep 2: ..."} value={body} onChange={(e) => setBody(e.target.value)} />
          </div>

          {/* Preview */}
          {(name || body) && (
            <div>
              <label className="text-xs opacity-60 mb-1 block">Preview (saved as data/skills/{name || "my-skill"}.md)</label>
              <pre className="rounded-lg bg-black/30 border border-white/5 p-3 text-xs font-mono overflow-x-auto max-h-48 overflow-y-auto whitespace-pre-wrap">{preview()}</pre>
            </div>
          )}

          <button onClick={save} className="inline-flex items-center gap-2 rounded-lg bg-[var(--color-lime)]/20 px-4 py-2 text-sm font-medium text-[var(--color-lime)] hover:bg-[var(--color-lime)]/30">
            <Save className="h-4 w-4" /> Save Skill
          </button>
        </div>
      </motion.div>
    </>
  );
}
