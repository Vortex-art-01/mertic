package gauge

import (
	"log"
	"net/http"
	"strconv"
)

type GaugeSaver interface {
	SaveGauge(name string, value float64)
}

func New(saver GaugeSaver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		saver.SaveGauge(name, value)

		log.Printf("gauge saved: %s = %g", name, value)

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
	}
}
