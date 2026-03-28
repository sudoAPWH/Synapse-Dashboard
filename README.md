# Synapse Dashboard

A lightweight, self-hosted monitoring dashboard for [Matrix Synapse](https://github.com/element-hq/synapse) homeservers. Built in Go with zero external dependencies — just the standard library and a single Chart.js CDN include.

Replaces Grafana with a purpose-built ~11MB Alpine container.

![Dashboard Preview](https://img.shields.io/badge/status-active-brightgreen)

## Features

- **19 metric cards** across Synapse and PostgreSQL
- **10 time-series charts** with 1H / 6H / 24H range selector
- **Customizable layout** — drag-and-drop to reorder widgets, add/remove cards and charts
- **Collapsible chart sections** — click section headers to collapse/expand
- **Light / Dark mode** toggle (dark by default, persisted in browser)
- **Hover tooltips** with exact timestamps and formatted values
- **Manual refresh button** + auto-refresh every 15 seconds
- **Persistent layout** — widget order, visibility, collapsed state, and theme saved to localStorage
- **Responsive design** for mobile and desktop
- **No external Go dependencies** — only the standard library

## Customization

Click the pencil icon in the header to enter edit mode:

- **Drag widgets** using the handle icon to reorder them
- **Remove widgets** with the X button
- **Add widgets** back using the + button (opens a side panel)
- **Reset layout** with the gear icon to restore defaults
- **Collapse/expand** chart sections by clicking the section header arrows

All layout changes are saved automatically and persist across page reloads.

## Metrics

### Synapse (12 cards, 6 charts)

| Card | Chart |
|------|-------|
| Uptime | CPU Usage (%) |
| CPU Usage | Memory (MB) |
| Memory | Request Rate (req/s) |
| Request Rate | Avg Response Time (s) |
| Avg Response Time | DB Transaction Rate (/s) |
| Daily Active Users | Cache Hit Ratio (%) |
| Rooms | |
| Events (1h) | |
| Cache Hit Ratio | |
| DB Transaction Rate | |
| File Descriptors | |
| Federation (1h in/out) | |

### PostgreSQL (7 cards, 4 charts)

| Card | Chart |
|------|-------|
| PG Uptime | Connections |
| Connections | Database Size (MB) |
| Active Queries | Cache Hit Ratio (%) |
| Database Size | Transaction Rate (/s) |
| Cache Hit Ratio | |
| Transaction Commit Rate | |
| Dead Tuples | |

## Prerequisites

- Docker and Docker Compose
- A running Synapse homeserver with metrics enabled
- Prometheus scraping Synapse
- [postgres-exporter](https://github.com/prometheus-community/postgres_exporter) for PostgreSQL metrics

## Setup

### 1. Enable Synapse metrics

Add a metrics listener to your `homeserver.yaml`:

```yaml
enable_metrics: true
listeners:
  - port: 8008
    tls: false
    type: http
    x_forwarded: true
    bind_addresses: ['0.0.0.0']
    resources:
      - names: [client, federation]
        compress: false
  - port: 9000
    tls: false
    type: metrics
    bind_addresses: ['0.0.0.0']
```

### 2. Configure Prometheus

Create a `prometheus.yml`:

```yaml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: synapse
    metrics_path: /_synapse/metrics
    static_configs:
      - targets: ["synapse:9000"]

  - job_name: postgres
    static_configs:
      - targets: ["postgres-exporter:9187"]
```

### 3. Add to docker-compose.yml

```yaml
services:
  # ... your existing synapse, db, etc. ...

  prometheus:
    image: prom/prometheus:latest
    container_name: prometheus
    restart: unless-stopped
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus_data:/prometheus
    networks:
      - matrix

  postgres-exporter:
    image: prometheuscommunity/postgres-exporter:latest
    container_name: postgres-exporter
    restart: unless-stopped
    environment:
      DATA_SOURCE_NAME: "postgresql://YOUR_USER:YOUR_PASS@db:5432/synapse?sslmode=disable"
    depends_on:
      - db
    networks:
      - matrix

  dashboard:
    build: ./dashboard
    container_name: synapse-dashboard
    restart: unless-stopped
    ports:
      - "3000:3000"
    environment:
      - PROMETHEUS_URL=http://prometheus:9090
    depends_on:
      - prometheus
    networks:
      - matrix

volumes:
  prometheus_data:
```

### 4. Start everything

```bash
docker compose up -d
```

The dashboard will be available at `http://localhost:3000`.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PROMETHEUS_URL` | `http://prometheus:9090` | Prometheus API endpoint |
| `PORT` | `3000` | Dashboard listen port |

## Running locally (without Docker)

```bash
go build -o dashboard .
PROMETHEUS_URL=http://localhost:9090 ./dashboard
```

## License

MIT
