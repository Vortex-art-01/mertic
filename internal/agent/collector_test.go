package agent

import (
	"strings"
	"testing"

	"github.com/shirou/gopsutil/v4/cpu"
)

var wantRuntimeGauges = []string{
	"Alloc", "BuckHashSys", "Frees", "GCCPUFraction", "GCSys",
	"HeapAlloc", "HeapIdle", "HeapInuse", "HeapObjects", "HeapReleased",
	"HeapSys", "LastGC", "Lookups", "MCacheInuse", "MCacheSys",
	"MSpanInuse", "MSpanSys", "Mallocs", "NextGC", "NumForcedGC",
	"NumGC", "OtherSys", "PauseTotalNs", "StackInuse", "StackSys",
	"Sys", "TotalAlloc", "RandomValue",
}

var wantSystemGauges = []string{"TotalMemory", "FreeMemory", "CPUutilization1"}

func TestCollectorPollRuntimeCollectsAllGauges(t *testing.T) {
	c := NewCollector()
	c.PollRuntime()

	gauges, _ := c.Take()
	for _, name := range wantRuntimeGauges {
		if _, ok := gauges[name]; !ok {
			t.Errorf("gauge %q not collected", name)
		}
	}
	if len(gauges) != len(wantRuntimeGauges) {
		t.Errorf("collected %d gauges, want %d", len(gauges), len(wantRuntimeGauges))
	}
}

func TestCollectorPollSystemCollectsGauges(t *testing.T) {
	c := NewCollector()
	if err := c.PollSystem(t.Context()); err != nil {
		t.Fatalf("PollSystem: %v", err)
	}

	gauges, _ := c.Take()
	for _, name := range wantSystemGauges {
		if _, ok := gauges[name]; !ok {
			t.Errorf("gauge %q not collected", name)
		}
	}
}

// Метрик CPUutilizationN столько, сколько ядер нашлось во время исполнения,
// и нумеруются они подряд с единицы.
func TestCollectorPollSystemCollectsEveryCPU(t *testing.T) {
	want, err := cpu.CountsWithContext(t.Context(), true)
	if err != nil {
		t.Fatalf("count cpus: %v", err)
	}

	c := NewCollector()
	if err := c.PollSystem(t.Context()); err != nil {
		t.Fatalf("PollSystem: %v", err)
	}

	gauges, _ := c.Take()

	got := 0
	for name := range gauges {
		if strings.HasPrefix(name, "CPUutilization") {
			got++
		}
	}
	if got != want {
		t.Errorf("collected %d CPUutilization gauges, want %d", got, want)
	}

	for core := 1; core <= want; core++ {
		if _, ok := gauges[cpuUtilizationMetric(core)]; !ok {
			t.Errorf("gauge %q not collected", cpuUtilizationMetric(core))
		}
	}
}

// PollCount считает только опросы runtime: метрики ОС его не трогают.
func TestCollectorPollCount(t *testing.T) {
	c := NewCollector()

	const polls = 3
	for range polls {
		c.PollRuntime()
	}
	if err := c.PollSystem(t.Context()); err != nil {
		t.Fatalf("PollSystem: %v", err)
	}

	if _, delta := c.Take(); delta != polls {
		t.Errorf("PollCount after %d polls = %d, want %d", polls, delta, polls)
	}
}

// Take отдаёт приращение целиком: повторный вызов без опросов вернёт ноль.
func TestCollectorTakeResetsPollCount(t *testing.T) {
	c := NewCollector()
	c.PollRuntime()
	c.Take()

	if _, delta := c.Take(); delta != 0 {
		t.Errorf("PollCount after Take = %d, want 0", delta)
	}
}

func TestCollectorAddPollCountReturnsDelta(t *testing.T) {
	c := NewCollector()
	c.PollRuntime()

	_, delta := c.Take()
	c.AddPollCount(delta)
	c.PollRuntime()

	if _, got := c.Take(); got != delta+1 {
		t.Errorf("PollCount = %d, want %d", got, delta+1)
	}
}

func TestCollectorTakeReturnsCopy(t *testing.T) {
	c := NewCollector()
	c.PollRuntime()

	gauges, _ := c.Take()
	gauges["Alloc"] = -1

	if next, _ := c.Take(); next["Alloc"] == -1 {
		t.Error("Take() must return a copy, not the internal map")
	}
}
