// Package unknowntype отклоняет обновление через URL метрики неизвестного типа.
package unknowntype

import "net/http"

// New возвращает хендлер, который на любой запрос отвечает 400 «unknown metric
// type». Сервер ставит его на общий маршрут POST /update/{type}/{name}/{value}
// рядом с маршрутами известных типов /update/gauge/... и /update/counter/...:
// chi выбирает статичный сегмент раньше параметра, поэтому сюда доходят только
// запросы с другим типом.
func New() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unknown metric type", http.StatusBadRequest)
	}
}
