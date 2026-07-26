package index

import (
	"html/template"
	"maps"
	"net/http"
	"slices"
	"strconv"
)

type MetricsLister interface {
	Gauges() map[string]float64
	Counters() map[string]int64
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

func New(lister MetricsLister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gauges := lister.Gauges()
		counters := lister.Counters()

		rows := make([]row, 0, len(gauges)+len(counters))
		for _, name := range slices.Sorted(maps.Keys(gauges)) {
			rows = append(rows, row{Name: name, Value: strconv.FormatFloat(gauges[name], 'f', -1, 64)})
		}
		for _, name := range slices.Sorted(maps.Keys(counters)) {
			rows = append(rows, row{Name: name, Value: strconv.FormatInt(counters[name], 10)})
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if err := pageTemplate.Execute(w, rows); err != nil {
			http.Error(w, "failed to render page", http.StatusInternalServerError)
		}
	}
}
