# mertic

![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-optional-4169E1?logo=postgresql&logoColor=white)

Сервис сбора метрик и алертинга на Go: **агент** снимает метрики рантайма и системы и пакетами отправляет их на **сервер**, а сервер хранит их в памяти с дампом в файл или в PostgreSQL и отдаёт через HTTP API.

Проект выполнен в рамках трека «Сервер сбора метрик и алертинга» курса Go-разработчика Яндекс Практикума.

## Содержание

- [Как это устроено](#как-это-устроено)
- [Возможности](#возможности)
- [Быстрый старт](#быстрый-старт)
- [Конфигурация](#конфигурация)
- [HTTP API](#http-api)
- [Подпись данных](#подпись-данных)
- [Метрики агента](#метрики-агента)
- [Структура проекта](#структура-проекта)
- [Тесты](#тесты)
- [Обновление шаблона](#обновление-шаблона)

## Как это устроено

```mermaid
flowchart LR
    subgraph agent["Агент"]
        rt["runtime.MemStats<br/>каждые POLL_INTERVAL"]
        sys["gopsutil: CPU, память<br/>каждые POLL_INTERVAL"]
        rep["пакет метрик<br/>каждые REPORT_INTERVAL"]
        workers["RATE_LIMIT воркеров<br/>gzip + HMAC + ретраи"]
        rt --> rep
        sys --> rep
        rep --> workers
    end

    subgraph server["Сервер"]
        mw["middleware:<br/>лог → gzip → подпись"]
        h["хендлеры"]
        mw --> h
    end

    workers -- "POST /updates/" --> mw
    h --> pg[("PostgreSQL")]
    h --> mem["память + JSON-дамп"]
```

Хранилище сервер выбирает при старте: если задана строка подключения `DATABASE_DSN`, метрики пишутся в PostgreSQL, иначе — в память с периодическим сохранением в файл.

## Возможности

**Сервер**

- два типа метрик: `gauge` (значение заменяется) и `counter` (значение накапливается);
- обновление и чтение через URL, через JSON и пакетом через `POST /updates/`; пакет проверяется целиком и применяется атомарно;
- HTML-страница со всеми метриками;
- хранение в PostgreSQL ([pgx](https://github.com/jackc/pgx)) с автоматическими миграциями ([goose](https://github.com/pressly/goose), встроены в бинарник);
- либо хранение в памяти с дампом в JSON-файл: по таймеру или синхронно после каждого изменения; запись атомарная, через временный файл;
- повтор подключения к базе при временных ошибках (паузы 1s, 3s, 5s);
- gzip для запросов и ответов (`application/json`, `text/html`);
- проверка подписи запросов и подпись ответов HMAC-SHA256;
- структурированные JSON-логи (`log/slog`) по каждому запросу: метод, URI, статус, размер, длительность;
- graceful shutdown по `SIGINT`/`SIGTERM`: незавершённые запросы дорабатывают, последний дамп записывается на диск.

**Агент**

- 27 метрик из `runtime.MemStats`, а также `RandomValue`, `PollCount`, свободная и общая память и загрузка каждого ядра CPU;
- отправка пакетом, сжатым gzip, с подписью HMAC-SHA256;
- ограничение числа одновременных запросов к серверу (`RATE_LIMIT`);
- повтор отправки при сетевых ошибках, ответах `429` и `5xx` (паузы 1s, 3s, 5s); если пакет так и не ушёл, приращение `PollCount` не теряется и уходит со следующим.

## Быстрый старт

Нужен Go 1.26 или новее. PostgreSQL — по желанию.

```bash
git clone https://github.com/Vortex-art-01/mertic.git
cd mertic
```

Запустите сервер (в памяти, с дампом в файл) и агент в соседнем терминале:

```bash
go run ./cmd/server
go run ./cmd/agent
```

Через несколько секунд метрики появятся на http://localhost:8080/.

### С PostgreSQL

Поднимите базу, например в Docker:

```bash
docker run -d --name metrics-db \
  -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=praktikum \
  -p 5432:5432 postgres:17
```

и передайте серверу строку подключения — таблицы создадутся при старте:

```bash
go run ./cmd/server -d "postgres://postgres:postgres@localhost:5432/praktikum?sslmode=disable"
```

### Сборка бинарников

```bash
go build -o cmd/server/server ./cmd/server
go build -o cmd/agent/agent ./cmd/agent
```

## Конфигурация

Оба приложения настраиваются флагами и переменными окружения. **Переменная окружения важнее флага**, флаг важнее значения по умолчанию.

### Сервер

| Флаг | Переменная          | По умолчанию           | Описание                                                                                   |
|------|---------------------|------------------------|--------------------------------------------------------------------------------------------|
| `-a` | `ADDRESS`           | `localhost:8080`       | адрес, на котором слушает HTTP-сервер                                                      |
| `-d` | `DATABASE_DSN`      | —                      | строка подключения к PostgreSQL; если задана, метрики хранятся в базе, а файл не используется |
| `-f` | `FILE_STORAGE_PATH` | `/tmp/metrics-db.json` | файл для дампа метрик; пустое значение — только память, без дампа                          |
| `-i` | `STORE_INTERVAL`    | `300`                  | интервал записи дампа, сек; `0` — писать после каждого изменения                           |
| `-r` | `RESTORE`           | `true`                 | загружать метрики из файла при старте                                                       |
| `-k` | `KEY`               | —                      | ключ подписи HMAC-SHA256; пустой — подпись выключена                                        |

### Агент

| Флаг | Переменная        | По умолчанию     | Описание                                             |
|------|-------------------|------------------|------------------------------------------------------|
| `-a` | `ADDRESS`         | `localhost:8080` | адрес сервера                                        |
| `-p` | `POLL_INTERVAL`   | `2`              | частота опроса метрик, сек (не меньше 1)             |
| `-r` | `REPORT_INTERVAL` | `10`             | частота отправки метрик на сервер, сек (не меньше 1) |
| `-k` | `KEY`             | —                | ключ подписи HMAC-SHA256; пустой — не подписывать    |
| `-l` | `RATE_LIMIT`      | `1`              | число одновременных запросов к серверу (не меньше 1) |

Пример:

```bash
ADDRESS=localhost:9090 KEY=secret go run ./cmd/server -i 0 -f ./metrics-db.json
go run ./cmd/agent -a localhost:9090 -k secret -p 1 -r 5 -l 3
```

## HTTP API

| Метод  | Путь                            | Тело запроса    | Ответ                                    |
|--------|---------------------------------|-----------------|------------------------------------------|
| `GET`  | `/`                             | —               | HTML-страница со всеми метриками          |
| `GET`  | `/ping`                         | —               | `200`, если база доступна (без базы — всегда `200`) |
| `GET`  | `/value/{type}/{name}`          | —               | значение метрики текстом                  |
| `POST` | `/value/`                       | `Metrics`       | метрика с текущим значением               |
| `POST` | `/update/{type}/{name}/{value}` | —               | `200` без тела                            |
| `POST` | `/update/`                      | `Metrics`       | метрика после обновления                  |
| `POST` | `/updates/`                     | `[]Metrics`     | `{"status":"ok"}`                         |

Формат `Metrics`:

```json
{
  "id": "Alloc",
  "type": "gauge",
  "value": 1024.5
}
```

| Поле    | Тип       | Описание                                    |
|---------|-----------|---------------------------------------------|
| `id`    | `string`  | имя метрики                                 |
| `type`  | `string`  | `gauge` или `counter`                       |
| `value` | `float64` | значение — только для `gauge`               |
| `delta` | `int64`   | приращение — только для `counter`           |

Коды ответов: `200` — успех; `400` — неизвестный тип, некорректное значение, невалидный JSON или подпись; `404` — не указано имя или метрика не найдена; `500` — ошибка хранилища.

### Примеры

```bash
# обновить через URL
curl -X POST http://localhost:8080/update/gauge/Temperature/36.6
curl -X POST http://localhost:8080/update/counter/Requests/5

# прочитать значение
curl http://localhost:8080/value/counter/Requests
# 5

# обновить через JSON — counter вернётся с накопленным значением
curl -X POST http://localhost:8080/update/ \
  -H 'Content-Type: application/json' \
  -d '{"id":"Requests","type":"counter","delta":3}'
# {"id":"Requests","type":"counter","delta":8}

# отправить пакет
curl -X POST http://localhost:8080/updates/ \
  -H 'Content-Type: application/json' \
  -d '[{"id":"Alloc","type":"gauge","value":1024},{"id":"PollCount","type":"counter","delta":1}]'

# прочитать через JSON
curl -X POST http://localhost:8080/value/ \
  -H 'Content-Type: application/json' \
  -d '{"id":"Alloc","type":"gauge"}'
```

## Подпись данных

Если серверу и агенту задан одинаковый ключ (`-k` / `KEY`), тело запроса подписывается HMAC-SHA256, а подпись в hex передаётся в заголовке `HashSHA256`. Подпись считается по несжатому телу.

- Агент подписывает каждый пакет.
- Сервер проверяет подпись, только если она пришла: запрос без заголовка или со значением `none` принимается, с неверной подписью — отклоняется с `400`. Так на сервере с ключом продолжают работать неподписанные `GET /ping`, `GET /value/...` и т. п.
- Сервер подписывает каждый свой ответ тем же ключом.

Подписанный запрос из консоли:

```bash
BODY='[{"id":"Alloc","type":"gauge","value":1024}]'
SIGN=$(printf '%s' "$BODY" | openssl dgst -sha256 -hmac secret | awk '{print $NF}')

curl -X POST http://localhost:8080/updates/ \
  -H 'Content-Type: application/json' \
  -H "HashSHA256: $SIGN" \
  -d "$BODY"
```

## Метрики агента

| Источник            | Тип       | Метрики                                                                                                                                                                                                                                                                                                                 |
|---------------------|-----------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `runtime.MemStats`  | `gauge`   | `Alloc`, `BuckHashSys`, `Frees`, `GCCPUFraction`, `GCSys`, `HeapAlloc`, `HeapIdle`, `HeapInuse`, `HeapObjects`, `HeapReleased`, `HeapSys`, `LastGC`, `Lookups`, `MCacheInuse`, `MCacheSys`, `MSpanInuse`, `MSpanSys`, `Mallocs`, `NextGC`, `NumForcedGC`, `NumGC`, `OtherSys`, `PauseTotalNs`, `StackInuse`, `StackSys`, `Sys`, `TotalAlloc` |
| система (gopsutil)  | `gauge`   | `TotalMemory`, `FreeMemory`, `CPUutilization1` … `CPUutilizationN` — по одной на ядро                                                                                                                                                                                                                                    |
| агент               | `gauge`   | `RandomValue` — случайное число при каждом опросе                                                                                                                                                                                                                                                                         |
| агент               | `counter` | `PollCount` — число опросов с прошлой успешной отправки                                                                                                                                                                                                                                                                   |

## Структура проекта

```
cmd/
  agent/          точка входа агента и разбор флагов
  server/         точка входа сервера, роутер и разбор флагов
internal/
  agent/          сбор метрик, отправка пакетами, классификация ошибок
  database/       пул соединений pgx и применение миграций
  dump/           сохранение метрик в файл и восстановление при старте
  handler/        HTTP-хендлеры, по пакету на эндпоинт
  hash/           подпись HMAC-SHA256
  logger/         JSON-логгер на slog
  middleware/     логирование, gzip, проверка и выдача подписи
  model/          модель Metrics
  pgerrors/       какие ошибки PostgreSQL стоит повторять
  repository/     хранилища: в памяти и в PostgreSQL
  retry/          повтор операций с паузами
migrations/       SQL-миграции goose, встраиваются в бинарник через embed
```

## Тесты

```bash
go test ./...
go vet ./...
```

Тесты хранилища на PostgreSQL без базы пропускаются. Чтобы прогнать их, укажите тестовую базу:

```bash
TEST_DATABASE_DSN='postgres://postgres:postgres@localhost:5432/praktikum?sslmode=disable' \
  go test ./internal/repository/
```

### Автотесты Практикума

В GitHub Actions на каждый pull request запускаются два набора:

- **autotests** — функциональные тесты сервера и агента из [go-autotests](https://github.com/Yandex-Practicum/go-autotests); ветка должна называться `iter<N>`, тогда прогоняются инкременты с первого по `N`-й;
- **go vet test** — `go vet` со статическим анализатором Практикума.

Как запустить автотесты локально, описано в [README go-autotests](https://github.com/Yandex-Practicum/go-autotests).

## Обновление шаблона

Репозиторий создан из шаблона [go-musthave-metrics-tpl](https://github.com/Yandex-Practicum/go-musthave-metrics-tpl). Чтобы подтянуть свежие автотесты:

```bash
git remote add -m v2 template https://github.com/Yandex-Practicum/go-musthave-metrics-tpl.git
git fetch template && git checkout template/v2 .github
```
