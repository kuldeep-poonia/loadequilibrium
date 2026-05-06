# Control Plane Dashboard

A production-grade enterprise dashboard for the Go-based distributed systems control plane.

## Quick Start

```bash
# 1. Install dependencies
npm install

# 2. Configure backend URL
cp .env.local.example .env.local
# Edit .env.local and set NEXT_PUBLIC_BACKEND_URL=http://your-backend:8080

# 3. Run development server
npm run dev

# 4. Open http://localhost:3000
```

## Architecture

```
src/
├── app/                  # Next.js App Router
│   ├── layout.tsx        # Root layout
│   ├── page.tsx          # Main dashboard page
│   └── globals.css       # Global styles + Tailwind
├── components/
│   ├── layout/
│   │   ├── Sidebar.tsx         # Navigation sidebar
│   │   └── ConnectionBar.tsx   # WebSocket status bar
│   ├── panels/
│   │   ├── OverviewPanel.tsx       # System status, stability, health
│   │   ├── TelemetryPanel.tsx      # Live streaming charts
│   │   ├── AutopilotPanel.tsx      # MPC, PID, directives, stability
│   │   ├── AnomalyPanel.tsx        # Events, alerts, pressure heatmap
│   │   ├── SimulationPanel.tsx     # Chaos, replay burst, sim results
│   │   ├── PolicyPanel.tsx         # Policy presets, topology sensitivity
│   │   ├── ControlPanel.tsx        # Actuators, feedback loop, coupling
│   │   ├── IntelligencePanel.tsx   # Prediction timelines, stochastic models
│   │   └── LogsPanel.tsx           # Event log, audit trail
│   └── ui/
│       └── index.tsx              # Shared primitives
├── hooks/
│   ├── useTickStream.ts          # WebSocket with reconnect logic
│   ├── useTelemetryHistory.ts    # Ring-buffer for chart history
│   └── useMutation.ts            # API mutation state management
├── services/
│   ├── api.ts                    # Centralized REST API layer
│   └── format.ts                 # Formatting utilities
└── types/
    ├── tick.ts                   # All WebSocket tick payload types
    └── api.ts                    # REST request/response types
```

## Backend Endpoints Used

### REST (POST)
| Endpoint | Purpose |
|---|---|
| `POST /api/v1/control/toggle` | Enable/disable actuation |
| `POST /api/v1/control/chaos-run` | Inject chaos scenario |
| `POST /api/v1/control/replay-burst` | Replay a traffic burst |
| `POST /api/v1/policy/update` | Switch policy preset |
| `POST /api/v1/runtime/step` | Manual runtime step |
| `POST /api/v1/sandbox/trigger` | Trigger sandbox experiment |
| `POST /api/v1/simulation/control` | Start/stop/reset simulation |
| `POST /api/v1/intelligence/rollout` | Force intelligence rollout |
| `POST /api/v1/alerts/ack` | Acknowledge an alert |

### REST (GET)
| Endpoint | Purpose |
|---|---|
| `GET /health` | Health check |
| `GET /api/v1/snapshot` | Full state snapshot on load |

### WebSocket
| Endpoint | Purpose |
|---|---|
| `ws://host/ws` | Live tick stream (TickPayload) |

## WebSocket Reconnect Policy

- Exponential backoff starting at 1s, capped at 30s
- Jitter up to 500ms per attempt
- Reconnect count displayed in connection bar
- Ping frames silently discarded

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `NEXT_PUBLIC_BACKEND_URL` | `http://localhost:8080` | Backend base URL |

## Production Build

```bash
npm run build
npm start
```
