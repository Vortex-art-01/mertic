package agent

import (
	"context"
	"fmt"
	"maps"
	"math/rand/v2"
	"runtime"
	"strconv"
	"sync"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

const pollCountMetric = "PollCount"

type Collector struct {
	mu        sync.Mutex
	gauges    map[string]float64
	pollCount int64
}

func NewCollector() *Collector {
	return &Collector{
		gauges: make(map[string]float64),
	}
}

func (c *Collector) PollRuntime() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.gauges["Alloc"] = float64(ms.Alloc)
	c.gauges["BuckHashSys"] = float64(ms.BuckHashSys)
	c.gauges["Frees"] = float64(ms.Frees)
	c.gauges["GCCPUFraction"] = ms.GCCPUFraction
	c.gauges["GCSys"] = float64(ms.GCSys)
	c.gauges["HeapAlloc"] = float64(ms.HeapAlloc)
	c.gauges["HeapIdle"] = float64(ms.HeapIdle)
	c.gauges["HeapInuse"] = float64(ms.HeapInuse)
	c.gauges["HeapObjects"] = float64(ms.HeapObjects)
	c.gauges["HeapReleased"] = float64(ms.HeapReleased)
	c.gauges["HeapSys"] = float64(ms.HeapSys)
	c.gauges["LastGC"] = float64(ms.LastGC)
	c.gauges["Lookups"] = float64(ms.Lookups)
	c.gauges["MCacheInuse"] = float64(ms.MCacheInuse)
	c.gauges["MCacheSys"] = float64(ms.MCacheSys)
	c.gauges["MSpanInuse"] = float64(ms.MSpanInuse)
	c.gauges["MSpanSys"] = float64(ms.MSpanSys)
	c.gauges["Mallocs"] = float64(ms.Mallocs)
	c.gauges["NextGC"] = float64(ms.NextGC)
	c.gauges["NumForcedGC"] = float64(ms.NumForcedGC)
	c.gauges["NumGC"] = float64(ms.NumGC)
	c.gauges["OtherSys"] = float64(ms.OtherSys)
	c.gauges["PauseTotalNs"] = float64(ms.PauseTotalNs)
	c.gauges["StackInuse"] = float64(ms.StackInuse)
	c.gauges["StackSys"] = float64(ms.StackSys)
	c.gauges["Sys"] = float64(ms.Sys)
	c.gauges["TotalAlloc"] = float64(ms.TotalAlloc)
	c.gauges["RandomValue"] = rand.Float64()

	c.pollCount++
}

func (c *Collector) PollSystem(ctx context.Context) error {
	memory, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return fmt.Errorf("read virtual memory: %w", err)
	}

	utilization, err := cpu.PercentWithContext(ctx, 0, true)
	if err != nil {
		return fmt.Errorf("read cpu utilization: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.gauges["TotalMemory"] = float64(memory.Total)
	c.gauges["FreeMemory"] = float64(memory.Free)

	for i, percent := range utilization {
		c.gauges[cpuUtilizationMetric(i+1)] = percent
	}

	return nil
}

func cpuUtilizationMetric(core int) string {
	return "CPUutilization" + strconv.Itoa(core)
}

func (c *Collector) Take() (map[string]float64, int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	gauges := make(map[string]float64, len(c.gauges))
	maps.Copy(gauges, c.gauges)

	delta := c.pollCount
	c.pollCount = 0

	return gauges, delta
}

func (c *Collector) AddPollCount(delta int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.pollCount += delta
}
