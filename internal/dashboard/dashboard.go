package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/Melphins/msync/internal/config"
	"github.com/Melphins/msync/internal/status"
)

// Server serves the local msync dashboard.
type Server struct {
	cfg *config.Config
}

// NewServer creates a dashboard server for the given configuration.
func NewServer(cfg *config.Config) *Server {
	return &Server{cfg: cfg}
}

// Handler returns the dashboard HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/status", s.handleStatus)
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(indexHTML))
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	targets := make([]targetSummary, 0, len(s.cfg.Targets))
	for name, target := range s.cfg.Targets {
		targets = append(targets, targetSummary{
			Name:         name,
			Type:         string(target.Type),
			Adapter:      target.Adapter,
			MigrationDir: target.MigrationDir,
		})
	}
	sort.Slice(targets, func(i, j int) bool {
		return targets[i].Name < targets[j].Name
	})

	writeJSON(w, http.StatusOK, configResponse{
		Project: projectSummary{
			Name: s.cfg.Project.Name,
		},
		Local: localSummary{
			Adapter:        s.cfg.Local.Adapter,
			MigrationDir:   s.cfg.Local.MigrationDir,
			MigrationTable: s.cfg.Local.MigrationTable,
		},
		Targets: targets,
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	target := r.URL.Query().Get("target")
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	result, err := status.NewRunner(s.cfg).Check(ctx, target)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, newStatusResponse(result))
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, errorResponse{Error: message})
}

// Serve starts the dashboard HTTP server and blocks until the server exits.
func Serve(ctx context.Context, cfg *config.Config, host string, port int) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	server := &http.Server{
		Addr:              addr,
		Handler:           NewServer(cfg).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return ctx.Err()
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

type configResponse struct {
	Project projectSummary  `json:"project"`
	Local   localSummary    `json:"local"`
	Targets []targetSummary `json:"targets"`
}

type projectSummary struct {
	Name string `json:"name"`
}

type localSummary struct {
	Adapter        string `json:"adapter"`
	MigrationDir   string `json:"migration_dir"`
	MigrationTable string `json:"migration_table"`
}

type targetSummary struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Adapter      string `json:"adapter"`
	MigrationDir string `json:"migration_dir"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type statusResponse struct {
	LocalVersion      string             `json:"local_version"`
	TargetVersion     string             `json:"target_version"`
	LocalCount        int                `json:"local_count"`
	TargetCount       int                `json:"target_count"`
	Status            string             `json:"status"`
	PendingMigrations []migrationSummary `json:"pending_migrations"`
	Notes             string             `json:"notes"`
}

type migrationSummary struct {
	Version  string `json:"version"`
	Name     string `json:"name"`
	FilePath string `json:"file_path"`
}

func newStatusResponse(result *status.CheckResult) statusResponse {
	pending := make([]migrationSummary, 0, len(result.PendingMigrations))
	for _, migration := range result.PendingMigrations {
		pending = append(pending, migrationSummary{
			Version:  migration.Version,
			Name:     migration.Name,
			FilePath: migration.FilePath,
		})
	}

	return statusResponse{
		LocalVersion:      result.LocalVersion,
		TargetVersion:     result.TargetVersion,
		LocalCount:        result.LocalCount,
		TargetCount:       result.TargetCount,
		Status:            string(result.Status),
		PendingMigrations: pending,
		Notes:             result.Notes,
	}
}

const indexHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>msync dashboard</title>
  <style>
    :root {
      color-scheme: light;
      --bg: #f7f8fa;
      --panel: #ffffff;
      --text: #1d2430;
      --muted: #667085;
      --line: #d9dee7;
      --accent: #1b6f8f;
      --good: #16833a;
      --warn: #a05a00;
      --bad: #b42318;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      background: var(--bg);
      color: var(--text);
      font: 14px/1.45 system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    }
    header {
      background: var(--panel);
      border-bottom: 1px solid var(--line);
      padding: 18px 24px;
    }
    main {
      max-width: 1120px;
      margin: 0 auto;
      padding: 22px 24px 36px;
    }
    h1, h2 {
      margin: 0;
      font-weight: 650;
      letter-spacing: 0;
    }
    h1 { font-size: 22px; }
    h2 { font-size: 15px; }
    .subtle {
      color: var(--muted);
      margin-top: 4px;
    }
    .toolbar {
      display: flex;
      gap: 12px;
      align-items: end;
      justify-content: space-between;
      margin-bottom: 18px;
      flex-wrap: wrap;
    }
    label {
      display: grid;
      gap: 6px;
      color: var(--muted);
      font-size: 12px;
    }
    select, button {
      min-height: 36px;
      border: 1px solid var(--line);
      border-radius: 6px;
      background: var(--panel);
      color: var(--text);
      padding: 0 10px;
      font: inherit;
    }
    button {
      background: var(--accent);
      border-color: var(--accent);
      color: #fff;
      cursor: pointer;
    }
    button:disabled {
      cursor: not-allowed;
      opacity: 0.65;
    }
    .grid {
      display: grid;
      grid-template-columns: repeat(4, minmax(0, 1fr));
      gap: 12px;
      margin-bottom: 18px;
    }
    .panel {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
      padding: 16px;
    }
    .metric {
      color: var(--muted);
      font-size: 12px;
      margin-bottom: 8px;
    }
    .value {
      font-size: 24px;
      font-weight: 700;
      overflow-wrap: anywhere;
    }
    .status {
      display: inline-flex;
      min-height: 28px;
      align-items: center;
      border-radius: 999px;
      padding: 0 10px;
      font-weight: 700;
      text-transform: uppercase;
      font-size: 12px;
    }
    .status.synced { background: #e8f6ee; color: var(--good); }
    .status.out_of_sync { background: #fff3df; color: var(--warn); }
    .status.ahead { background: #fdebea; color: var(--bad); }
    table {
      width: 100%;
      border-collapse: collapse;
      margin-top: 12px;
    }
    th, td {
      border-bottom: 1px solid var(--line);
      padding: 10px 8px;
      text-align: left;
      vertical-align: top;
    }
    th {
      color: var(--muted);
      font-size: 12px;
      font-weight: 650;
    }
    td {
      overflow-wrap: anywhere;
    }
    .empty, .error {
      border: 1px solid var(--line);
      border-radius: 8px;
      background: var(--panel);
      padding: 14px 16px;
      color: var(--muted);
    }
    .error {
      border-color: #f3b4ae;
      color: var(--bad);
      background: #fff7f6;
    }
    @media (max-width: 760px) {
      header { padding: 16px; }
      main { padding: 16px; }
      .grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
      .toolbar { align-items: stretch; }
      label, button { width: 100%; }
    }
  </style>
</head>
<body>
  <header>
    <h1>msync dashboard</h1>
    <div class="subtle" id="project">Loading project...</div>
  </header>
  <main>
    <div class="toolbar">
      <label>
        Target
        <select id="target"></select>
      </label>
      <button id="refresh" type="button">Refresh</button>
    </div>
    <div id="message"></div>
    <section class="grid" aria-live="polite">
      <div class="panel">
        <div class="metric">Status</div>
        <div id="syncStatus" class="status">Unknown</div>
      </div>
      <div class="panel">
        <div class="metric">Local version</div>
        <div id="localVersion" class="value">-</div>
      </div>
      <div class="panel">
        <div class="metric">Target version</div>
        <div id="targetVersion" class="value">-</div>
      </div>
      <div class="panel">
        <div class="metric">Pending</div>
        <div id="pendingCount" class="value">-</div>
      </div>
    </section>
    <section class="panel">
      <h2>Pending migrations</h2>
      <div id="pending"></div>
    </section>
  </main>
  <script>
    const state = { targets: [] };
    const targetEl = document.getElementById('target');
    const refreshEl = document.getElementById('refresh');
    const messageEl = document.getElementById('message');

    async function loadConfig() {
      const res = await fetch('/api/config');
      const cfg = await res.json();
      if (!res.ok) throw new Error(cfg.error || 'Unable to load config');
      document.getElementById('project').textContent = cfg.project.name || 'Local project';
      state.targets = cfg.targets || [];
      targetEl.innerHTML = state.targets.map(t => '<option value="' + escapeHtml(t.name) + '">' + escapeHtml(t.name) + ' (' + escapeHtml(t.adapter) + ')</option>').join('');
    }

    async function loadStatus() {
      messageEl.innerHTML = '';
      refreshEl.disabled = true;
      try {
        const target = targetEl.value;
        const res = await fetch('/api/status?target=' + encodeURIComponent(target));
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'Status check failed');
        renderStatus(data);
      } catch (err) {
        messageEl.innerHTML = '<div class="error">' + escapeHtml(err.message) + '</div>';
      } finally {
        refreshEl.disabled = false;
      }
    }

    function renderStatus(data) {
      const statusEl = document.getElementById('syncStatus');
      statusEl.className = 'status ' + data.status;
      statusEl.textContent = data.status || 'unknown';
      document.getElementById('localVersion').textContent = data.local_version || 'none';
      document.getElementById('targetVersion').textContent = data.target_version || 'none';
      document.getElementById('pendingCount').textContent = String((data.pending_migrations || []).length);

      const pending = data.pending_migrations || [];
      if (pending.length === 0) {
        document.getElementById('pending').innerHTML = '<div class="empty">No pending migrations.</div>';
        return;
      }
      const rows = pending.map(m => '<tr><td>' + escapeHtml(m.version) + '</td><td>' + escapeHtml(m.name) + '</td><td>' + escapeHtml(m.file_path) + '</td></tr>').join('');
      document.getElementById('pending').innerHTML = '<table><thead><tr><th>Version</th><th>Name</th><th>File</th></tr></thead><tbody>' + rows + '</tbody></table>';
    }

    function escapeHtml(value) {
      return String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
    }

    refreshEl.addEventListener('click', loadStatus);
    targetEl.addEventListener('change', loadStatus);

    loadConfig().then(loadStatus).catch(err => {
      messageEl.innerHTML = '<div class="error">' + escapeHtml(err.message) + '</div>';
    });
  </script>
</body>
</html>
`
