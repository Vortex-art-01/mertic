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
	databaseDSN   string
	key           string
}

func parseFlags() config {
	var (
		runAddr       string
		storeInterval int64
		fileStorage   string
		restore       bool
		databaseDSN   string
		key           string
	)

	flag.StringVar(&runAddr, "a", "localhost:8080", "адрес эндпоинта HTTP-сервера")
	flag.Int64Var(&storeInterval, "i", 300, "интервал сохранения метрик на диск, сек (0 — писать синхронно)")
	flag.StringVar(&fileStorage, "f", "/tmp/metrics-db.json", "путь до файла с сохранёнными метриками")
	flag.BoolVar(&restore, "r", true, "загружать ранее сохранённые метрики при старте")
	flag.StringVar(&databaseDSN, "d", "", "строка подключения к базе данных PostgreSQL")
	flag.StringVar(&key, "k", "",
		"ключ подписи данных (пустой — не подписывать; с ключом сервер проверяет только запросы с подписью)")

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

	if env, ok := os.LookupEnv("DATABASE_DSN"); ok {
		databaseDSN = env
	}

	if env, ok := os.LookupEnv("KEY"); ok {
		key = env
	}

	if storeInterval < 0 {
		storeInterval = 0
	}

	return config{
		runAddr:       runAddr,
		storeInterval: time.Duration(storeInterval) * time.Second,
		fileStorage:   fileStorage,
		restore:       restore,
		databaseDSN:   databaseDSN,
		key:           key,
	}
}
