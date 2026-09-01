//go:build integration

package test

import (
	"context"
	"net"
	"path/filepath"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	databasev1 "github.com/Muxcore-Media/core/proto/gen/muxcore/database/v1"
	"github.com/Muxcore-Media/database-sqlite/internal/db"
	"github.com/Muxcore-Media/database-sqlite/internal/server"
)

func TestDatabaseServiceIntegration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "integration.db")
	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = d.Close(context.Background()) }()

	srv := server.New(d)
	lis := bufconn.Listen(1024 * 1024)
	gs := grpc.NewServer()
	srv.RegisterWithGRPC(gs)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := databasev1.NewDatabaseServiceClient(conn)
	ctx := context.Background()

	if _, err := client.Exec(ctx, &databasev1.ExecRequest{
		Query: "CREATE TABLE integration (id INTEGER PRIMARY KEY, label TEXT)",
	}); err != nil {
		t.Fatalf("Exec: %v", err)
	}

	if _, err := client.Exec(ctx, &databasev1.ExecRequest{
		Query:  "INSERT INTO integration (label) VALUES (?)",
		Args:   []*databasev1.Value{{Kind: &databasev1.Value_StringVal{StringVal: "ok"}}},
	}); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	resp, err := client.Query(ctx, &databasev1.QueryRequest{Query: "SELECT label FROM integration"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(resp.Rows) != 1 || resp.Rows[0].Values[0].GetStringVal() != "ok" {
		t.Fatalf("resp=%+v", resp)
	}
}
