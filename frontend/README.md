# Microgrid Cloud Dev Frontend (React)

## Quick start
```bash
cd frontend
npm install
npm run dev
```

## Config
- Default API base: `http://localhost:8080`
- Override with env: `VITE_API_BASE_URL=http://localhost:8080 npm run dev`
- Or create a `.env` file with `VITE_API_BASE_URL=http://localhost:8080`

## Docker Compose
Run from project root:
```bash
docker-compose -f docker-compose.dev.yml up frontend
```

## Auth
Paste a JWT token in the UI. For dev, you can generate one with:
```bash
source scripts/lib_auth.sh
jwt_token_hs256 dev-secret-change-me tenant-demo admin runbook-user 3600
```
