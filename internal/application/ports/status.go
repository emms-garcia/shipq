package ports

import (
	"context"

	"github.com/emms-garcia/shipq/internal/domain/identity"
)

type HostStatus struct {
	HostVersion            string
	ActiveProfileID        identity.ID
	MonitorState           string
	NotificationPermission string
	CoverageState          string
}

type StatusReader interface {
	ReadStatus(context.Context) (HostStatus, error)
}
