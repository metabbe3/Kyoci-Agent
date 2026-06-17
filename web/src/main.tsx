import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { MotionConfig } from "motion/react";
import { Toaster } from "sonner";
import App from "./App";
import { MeshBackground } from "@/components/effects/MeshBackground";
import { GrainOverlay } from "@/components/effects/GrainOverlay";
import "./index.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <MotionConfig reducedMotion="user">
        <MeshBackground />
        <GrainOverlay />
        <App />
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
    </BrowserRouter>
  </StrictMode>
);
