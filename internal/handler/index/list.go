package index

import (
	"bytes"
	"context"
	"html/template"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strconv"
)

type MetricsLister interface {
	Gauges(ctx context.Context) (map[string]float64, error)
	Counters(ctx context.Context) (map[string]int64, error)
}

type row struct {
	Name  string
	Value string
}

var pageTemplate = template.Must(template.New("index").Parse(`<!DOCTYPE html>
<html lang="ru">
<head>
	<meta charset="utf-8">
	<title>Metrics</title>
</head>
<body>
	<h1>Metrics</h1>
	<ul>
	{{- range .}}
		<li>{{.Name}} = {{.Value}}</li>
	{{- end}}
	</ul>
</body>
</html>
`))

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

		rows := make([]row, 0, len(gauges)+len(counters))
		for _, name := range slices.Sorted(maps.Keys(gauges)) {
			rows = append(rows, row{Name: name, Value: strconv.FormatFloat(gauges[name], 'f', -1, 64)})
		}
		for _, name := range slices.Sorted(maps.Keys(counters)) {
			rows = append(rows, row{Name: name, Value: strconv.FormatInt(counters[name], 10)})
		}

		var buf bytes.Buffer

		if err := pageTemplate.Execute(&buf, rows); err != nil {
			l.Error("index: failed to render page", slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(w)
	}
}
