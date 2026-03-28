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
<script src="https://cdn.jsdelivr.net/npm/sortablejs@1.15.6/Sortable.min.js"></script>
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
  }
  [data-theme="light"] {
    --bg: #f8f9fc;
    --card: #ffffff;
    --border: #e2e5ed;
    --text: #1a1d27;
    --muted: #64687a;
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
  .header h1 { font-size: 20px; font-weight: 600; letter-spacing: -0.5px; }
  .header .meta { color: var(--muted); font-size: 13px; }
  .header-right { display: flex; align-items: center; gap: 12px; }
  .hdr-btn {
    background: var(--card);
    border: 1px solid var(--border);
    color: var(--muted);
    width: 36px; height: 36px;
    border-radius: 8px;
    cursor: pointer;
    display: flex; align-items: center; justify-content: center;
    font-size: 16px;
    transition: all 0.2s;
  }
  .hdr-btn:hover { border-color: var(--accent); color: var(--accent); }
  .hdr-btn.active { background: var(--accent); color: #fff; border-color: var(--accent); }
  .controls {
    display: flex; gap: 8px; padding: 16px 32px;
  }
  .controls button {
    background: var(--card);
    border: 1px solid var(--border);
    color: var(--muted);
    padding: 6px 14px; border-radius: 6px;
    cursor: pointer; font-size: 13px; transition: all 0.2s;
  }
  .controls button:hover { border-color: var(--accent); }
  .controls button.active { background: var(--accent); color: white; border-color: var(--accent); }
  .section-title {
    font-size: 15px; font-weight: 700; text-transform: uppercase;
    letter-spacing: 1.5px; color: var(--text); padding: 20px 0 12px;
    border-bottom: 2px solid var(--accent);
    display: inline-block;
  }
  .section-title-wrap {
    padding: 0 32px; margin-bottom: 4px;
  }
  .section-title-wrap.collapsible .section-title {
    cursor: pointer; user-select: none;
  }
  .section-title .arrow {
    font-size: 12px; margin-left: 10px; display: inline-block;
    transition: transform 0.3s;
  }
  .section-title-wrap.collapsed .arrow {
    transform: rotate(-90deg);
  }
  .chart-section-content {
    overflow: hidden; transition: max-height 0.4s ease, opacity 0.3s ease;
    max-height: 1000px; opacity: 1;
  }
  .chart-section-content.collapsed {
    max-height: 0; opacity: 0;
  }
  .section-spacer {
    height: 24px;
  }
  .widget-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: 12px; padding: 0 32px 24px;
    min-height: 40px;
  }
  .chart-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px; padding: 0 32px 32px;
    min-height: 40px;
  }
  .widget {
    background: var(--card);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 18px;
    transition: all 0.2s;
    position: relative;
  }
  .widget:hover { border-color: var(--accent); }
  .widget .label {
    font-size: 11px; text-transform: uppercase;
    letter-spacing: 0.5px; color: var(--muted); margin-bottom: 6px;
  }
  .widget .value { font-size: 26px; font-weight: 700; letter-spacing: -1px; }
  .widget .sub { font-size: 11px; color: var(--muted); margin-top: 3px; }
  .widget.chart-widget { padding: 20px; }
  .widget.chart-widget h3 {
    font-size: 13px; font-weight: 500; margin-bottom: 12px; color: var(--muted);
  }
  .widget.chart-widget canvas { width: 100% !important; height: 180px !important; }
  .widget .remove-btn {
    display: none;
    position: absolute; top: 6px; right: 6px;
    background: var(--red); color: #fff; border: none;
    width: 22px; height: 22px; border-radius: 50%;
    cursor: pointer; font-size: 13px; line-height: 22px; text-align: center;
  }
  .widget .drag-handle {
    display: none;
    position: absolute; top: 6px; left: 6px;
    color: var(--muted); cursor: grab; font-size: 14px;
    user-select: none;
  }
  body.edit-mode .widget .remove-btn,
  body.edit-mode .widget .drag-handle { display: block; }
  body.edit-mode .widget { cursor: grab; border-style: dashed; }
  .widget.sortable-ghost { opacity: 0.4; }
  .widget.sortable-chosen { box-shadow: 0 4px 20px rgba(99,102,241,0.3); }
  .status-dot {
    display: inline-block; width: 8px; height: 8px; border-radius: 50%;
    background: var(--green); margin-right: 8px; animation: pulse 2s infinite;
  }
  @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.5; } }

  /* Add widget panel */
  .add-panel {
    display: none; position: fixed; top: 0; right: 0;
    width: 320px; height: 100vh; background: var(--card);
    border-left: 1px solid var(--border);
    z-index: 100; overflow-y: auto; padding: 24px;
    box-shadow: -4px 0 20px rgba(0,0,0,0.3);
  }
  .add-panel.open { display: block; }
  .add-panel h2 { font-size: 16px; margin-bottom: 16px; }
  .add-panel .add-item {
    display: flex; justify-content: space-between; align-items: center;
    padding: 10px 12px; margin-bottom: 8px;
    background: var(--bg); border: 1px solid var(--border); border-radius: 8px;
  }
  .add-panel .add-item span { font-size: 13px; }
  .add-panel .add-item button {
    background: var(--accent); color: #fff; border: none;
    padding: 4px 12px; border-radius: 4px; cursor: pointer; font-size: 12px;
  }
  .add-panel .close-panel {
    position: absolute; top: 16px; right: 16px;
    background: none; border: none; color: var(--muted);
    font-size: 20px; cursor: pointer;
  }
  .overlay {
    display: none; position: fixed; top: 0; left: 0;
    width: 100%; height: 100%; background: rgba(0,0,0,0.4); z-index: 99;
  }
  .overlay.open { display: block; }

  @media (max-width: 900px) {
    .chart-grid { grid-template-columns: 1fr; }
    .widget-grid { grid-template-columns: repeat(2, 1fr); }
    .header, .controls, .widget-grid, .chart-grid, .section-title { padding-left: 16px; padding-right: 16px; }
  }
</style>
</head>
<body>

<div class="header">
  <h1><span class="status-dot"></span>Synapse Dashboard</h1>
  <div class="header-right">
    <div class="meta" id="meta">Loading...</div>
    <button class="hdr-btn" onclick="refresh()" id="refresh-btn" title="Refresh now">&#8635;</button>
    <button class="hdr-btn" onclick="toggleEdit()" id="edit-btn" title="Edit layout">&#9998;</button>
    <button class="hdr-btn" onclick="openAddPanel()" id="add-btn" title="Add widget" style="display:none">+</button>
    <button class="hdr-btn" onclick="resetLayout()" id="reset-btn" title="Reset layout" style="display:none;font-size:14px;">&#9881;</button>
    <button class="hdr-btn" onclick="toggleTheme()" id="theme-btn" title="Toggle theme">&#9790;</button>
  </div>
</div>

<div class="controls">
  <button onclick="setRange('1h')" id="btn-1h" class="active">1H</button>
  <button onclick="setRange('6h')" id="btn-6h">6H</button>
  <button onclick="setRange('24h')" id="btn-24h">24H</button>
</div>

<div class="section-title-wrap"><div class="section-title">Synapse</div></div>
<div class="widget-grid" id="grid-synapse-cards"></div>

<div class="section-title-wrap"><div class="section-title">PostgreSQL</div></div>
<div class="widget-grid" id="grid-pg-cards"></div>

<div class="section-spacer"></div>

<div class="section-title-wrap collapsible" onclick="toggleSection('synapse-charts')"><div class="section-title">Synapse Charts <span class="arrow" id="arrow-synapse-charts">&#9660;</span></div></div>
<div class="chart-section-content" id="content-synapse-charts">
  <div class="chart-grid" id="grid-synapse-charts"></div>
</div>

<div class="section-title-wrap collapsible" onclick="toggleSection('pg-charts')"><div class="section-title">PostgreSQL Charts <span class="arrow" id="arrow-pg-charts">&#9660;</span></div></div>
<div class="chart-section-content" id="content-pg-charts">
  <div class="chart-grid" id="grid-pg-charts"></div>
</div>

<footer style="text-align:center;padding:24px 0 32px;color:var(--muted);font-size:12px;">
  Built by <a href="https://github.com/sudoAPWH" style="color:var(--accent);text-decoration:none;">sudoAPWH</a>
  &middot;
  <a href="https://github.com/sudoAPWH/Synapse-Dashboard" style="color:var(--accent);text-decoration:none;">GitHub</a>
</footer>

<div class="overlay" id="overlay" onclick="closeAddPanel()"></div>
<div class="add-panel" id="add-panel">
  <button class="close-panel" onclick="closeAddPanel()">&times;</button>
  <h2>Add Widgets</h2>
  <div id="add-list"></div>
</div>

<script>
// --- Widget definitions ---
const allWidgets = [
  // Synapse cards
  { id: 'uptime', label: 'Uptime', section: 'synapse-cards', type: 'card', key: 'uptime' },
  { id: 'cpu', label: 'CPU Usage', section: 'synapse-cards', type: 'card', key: 'cpu' },
  { id: 'memory', label: 'Memory', section: 'synapse-cards', type: 'card', key: 'memory' },
  { id: 'request_rate', label: 'Request Rate', section: 'synapse-cards', type: 'card', key: 'request_rate' },
  { id: 'avg_resp_time', label: 'Avg Response', section: 'synapse-cards', type: 'card', key: 'avg_resp_time' },
  { id: 'dau', label: 'Daily Active Users', section: 'synapse-cards', type: 'card', key: 'dau' },
  { id: 'rooms', label: 'Rooms', section: 'synapse-cards', type: 'card', key: 'rooms' },
  { id: 'events_sent', label: 'Events (1h)', section: 'synapse-cards', type: 'card', key: 'events_sent' },
  { id: 'cache_hit_ratio', label: 'Cache Hit Ratio', section: 'synapse-cards', type: 'card', key: 'cache_hit_ratio' },
  { id: 'db_txn_rate', label: 'DB Txn Rate', section: 'synapse-cards', type: 'card', key: 'db_txn_rate' },
  { id: 'fds', label: 'File Descriptors', section: 'synapse-cards', type: 'card', key: 'open_fds', subKey: 'max_fds', subFmt: 'of {v} max' },
  { id: 'fed', label: 'Federation (1h)', section: 'synapse-cards', type: 'card', key: 'fed_pdus_in', valueFmt: '{v} in', subKey: 'fed_pdus_out', subFmt: '{v} out' },

  // PG cards
  { id: 'pg_uptime', label: 'PG Uptime', section: 'pg-cards', type: 'card', key: 'pg_uptime' },
  { id: 'pg_connections', label: 'Connections', section: 'pg-cards', type: 'card', key: 'pg_connections' },
  { id: 'pg_active_queries', label: 'Active Queries', section: 'pg-cards', type: 'card', key: 'pg_active_queries' },
  { id: 'pg_db_size', label: 'Database Size', section: 'pg-cards', type: 'card', key: 'pg_db_size' },
  { id: 'pg_cache_hit', label: 'Cache Hit Ratio', section: 'pg-cards', type: 'card', key: 'pg_cache_hit' },
  { id: 'pg_txn_rate', label: 'Txn Commit Rate', section: 'pg-cards', type: 'card', key: 'pg_txn_rate' },
  { id: 'pg_dead_tuples', label: 'Dead Tuples', section: 'pg-cards', type: 'card', key: 'pg_dead_tuples' },

  // Synapse charts
  { id: 'chart-cpu', label: 'CPU Usage (%)', section: 'synapse-charts', type: 'chart', metric: 'cpu', color: '#6366f1', unit: '%' },
  { id: 'chart-memory', label: 'Memory (MB)', section: 'synapse-charts', type: 'chart', metric: 'memory', color: '#22c55e', unit: ' MB' },
  { id: 'chart-requests', label: 'Request Rate (req/s)', section: 'synapse-charts', type: 'chart', metric: 'requests', color: '#f59e0b', unit: '/s' },
  { id: 'chart-response', label: 'Avg Response Time (s)', section: 'synapse-charts', type: 'chart', metric: 'response_time', color: '#ef4444', unit: 's' },
  { id: 'chart-db-txn', label: 'DB Transaction Rate (/s)', section: 'synapse-charts', type: 'chart', metric: 'db_txn', color: '#8b5cf6', unit: '/s' },
  { id: 'chart-cache', label: 'Cache Hit Ratio (%)', section: 'synapse-charts', type: 'chart', metric: 'cache_hit', color: '#06b6d4', unit: '%' },

  // PG charts
  { id: 'chart-pg-conn', label: 'Connections', section: 'pg-charts', type: 'chart', metric: 'pg_connections', color: '#ec4899', unit: '' },
  { id: 'chart-pg-size', label: 'Database Size (MB)', section: 'pg-charts', type: 'chart', metric: 'pg_size', color: '#14b8a6', unit: ' MB' },
  { id: 'chart-pg-cache', label: 'Cache Hit Ratio (%)', section: 'pg-charts', type: 'chart', metric: 'pg_cache_hit', color: '#f97316', unit: '%' },
  { id: 'chart-pg-txn', label: 'Transaction Rate (/s)', section: 'pg-charts', type: 'chart', metric: 'pg_txn_rate', color: '#a855f7', unit: '/s' },
];

const sectionGrids = {
  'synapse-cards': 'grid-synapse-cards',
  'pg-cards': 'grid-pg-cards',
  'synapse-charts': 'grid-synapse-charts',
  'pg-charts': 'grid-pg-charts',
};

// --- Layout persistence ---
function getDefaultLayout() {
  return allWidgets.map(w => ({ id: w.id, visible: true }));
}

function loadLayout() {
  try {
    const saved = localStorage.getItem('dashboard-layout');
    if (saved) {
      const layout = JSON.parse(saved);
      // Merge: add any new widgets not in saved layout
      const savedIds = new Set(layout.map(l => l.id));
      allWidgets.forEach(w => {
        if (!savedIds.has(w.id)) layout.push({ id: w.id, visible: true });
      });
      return layout.filter(l => allWidgets.some(w => w.id === l.id));
    }
  } catch(e) {}
  return getDefaultLayout();
}

function saveLayout() {
  const layout = [];
  Object.keys(sectionGrids).forEach(section => {
    const grid = document.getElementById(sectionGrids[section]);
    const items = grid.querySelectorAll('.widget');
    items.forEach(el => {
      layout.push({ id: el.dataset.widgetId, visible: true });
    });
  });
  // Add hidden widgets
  allWidgets.forEach(w => {
    if (!layout.some(l => l.id === w.id)) {
      layout.push({ id: w.id, visible: false });
    }
  });
  localStorage.setItem('dashboard-layout', JSON.stringify(layout));
}

// --- Theme ---
let currentTheme = localStorage.getItem('theme') || 'dark';
document.documentElement.setAttribute('data-theme', currentTheme);

function applyTheme(theme) {
  currentTheme = theme;
  document.documentElement.setAttribute('data-theme', theme);
  document.getElementById('theme-btn').innerHTML = theme === 'dark' ? '&#9790;' : '&#9788;';
  localStorage.setItem('theme', theme);
  if (Object.keys(chartInstances).length > 0) updateChartColors();
}
function toggleTheme() { applyTheme(currentTheme === 'dark' ? 'light' : 'dark'); }
function getGridColor() { return currentTheme === 'dark' ? '#2a2d3a' : '#e2e5ed'; }
function getTickColor() { return currentTheme === 'dark' ? '#8b8fa3' : '#64687a'; }
function getTooltipBg() { return currentTheme === 'dark' ? '#1a1d27' : '#ffffff'; }
function getTooltipBorder() { return currentTheme === 'dark' ? '#3a3d4a' : '#d0d3dc'; }
function getTooltipText() { return currentTheme === 'dark' ? '#e1e4ed' : '#1a1d27'; }

// --- Edit mode ---
let editMode = false;
function toggleSection(id) {
  const content = document.getElementById('content-' + id);
  const wrap = content.previousElementSibling;
  content.classList.toggle('collapsed');
  wrap.classList.toggle('collapsed');
  // Save collapsed state
  const collapsed = JSON.parse(localStorage.getItem('collapsed-sections') || '{}');
  collapsed[id] = content.classList.contains('collapsed');
  localStorage.setItem('collapsed-sections', JSON.stringify(collapsed));
}

function restoreCollapsed() {
  const collapsed = JSON.parse(localStorage.getItem('collapsed-sections') || '{}');
  Object.keys(collapsed).forEach(id => {
    if (collapsed[id]) {
      const content = document.getElementById('content-' + id);
      const wrap = content ? content.previousElementSibling : null;
      if (content) content.classList.add('collapsed');
      if (wrap) wrap.classList.add('collapsed');
    }
  });
}

function toggleEdit() {
  editMode = !editMode;
  document.body.classList.toggle('edit-mode', editMode);
  document.getElementById('edit-btn').classList.toggle('active', editMode);
  document.getElementById('add-btn').style.display = editMode ? 'flex' : 'none';
  document.getElementById('reset-btn').style.display = editMode ? 'flex' : 'none';
  if (!editMode) { saveLayout(); closeAddPanel(); }
}

function resetLayout() {
  localStorage.removeItem('dashboard-layout');
  renderWidgets();
  if (editMode) toggleEdit();
}

// --- Add panel ---
function openAddPanel() {
  renderAddList();
  document.getElementById('add-panel').classList.add('open');
  document.getElementById('overlay').classList.add('open');
}
function closeAddPanel() {
  document.getElementById('add-panel').classList.remove('open');
  document.getElementById('overlay').classList.remove('open');
}
function renderAddList() {
  const list = document.getElementById('add-list');
  const visibleIds = new Set();
  Object.keys(sectionGrids).forEach(section => {
    document.getElementById(sectionGrids[section]).querySelectorAll('.widget').forEach(el => {
      visibleIds.add(el.dataset.widgetId);
    });
  });
  const hidden = allWidgets.filter(w => !visibleIds.has(w.id));
  if (hidden.length === 0) {
    list.innerHTML = '<p style="color:var(--muted);font-size:13px">All widgets are visible.</p>';
    return;
  }
  list.innerHTML = hidden.map(w =>
    '<div class="add-item"><span>' + w.label + '</span><button onclick="addWidget(\'' + w.id + '\')">Add</button></div>'
  ).join('');
}

function addWidget(id) {
  const w = allWidgets.find(x => x.id === id);
  if (!w) return;
  const grid = document.getElementById(sectionGrids[w.section]);
  const el = createWidgetElement(w);
  grid.appendChild(el);
  if (w.type === 'chart') {
    createChartInstance(w);
    fetchChart(w.metric, w.id);
  }
  saveLayout();
  renderAddList();
}

// --- Widget rendering ---
const chartInstances = {};

function createWidgetElement(w) {
  const el = document.createElement('div');
  el.className = 'widget' + (w.type === 'chart' ? ' chart-widget' : '');
  el.dataset.widgetId = w.id;
  el.innerHTML = '<span class="drag-handle">&#9776;</span><button class="remove-btn" onclick="removeWidget(this)">&times;</button>';
  if (w.type === 'card') {
    el.innerHTML += '<div class="label">' + w.label + '</div><div class="value" id="val-' + w.id + '">--</div>';
    if (w.subKey) el.innerHTML += '<div class="sub" id="sub-' + w.id + '"></div>';
  } else {
    el.innerHTML += '<h3>' + w.label + '</h3><canvas id="canvas-' + w.id + '"></canvas>';
  }
  return el;
}

function removeWidget(btn) {
  const el = btn.closest('.widget');
  const wid = el.dataset.widgetId;
  const w = allWidgets.find(x => x.id === wid);
  if (w && w.type === 'chart' && chartInstances[wid]) {
    chartInstances[wid].destroy();
    delete chartInstances[wid];
  }
  el.remove();
  saveLayout();
}

function fmtTime(ms) {
  const d = new Date(ms);
  return d.getHours().toString().padStart(2,'0') + ':' + d.getMinutes().toString().padStart(2,'0');
}

function tooltipConfig() {
  return {
    enabled: true, mode: 'index', intersect: false,
    backgroundColor: getTooltipBg(), borderColor: getTooltipBorder(), borderWidth: 1,
    titleColor: getTooltipText(), bodyColor: getTooltipText(),
    titleFont: { size: 12, weight: '600' }, bodyFont: { size: 13 },
    padding: 12, cornerRadius: 8, displayColors: true,
    callbacks: {
      title: function(items) {
        if (!items.length) return '';
        const d = new Date(items[0].parsed.x);
        return d.toLocaleDateString() + ' ' + d.toLocaleTimeString();
      },
      label: function(ctx) {
        const w = allWidgets.find(x => 'canvas-' + x.id === ctx.chart.canvas.id);
        const unit = w ? w.unit : '';
        return ' ' + ctx.parsed.y.toFixed(2) + unit;
      }
    }
  };
}

function createChartInstance(w) {
  const canvas = document.getElementById('canvas-' + w.id);
  if (!canvas) return;
  chartInstances[w.id] = new Chart(canvas.getContext('2d'), {
    type: 'line',
    data: { datasets: [{ data: [] }] },
    options: {
      responsive: true, maintainAspectRatio: false,
      animation: { duration: 300 },
      interaction: { mode: 'index', intersect: false },
      layout: { padding: 0 },
      plugins: { legend: { display: false }, tooltip: tooltipConfig() },
      scales: {
        x: {
          type: 'linear', grid: { color: getGridColor() },
          ticks: { color: getTickColor(), maxTicksLimit: 6, font: { size: 11 }, callback: function(v) { return fmtTime(v); } },
          offset: false
        },
        y: {
          grid: { color: getGridColor() },
          ticks: { color: getTickColor(), font: { size: 11 } },
          beginAtZero: true
        }
      },
      elements: {
        point: { radius: 0, hoverRadius: 5, hoverBackgroundColor: w.color, hoverBorderColor: '#fff', hoverBorderWidth: 2 },
        line: { borderWidth: 2, borderColor: w.color, backgroundColor: w.color + '20', fill: true, tension: 0.3 }
      }
    }
  });
}

function updateChartColors() {
  Object.keys(chartInstances).forEach(id => {
    const c = chartInstances[id];
    if (!c) return;
    c.options.scales.x.grid.color = getGridColor();
    c.options.scales.y.grid.color = getGridColor();
    c.options.scales.x.ticks.color = getTickColor();
    c.options.scales.y.ticks.color = getTickColor();
    c.options.plugins.tooltip = tooltipConfig();
    c.update('none');
  });
}

// --- Render all widgets from layout ---
let sortables = [];

function renderWidgets() {
  // Destroy existing charts
  Object.keys(chartInstances).forEach(id => { chartInstances[id].destroy(); delete chartInstances[id]; });
  sortables.forEach(s => s.destroy());
  sortables = [];

  // Clear grids
  Object.values(sectionGrids).forEach(gid => { document.getElementById(gid).innerHTML = ''; });

  const layout = loadLayout();
  layout.forEach(item => {
    if (!item.visible) return;
    const w = allWidgets.find(x => x.id === item.id);
    if (!w) return;
    const grid = document.getElementById(sectionGrids[w.section]);
    grid.appendChild(createWidgetElement(w));
  });

  // Create chart instances
  allWidgets.filter(w => w.type === 'chart').forEach(w => {
    if (document.getElementById('canvas-' + w.id)) createChartInstance(w);
  });

  // Init SortableJS on each grid
  Object.values(sectionGrids).forEach(gid => {
    const grid = document.getElementById(gid);
    sortables.push(new Sortable(grid, {
      animation: 200,
      handle: '.drag-handle',
      ghostClass: 'sortable-ghost',
      chosenClass: 'sortable-chosen',
      disabled: false,
      onEnd: function() { saveLayout(); }
    }));
  });
}

// --- Data fetching ---
let currentRange = '1h';

async function fetchStats() {
  try {
    const resp = await fetch('/api/stats');
    const d = await resp.json();
    const setVal = (id, val) => { const el = document.getElementById('val-' + id); if (el) el.textContent = val; };
    const setSub = (id, val) => { const el = document.getElementById('sub-' + id); if (el) el.textContent = val; };

    setVal('uptime', d.uptime);
    setVal('cpu', d.cpu);
    setVal('memory', d.memory);
    setVal('request_rate', d.request_rate);
    setVal('avg_resp_time', d.avg_resp_time);
    setVal('dau', d.dau);
    setVal('rooms', d.rooms);
    setVal('events_sent', d.events_sent);
    setVal('cache_hit_ratio', d.cache_hit_ratio);
    setVal('db_txn_rate', d.db_txn_rate);
    setVal('fds', d.open_fds); setSub('fds', 'of ' + d.max_fds + ' max');
    setVal('fed', d.fed_pdus_in + ' in'); setSub('fed', d.fed_pdus_out + ' out');
    document.getElementById('meta').textContent = 'Synapse ' + d.version + ' | ' + d.python_version;
    setVal('pg_uptime', d.pg_uptime);
    setVal('pg_connections', d.pg_connections);
    setVal('pg_active_queries', d.pg_active_queries);
    setVal('pg_db_size', d.pg_db_size);
    setVal('pg_cache_hit', d.pg_cache_hit);
    setVal('pg_txn_rate', d.pg_txn_rate);
    setVal('pg_dead_tuples', d.pg_dead_tuples);
  } catch(e) { console.error('stats fetch failed:', e); }
}

async function fetchChart(metric, chartId) {
  try {
    const c = chartInstances[chartId];
    if (!c) return;
    const resp = await fetch('/api/chart?metric=' + metric + '&range=' + currentRange);
    const points = await resp.json();
    const data = (points || []).map(p => ({ x: p.t, y: p.y }));
    c.data.datasets[0].data = data;
    if (data.length > 0) {
      c.options.scales.x.min = data[0].x;
      c.options.scales.x.max = data[data.length - 1].x;
    }
    c.update('none');
  } catch(e) { console.error('chart fetch failed:', e); }
}

function fetchAllCharts() {
  allWidgets.filter(w => w.type === 'chart').forEach(w => {
    if (chartInstances[w.id]) fetchChart(w.metric, w.id);
  });
}

function setRange(r) {
  currentRange = r;
  document.querySelectorAll('.controls button').forEach(b => b.classList.remove('active'));
  document.getElementById('btn-' + r).classList.add('active');
  fetchAllCharts();
}

function refresh() { fetchStats(); fetchAllCharts(); }

// --- Init ---
document.getElementById('theme-btn').innerHTML = currentTheme === 'dark' ? '&#9790;' : '&#9788;';
renderWidgets();
restoreCollapsed();
refresh();
setInterval(refresh, 15000);
</script>
</body>
</html>
`
