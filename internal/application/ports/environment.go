package ports

import (
	"time"

	"github.com/emms-garcia/shipq/internal/domain/identity"
)

type Clock interface {
	Now() time.Time
}

type IDGenerator interface {
	NewID() (identity.ID, error)
}
