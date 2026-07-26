package counter

import (
	"log"
	"net/http"
	"strconv"
)

type CounterSaver interface {
	AddCounter(name string, value int64)
	GetCounter(name string) (int64, bool)
}

func New(saver CounterSaver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		saver.AddCounter(name, value)

		res, _ := saver.GetCounter(name)
		log.Printf("counter saved: %s = %d", name, res)

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
	}
}
