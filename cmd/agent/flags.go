package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
)

var (
	flagRunAddr        string
	flagReportInterval int
	flagPollInterval   int
	flagKey            string
	flagRateLimit      int
)

func parseFlags() error {
	flag.StringVar(&flagRunAddr, "a", "localhost:8080", "адрес эндпоинта HTTP-сервера")
	flag.IntVar(&flagReportInterval, "r", 10, "частота отправки метрик на сервер, сек (минимум 1)")
	flag.IntVar(&flagPollInterval, "p", 2, "частота опроса метрик из пакета runtime, сек (минимум 1)")
	flag.StringVar(&flagKey, "k", "", "ключ подписи передаваемых данных (пустой — не подписывать)")
	flag.IntVar(&flagRateLimit, "l", 1, "количество одновременно исходящих запросов на сервер (минимум 1)")

	flag.Parse()

	envString("ADDRESS", &flagRunAddr)
	envString("KEY", &flagKey)

	if err := errors.Join(
		envInt("REPORT_INTERVAL", &flagReportInterval),
		envInt("POLL_INTERVAL", &flagPollInterval),
		envInt("RATE_LIMIT", &flagRateLimit),
	); err != nil {
		return err
	}

	return validateFlags()
}

func envString(name string, dst *string) {
	if v, ok := os.LookupEnv(name); ok {
		*dst = v
	}
}

func envInt(name string, dst *int) error {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return nil
	}

	v, err := strconv.Atoi(raw)
	if err != nil {
		return fmt.Errorf("некорректное значение %s: %w", name, err)
	}

	*dst = v

	return nil
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
