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
	flagRateLimit      int
)

func parseFlags() error {
	flag.StringVar(&flagRunAddr, "a", "localhost:8080", "адрес эндпоинта HTTP-сервера")
	flag.Int64Var(&flagReportInterval, "r", 10, "частота отправки метрик на сервер, сек (минимум 1)")
	flag.Int64Var(&flagPollInterval, "p", 2, "частота опроса метрик из пакета runtime, сек (минимум 1)")
	flag.StringVar(&flagKey, "k", "", "ключ подписи передаваемых данных (пустой — не подписывать)")
	flag.IntVar(&flagRateLimit, "l", 1, "количество одновременно исходящих запросов на сервер (минимум 1)")

	flag.Parse()

	if env, ok := os.LookupEnv("ADDRESS"); ok {
		flagRunAddr = env
	}

	if env, ok := os.LookupEnv("REPORT_INTERVAL"); ok {
		v, err := strconv.ParseInt(env, 10, 64)

		if err != nil {
			return fmt.Errorf("некорректное значение REPORT_INTERVAL: %w", err)
		}

		flagReportInterval = v
	}

	if env, ok := os.LookupEnv("POLL_INTERVAL"); ok {
		v, err := strconv.ParseInt(env, 10, 64)

		if err != nil {
			return fmt.Errorf("некорректное значение POLL_INTERVAL: %w", err)
		}

		flagPollInterval = v
	}

	if env, ok := os.LookupEnv("KEY"); ok {
		flagKey = env
	}

	if env, ok := os.LookupEnv("RATE_LIMIT"); ok {
		v, err := strconv.Atoi(env)

		if err != nil {
			return fmt.Errorf("некорректное значение RATE_LIMIT: %w", err)
		}

		flagRateLimit = v
	}

	return validateFlags()
}

func validateFlags() error {
	if flagReportInterval < 1 {
		return fmt.Errorf(
			"частота отправки метрик (-r / REPORT_INTERVAL) должна быть не меньше 1 секунды, получено %d",
			flagReportInterval)
	}

	if flagPollInterval < 1 {
		return fmt.Errorf(
			"частота опроса метрик (-p / POLL_INTERVAL) должна быть не меньше 1 секунды, получено %d",
			flagPollInterval)
	}

	if flagRateLimit < 1 {
		return fmt.Errorf(
			"количество одновременных запросов (-l / RATE_LIMIT) должно быть не меньше 1, получено %d",
			flagRateLimit)
	}

	return nil
}
