# Botainer Mini App - Implementation Plan

## 📋 Project Overview

**Goal**: Create a Telegram Mini App for Botainer with visual dashboard and interactive container management.

**Timeline**: 6 months (Q2-Q4 2026)

**Tech Stack**:
- Frontend: React + TypeScript + Vite
- Backend: Go (extend current bot)
- Real-time: WebSocket
- UI: Tailwind CSS + shadcn/ui
- Charts: Recharts
- Terminal: xterm.js

---

## 🏗️ Project Structure

```
botainer/
├── main.go                    # Current bot (keep as is)
├── api/                       # NEW: API for Mini App
│   ├── server.go             # HTTP + WebSocket server
│   ├── handlers.go           # API endpoints
│   ├── websocket.go          # Real-time updates
│   └── auth.go               # Telegram auth validation
├── webapp/                    # NEW: Mini App frontend
│   ├── src/
│   │   ├── components/       # React components
│   │   ├── pages/            # Page components
│   │   ├── hooks/            # Custom hooks
│   │   ├── lib/              # Utilities
│   │   ├── types/            # TypeScript types
│   │   └── App.tsx           # Main app
│   ├── public/
│   │   └── index.html        # Entry point
│   ├── package.json
│   ├── vite.config.ts
│   └── tsconfig.json
├── docker-compose.yml         # Update: add nginx
└── nginx.conf                 # NEW: Reverse proxy
```

---

## 📅 Phase 1: MVP (Weeks 1-8)

### Week 1-2: Setup & Infrastructure

#### Backend Setup
- [ ] Create `api/` directory structure
- [ ] Implement HTTP server (port 8080)
- [ ] Implement WebSocket server
- [ ] Add Telegram auth validation
- [ ] Create basic API endpoints:
  - `GET /api/containers` - List containers
  - `GET /api/containers/:id` - Get container details
  - `POST /api/containers/:id/start` - Start container
  - `POST /api/containers/:id/stop` - Stop container
  - `POST /api/containers/:id/restart` - Restart container
  - `GET /api/stats` - System stats
  - `WS /api/ws` - WebSocket connection

#### Frontend Setup
- [ ] Initialize React + TypeScript + Vite project
- [ ] Setup Tailwind CSS + shadcn/ui
- [ ] Configure Telegram WebApp SDK
- [ ] Create basic layout (Sidebar + Main content)
- [ ] Setup routing (React Router)
- [ ] Create API client with fetch wrapper
- [ ] Setup WebSocket client

#### Infrastructure
- [ ] Add nginx to docker-compose.yml
- [ ] Configure nginx as reverse proxy:
  - `/` → Frontend (static files)
  - `/api` → Backend (Go server)
  - `/api/ws` → WebSocket
- [ ] Setup HTTPS with self-signed cert (for testing)
- [ ] Configure BotFather with `/setmenubutton`

**Deliverable**: Basic infrastructure running, Mini App opens from Telegram

---

### Week 3-4: Dashboard & Container List

#### Dashboard Page
- [ ] Create Dashboard component
- [ ] Display summary cards:
  - Total containers
  - Running containers
  - Stopped containers
  - CPU usage
  - RAM usage
- [ ] Add pie chart for container status distribution
- [ ] Add quick actions section

#### Container List Page
- [ ] Create ContainerList component
- [ ] Display table with columns:
  - Status indicator (🟢🔴🟡)
  - Name
  - Image
  - State
  - CPU %
  - RAM %
  - Actions
- [ ] Add filters: All / Running / Stopped / Paused
- [ ] Add search bar
- [ ] Implement real-time updates via WebSocket
- [ ] Add action buttons: Start, Stop, Restart

**Deliverable**: Dashboard + Container List working with real data

---

### Week 5-6: Container Detail & Logs

#### Container Detail Page
- [ ] Create ContainerDetail component
- [ ] Add tabs:
  - Overview (status, image, ports, volumes)
  - Logs (live logs viewer)
  - Stats (CPU/RAM charts)
  - Config (JSON viewer)
- [ ] Implement navigation from list to detail

#### Logs Viewer
- [ ] Integrate xterm.js
- [ ] Stream logs via WebSocket
- [ ] Add controls:
  - Auto-scroll toggle
  - Clear logs
  - Download logs
  - Search in logs
- [ ] Add syntax highlighting for common patterns

**Deliverable**: Container detail view with live logs

---

### Week 7-8: Stats & Monitoring

#### Stats Page
- [ ] Create Stats component
- [ ] Add system overview:
  - CPU usage chart (last 1h)
  - RAM usage chart (last 1h)
  - Disk usage
- [ ] Add per-container resource usage
- [ ] Implement chart with Recharts
- [ ] Add refresh interval selector (5s, 10s, 30s, 1m)

#### Real-time Updates
- [ ] Implement WebSocket message types:
  - `container_status` - Container state changed
  - `stats_update` - Resource usage update
  - `log_line` - New log line
- [ ] Handle reconnection logic
- [ ] Add connection status indicator

**Deliverable**: MVP complete - Dashboard, List, Detail, Logs, Stats

---

## 📅 Phase 2: Advanced Features (Weeks 9-16)

### Week 9-10: Container Creation

#### Create Container Wizard
- [ ] Create multi-step form:
  - Step 1: Image selection
  - Step 2: Basic config (name, restart policy)
  - Step 3: Port mapping
  - Step 4: Volume mounting
  - Step 5: Environment variables
  - Step 6: Network selection
  - Step 7: Review & Create
- [ ] Add validation for each step
- [ ] Show preview of docker run command
- [ ] Implement API endpoint: `POST /api/containers`

**Deliverable**: Visual container creation wizard

---

### Week 11-12: Docker Compose Manager

#### Compose Projects
- [ ] Create ComposeProjects component
- [ ] List detected projects
- [ ] Show services per project with status
- [ ] Add actions: Up, Down, Restart, Pull
- [ ] Implement API endpoints:
  - `GET /api/compose/projects`
  - `GET /api/compose/projects/:name`
  - `POST /api/compose/projects/:name/up`
  - `POST /api/compose/projects/:name/down`
  - `POST /api/compose/projects/:name/restart`

#### YAML Editor
- [ ] Integrate Monaco Editor
- [ ] Add syntax highlighting for YAML
- [ ] Add validation
- [ ] Implement save functionality

**Deliverable**: Compose project management

---

### Week 13-14: Images & Updates

#### Images Library
- [ ] Create Images component
- [ ] Display grid of image cards
- [ ] Show: Repository, Tag, Size, Created
- [ ] Add filters: Used / Unused
- [ ] Add actions: Pull, Remove, Inspect
- [ ] Implement API endpoints:
  - `GET /api/images`
  - `POST /api/images/pull`
  - `DELETE /api/images/:id`

#### Update Center
- [ ] Create UpdateCenter component
- [ ] Show available updates
- [ ] Display: Current version → New version
- [ ] Add bulk update functionality
- [ ] Implement API endpoint: `POST /api/updates/check`

**Deliverable**: Image management and updates

---

### Week 15-16: Networks & Volumes

#### Network Visualizer
- [ ] Create NetworkVisualizer component
- [ ] Use D3.js or React Flow for diagram
- [ ] Show containers as nodes
- [ ] Show networks as connections
- [ ] Add interactive tooltips
- [ ] Implement API endpoint: `GET /api/networks`

#### Volumes Manager
- [ ] Create Volumes component
- [ ] Display table with: Name, Driver, Size, Containers
- [ ] Add filters: In use / Unused
- [ ] Add actions: Inspect, Remove
- [ ] Implement API endpoints:
  - `GET /api/volumes`
  - `DELETE /api/volumes/:name`

**Deliverable**: Network visualization and volume management

---

## 📅 Phase 3: Polish & Advanced (Weeks 17-24)

### Week 17-18: Templates

#### Template Library
- [ ] Create Templates component
- [ ] Display template cards with preview
- [ ] Add categories
- [ ] Implement template editor
- [ ] Add import/export functionality
- [ ] Implement API endpoints:
  - `GET /api/templates`
  - `POST /api/templates`
  - `POST /api/templates/:id/deploy`

**Deliverable**: Template system

---

### Week 19-20: Alerts & Health Checks

#### Alerts Configuration
- [ ] Create Alerts component
- [ ] Configure thresholds per container
- [ ] Show active alerts
- [ ] Add alert history
- [ ] Implement API endpoints:
  - `GET /api/alerts`
  - `POST /api/alerts`

#### Health Checks
- [ ] Create HealthChecks component
- [ ] Configure HTTP/TCP/Command checks
- [ ] Show health status
- [ ] Add test functionality

**Deliverable**: Monitoring and alerts

---

### Week 21-22: Security & Audit

#### Security Scanner
- [ ] Create SecurityScanner component
- [ ] Integrate Trivy scanning
- [ ] Display vulnerabilities by severity
- [ ] Show CVE details
- [ ] Implement API endpoint: `POST /api/scan/:image`

#### Audit Log
- [ ] Create AuditLog component
- [ ] Display action history
- [ ] Add filters and search
- [ ] Export functionality

**Deliverable**: Security and audit features

---

### Week 23-24: Polish & Testing

#### UI/UX Polish
- [ ] Add loading states
- [ ] Add error boundaries
- [ ] Improve animations
- [ ] Add keyboard shortcuts
- [ ] Optimize performance
- [ ] Add dark mode support

#### Testing
- [ ] Write unit tests for components
- [ ] Write integration tests for API
- [ ] End-to-end testing with Playwright
- [ ] Performance testing
- [ ] Security testing

#### Documentation
- [ ] User guide
- [ ] API documentation
- [ ] Deployment guide
- [ ] Contributing guide

**Deliverable**: Production-ready Mini App

---

## 🔧 Technical Details

### API Authentication

```go
// Validate Telegram initData
func validateTelegramAuth(initData string) (*TelegramUser, error) {
    // Parse initData
    params := parseInitData(initData)
    
    // Verify hash
    hash := params["hash"]
    delete(params, "hash")
    
    // Create data-check-string
    keys := sortKeys(params)
    var pairs []string
    for _, key := range keys {
        pairs = append(pairs, fmt.Sprintf("%s=%s", key, params[key]))
    }
    dataCheckString := strings.Join(pairs, "\n")
    
    // Compute secret key
    secretKey := hmacSHA256([]byte("WebAppData"), []byte(botToken))
    
    // Compute hash
    computedHash := hex.EncodeToString(hmacSHA256(secretKey, []byte(dataCheckString)))
    
    if computedHash != hash {
        return nil, errors.New("invalid hash")
    }
    
    // Parse user data
    user := parseUser(params["user"])
    return user, nil
}
```

### WebSocket Message Format

```typescript
// Client → Server
interface WSMessage {
  type: 'subscribe' | 'unsubscribe' | 'ping'
  resource?: 'containers' | 'stats' | 'logs'
  id?: string
}

// Server → Client
interface WSEvent {
  type: 'container_status' | 'stats_update' | 'log_line' | 'pong'
  data: any
  timestamp: number
}
```

### API Response Format

```typescript
interface APIResponse<T> {
  success: boolean
  data?: T
  error?: string
  timestamp: number
}
```

---

## 🚀 Deployment Strategy

### Development
```bash
# Backend
cd /home/ubuntu/botainer
go run main.go

# Frontend
cd webapp
npm run dev
```

### Production
```bash
# Build frontend
cd webapp
npm run build

# Build backend
go build -o botainer main.go

# Deploy with docker-compose
docker compose up -d --build
```

### Nginx Configuration
```nginx
server {
    listen 443 ssl;
    server_name botainer.local;

    ssl_certificate /etc/nginx/ssl/cert.pem;
    ssl_certificate_key /etc/nginx/ssl/key.pem;

    # Frontend
    location / {
        root /usr/share/nginx/html;
        try_files $uri $uri/ /index.html;
    }

    # API
    location /api {
        proxy_pass http://botainer:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

---

## 📊 Success Metrics

### Performance
- [ ] Initial load < 2s
- [ ] Time to interactive < 3s
- [ ] WebSocket latency < 100ms
- [ ] API response time < 200ms

### Quality
- [ ] Test coverage > 80%
- [ ] Zero critical security issues
- [ ] Lighthouse score > 90
- [ ] Accessibility score > 90

### User Experience
- [ ] Works on mobile, tablet, desktop
- [ ] Supports Chrome, Firefox, Safari
- [ ] Dark mode support
- [ ] Keyboard navigation

---

## 🎯 Next Steps

1. **Week 1**: Setup project structure
2. **Week 1**: Initialize frontend with Vite + React + TypeScript
3. **Week 1**: Create basic API server in Go
4. **Week 2**: Implement Telegram auth validation
5. **Week 2**: Setup nginx and docker-compose
6. **Week 2**: Configure BotFather and test Mini App opening

**Ready to start?** Let's begin with Week 1 setup! 🚀
