package handler

import (
	"net/http"
	"strconv"
)

// UpdateGauge обрабатывает POST /update/gauge/<ИМЯ_МЕТРИКИ>/<ЗНАЧЕНИЕ_МЕТРИКИ>.
// Новое значение замещает предыдущее.
func (h *Handler) UpdateGauge(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "metric name is required", http.StatusNotFound)
		return
	}

	value, err := strconv.ParseFloat(r.PathValue("value"), 64)
	if err != nil {
		http.Error(w, "invalid gauge value", http.StatusBadRequest)
		return
	}

	h.repo.SetGauge(name, value)
	writeOK(w)
}
