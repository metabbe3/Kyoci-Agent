import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Vite configuration for the System Monitor Dashboard frontend.
// - React plugin enables JSX / Fast Refresh during development.
// - Dev server proxies /api requests to the local backend on port 3001,
//   keeping the browser origin clean and avoiding CORS during local dev.
// - Build output is emitted to dist/ for static serving.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:3001',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: true,
  },
})
