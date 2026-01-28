import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const target = env.VITE_API_PROXY_TARGET || 'http://localhost:8081'
  const proxyPaths = ['/api', '/analytics', '/ingest', '/healthz', '/metrics']

  return {
    plugins: [react()],
    server: {
      proxy: Object.fromEntries(
        proxyPaths.map((path) => [path, { target, changeOrigin: true }])
      )
    }
  }
})
