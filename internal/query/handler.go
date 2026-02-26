package query

import (
	"github.com/danielleslie/clicknest/internal/storage"
)

type Handler struct {
	events *storage.DuckDB
	meta   *storage.SQLite
}

func NewHandler(events *storage.DuckDB, meta *storage.SQLite) *Handler {
	return &Handler{events: events, meta: meta}
}
