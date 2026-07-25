package api

import (
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

type flowEvent struct {
	At         time.Time `json:"at"`
	Method     string    `json:"method"`
	Route      string    `json:"route"`
	Group      string    `json:"group"`
	Status     int       `json:"status"`
	DurationMs int64     `json:"duration_ms"`
}

type flowGroupSummary struct {
	Group          string    `json:"group"`
	Requests       int64     `json:"requests"`
	Errors         int64     `json:"errors"`
	AverageLatency int64     `json:"avg_latency_ms"`
	LastStatus     int       `json:"last_status"`
	LastRoute      string    `json:"last_route"`
	LastSeen       time.Time `json:"last_seen"`
}

type flowTotals struct {
	Requests int64 `json:"requests"`
	Errors   int64 `json:"errors"`
}

type flowTemplate struct {
	Name  string   `json:"name"`
	Steps []string `json:"steps"`
}

type flowDashboardPayload struct {
	Service        string             `json:"service"`
	GeneratedAt    time.Time          `json:"generated_at"`
	Uptime         string             `json:"uptime"`
	Totals         flowTotals         `json:"totals"`
	Groups         []flowGroupSummary `json:"groups"`
	Recent         []flowEvent        `json:"recent"`
	SuggestedFlows []flowTemplate     `json:"suggested_flows"`
	Notes          []string           `json:"notes"`
}

type flowGroupAggregate struct {
	Requests       int64
	Errors         int64
	TotalLatencyMs int64
	LastStatus     int
	LastRoute      string
	LastSeen       time.Time
}

type flowTracker struct {
	mu        sync.RWMutex
	startedAt time.Time
	totals    flowTotals
	groups    map[string]*flowGroupAggregate
	recent    []flowEvent
	maxRecent int
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func newFlowTracker(maxRecent int) *flowTracker {
	if maxRecent <= 0 {
		maxRecent = 300
	}
	return &flowTracker{
		startedAt: time.Now().UTC(),
		groups:    make(map[string]*flowGroupAggregate),
		recent:    make([]flowEvent, 0, maxRecent),
		maxRecent: maxRecent,
	}
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.statusCode = code
	sr.ResponseWriter.WriteHeader(code)
}

func (sr *statusRecorder) Write(b []byte) (int, error) {
	if sr.statusCode == 0 {
		sr.statusCode = http.StatusOK
	}
	return sr.ResponseWriter.Write(b)
}

func (ft *flowTracker) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now().UTC()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.statusCode == 0 {
			rec.statusCode = http.StatusOK
		}

		route := r.URL.Path
		if rc := chi.RouteContext(r.Context()); rc != nil {
			if rp := strings.TrimSpace(rc.RoutePattern()); rp != "" {
				route = rp
			}
		}

		event := flowEvent{
			At:         start,
			Method:     r.Method,
			Route:      route,
			Group:      groupFromPath(route),
			Status:     rec.statusCode,
			DurationMs: time.Since(start).Milliseconds(),
		}
		ft.record(event)
	})
}

func (ft *flowTracker) record(event flowEvent) {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	ft.totals.Requests++
	if event.Status >= http.StatusBadRequest {
		ft.totals.Errors++
	}

	agg := ft.groups[event.Group]
	if agg == nil {
		agg = &flowGroupAggregate{}
		ft.groups[event.Group] = agg
	}
	agg.Requests++
	if event.Status >= http.StatusBadRequest {
		agg.Errors++
	}
	agg.TotalLatencyMs += event.DurationMs
	agg.LastStatus = event.Status
	agg.LastRoute = event.Route
	agg.LastSeen = event.At

	ft.recent = append(ft.recent, event)
	if len(ft.recent) > ft.maxRecent {
		ft.recent = ft.recent[len(ft.recent)-ft.maxRecent:]
	}
}

func (ft *flowTracker) Snapshot() flowDashboardPayload {
	ft.mu.RLock()
	defer ft.mu.RUnlock()

	groups := make([]flowGroupSummary, 0, len(ft.groups))
	for group, agg := range ft.groups {
		avgLatency := int64(0)
		if agg.Requests > 0 {
			avgLatency = agg.TotalLatencyMs / agg.Requests
		}
		groups = append(groups, flowGroupSummary{
			Group:          group,
			Requests:       agg.Requests,
			Errors:         agg.Errors,
			AverageLatency: avgLatency,
			LastStatus:     agg.LastStatus,
			LastRoute:      agg.LastRoute,
			LastSeen:       agg.LastSeen,
		})
	}

	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Requests == groups[j].Requests {
			return groups[i].Group < groups[j].Group
		}
		return groups[i].Requests > groups[j].Requests
	})

	recent := make([]flowEvent, 0, len(ft.recent))
	for i := len(ft.recent) - 1; i >= 0; i-- {
		recent = append(recent, ft.recent[i])
	}

	return flowDashboardPayload{
		Service:        "FINIX-backend",
		GeneratedAt:    time.Now().UTC(),
		Uptime:         time.Since(ft.startedAt).Round(time.Second).String(),
		Totals:         ft.totals,
		Groups:         groups,
		Recent:         recent,
		SuggestedFlows: curatedFlows(),
		Notes: []string{
			"This dashboard reports live API traffic grouped by feature area.",
			"Use it with the Flutter app running to inspect onboarding, auth, and transaction flow paths.",
			"No sensitive payload fields are stored in flow telemetry.",
		},
	}
}

func groupFromPath(path string) string {
	p := strings.ToLower(strings.TrimSpace(path))
	if p == "" || p == "/" {
		return "system"
	}

	switch {
	case strings.HasPrefix(p, "/healthz"), strings.HasPrefix(p, "/readyz"), strings.HasPrefix(p, "/v1/system"), strings.HasPrefix(p, "/ops"):
		return "system"
	case strings.HasPrefix(p, "/v1/auth"):
		return "auth"
	case strings.HasPrefix(p, "/v1/dashboard"):
		return "dashboard"
	case strings.HasPrefix(p, "/v1/transactions"):
		return "transactions"
	case strings.HasPrefix(p, "/v1/goals"):
		return "goals"
	case strings.HasPrefix(p, "/v1/portfolio"):
		return "portfolio"
	case strings.HasPrefix(p, "/v1/security"):
		return "security"
	case strings.HasPrefix(p, "/v1/insights"):
		return "insights"
	case strings.HasPrefix(p, "/v1/tax"):
		return "tax"
	case strings.HasPrefix(p, "/v1/chatbot"):
		return "chatbot"
	case strings.HasPrefix(p, "/v1/settings"):
		return "settings"
	case strings.HasPrefix(p, "/v1/aiml"):
		return "aiml"
	case strings.HasPrefix(p, "/v1/simulations"):
		return "simulation"
	case strings.HasPrefix(p, "/v1/accounts"), strings.HasPrefix(p, "/v1/aggregator"):
		return "accounts"
	default:
		return "other"
	}
}

func curatedFlows() []flowTemplate {
	return []flowTemplate{
		{
			Name: "Auth bootstrap",
			Steps: []string{
				"POST /v1/auth/register",
				"POST /v1/auth/ekyc/verify",
				"POST /v1/auth/biometric/register",
				"POST /v1/auth/login/challenge",
				"POST /v1/auth/login/verify",
			},
		},
		{
			Name: "Dashboard and health",
			Steps: []string{
				"GET /v1/dashboard",
				"GET /v1/health-score",
				"GET /v1/transactions/history",
			},
		},
		{
			Name: "Transaction risk flow",
			Steps: []string{
				"POST /v1/transactions/initiate",
				"POST /v1/transactions/override",
				"POST /v1/security/emergency-freeze",
				"POST /v1/security/unfreeze",
			},
		},
		{
			Name: "Portfolio and insights",
			Steps: []string{
				"GET /v1/portfolio/summary",
				"GET /v1/portfolio/investments",
				"GET /v1/insights/feed",
				"GET /v1/tax/dashboard",
			},
		},
	}
}

func (api *API) flowDashboardData(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, api.flow.Snapshot())
}

func (api *API) flowDashboardPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html>
<html>
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width,initial-scale=1" />
  <title>FINIX Flow Dashboard</title>
  <style>
    :root { color-scheme: light; }
    body { font-family: Segoe UI, Arial, sans-serif; margin: 20px; color: #1f2937; background: #f8fafc; }
    h1 { margin: 0 0 8px; }
    .meta { color: #4b5563; margin-bottom: 16px; }
    .cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 12px; margin-bottom: 16px; }
    .card { background: #ffffff; border-radius: 10px; padding: 12px; box-shadow: 0 2px 8px rgba(0,0,0,.08); }
    .table { width: 100%; border-collapse: collapse; background: #ffffff; border-radius: 10px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,.08); }
    th, td { padding: 10px; border-bottom: 1px solid #e5e7eb; font-size: 14px; text-align: left; }
    th { background: #eef2ff; color: #111827; }
    .section { margin-top: 18px; }
    code { background: #f3f4f6; padding: 2px 6px; border-radius: 6px; }
    ul { margin-top: 6px; }
  </style>
</head>
<body>
  <h1>FINIX Backend Flow Dashboard</h1>
  <div class="meta" id="meta">Loading...</div>

  <div class="cards">
    <div class="card"><strong>Total requests</strong><div id="totalRequests">-</div></div>
    <div class="card"><strong>Total errors</strong><div id="totalErrors">-</div></div>
    <div class="card"><strong>Uptime</strong><div id="uptime">-</div></div>
    <div class="card"><strong>Last refresh</strong><div id="lastRefresh">-</div></div>
  </div>

  <div class="section">
    <h2>Traffic by Group</h2>
    <table class="table" id="groupsTable">
      <thead><tr><th>Group</th><th>Requests</th><th>Errors</th><th>Avg ms</th><th>Last status</th><th>Last route</th></tr></thead>
      <tbody></tbody>
    </table>
  </div>

  <div class="section">
    <h2>Recent Requests</h2>
    <table class="table" id="recentTable">
      <thead><tr><th>Time (UTC)</th><th>Method</th><th>Route</th><th>Group</th><th>Status</th><th>Duration ms</th></tr></thead>
      <tbody></tbody>
    </table>
  </div>

  <div class="section">
    <h2>Suggested End-to-End Flows</h2>
    <div id="flows"></div>
  </div>

  <script>
    async function load() {
      const response = await fetch('/v1/system/flow-dashboard');
      const data = await response.json();

      document.getElementById('meta').textContent = data.service + ' telemetry snapshot';
      document.getElementById('totalRequests').textContent = data.totals.requests;
      document.getElementById('totalErrors').textContent = data.totals.errors;
      document.getElementById('uptime').textContent = data.uptime;
      document.getElementById('lastRefresh').textContent = new Date(data.generated_at).toLocaleTimeString();

      const groupsBody = document.querySelector('#groupsTable tbody');
      groupsBody.innerHTML = '';
      for (const g of data.groups) {
        const tr = document.createElement('tr');
	        tr.innerHTML = '<td>' + g.group + '</td>' +
	          '<td>' + g.requests + '</td>' +
	          '<td>' + g.errors + '</td>' +
	          '<td>' + g.avg_latency_ms + '</td>' +
	          '<td>' + g.last_status + '</td>' +
	          '<td><code>' + g.last_route + '</code></td>';
        groupsBody.appendChild(tr);
      }

      const recentBody = document.querySelector('#recentTable tbody');
      recentBody.innerHTML = '';
      for (const r of data.recent.slice(0, 30)) {
        const tr = document.createElement('tr');
	        tr.innerHTML = '<td>' + new Date(r.at).toISOString() + '</td>' +
	          '<td>' + r.method + '</td>' +
	          '<td><code>' + r.route + '</code></td>' +
	          '<td>' + r.group + '</td>' +
	          '<td>' + r.status + '</td>' +
	          '<td>' + r.duration_ms + '</td>';
        recentBody.appendChild(tr);
      }

      const flows = document.getElementById('flows');
      flows.innerHTML = '';
      for (const f of data.suggested_flows) {
        const card = document.createElement('div');
        card.className = 'card';
        card.style.marginBottom = '10px';
	        const list = f.steps.map(s => '<li><code>' + s + '</code></li>').join('');
	        card.innerHTML = '<strong>' + f.name + '</strong><ul>' + list + '</ul>';
        flows.appendChild(card);
      }
    }

    load();
    setInterval(load, 3000);
  </script>
</body>
</html>`))
}
