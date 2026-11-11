package application

import (
	"context"
	"errors"

	"github.com/emms-garcia/shipq/internal/application/ports"
	"github.com/emms-garcia/shipq/internal/domain/identity"
)

var ErrHostUnavailable = errors.New("host status is unavailable")

func GetStatus(ctx context.Context, reader ports.StatusReader) (ports.HostStatus, error) {
	if err := ctx.Err(); err != nil {
		return ports.HostStatus{}, err
	}
	if reader == nil {
		return ports.HostStatus{}, ErrHostUnavailable
	}
	status, err := reader.ReadStatus(ctx)
	if err != nil {
		return ports.HostStatus{}, err
	}
	if status.HostVersion == "" || status.ActiveProfileID == (identity.ID{}) {
		return ports.HostStatus{}, ErrHostUnavailable
	}
	return status, nil
}
