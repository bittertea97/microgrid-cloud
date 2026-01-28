# Web Frontend (Vite + React + TS + Antd)

## Getting started

```bash
cd web
pnpm install
pnpm dev
```

The dev server runs on Vite's default port (usually `5173`).

## Proxy & backend

The Vite dev server proxies API traffic to the backend so the frontend can call relative paths.

Proxied paths:
- `/api`
- `/analytics`
- `/ingest`
- `/healthz`
- `/metrics`

Default backend target: `http://localhost:8081`

Override the target with:

```bash
VITE_API_PROXY_TARGET=http://localhost:8081 pnpm dev
```
