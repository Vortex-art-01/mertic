package handler

import (
	"net/http"
	"strconv"
)

// UpdateCounter обрабатывает POST /update/counter/<ИМЯ_МЕТРИКИ>/<ЗНАЧЕНИЕ_МЕТРИКИ>.
// Новое значение добавляется к уже известному серверу.
func (h *Handler) UpdateCounter(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "metric name is required", http.StatusNotFound)
		return
	}

	value, err := strconv.ParseInt(r.PathValue("value"), 10, 64)
	if err != nil {
		http.Error(w, "invalid counter value", http.StatusBadRequest)
		return
	}

	h.repo.AddCounter(name, value)
	writeOK(w)
}
