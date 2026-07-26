package agent

import "testing"

var wantGauges = []string{
	"Alloc", "BuckHashSys", "Frees", "GCCPUFraction", "GCSys",
	"HeapAlloc", "HeapIdle", "HeapInuse", "HeapObjects", "HeapReleased",
	"HeapSys", "LastGC", "Lookups", "MCacheInuse", "MCacheSys",
	"MSpanInuse", "MSpanSys", "Mallocs", "NextGC", "NumForcedGC",
	"NumGC", "OtherSys", "PauseTotalNs", "StackInuse", "StackSys",
	"Sys", "TotalAlloc", "RandomValue",
}

func TestCollectorPollCollectsAllGauges(t *testing.T) {
	c := NewCollector()
	c.Poll()

	gauges := c.Gauges()
	for _, name := range wantGauges {
		if _, ok := gauges[name]; !ok {
			t.Errorf("gauge %q not collected", name)
		}
	}
	if len(gauges) != len(wantGauges) {
		t.Errorf("collected %d gauges, want %d", len(gauges), len(wantGauges))
	}
}

func TestCollectorPollCount(t *testing.T) {
	c := NewCollector()
	if got := c.PollCount(); got != 0 {
		t.Fatalf("PollCount before polls = %d, want 0", got)
	}

	const polls = 3
	for range polls {
		c.Poll()
	}
	if got := c.PollCount(); got != polls {
		t.Errorf("PollCount after %d polls = %d, want %d", polls, got, polls)
	}
}

func TestCollectorGaugesReturnsCopy(t *testing.T) {
	c := NewCollector()
	c.Poll()

	gauges := c.Gauges()
	gauges["Alloc"] = -1

	if c.Gauges()["Alloc"] == -1 {
		t.Error("Gauges() must return a copy, not the internal map")
	}
}
