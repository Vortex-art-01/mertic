package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"
)

type config struct {
	runAddr       string
	storeInterval time.Duration
	fileStorage   string
	restore       bool
}

func parseFlags() config {
	var (
		runAddr       string
		storeInterval int64
		fileStorage   string
		restore       bool
	)

	flag.StringVar(&runAddr, "a", "localhost:8080", "адрес эндпоинта HTTP-сервера")
	flag.Int64Var(&storeInterval, "i", 300, "интервал сохранения метрик на диск, сек (0 — писать синхронно)")
	flag.StringVar(&fileStorage, "f", "/tmp/metrics-db.json", "путь до файла с сохранёнными метриками")
	flag.BoolVar(&restore, "r", true, "загружать ранее сохранённые метрики при старте")

	flag.Parse()

	//if env := os.Getenv("ADDRESS"); env != "" {
	if env, ok := os.LookupEnv("ADDRESS"); ok {
		runAddr = env
	}

	if env, ok := os.LookupEnv("STORE_INTERVAL"); ok {
		v, err := strconv.ParseInt(env, 10, 64)

		if err != nil {
			panic(fmt.Sprintf("Parse error STORE_INTERVAL: %s", err))
		}

		storeInterval = v
	}

	if env, ok := os.LookupEnv("FILE_STORAGE_PATH"); ok {
		fileStorage = env
	}

	if env, ok := os.LookupEnv("RESTORE"); ok {
		v, err := strconv.ParseBool(env)

		if err != nil {
			panic(fmt.Sprintf("Parse error RESTORE: %s", err))
		}

		restore = v
	}

	if storeInterval < 0 {
		storeInterval = 0
	}

	return config{
		runAddr:       runAddr,
		storeInterval: time.Duration(storeInterval) * time.Second,
		fileStorage:   fileStorage,
		restore:       restore,
	}
}
