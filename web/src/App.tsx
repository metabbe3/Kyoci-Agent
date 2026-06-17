import { Routes, Route, Navigate, useLocation } from "react-router-dom";
import { AnimatePresence, motion } from "motion/react";
import { Sidebar } from "@/components/layout/Sidebar";
import { OverviewPanel } from "@/components/panels/OverviewPanel";
import { ChatPanel } from "@/components/panels/ChatPanel";
import { AgentsPanel } from "@/components/panels/AgentsPanel";
import { ProvidersPanel } from "@/components/panels/ProvidersPanel";
import { SkillsPanel } from "@/components/panels/SkillsPanel";
import { ModelsPanel } from "@/components/panels/ModelsPanel";
import { HardwarePanel } from "@/components/panels/HardwarePanel";
import { StatusPanel } from "@/components/panels/StatusPanel";
import { SettingsPanel } from "@/components/panels/SettingsPanel";
import { pageVariants } from "@/lib/motion";

function AnimatedRoutes() {
  const location = useLocation();
  return (
    <AnimatePresence mode="wait">
      <motion.div
        key={location.pathname}
        variants={pageVariants}
        initial="initial"
        animate="animate"
        exit="exit"
        className="min-h-screen"
      >
        <Routes location={location}>
          <Route path="/" element={<Navigate to="/overview" replace />} />
          <Route path="/chat" element={<ChatPanel />} />
          <Route path="/overview" element={<OverviewPanel />} />
          <Route path="/agents" element={<AgentsPanel />} />
          <Route path="/providers" element={<ProvidersPanel />} />
          <Route path="/skills" element={<SkillsPanel />} />
          <Route path="/models" element={<ModelsPanel />} />
          <Route path="/hardware" element={<HardwarePanel />} />
          <Route path="/status" element={<StatusPanel />} />
          <Route path="/settings" element={<SettingsPanel />} />
          <Route path="*" element={<Navigate to="/overview" replace />} />
        </Routes>
      </motion.div>
    </AnimatePresence>
  );
}

export default function App() {
  return (
    <div className="relative min-h-screen">
      <Sidebar />
      {/* Main content offset by sidebar width. Sidebar is fixed. */}
      <div className="lg:pl-[248px]">
        <AnimatedRoutes />
      </div>
    </div>
  );
}
