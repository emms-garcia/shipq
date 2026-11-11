package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/emms-garcia/shipq/internal/application"
	"github.com/emms-garcia/shipq/internal/application/ports"
	"github.com/emms-garcia/shipq/internal/domain/identity"
)

type statusReader struct {
	status  ports.HostStatus
	err     error
	context context.Context
}

func (r *statusReader) ReadStatus(ctx context.Context) (ports.HostStatus, error) {
	r.context = ctx
	return r.status, r.err
}

func TestGetStatus(t *testing.T) {
	id, _ := identity.ParseID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	ctx := context.Background()
	reader := &statusReader{status: ports.HostStatus{
		HostVersion: "0.0.0-dev", ActiveProfileID: id,
		MonitorState: "unavailable", NotificationPermission: "unavailable", CoverageState: "unknown",
	}}
	status, err := application.GetStatus(ctx, reader)
	if err != nil || status != reader.status || reader.context != ctx {
		t.Fatalf("status port not used: %+v, %v", status, err)
	}
	failure := errors.New("provider failure")
	reader.err = failure
	status, err = application.GetStatus(ctx, reader)
	if !errors.Is(err, failure) || status != (ports.HostStatus{}) {
		t.Fatal("failed status returned partial success")
	}
	if _, err := application.GetStatus(ctx, nil); !errors.Is(err, application.ErrHostUnavailable) {
		t.Fatal("missing host not unavailable")
	}
	reader.err = nil
	reader.status.ActiveProfileID = identity.ID{}
	if _, err := application.GetStatus(ctx, reader); !errors.Is(err, application.ErrHostUnavailable) {
		t.Fatal("uninitialized host reported ready")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := application.GetStatus(cancelled, reader); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled status read accepted")
	}
}
