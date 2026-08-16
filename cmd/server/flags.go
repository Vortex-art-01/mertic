package main

import (
	"flag"
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

	if env := os.Getenv("ADDRESS"); env != "" {
		runAddr = env
	}
	if env := os.Getenv("STORE_INTERVAL"); env != "" {
		if v, err := strconv.ParseInt(env, 10, 64); err == nil {
			storeInterval = v
		}
	}
	if env, ok := os.LookupEnv("FILE_STORAGE_PATH"); ok {
		fileStorage = env
	}
	if env := os.Getenv("RESTORE"); env != "" {
		if v, err := strconv.ParseBool(env); err == nil {
			restore = v
		}
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
