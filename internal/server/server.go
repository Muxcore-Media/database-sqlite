package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/Muxcore-Media/database-sqlite/internal/db"
)

type Server struct {
	database      *db.Database
	execCount     atomic.Int64
	queryCount    atomic.Int64
	migrateCount  atomic.Int64
}

func New(d *db.Database) *Server {
	return &Server{database: d}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/db/exec", s.handleExec)
	mux.HandleFunc("/v1/db/query", s.handleQuery)
	mux.HandleFunc("/v1/db/transaction", s.handleTransaction)
	mux.HandleFunc("/v1/db/migrate", s.handleMigrate)
	mux.HandleFunc("/v1/db/rollback", s.handleRollback)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/metrics", s.handleMetrics)
	return mux
}

func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Query string `json:"query"`
		Args  []any  `json:"args"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	n, err := s.database.Exec(r.Context(), req.Query, req.Args...)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err), http.StatusBadRequest)
		return
	}
	s.execCount.Add(1)
	writeJSON(w, http.StatusOK, map[string]any{"rows_affected": n})
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Query string `json:"query"`
		Args  []any  `json:"args"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	rows, err := s.database.Query(r.Context(), req.Query, req.Args...)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err), http.StatusBadRequest)
		return
	}
	defer rows.Close()

	var results []map[string]any
	cols := guessColumns(req.Query)
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"scan: %s"}`, err), http.StatusInternalServerError)
			return
		}
		row := make(map[string]any)
		for i, col := range cols {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	s.queryCount.Add(1)
	writeJSON(w, http.StatusOK, map[string]any{"rows": results, "count": len(results)})
}

func (s *Server) handleTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Statements []struct {
			Query string `json:"query"`
			Args  []any  `json:"args"`
		} `json:"statements"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 10<<20)).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	err := s.database.Transaction(r.Context(), func(tx *db.Tx) error {
		for _, stmt := range req.Statements {
			if _, err := tx.Exec(r.Context(), stmt.Query, stmt.Args...); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleMigrate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Migrations []struct {
			Version int    `json:"version"`
			Name    string `json:"name"`
			Up      string `json:"up"`
			Down    string `json:"down"`
		} `json:"migrations"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 10<<20)).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	migs := make([]db.Migration, len(req.Migrations))
	for i, m := range req.Migrations {
		migs[i] = db.Migration{
			Version: m.Version,
			Name:    m.Name,
			Up:      m.Up,
			Down:    m.Down,
		}
	}

	if err := s.database.Migrate(r.Context(), migs); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err), http.StatusBadRequest)
		return
	}
	s.migrateCount.Add(1)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TargetVersion int `json:"target_version"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if err := s.database.Rollback(r.Context(), req.TargetVersion); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.database.Health(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unhealthy", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString("# HELP db_exec_total Total database exec operations\n")
	b.WriteString("# TYPE db_exec_total counter\n")
	fmt.Fprintf(&b, "db_exec_total %d\n", s.execCount.Load())
	b.WriteString("# HELP db_query_total Total database query operations\n")
	b.WriteString("# TYPE db_query_total counter\n")
	fmt.Fprintf(&b, "db_query_total %d\n", s.queryCount.Load())
	b.WriteString("# HELP db_migrate_total Total database migration operations\n")
	b.WriteString("# TYPE db_migrate_total counter\n")
	fmt.Fprintf(&b, "db_migrate_total %d\n", s.migrateCount.Load())
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.Write([]byte(b.String()))
}

func guessColumns(query string) []string {
	upper := strings.ToUpper(strings.TrimSpace(query))
	selectIdx := strings.Index(upper, "SELECT ")
	if selectIdx == -1 {
		return []string{"col"}
	}
	fromIdx := strings.Index(upper[selectIdx:], " FROM ")
	if fromIdx == -1 {
		return []string{"col"}
	}
	colsPart := upper[selectIdx+7 : selectIdx+fromIdx]
	colsPart = strings.TrimSpace(colsPart)

	if colsPart == "*" {
		return []string{"col"}
	}

	var cols []string
	for _, part := range strings.Split(colsPart, ",") {
		part = strings.TrimSpace(part)
		if idx := strings.LastIndex(part, " AS "); idx != -1 {
			part = strings.TrimSpace(part[idx+4:])
		} else if idx := strings.LastIndex(part, "."); idx != -1 {
			part = part[idx+1:]
		}
		cols = append(cols, strings.TrimSpace(part))
	}
	if len(cols) == 0 {
		return []string{"col"}
	}
	return cols
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
