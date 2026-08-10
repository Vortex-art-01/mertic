package main

import (
	"flag"
	"os"
	"strconv"
)

var (
	flagRunAddr        string
	flagReportInterval int64
	flagPollInterval   int64
)

func parseFlags() {
	flag.StringVar(&flagRunAddr, "a", "localhost:8080", "адрес эндпоинта HTTP-сервера")
	flag.Int64Var(&flagReportInterval, "r", 10, "частота отправки метрик на сервер, сек")
	flag.Int64Var(&flagPollInterval, "p", 2, "частота опроса метрик из пакета runtime, сек")

	flag.Parse()

	if env := os.Getenv("ADDRESS"); env != "" {
		flagRunAddr = env
	}
	if env := os.Getenv("REPORT_INTERVAL"); env != "" {
		if v, err := strconv.ParseInt(env, 10, 64); err == nil {
			flagReportInterval = v
		}
	}
	if env := os.Getenv("POLL_INTERVAL"); env != "" {
		if v, err := strconv.ParseInt(env, 10, 64); err == nil {
			flagPollInterval = v
		}
	}
}
