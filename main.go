package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"
)

var prometheusURL string

func init() {
	prometheusURL = os.Getenv("PROMETHEUS_URL")
	if prometheusURL == "" {
		prometheusURL = "http://prometheus:9090"
	}
}

type promResponse struct {
	Status string   `json:"status"`
	Data   promData `json:"data"`
}

type promData struct {
	ResultType string       `json:"resultType"`
	Result     []promResult `json:"result"`
}

type promResult struct {
	Metric map[string]string `json:"metric"`
	Value  [2]interface{}    `json:"value"`
	Values [][2]interface{}  `json:"values"`
}

func queryProm(query string) (*promResponse, error) {
	resp, err := http.Get(prometheusURL + "/api/v1/query?query=" + url.QueryEscape(query))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result promResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func queryPromRange(query string, duration string) (*promResponse, error) {
	end := time.Now().Unix()
	var start int64
	switch duration {
	case "1h":
		start = end - 3600
	case "6h":
		start = end - 21600
	case "24h":
		start = end - 86400
	default:
		start = end - 3600
	}
	step := (end - start) / 120

	u := fmt.Sprintf("%s/api/v1/query_range?query=%s&start=%d&end=%d&step=%d",
		prometheusURL, url.QueryEscape(query), start, end, step)
	resp, err := http.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result promResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func getValue(resp *promResponse) string {
	if resp == nil || resp.Status != "success" || len(resp.Data.Result) == 0 {
		return "N/A"
	}
	if v, ok := resp.Data.Result[0].Value[1].(string); ok {
		return v
	}
	return "N/A"
}

func getMetric(resp *promResponse, key string) string {
	if resp == nil || resp.Status != "success" || len(resp.Data.Result) == 0 {
		return "N/A"
	}
	if v, ok := resp.Data.Result[0].Metric[key]; ok {
		return v
	}
	return "N/A"
}

func formatFloat(raw string, format string) string {
	if raw == "N/A" {
		return raw
	}
	var v float64
	fmt.Sscanf(raw, "%f", &v)
	return fmt.Sprintf(format, v)
}

type dashboardData struct {
	// Synapse
	Version       string `json:"version"`
	Uptime        string `json:"uptime"`
	CPU           string `json:"cpu"`
	Memory        string `json:"memory"`
	DAU           string `json:"dau"`
	RequestRate   string `json:"request_rate"`
	OpenFDs       string `json:"open_fds"`
	MaxFDs        string `json:"max_fds"`
	FedPDUsIn     string `json:"fed_pdus_in"`
	FedPDUsOut    string `json:"fed_pdus_out"`
	CacheHitRatio string `json:"cache_hit_ratio"`
	PythonVersion string `json:"python_version"`
	EventsSent    string `json:"events_sent"`
	Rooms         string `json:"rooms"`
	DBTxnRate     string `json:"db_txn_rate"`
	AvgRespTime   string `json:"avg_resp_time"`

	// Postgres
	PGConnections  string `json:"pg_connections"`
	PGDBSize       string `json:"pg_db_size"`
	PGCacheHit     string `json:"pg_cache_hit"`
	PGTxnRate      string `json:"pg_txn_rate"`
	PGDeadTuples   string `json:"pg_dead_tuples"`
	PGUptime       string `json:"pg_uptime"`
	PGActiveQueries string `json:"pg_active_queries"`
}

func handleAPI(w http.ResponseWriter, r *http.Request) {
	// Synapse metrics
	buildInfo, _ := queryProm("synapse_build_info")
	cpu, _ := queryProm("rate(process_cpu_seconds_total{job=\"synapse\"}[5m]) * 100")
	mem, _ := queryProm("process_resident_memory_bytes{job=\"synapse\"} / 1024 / 1024")
	dau, _ := queryProm("synapse_admin_daily_active_users")
	reqRate, _ := queryProm("sum(rate(synapse_http_server_requests_received_total{job=\"synapse\"}[5m]))")
	openFDs, _ := queryProm("process_open_fds{job=\"synapse\"}")
	maxFDs, _ := queryProm("process_max_fds{job=\"synapse\"}")
	uptime, _ := queryProm("time() - process_start_time_seconds{job=\"synapse\"}")
	fedIn, _ := queryProm("increase(synapse_federation_server_received_pdus_total[1h])")
	fedOut, _ := queryProm("increase(synapse_federation_client_sent_transactions_total[1h])")
	cacheHit, _ := queryProm("sum(rate(synapse_util_caches_cache_hits[5m])) / (sum(rate(synapse_util_caches_cache_hits[5m])) + sum(rate(synapse_util_caches_cache[5m])) + 0.001) * 100")
	eventsSent, _ := queryProm("increase(synapse_http_server_response_count_total{job=\"synapse\"}[1h])")
	rooms, _ := queryProm("synapse_notifier_rooms")
	dbTxnRate, _ := queryProm("sum(rate(synapse_storage_transaction_time_count_total[5m]))")
	avgResp, _ := queryProm("sum(rate(synapse_http_server_response_time_seconds_sum{job=\"synapse\"}[5m])) / sum(rate(synapse_http_server_response_time_seconds_count{job=\"synapse\"}[5m]))")

	// Postgres metrics
	pgConn, _ := queryProm("sum(pg_stat_activity_count{datname=\"synapse\"})")
	pgSize, _ := queryProm("pg_database_size_bytes{datname=\"synapse\"} / 1024 / 1024")
	pgCacheHit, _ := queryProm("pg_stat_database_blks_hit{datname=\"synapse\"} / (pg_stat_database_blks_hit{datname=\"synapse\"} + pg_stat_database_blks_read{datname=\"synapse\"} + 0.001) * 100")
	pgTxnRate, _ := queryProm("rate(pg_stat_database_xact_commit{datname=\"synapse\"}[5m])")
	pgDead, _ := queryProm("sum(pg_stat_user_tables_n_dead_tup{datname=\"synapse\"})")
	pgUptime, _ := queryProm("time() - process_start_time_seconds{job=\"postgres\"}")
	pgActive, _ := queryProm("sum(pg_stat_activity_count{datname=\"synapse\",state=\"active\"})")

	// Format uptime
	formatUptime := func(resp *promResponse) string {
		if v := getValue(resp); v != "N/A" {
			var secs float64
			fmt.Sscanf(v, "%f", &secs)
			d := time.Duration(secs) * time.Second
			days := int(d.Hours()) / 24
			hours := int(d.Hours()) % 24
			mins := int(d.Minutes()) % 60
			if days > 0 {
				return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
			}
			return fmt.Sprintf("%dh %dm", hours, mins)
		}
		return "N/A"
	}

	data := dashboardData{
		Version:       getMetric(buildInfo, "version"),
		Uptime:        formatUptime(uptime),
		CPU:           formatFloat(getValue(cpu), "%.1f%%"),
		Memory:        formatFloat(getValue(mem), "%.0f MB"),
		DAU:           getValue(dau),
		RequestRate:   formatFloat(getValue(reqRate), "%.1f/s"),
		OpenFDs:       formatFloat(getValue(openFDs), "%.0f"),
		MaxFDs:        formatFloat(getValue(maxFDs), "%.0f"),
		FedPDUsIn:     formatFloat(getValue(fedIn), "%.0f"),
		FedPDUsOut:    formatFloat(getValue(fedOut), "%.0f"),
		CacheHitRatio: formatFloat(getValue(cacheHit), "%.1f%%"),
		PythonVersion: getMetric(buildInfo, "pythonversion"),
		EventsSent:    formatFloat(getValue(eventsSent), "%.0f"),
		Rooms:         formatFloat(getValue(rooms), "%.0f"),
		DBTxnRate:     formatFloat(getValue(dbTxnRate), "%.1f/s"),
		AvgRespTime:   formatFloat(getValue(avgResp), "%.3fs"),

		PGConnections:  formatFloat(getValue(pgConn), "%.0f"),
		PGDBSize:       formatFloat(getValue(pgSize), "%.0f MB"),
		PGCacheHit:     formatFloat(getValue(pgCacheHit), "%.1f%%"),
		PGTxnRate:      formatFloat(getValue(pgTxnRate), "%.1f/s"),
		PGDeadTuples:   formatFloat(getValue(pgDead), "%.0f"),
		PGUptime:       formatUptime(pgUptime),
		PGActiveQueries: formatFloat(getValue(pgActive), "%.0f"),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

type chartPoint struct {
	T int64   `json:"t"`
	Y float64 `json:"y"`
}

func handleChartAPI(w http.ResponseWriter, r *http.Request) {
	metric := r.URL.Query().Get("metric")
	duration := r.URL.Query().Get("range")
	if duration == "" {
		duration = "1h"
	}

	queries := map[string]string{
		"cpu":            "rate(process_cpu_seconds_total{job=\"synapse\"}[5m]) * 100",
		"memory":         "process_resident_memory_bytes{job=\"synapse\"} / 1024 / 1024",
		"requests":       "sum(rate(synapse_http_server_requests_received_total{job=\"synapse\"}[5m]))",
		"response_time":  "sum(rate(synapse_http_server_response_time_seconds_sum{job=\"synapse\"}[5m])) / sum(rate(synapse_http_server_response_time_seconds_count{job=\"synapse\"}[5m]))",
		"federation_in":  "rate(synapse_federation_server_received_pdus_total[5m])",
		"federation_out": "rate(synapse_federation_client_sent_transactions_total[5m])",
		"open_fds":       "process_open_fds{job=\"synapse\"}",
		"db_txn":         "sum(rate(synapse_storage_transaction_time_count_total[5m]))",
		"cache_hit":      "sum(rate(synapse_util_caches_cache_hits[5m])) / (sum(rate(synapse_util_caches_cache_hits[5m])) + sum(rate(synapse_util_caches_cache[5m])) + 0.001) * 100",
		"pg_connections": "sum(pg_stat_activity_count{datname=\"synapse\"})",
		"pg_size":        "pg_database_size_bytes{datname=\"synapse\"} / 1024 / 1024",
		"pg_cache_hit":   "pg_stat_database_blks_hit{datname=\"synapse\"} / (pg_stat_database_blks_hit{datname=\"synapse\"} + pg_stat_database_blks_read{datname=\"synapse\"} + 0.001) * 100",
		"pg_txn_rate":    "rate(pg_stat_database_xact_commit{datname=\"synapse\"}[5m])",
	}

	query, ok := queries[metric]
	if !ok {
		http.Error(w, "unknown metric", 400)
		return
	}

	resp, err := queryPromRange(query, duration)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	var points []chartPoint
	if resp.Status == "success" && len(resp.Data.Result) > 0 {
		for _, v := range resp.Data.Result[0].Values {
			ts, _ := v[0].(float64)
			var val float64
			if s, ok := v[1].(string); ok {
				fmt.Sscanf(s, "%f", &val)
			}
			points = append(points, chartPoint{T: int64(ts) * 1000, Y: val})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(points)
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, indexHTML)
	})
	http.HandleFunc("/api/stats", handleAPI)
	http.HandleFunc("/api/chart", handleChartAPI)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	log.Printf("Synapse dashboard listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

var indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Synapse Dashboard</title>
<script src="https://cdn.jsdelivr.net/npm/chart.js@4"></script>
<style>
  :root {
    --bg: #0f1117;
    --card: #1a1d27;
    --border: #2a2d3a;
    --text: #e1e4ed;
    --muted: #8b8fa3;
    --accent: #6366f1;
    --green: #22c55e;
    --amber: #f59e0b;
    --red: #ef4444;
    --cyan: #06b6d4;
    --chart-grid: #2a2d3a;
    --tooltip-bg: #1a1d27;
    --tooltip-border: #3a3d4a;
  }
  [data-theme="light"] {
    --bg: #f8f9fc;
    --card: #ffffff;
    --border: #e2e5ed;
    --text: #1a1d27;
    --muted: #64687a;
    --chart-grid: #e2e5ed;
    --tooltip-bg: #ffffff;
    --tooltip-border: #d0d3dc;
  }
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
    background: var(--bg);
    color: var(--text);
    min-height: 100vh;
    transition: background 0.3s, color 0.3s;
  }
  .header {
    padding: 24px 32px;
    border-bottom: 1px solid var(--border);
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  .header-left {
    display: flex;
    align-items: center;
    gap: 16px;
  }
  .header h1 {
    font-size: 20px;
    font-weight: 600;
    letter-spacing: -0.5px;
  }
  .header .meta {
    color: var(--muted);
    font-size: 13px;
  }
  .header-right {
    display: flex;
    align-items: center;
    gap: 16px;
  }
  .theme-toggle {
    background: var(--card);
    border: 1px solid var(--border);
    color: var(--muted);
    width: 36px;
    height: 36px;
    border-radius: 8px;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 16px;
    transition: all 0.2s;
  }
  .theme-toggle:hover { border-color: var(--accent); color: var(--accent); }
  .controls {
    display: flex;
    gap: 8px;
    padding: 16px 32px;
  }
  .controls button {
    background: var(--card);
    border: 1px solid var(--border);
    color: var(--muted);
    padding: 6px 14px;
    border-radius: 6px;
    cursor: pointer;
    font-size: 13px;
    transition: all 0.2s;
  }
  .controls button:hover { border-color: var(--accent); }
  .controls button.active {
    background: var(--accent);
    color: white;
    border-color: var(--accent);
  }
  .section-title {
    font-size: 13px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 1px;
    color: var(--muted);
    padding: 8px 32px 12px;
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: 12px;
    padding: 0 32px 24px;
  }
  .card {
    background: var(--card);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 18px;
    transition: all 0.2s;
  }
  .card:hover { border-color: var(--accent); }
  .card .label {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    color: var(--muted);
    margin-bottom: 6px;
  }
  .card .value {
    font-size: 26px;
    font-weight: 700;
    letter-spacing: -1px;
  }
  .card .sub {
    font-size: 11px;
    color: var(--muted);
    margin-top: 3px;
  }
  .charts {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
    padding: 0 32px 32px;
  }
  .chart-card {
    background: var(--card);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 20px;
    transition: border-color 0.2s;
  }
  .chart-card:hover { border-color: var(--accent); }
  .chart-card h3 {
    font-size: 13px;
    font-weight: 500;
    margin-bottom: 12px;
    color: var(--muted);
  }
  .chart-card canvas {
    width: 100% !important;
    height: 180px !important;
  }
  .status-dot {
    display: inline-block;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--green);
    margin-right: 8px;
    animation: pulse 2s infinite;
  }
  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.5; }
  }
  @media (max-width: 900px) {
    .charts { grid-template-columns: 1fr; }
    .grid { grid-template-columns: repeat(2, 1fr); }
    .header, .controls, .grid, .charts, .section-title { padding-left: 16px; padding-right: 16px; }
  }
</style>
</head>
<body>

<div class="header">
  <div class="header-left">
    <h1><span class="status-dot"></span>Synapse Dashboard</h1>
  </div>
  <div class="header-right">
    <div class="meta" id="meta">Loading...</div>
    <button class="theme-toggle" onclick="toggleTheme()" id="theme-btn" title="Toggle theme">&#9790;</button>
  </div>
</div>

<div class="controls">
  <button onclick="setRange('1h')" id="btn-1h" class="active">1H</button>
  <button onclick="setRange('6h')" id="btn-6h">6H</button>
  <button onclick="setRange('24h')" id="btn-24h">24H</button>
</div>

<div class="section-title">Synapse</div>
<div class="grid">
  <div class="card">
    <div class="label">Uptime</div>
    <div class="value" id="uptime">--</div>
  </div>
  <div class="card">
    <div class="label">CPU Usage</div>
    <div class="value" id="cpu">--</div>
  </div>
  <div class="card">
    <div class="label">Memory</div>
    <div class="value" id="memory">--</div>
  </div>
  <div class="card">
    <div class="label">Request Rate</div>
    <div class="value" id="request_rate">--</div>
  </div>
  <div class="card">
    <div class="label">Avg Response</div>
    <div class="value" id="avg_resp_time">--</div>
  </div>
  <div class="card">
    <div class="label">Daily Active Users</div>
    <div class="value" id="dau">--</div>
  </div>
  <div class="card">
    <div class="label">Rooms</div>
    <div class="value" id="rooms">--</div>
  </div>
  <div class="card">
    <div class="label">Events (1h)</div>
    <div class="value" id="events_sent">--</div>
  </div>
  <div class="card">
    <div class="label">Cache Hit Ratio</div>
    <div class="value" id="cache_hit_ratio">--</div>
  </div>
  <div class="card">
    <div class="label">DB Txn Rate</div>
    <div class="value" id="db_txn_rate">--</div>
  </div>
  <div class="card">
    <div class="label">File Descriptors</div>
    <div class="value" id="fds">--</div>
    <div class="sub" id="fds-sub"></div>
  </div>
  <div class="card">
    <div class="label">Federation (1h)</div>
    <div class="value" id="fed">--</div>
    <div class="sub" id="fed-sub"></div>
  </div>
</div>

<div class="section-title">PostgreSQL</div>
<div class="grid">
  <div class="card">
    <div class="label">PG Uptime</div>
    <div class="value" id="pg_uptime">--</div>
  </div>
  <div class="card">
    <div class="label">Connections</div>
    <div class="value" id="pg_connections">--</div>
  </div>
  <div class="card">
    <div class="label">Active Queries</div>
    <div class="value" id="pg_active_queries">--</div>
  </div>
  <div class="card">
    <div class="label">Database Size</div>
    <div class="value" id="pg_db_size">--</div>
  </div>
  <div class="card">
    <div class="label">Cache Hit Ratio</div>
    <div class="value" id="pg_cache_hit">--</div>
  </div>
  <div class="card">
    <div class="label">Txn Commit Rate</div>
    <div class="value" id="pg_txn_rate">--</div>
  </div>
  <div class="card">
    <div class="label">Dead Tuples</div>
    <div class="value" id="pg_dead_tuples">--</div>
  </div>
</div>

<div class="section-title">Synapse Charts</div>
<div class="charts">
  <div class="chart-card">
    <h3>CPU Usage (%)</h3>
    <canvas id="chart-cpu"></canvas>
  </div>
  <div class="chart-card">
    <h3>Memory (MB)</h3>
    <canvas id="chart-memory"></canvas>
  </div>
  <div class="chart-card">
    <h3>Request Rate (req/s)</h3>
    <canvas id="chart-requests"></canvas>
  </div>
  <div class="chart-card">
    <h3>Avg Response Time (s)</h3>
    <canvas id="chart-response"></canvas>
  </div>
  <div class="chart-card">
    <h3>DB Transaction Rate (/s)</h3>
    <canvas id="chart-db-txn"></canvas>
  </div>
  <div class="chart-card">
    <h3>Cache Hit Ratio (%)</h3>
    <canvas id="chart-cache"></canvas>
  </div>
</div>

<div class="section-title">PostgreSQL Charts</div>
<div class="charts">
  <div class="chart-card">
    <h3>Connections</h3>
    <canvas id="chart-pg-conn"></canvas>
  </div>
  <div class="chart-card">
    <h3>Database Size (MB)</h3>
    <canvas id="chart-pg-size"></canvas>
  </div>
  <div class="chart-card">
    <h3>Cache Hit Ratio (%)</h3>
    <canvas id="chart-pg-cache"></canvas>
  </div>
  <div class="chart-card">
    <h3>Transaction Rate (/s)</h3>
    <canvas id="chart-pg-txn"></canvas>
  </div>
</div>

<script>
// Theme management
let currentTheme = localStorage.getItem('theme') || 'dark';
document.documentElement.setAttribute('data-theme', currentTheme);

function applyTheme(theme) {
  currentTheme = theme;
  document.documentElement.setAttribute('data-theme', theme);
  document.getElementById('theme-btn').innerHTML = theme === 'dark' ? '&#9790;' : '&#9788;';
  localStorage.setItem('theme', theme);
  if (Object.keys(charts).length > 0) updateChartColors();
}

function toggleTheme() {
  applyTheme(currentTheme === 'dark' ? 'light' : 'dark');
}

function getGridColor() {
  return currentTheme === 'dark' ? '#2a2d3a' : '#e2e5ed';
}
function getTickColor() {
  return currentTheme === 'dark' ? '#8b8fa3' : '#64687a';
}
function getTooltipBg() {
  return currentTheme === 'dark' ? '#1a1d27' : '#ffffff';
}
function getTooltipBorder() {
  return currentTheme === 'dark' ? '#3a3d4a' : '#d0d3dc';
}
function getTooltipText() {
  return currentTheme === 'dark' ? '#e1e4ed' : '#1a1d27';
}

let currentRange = '1h';
const charts = {};

const tooltipConfig = () => ({
  enabled: true,
  mode: 'index',
  intersect: false,
  backgroundColor: getTooltipBg(),
  borderColor: getTooltipBorder(),
  borderWidth: 1,
  titleColor: getTooltipText(),
  bodyColor: getTooltipText(),
  titleFont: { size: 12, weight: '600' },
  bodyFont: { size: 13 },
  padding: 12,
  cornerRadius: 8,
  displayColors: true,
  callbacks: {
    title: function(items) {
      if (!items.length) return '';
      const d = new Date(items[0].parsed.x);
      return d.toLocaleDateString() + ' ' + d.toLocaleTimeString();
    },
    label: function(ctx) {
      let v = ctx.parsed.y;
      let unit = chartUnits[ctx.chart.canvas.id] || '';
      if (unit === '%' || unit === ' MB' || unit === '/s' || unit === 's') {
        return ' ' + v.toFixed(2) + unit;
      }
      return ' ' + v.toFixed(2);
    }
  }
});

const chartUnits = {
  'chart-cpu': '%',
  'chart-memory': ' MB',
  'chart-requests': '/s',
  'chart-response': 's',
  'chart-db-txn': '/s',
  'chart-cache': '%',
  'chart-pg-conn': '',
  'chart-pg-size': ' MB',
  'chart-pg-cache': '%',
  'chart-pg-txn': '/s'
};

function fmtTime(ms) {
  const d = new Date(ms);
  return d.getHours().toString().padStart(2,'0') + ':' + d.getMinutes().toString().padStart(2,'0');
}

const chartOpts = (color) => ({
  responsive: true,
  maintainAspectRatio: false,
  animation: { duration: 300 },
  interaction: { mode: 'index', intersect: false },
  plugins: {
    legend: { display: false },
    tooltip: tooltipConfig()
  },
  scales: {
    x: {
      type: 'linear',
      grid: { color: getGridColor() },
      ticks: {
        color: getTickColor(),
        maxTicksLimit: 6,
        font: { size: 11 },
        callback: function(val) { return fmtTime(val); }
      }
    },
    y: {
      grid: { color: getGridColor() },
      ticks: { color: getTickColor(), font: { size: 11 } },
      beginAtZero: true
    }
  },
  elements: {
    point: { radius: 0, hoverRadius: 5, hoverBackgroundColor: color, hoverBorderColor: '#fff', hoverBorderWidth: 2 },
    line: { borderWidth: 2, borderColor: color, backgroundColor: color + '20', fill: true, tension: 0.3 }
  }
});

function createChart(id, color) {
  const ctx = document.getElementById(id).getContext('2d');
  charts[id] = new Chart(ctx, {
    type: 'line',
    data: { datasets: [{ data: [] }] },
    options: chartOpts(color)
  });
  charts[id]._color = color;
}

function updateChartColors() {
  Object.keys(charts).forEach(id => {
    const c = charts[id];
    if (!c) return;
    c.options.scales.x.grid.color = getGridColor();
    c.options.scales.y.grid.color = getGridColor();
    c.options.scales.x.ticks.color = getTickColor();
    c.options.scales.y.ticks.color = getTickColor();
    c.options.plugins.tooltip = tooltipConfig();
    c.update('none');
  });
}

// Synapse charts
createChart('chart-cpu', '#6366f1');
createChart('chart-memory', '#22c55e');
createChart('chart-requests', '#f59e0b');
createChart('chart-response', '#ef4444');
createChart('chart-db-txn', '#8b5cf6');
createChart('chart-cache', '#06b6d4');

// Postgres charts
createChart('chart-pg-conn', '#ec4899');
createChart('chart-pg-size', '#14b8a6');
createChart('chart-pg-cache', '#f97316');
createChart('chart-pg-txn', '#a855f7');

async function fetchStats() {
  try {
    const resp = await fetch('/api/stats');
    const d = await resp.json();
    // Synapse
    document.getElementById('uptime').textContent = d.uptime;
    document.getElementById('cpu').textContent = d.cpu;
    document.getElementById('memory').textContent = d.memory;
    document.getElementById('request_rate').textContent = d.request_rate;
    document.getElementById('avg_resp_time').textContent = d.avg_resp_time;
    document.getElementById('dau').textContent = d.dau;
    document.getElementById('rooms').textContent = d.rooms;
    document.getElementById('events_sent').textContent = d.events_sent;
    document.getElementById('cache_hit_ratio').textContent = d.cache_hit_ratio;
    document.getElementById('db_txn_rate').textContent = d.db_txn_rate;
    document.getElementById('fds').textContent = d.open_fds;
    document.getElementById('fds-sub').textContent = 'of ' + d.max_fds + ' max';
    document.getElementById('fed').textContent = d.fed_pdus_in + ' in';
    document.getElementById('fed-sub').textContent = d.fed_pdus_out + ' out';
    document.getElementById('meta').textContent = 'Synapse ' + d.version + ' | ' + d.python_version;
    // Postgres
    document.getElementById('pg_uptime').textContent = d.pg_uptime;
    document.getElementById('pg_connections').textContent = d.pg_connections;
    document.getElementById('pg_active_queries').textContent = d.pg_active_queries;
    document.getElementById('pg_db_size').textContent = d.pg_db_size;
    document.getElementById('pg_cache_hit').textContent = d.pg_cache_hit;
    document.getElementById('pg_txn_rate').textContent = d.pg_txn_rate;
    document.getElementById('pg_dead_tuples').textContent = d.pg_dead_tuples;
  } catch(e) {
    console.error('stats fetch failed:', e);
  }
}

async function fetchChart(metric, chartId) {
  try {
    const resp = await fetch('/api/chart?metric=' + metric + '&range=' + currentRange);
    const points = await resp.json();
    charts[chartId].data.datasets[0].data = (points || []).map(p => ({ x: p.t, y: p.y }));
    charts[chartId].update('none');
  } catch(e) {
    console.error('chart fetch failed:', e);
  }
}

function fetchAllCharts() {
  // Synapse
  fetchChart('cpu', 'chart-cpu');
  fetchChart('memory', 'chart-memory');
  fetchChart('requests', 'chart-requests');
  fetchChart('response_time', 'chart-response');
  fetchChart('db_txn', 'chart-db-txn');
  fetchChart('cache_hit', 'chart-cache');
  // Postgres
  fetchChart('pg_connections', 'chart-pg-conn');
  fetchChart('pg_size', 'chart-pg-size');
  fetchChart('pg_cache_hit', 'chart-pg-cache');
  fetchChart('pg_txn_rate', 'chart-pg-txn');
}

function setRange(r) {
  currentRange = r;
  document.querySelectorAll('.controls button').forEach(b => b.classList.remove('active'));
  document.getElementById('btn-' + r).classList.add('active');
  fetchAllCharts();
}

function refresh() {
  fetchStats();
  fetchAllCharts();
}

// Apply initial theme button icon
document.getElementById('theme-btn').innerHTML = currentTheme === 'dark' ? '&#9790;' : '&#9788;';

refresh();
setInterval(refresh, 15000);
</script>
</body>
</html>
`
