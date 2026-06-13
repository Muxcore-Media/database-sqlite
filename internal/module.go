package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"

	"google.golang.org/grpc"

	"github.com/Muxcore-Media/core/pkg/contracts"
	"github.com/Muxcore-Media/database-sqlite/internal/db"
	"github.com/Muxcore-Media/database-sqlite/internal/server"
)

type Module struct {
	database *db.Database
	srv      *server.Server
	grpcSrv  *grpc.Server
	lis      net.Listener

	id       string
	dbPath   string
	grpcAddr string
}

type Config struct {
	ID       string
	DBPath   string
	GRPCAddr string
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "database-sqlite"
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "muxcore.db"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":9700"
	}
	if v := os.Getenv("SQLITE_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("DATABASE_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}
	return &Module{
		id:       cfg.ID,
		dbPath:   cfg.DBPath,
		grpcAddr: cfg.GRPCAddr,
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "Database SQLite",
		Version:      "0.1.0",
		Roles:        []string{"infrastructure"},
		Description:  "SQLite database provider (pure Go, no CGO)",
		Author:       "MuxCore",
		Capabilities: []string{contracts.CapabilityDatabase},
		Contracts: []contracts.ContractDeclaration{
			{
				Repo:      "github.com/Muxcore-Media/core/pkg/contracts",
				Interface: "DatabaseProvider",
				Version:   "v0.4.0",
			},
		},
		MinCoreVersion: "0.4.0",
		HTTPAddr:       m.grpcAddr,
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
	m.grpcSrv = grpc.NewServer()
	m.srv.RegisterWithGRPC(m.grpcSrv)

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
		m.database.Close(ctx)
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
