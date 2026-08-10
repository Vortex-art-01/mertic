package value

import (
	"net/http"
	"strconv"

	"github.com/Vortex-art-01/mertic/internal/model"
)

type ValueGetter interface {
	GetGauge(name string) (float64, bool)
	GetCounter(name string) (int64, bool)
}

func New(getter ValueGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		var body string
		switch r.PathValue("type") {
		case model.Gauge:
			v, ok := getter.GetGauge(name)
			if !ok {
				http.Error(w, "metric not found", http.StatusNotFound)
				return
			}
			body = strconv.FormatFloat(v, 'f', -1, 64)
		case model.Counter:
			v, ok := getter.GetCounter(name)
			if !ok {
				http.Error(w, "metric not found", http.StatusNotFound)
				return
			}
			body = strconv.FormatInt(v, 10)
		default:
			http.Error(w, "unknown metric type", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))

	}
}
