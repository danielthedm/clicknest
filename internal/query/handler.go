package query

import (
	ghub "github.com/danielthedm/clicknest/internal/github"
	"github.com/danielthedm/clicknest/internal/storage"
)

type Handler struct {
	events  *storage.DuckDB
	meta    *storage.SQLite
	matcher *ghub.Matcher
}

func NewHandler(events *storage.DuckDB, meta *storage.SQLite) *Handler {
	return &Handler{events: events, meta: meta}
}

func (h *Handler) SetMatcher(m *ghub.Matcher) {
	h.matcher = m
}
