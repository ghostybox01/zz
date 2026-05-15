import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { execSync } from 'node:child_process'

const RECON_BACKEND = process.env.RECONX_BACKEND ?? 'http://localhost:5000'
const RECON_REPO    = process.env.RECONX_REPO    ?? '' // e.g. "myuser/reconx"

function gitSha(): string {
  try {
    return execSync('git rev-parse HEAD', { stdio: ['ignore', 'pipe', 'ignore'] })
      .toString()
      .trim()
      .slice(0, 12)
  } catch {
    return 'dev'
  }
}

export default defineConfig({
  plugins: [react()],
  define: {
    __RECONX_BUILD_SHA__: JSON.stringify(gitSha()),
    __RECONX_BUILD_AT__:  JSON.stringify(new Date().toISOString()),
    __RECONX_REPO__:      JSON.stringify(RECON_REPO),
  },
  server: {
    proxy: {
      '/api':      { target: RECON_BACKEND, changeOrigin: true },
      '/socket.io': { target: RECON_BACKEND, changeOrigin: true, ws: true },
    },
  },
})
