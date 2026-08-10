package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	databasev1 "github.com/Muxcore-Media/core/proto/gen/muxcore/database/v1"
	"github.com/Muxcore-Media/database-sqlite/internal/db"
)

type Server struct {
	databasev1.UnimplementedDatabaseServiceServer
	dbPtr        atomic.Pointer[db.Database]
	execCount    atomic.Int64
	queryCount   atomic.Int64
	migrateCount atomic.Int64
}

func New(d *db.Database) *Server {
	s := &Server{}
	s.dbPtr.Store(d)
	return s
}

// ReplaceDatabase swaps the backing SQLite handle. Returns the previous DB (caller should Close).
func (s *Server) ReplaceDatabase(d *db.Database) *db.Database {
	return s.dbPtr.Swap(d)
}

func (s *Server) db() *db.Database {
	return s.dbPtr.Load()
}

func (s *Server) RegisterWithGRPC(srv *grpc.Server) {
	databasev1.RegisterDatabaseServiceServer(srv, s)
}

func (s *Server) Exec(ctx context.Context, req *databasev1.ExecRequest) (*databasev1.ExecResponse, error) {
	if req.GetQuery() == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}
	args := protoArgsToAny(req.GetArgs())
	n, err := s.db().Exec(ctx, req.GetQuery(), args...)
	if err != nil {
		slog.Error("database: exec failed", "error", err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	s.execCount.Add(1)
	return &databasev1.ExecResponse{RowsAffected: n}, nil
}

func (s *Server) Query(ctx context.Context, req *databasev1.QueryRequest) (*databasev1.QueryResponse, error) {
	if req.GetQuery() == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}
	args := protoArgsToAny(req.GetArgs())
	rows, err := s.db().Query(ctx, req.GetQuery(), args...)
	if err != nil {
		slog.Error("database: query failed", "error", err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	defer rows.Close()

	cols := guessColumns(req.GetQuery())
	resp := &databasev1.QueryResponse{Columns: cols}

	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("scan: %s", err))
		}
		protoRow := &databasev1.Row{}
		for _, v := range values {
			protoRow.Values = append(protoRow.Values, anyToProtoValue(v))
		}
		resp.Rows = append(resp.Rows, protoRow)
	}

	resp.Count = int32(len(resp.Rows))
	s.queryCount.Add(1)
	return resp, nil
}

func (s *Server) Transaction(ctx context.Context, req *databasev1.TransactionRequest) (*databasev1.TransactionResponse, error) {
	stmts := req.GetStatements()
	if len(stmts) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one statement is required")
	}

	err := s.db().Transaction(ctx, func(tx *db.Tx) error {
		for _, stmt := range stmts {
			args := protoArgsToAny(stmt.GetArgs())
			if _, err := tx.Exec(ctx, stmt.GetQuery(), args...); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &databasev1.TransactionResponse{Status: "ok"}, nil
}

func (s *Server) Migrate(ctx context.Context, req *databasev1.MigrateRequest) (*databasev1.MigrateResponse, error) {
	migs := req.GetMigrations()
	converted := make([]db.Migration, len(migs))
	for i, m := range migs {
		converted[i] = db.Migration{
			Version: int(m.GetVersion()),
			Name:    m.GetName(),
			Up:      m.GetUpSql(),
			Down:    m.GetDownSql(),
		}
	}
	if err := s.db().Migrate(ctx, converted); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	s.migrateCount.Add(1)
	return &databasev1.MigrateResponse{Status: "ok"}, nil
}

func (s *Server) Rollback(ctx context.Context, req *databasev1.RollbackRequest) (*databasev1.RollbackResponse, error) {
	if err := s.db().Rollback(ctx, int(req.GetTargetVersion())); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &databasev1.RollbackResponse{Status: "ok"}, nil
}

func (s *Server) Metrics() string {
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
	return b.String()
}

func protoArgsToAny(args []*databasev1.Value) []any {
	result := make([]any, len(args))
	for i, a := range args {
		if a == nil {
			result[i] = nil
			continue
		}
		switch v := a.Kind.(type) {
		case *databasev1.Value_StringVal:
			result[i] = v.StringVal
		case *databasev1.Value_IntVal:
			result[i] = v.IntVal
		case *databasev1.Value_FloatVal:
			result[i] = v.FloatVal
		case *databasev1.Value_BoolVal:
			result[i] = v.BoolVal
		case *databasev1.Value_BytesVal:
			result[i] = v.BytesVal
		case *databasev1.Value_NullVal:
			result[i] = nil
		}
	}
	return result
}

func anyToProtoValue(v any) *databasev1.Value {
	switch val := v.(type) {
	case string:
		return &databasev1.Value{Kind: &databasev1.Value_StringVal{StringVal: val}}
	case int64:
		return &databasev1.Value{Kind: &databasev1.Value_IntVal{IntVal: val}}
	case int:
		return &databasev1.Value{Kind: &databasev1.Value_IntVal{IntVal: int64(val)}}
	case float64:
		return &databasev1.Value{Kind: &databasev1.Value_FloatVal{FloatVal: val}}
	case bool:
		return &databasev1.Value{Kind: &databasev1.Value_BoolVal{BoolVal: val}}
	case []byte:
		return &databasev1.Value{Kind: &databasev1.Value_BytesVal{BytesVal: val}}
	default:
		return &databasev1.Value{Kind: &databasev1.Value_NullVal{NullVal: true}}
	}
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
