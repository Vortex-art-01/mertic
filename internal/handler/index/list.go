package index

import (
	"context"
	"html"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
)

type MetricsLister interface {
	Gauges(ctx context.Context) (map[string]float64, error)
	Counters(ctx context.Context) (map[string]int64, error)
}

const (
	pageHead = `<!DOCTYPE html>
<html lang="ru">
<head>
	<meta charset="utf-8">
	<title>Metrics</title>
</head>
<body>
	<h1>Metrics</h1>
	<ul>`

	pageFoot = `
	</ul>
</body>
</html>
`

	rowHead = "\n\t\t<li>"
	rowSep  = " = "
	rowFoot = "</li>"

	// rowSize — запас на строку списка: теги, имя и значение обычно в него умещаются,
	// и страница собирается за одно выделение памяти.
	rowSize = 64
)

func New(lister MetricsLister, l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gauges, err := lister.Gauges(r.Context())
		if err != nil {
			l.Error("index: failed to list gauges", slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		counters, err := lister.Counters(r.Context())
		if err != nil {
			l.Error("index: failed to list counters", slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(renderPage(gauges, counters))
	}
}

// renderPage собирает страницу вручную, а не через html/template: шаблон делал на
// каждую строку списка два десятка аллокаций через reflect, и при 1% запросов к
// странице на неё приходилось больше четверти всех объектов сервера. Имя метрики
// задаёт клиент, поэтому оно экранируется; значения — числа, экранировать в них нечего.
func renderPage(gauges map[string]float64, counters map[string]int64) []byte {
	size := len(pageHead) + len(pageFoot) + (len(gauges)+len(counters))*rowSize

	page := make([]byte, 0, size)
	page = append(page, pageHead...)

	for _, name := range sortedNames(gauges) {
		page = appendRowHead(page, name)
		page = strconv.AppendFloat(page, gauges[name], 'f', -1, 64)
		page = append(page, rowFoot...)
	}

	for _, name := range sortedNames(counters) {
		page = appendRowHead(page, name)
		page = strconv.AppendInt(page, counters[name], 10)
		page = append(page, rowFoot...)
	}

	return append(page, pageFoot...)
}

func appendRowHead(page []byte, name string) []byte {
	page = append(page, rowHead...)
	page = append(page, html.EscapeString(name)...)

	return append(page, rowSep...)
}

func sortedNames[V any](metrics map[string]V) []string {
	names := make([]string, 0, len(metrics))
	for name := range metrics {
		names = append(names, name)
	}

	slices.Sort(names)

	return names
}
