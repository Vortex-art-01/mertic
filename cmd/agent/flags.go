package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
)

var (
	flagRunAddr        string
	flagReportInterval int64
	flagPollInterval   int64
	flagKey            string
)

func parseFlags() {
	flag.StringVar(&flagRunAddr, "a", "localhost:8080", "адрес эндпоинта HTTP-сервера")
	flag.Int64Var(&flagReportInterval, "r", 10, "частота отправки метрик на сервер, сек")
	flag.Int64Var(&flagPollInterval, "p", 2, "частота опроса метрик из пакета runtime, сек")
	flag.StringVar(&flagKey, "k", "", "ключ подписи передаваемых данных (пустой — не подписывать)")

	flag.Parse()

	if env, ok := os.LookupEnv("ADDRESS"); ok {
		flagRunAddr = env
	}

	if env, ok := os.LookupEnv("REPORT_INTERVAL"); ok {
		v, err := strconv.ParseInt(env, 10, 64)

		if err != nil {
			panic(fmt.Sprintf("Parse error REPORT_INTERVAL: %s", err))
		}

		flagReportInterval = v
	}

	if env, ok := os.LookupEnv("POLL_INTERVAL"); ok {
		v, err := strconv.ParseInt(env, 10, 64)

		if err != nil {
			panic(fmt.Sprintf("Parse error POLL_INTERVAL: %s", err))
		}

		flagPollInterval = v
	}

	if env, ok := os.LookupEnv("KEY"); ok {
		flagKey = env
	}
}
