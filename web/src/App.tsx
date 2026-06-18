import { Routes, Route, Navigate, useLocation } from "react-router-dom";
import { AnimatePresence, motion } from "motion/react";
import { Sidebar } from "@/components/layout/Sidebar";
import { PanelBoundary } from "@/components/PanelBoundary";
import { OverviewPanel } from "@/components/panels/OverviewPanel";
import { ChatPanel } from "@/components/panels/ChatPanel";
import { AgentsPanel } from "@/components/panels/AgentsPanel";
import { ProvidersPanel } from "@/components/panels/ProvidersPanel";
import { SkillsPanel } from "@/components/panels/SkillsPanel";
import { ModelsPanel } from "@/components/panels/ModelsPanel";
import { HardwarePanel } from "@/components/panels/HardwarePanel";
import { StatusPanel } from "@/components/panels/StatusPanel";
import { SettingsPanel } from "@/components/panels/SettingsPanel";
import { ActivityPanel } from "@/components/panels/ActivityPanel";
import { pageVariants } from "@/lib/motion";

/** Wrap a panel element in a per-route boundary so one crash doesn't blank the app. */
function panel(name: string, el: React.ReactNode) {
  return <PanelBoundary name={name}>{el}</PanelBoundary>;
}

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
          <Route path="/chat" element={panel("Chat", <ChatPanel />)} />
          <Route path="/overview" element={panel("Overview", <OverviewPanel />)} />
          <Route path="/agents" element={panel("Agents", <AgentsPanel />)} />
          <Route path="/activity" element={panel("Live Activity", <ActivityPanel />)} />
          <Route path="/providers" element={panel("Providers", <ProvidersPanel />)} />
          <Route path="/skills" element={panel("Skills", <SkillsPanel />)} />
          <Route path="/models" element={panel("Models", <ModelsPanel />)} />
          <Route path="/hardware" element={panel("Hardware", <HardwarePanel />)} />
          <Route path="/status" element={panel("Status", <StatusPanel />)} />
          <Route path="/settings" element={panel("Settings", <SettingsPanel />)} />
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
