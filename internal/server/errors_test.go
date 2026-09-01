package server

import (
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMapDBError(t *testing.T) {
	t.Run("syntax", func(t *testing.T) {
		err := mapDBError(errors.New("sqlite exec: syntax error near SELEC"))
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("code=%v", status.Code(err))
		}
	})
	t.Run("constraint", func(t *testing.T) {
		err := mapDBError(errors.New("sqlite exec: constraint failed: UNIQUE constraint failed"))
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("code=%v", status.Code(err))
		}
	})
	t.Run("driver", func(t *testing.T) {
		err := mapDBError(errors.New("unexpected driver fault"))
		if status.Code(err) != codes.Internal {
			t.Fatalf("code=%v", status.Code(err))
		}
	})
}
