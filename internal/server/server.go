package server

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	databasev1 "github.com/Muxcore-Media/core/proto/gen/muxcore/database/v1"
	"github.com/Muxcore-Media/database-sqlite/internal/db"
)

type Server struct {
	databasev1.UnimplementedDatabaseServiceServer
	dbPtr    dbPtr
	inflight sync.WaitGroup
}

type dbPtr struct {
	mu sync.RWMutex
	d  *db.Database
}

func New(d *db.Database) *Server {
	s := &Server{}
	s.dbPtr.set(d)
	return s
}

// ReplaceDatabase swaps the backing SQLite handle. Returns the previous DB (caller should Drain then Close).
func (s *Server) ReplaceDatabase(d *db.Database) *db.Database {
	return s.dbPtr.swap(d)
}

// Drain waits for in-flight RPC handlers to finish.
func (s *Server) Drain() {
	s.inflight.Wait()
}

func (p *dbPtr) get() *db.Database {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.d
}

func (p *dbPtr) set(d *db.Database) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.d = d
}

func (p *dbPtr) swap(d *db.Database) *db.Database {
	p.mu.Lock()
	defer p.mu.Unlock()
	old := p.d
	p.d = d
	return old
}

func (s *Server) RegisterWithGRPC(srv *grpc.Server) {
	databasev1.RegisterDatabaseServiceServer(srv, s)
}

func (s *Server) withDB(fn func(*db.Database) error) error {
	s.inflight.Add(1)
	defer s.inflight.Done()
	return fn(s.dbPtr.get())
}

func (s *Server) Exec(ctx context.Context, req *databasev1.ExecRequest) (*databasev1.ExecResponse, error) {
	if req.GetQuery() == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}
	args := protoArgsToAny(req.GetArgs())
	var n int64
	err := s.withDB(func(d *db.Database) error {
		var execErr error
		n, execErr = d.Exec(ctx, req.GetQuery(), args...)
		return execErr
	})
	if err != nil {
		slog.Error("database: exec failed", "error", err)
		return nil, mapDBError(err)
	}
	return &databasev1.ExecResponse{RowsAffected: n}, nil
}

func (s *Server) Query(ctx context.Context, req *databasev1.QueryRequest) (*databasev1.QueryResponse, error) {
	if req.GetQuery() == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}
	args := protoArgsToAny(req.GetArgs())

	var resp *databasev1.QueryResponse
	err := s.withDB(func(d *db.Database) error {
		rows, err := d.Query(ctx, req.GetQuery(), args...)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		cols, err := rows.Columns()
		if err != nil {
			return fmt.Errorf("columns: %w", err)
		}
		resp = &databasev1.QueryResponse{Columns: cols}

		for rows.Next() {
			values := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range values {
				ptrs[i] = &values[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				return fmt.Errorf("scan: %w", err)
			}
			protoRow := &databasev1.Row{}
			for _, v := range values {
				protoRow.Values = append(protoRow.Values, anyToProtoValue(v))
			}
			resp.Rows = append(resp.Rows, protoRow)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("rows: %w", err)
		}
		resp.Count = int32(len(resp.Rows))
		return nil
	})
	if err != nil {
		slog.Error("database: query failed", "error", err)
		return nil, mapDBError(err)
	}
	return resp, nil
}

func (s *Server) Transaction(ctx context.Context, req *databasev1.TransactionRequest) (*databasev1.TransactionResponse, error) {
	stmts := req.GetStatements()
	if len(stmts) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one statement is required")
	}

	err := s.withDB(func(d *db.Database) error {
		return d.Transaction(ctx, func(tx *db.Tx) error {
			for _, stmt := range stmts {
				args := protoArgsToAny(stmt.GetArgs())
				if _, err := tx.Exec(ctx, stmt.GetQuery(), args...); err != nil {
					return err
				}
			}
			return nil
		})
	})
	if err != nil {
		return nil, mapDBError(err)
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
	err := s.withDB(func(d *db.Database) error {
		return d.Migrate(ctx, converted)
	})
	if err != nil {
		return nil, mapDBError(err)
	}
	return &databasev1.MigrateResponse{Status: "ok"}, nil
}

func (s *Server) Rollback(ctx context.Context, req *databasev1.RollbackRequest) (*databasev1.RollbackResponse, error) {
	err := s.withDB(func(d *db.Database) error {
		return d.Rollback(ctx, int(req.GetTargetVersion()))
	})
	if err != nil {
		return nil, mapDBError(err)
	}
	return &databasev1.RollbackResponse{Status: "ok"}, nil
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
