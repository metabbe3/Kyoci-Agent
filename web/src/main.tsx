import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { MotionConfig } from "motion/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { Toaster } from "sonner";
import App from "./App";
import { ErrorBoundary } from "@/components/ErrorBoundary";
import { MeshBackground } from "@/components/effects/MeshBackground";
import { GrainOverlay } from "@/components/effects/GrainOverlay";
import { queryClient } from "@/lib/queryClient";
import { installObservers } from "@/lib/observability";
import "./index.css";

// Mirror uncaught errors/rejections to the dev console; no-op in prod until a
// real sink (Sentry/OTel) is wired through installObservers(reporter).
installObservers();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <QueryClientProvider client={queryClient}>
        <MotionConfig reducedMotion="user">
          <MeshBackground />
          <GrainOverlay />
          <ErrorBoundary>
            <App />
          </ErrorBoundary>
        <Toaster
          position="bottom-right"
          toastOptions={{
            style: {
              background: "rgba(20, 23, 31, 0.85)",
              backdropFilter: "blur(12px)",
              border: "1px solid rgba(255,255,255,0.1)",
              color: "#f5f6f8",
            },
          }}
        />
      </MotionConfig>
      </QueryClientProvider>
    </BrowserRouter>
  </StrictMode>
);
