package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/Muxcore-Media/core/pkg/contracts"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	"github.com/Muxcore-Media/database-sqlite/internal/db"
	"github.com/Muxcore-Media/database-sqlite/internal/grpctls"
	"github.com/Muxcore-Media/database-sqlite/internal/server"
)

type Module struct {
	database *db.Database
	srv      *server.Server
	grpcSrv  *grpc.Server
	lis      net.Listener

	id          string
	cfgMu       sync.RWMutex
	dbPath      string
	grpcAddr    string
	moduleToken string
}

type Config struct {
	ID          string
	DBPath      string
	GRPCAddr    string
	ModuleToken string
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "database-sqlite"
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "muxcore.db"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = "127.0.0.1:9700"
	}
	if v := os.Getenv("SQLITE_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("DATABASE_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}
	if cfg.ModuleToken == "" {
		cfg.ModuleToken = moduleTokenFromEnv()
	}
	return &Module{
		id:          cfg.ID,
		dbPath:      cfg.DBPath,
		grpcAddr:    cfg.GRPCAddr,
		moduleToken: cfg.ModuleToken,
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "Database SQLite",
		Version:      "0.1.5",
		Roles:        []string{"infrastructure"},
		Description:  "SQLite database provider (pure Go, no CGO)",
		Author:       "MuxCore",
		Capabilities: []string{contracts.CapabilityDatabase, "database.sqlite", "settings"},
		HTTPAddr:     m.grpcAddr,
	}
}

func (m *Module) Init(ctx context.Context) error {
	d, err := db.Open(m.dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	m.database = d
	m.srv = server.New(d)

	lis, err := net.Listen("tcp", m.grpcAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", m.grpcAddr, err)
	}
	m.lis = lis

	slog.Info("database-sqlite initialized", "db", m.dbPath, "addr", m.grpcAddr)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	var grpcOpts []grpc.ServerOption
	tlsCfg, err := grpctls.ServerConfig(m.dbPath)
	if err != nil {
		return fmt.Errorf("gRPC TLS: %w", err)
	}
	if tlsCfg != nil {
		grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(tlsCfg)))
		slog.Info("database-sqlite gRPC TLS enabled", "addr", m.grpcAddr)
	} else {
		slog.Warn("database-sqlite gRPC listening without TLS (dev only)",
			"addr", m.grpcAddr,
			"hint", "unset MUXCORE_INSECURE_DISABLE_TLS for production",
		)
	}
	grpcOpts = append(grpcOpts, grpc.UnaryInterceptor(authUnaryInterceptor(m.moduleToken)))
	m.grpcSrv = grpc.NewServer(grpcOpts...)
	m.srv.RegisterWithGRPC(m.grpcSrv)
	modulesdk.RegisterSettings(m.grpcSrv, m.id, m)

	go func() {
		slog.Info("database-sqlite gRPC service started", "addr", m.grpcAddr)
		if err := m.grpcSrv.Serve(m.lis); err != nil {
			slog.Error("database-sqlite gRPC serve error", "error", err)
		}
	}()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	if m.database != nil {
		_ = m.database.Close(ctx)
	}
	slog.Info("database-sqlite stopped")
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	if m.database == nil {
		return fmt.Errorf("not initialized")
	}
	return m.database.Health(ctx)
}
