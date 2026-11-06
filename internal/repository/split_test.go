package repository

import (
	"maps"
	"testing"

	"github.com/Vortex-art-01/mertic/internal/model"
)

// Разбор пакета живёт отдельно от базы, поэтому проверяется и без неё: у gauge
// остаётся последнее значение, приращения counter складываются, а метрики без
// значения и неизвестных типов в запись не попадают.
func TestSplitBatch(t *testing.T) {
	first, last := 1.5, 2.5
	two, three := int64(2), int64(3)

	gauges, counters := splitBatch([]model.Metrics{
		{ID: "Alloc", MType: model.Gauge, Value: &first},
		{ID: "Alloc", MType: model.Gauge, Value: &last},
		{ID: "PollCount", MType: model.Counter, Delta: &two},
		{ID: "PollCount", MType: model.Counter, Delta: &three},
		{ID: "NoValue", MType: model.Gauge},
		{ID: "NoDelta", MType: model.Counter},
		{ID: "Unknown", MType: "histogram", Value: &first},
	})

	if want := map[string]float64{"Alloc": 2.5}; !maps.Equal(gauges, want) {
		t.Errorf("gauges = %v, want %v", gauges, want)
	}
	if want := map[string]int64{"PollCount": 5}; !maps.Equal(counters, want) {
		t.Errorf("counters = %v, want %v", counters, want)
	}
}
