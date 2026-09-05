package internal

import (
	"context"
	"net"
	"path/filepath"
	"testing"

	databasev1 "github.com/Muxcore-Media/core/proto/gen/muxcore/database/v1"
	"github.com/Muxcore-Media/database-sqlite/internal/db"
	"github.com/Muxcore-Media/database-sqlite/internal/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const testModuleToken = "test-database-module-token"

func startAuthServer(t *testing.T, d *db.Database, moduleToken string) *bufconn.Listener {
	t.Helper()
	srv := server.New(d)
	lis := bufconn.Listen(1 << 20)
	grpcSrv := grpc.NewServer(grpc.UnaryInterceptor(authUnaryInterceptor(moduleToken)))
	srv.RegisterWithGRPC(grpcSrv)
	go func() { _ = grpcSrv.Serve(lis) }()
	t.Cleanup(func() { grpcSrv.Stop() })
	return lis
}

func testDatabase(t *testing.T) *db.Database {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close(context.Background()) })
	return d
}

func rpcContext(md metadata.MD) context.Context {
	return metadata.NewOutgoingContext(context.Background(), md)
}

func expectUnauthenticated(t *testing.T, err error) {
	t.Helper()
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status, got %v", err)
	}
	if st.Code() != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v: %s", st.Code(), st.Message())
	}
}

func TestAuthorizeDatabaseRPC_RejectsAnonymous(t *testing.T) {
	d := testDatabase(t)
	lis := startAuthServer(t, d, testModuleToken)
	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	client := databasev1.NewDatabaseServiceClient(conn)

	_, err = client.Query(context.Background(), &databasev1.QueryRequest{Query: "SELECT 1"})
	expectUnauthenticated(t, err)

	_, err = client.Exec(context.Background(), &databasev1.ExecRequest{Query: "SELECT 1"})
	expectUnauthenticated(t, err)

	_, err = client.Migrate(context.Background(), &databasev1.MigrateRequest{})
	expectUnauthenticated(t, err)

	_, err = client.Transaction(context.Background(), &databasev1.TransactionRequest{
		Statements: []*databasev1.Statement{{Query: "SELECT 1"}},
	})
	expectUnauthenticated(t, err)

	_, err = client.Rollback(context.Background(), &databasev1.RollbackRequest{})
	expectUnauthenticated(t, err)
}

func TestAuthorizeDatabaseRPC_RejectsPublicCaller(t *testing.T) {
	d := testDatabase(t)
	lis := startAuthServer(t, d, testModuleToken)
	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	client := databasev1.NewDatabaseServiceClient(conn)

	_, err = client.Query(rpcContext(metadata.Pairs(callerIDMetadataKey, "_public")),
		&databasev1.QueryRequest{Query: "SELECT 1"})
	expectUnauthenticated(t, err)
}

func TestAuthorizeDatabaseRPC_AcceptsMeshCallerID(t *testing.T) {
	d := testDatabase(t)
	lis := startAuthServer(t, d, testModuleToken)
	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	client := databasev1.NewDatabaseServiceClient(conn)
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(callerIDMetadataKey, "auth-local"))

	_, err = client.Exec(ctx, &databasev1.ExecRequest{
		Query: "CREATE TABLE IF NOT EXISTS auth_test (id INTEGER PRIMARY KEY)",
	})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}

	_, err = client.Query(ctx, &databasev1.QueryRequest{Query: "SELECT COUNT(*) FROM auth_test"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
}

func TestAuthorizeDatabaseRPC_AcceptsModuleToken(t *testing.T) {
	d := testDatabase(t)
	lis := startAuthServer(t, d, testModuleToken)
	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	client := databasev1.NewDatabaseServiceClient(conn)
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+testModuleToken))

	_, err = client.Query(ctx, &databasev1.QueryRequest{Query: "SELECT 1"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
}

func TestAuthorizeDatabaseRPC_RejectsWrongModuleToken(t *testing.T) {
	d := testDatabase(t)
	lis := startAuthServer(t, d, testModuleToken)
	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	client := databasev1.NewDatabaseServiceClient(conn)
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", "Bearer wrong-token"))

	_, err = client.Query(ctx, &databasev1.QueryRequest{Query: "SELECT 1"})
	expectUnauthenticated(t, err)
}

func TestModuleStart_RejectsUnauthenticatedOverGRPC(t *testing.T) {
	dir := t.TempDir()
	m := NewModule(Config{
		DBPath:      filepath.Join(dir, "test.db"),
		GRPCAddr:    "127.0.0.1:0",
		ModuleToken: testModuleToken,
	})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Stop(ctx) }()

	addr := m.lis.Addr().String()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	client := databasev1.NewDatabaseServiceClient(conn)
	_, err = client.Query(context.Background(), &databasev1.QueryRequest{Query: "SELECT 1"})
	expectUnauthenticated(t, err)
}

func TestDefaultGRPCAddr(t *testing.T) {
	m := NewModule(Config{})
	if m.grpcAddr != "127.0.0.1:9700" {
		t.Fatalf("grpcAddr=%q, want 127.0.0.1:9700", m.grpcAddr)
	}
}
