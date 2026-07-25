package handler

import (
	"net/http"

	"github.com/Vortex-art-01/mertic/internal/repository"
)

type Handler struct {
	repo repository.Repository
}

func NewHandler(repo repository.Repository) *Handler {
	return &Handler{repo: repo}
}

// NewRouter собирает маршруты сервера. Хендлеры разделены по типам метрик:
// у каждого типа свой маршрут и свой обработчик.
// Запрос без имени метрики (например, POST /update/counter/527) не совпадает
// ни с одним шаблоном и получает http.StatusNotFound от ServeMux.
func NewRouter(repo repository.Repository) http.Handler {
	h := NewHandler(repo)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /update/gauge/{name}/{value}", h.UpdateGauge)
	mux.HandleFunc("POST /update/counter/{name}/{value}", h.UpdateCounter)
	mux.HandleFunc("POST /update/{type}/{name}/{value}", h.UpdateUnknownType)
	return mux
}

// UpdateUnknownType ловит запросы с типом метрики, для которого нет
// специализированного хендлера.
func (h *Handler) UpdateUnknownType(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "unknown metric type", http.StatusBadRequest)
}

func writeOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
}
