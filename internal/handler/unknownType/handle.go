package unknownType

import "net/http"

func New() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unknown metric type", http.StatusBadRequest)
	}
}
