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
- [Аудит](#аудит)
- [Метрики агента](#метрики-агента)
- [Структура проекта](#структура-проекта)
- [Документация](#документация)
- [Тесты](#тесты)
- [Производительность](#производительность)
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
        mw["middleware:<br/>лог → gzip → предел тела → подпись"]
        h["хендлеры"]
        mw --> h
    end

    workers -- "POST /updates/" --> mw
    h --> pg[("PostgreSQL")]
    h --> mem["память + JSON-дамп"]
    h -.->|"событие аудита"| sinks["приёмники аудита:<br/>файл, URL"]
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
- gzip для запросов и ответов (`application/json`, `text/html`); ответы до 256 байт уходят gzip-потоком без сжатия: на таких размерах deflate ничего не экономит;
- проверка подписи запросов и подпись ответов HMAC-SHA256;
- предел тела запроса — 1 МБ после распаковки gzip: на больший запрос сервер отвечает `413`, так что ни огромный пакет, ни gzip-бомба не займут его память;
- структурированные JSON-логи (`log/slog`) по каждому запросу: метод, URI, статус, размер, длительность;
- аудит принятых метрик: события пишутся в файл и отправляются на удалённый сервер, не задерживая ответ клиенту;
- graceful shutdown по `SIGINT`/`SIGTERM`: незавершённые запросы дорабатывают, последний дамп записывается на диск, очереди аудита разбираются.

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
| `--audit-file` | `AUDIT_FILE` | —                 | файл для логов [аудита](#аудит); не задан — в файл не писать                                |
| `--audit-url`  | `AUDIT_URL`  | —                 | полный URL для логов [аудита](#аудит); не задан — на сервер не отправлять                   |
| `--pprof-addr` | `PPROF_ADDRESS` | —              | адрес профилировщика `net/http/pprof`; не задан — выключен. Профили раскрывают внутренности процесса, поэтому слушать стоит только локальный адрес |

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

Коды ответов: `200` — успех; `400` — неизвестный тип, некорректное значение, невалидный JSON или подпись; `404` — не указано имя или метрика не найдена; `413` — тело запроса больше 1 МБ после распаковки; `500` — ошибка хранилища.

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

## Аудит

После успешной обработки метрик сервер формирует событие аудита и рассылает его всем заданным приёмникам. Устроено по паттерну «Наблюдатель»: хендлеры уведомляют субъект (`audit.Subject`) и не знают, кто на него подписан.

| Приёмник         | Настройка                     | Что делает                                                           |
|------------------|-------------------------------|----------------------------------------------------------------------|
| файл             | `--audit-file` / `AUDIT_FILE` | дописывает событие в конец файла отдельной строкой                   |
| удалённый сервер | `--audit-url` / `AUDIT_URL`   | отправляет событие методом `POST` с `Content-Type: application/json` |

Приёмники независимы: можно задать любой из них или оба, а если не задан ни один, аудит выключен и хендлеры даже не собирают события. Неверный путь или URL останавливают сервер при старте.

```bash
go run ./cmd/server --audit-file ./audit.log --audit-url http://localhost:9090/audit
```

Формат события:

```json
{
  "ts": 1793784813,
  "metrics": ["Alloc", "Frees"],
  "ip_address": "192.168.0.42"
}
```

| Поле         | Тип        | Описание                                                        |
|--------------|------------|-----------------------------------------------------------------|
| `ts`         | `int64`    | время события, unix timestamp в секундах                        |
| `metrics`    | `[]string` | имена принятых метрик; на пакет — одно событие со всеми именами |
| `ip_address` | `string`   | IP-адрес, с которого пришёл запрос                              |

Событие появляется после `POST /update/{type}/{name}/{value}`, `POST /update/` и `POST /updates/`. Отклонённые запросы и пустой пакет в аудит не попадают.

Поведение при сбоях и в особых случаях:

- ответ клиенту не зависит от приёмников: у каждого своя очередь на 1024 события и своя горутина, поэтому медленный или недоступный приёмник не задерживает ни приём метрик, ни другой приёмник;
- на одну доставку отводится 5 секунд, повторов нет: если приёмник недоступен или ответил не `2xx`, событие записывается в лог сервера;
- если очередь приёмника переполнена, новое событие для него теряется, и об этом тоже остаётся запись в логе;
- при остановке сервер до 5 секунд ждёт разбора очередей, затем отменяет незавершённые отправки; недоставленные события остаются в логе;
- `ip_address` — адрес соединения: заголовки вроде `X-Forwarded-For` не учитываются, потому что их может подделать клиент, так что за прокси в событии будет адрес прокси;
- файл открывается заново на каждое событие, поэтому внешняя ротация лога работает без перезапуска сервера.

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
  audit/          события аудита, их рассылка и приёмники: файл и удалённый сервер
  database/       пул соединений pgx и применение миграций
  dump/           сохранение метрик в файл и восстановление при старте
  gzippool/       переиспользование компрессоров gzip
  handler/        HTTP-хендлеры, по пакету на эндпоинт
  hash/           подпись HMAC-SHA256
  httpbody/       буферы из пула для тел запросов и ответов, разбор JSON
  logger/         JSON-логгер на slog
  middleware/     логирование, gzip, предел размера тела, проверка и выдача подписи
  model/          модель Metrics
  pgerrors/       какие ошибки PostgreSQL стоит повторять
  repository/     хранилища: в памяти и в PostgreSQL
  retry/          повтор операций с паузами
migrations/       SQL-миграции goose, встраиваются в бинарник через embed
profiles/         профили памяти сервера до и после оптимизации
scripts/          генератор нагрузки для снятия профилей
```

## Документация

Пакеты, хендлеры, хранилища и мидлвари описаны в doc-комментариях. Описание сервера с флагами и списком эндпоинтов лежит в `cmd/server`, форматы тел и коды ответов — в пакетах `internal/handler`:

```bash
go doc ./cmd/server
go doc -all ./internal/handler/updatejson
```

Работу с каждым эндпоинтом показывают примеры — функции `Example…` в файлах `example_test.go` рядом с хендлерами. Отдельно есть примеры сжатия и подписи (`internal/middleware`) и отправки пакета агентом (`internal/agent`). `go test` запускает примеры и сверяет их вывод с комментарием `// Output:`, так что пример, разошедшийся с кодом, роняет тесты:

```bash
go test -run Example ./...
```

В браузере документацию вместе с примерами показывает [godoc](https://pkg.go.dev/golang.org/x/tools/cmd/godoc). Запустите его из корня репозитория:

```bash
go install golang.org/x/tools/cmd/godoc@latest
godoc -http=localhost:6060
```

Пакеты из `internal/` godoc прячет из общего списка, но открывает по прямой ссылке, например http://localhost:6060/pkg/github.com/Vortex-art-01/mertic/internal/handler/updatejson/. Полный список вместе с ними — по адресу http://localhost:6060/pkg/github.com/Vortex-art-01/mertic/?m=all, в этом режиме видны и неэкспортированные имена.

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

Покрытие кода тестами — 77,2% без базы:

```bash
go test -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1
```

### Автотесты Практикума

В GitHub Actions на каждый pull request запускаются два набора:

- **autotests** — функциональные тесты сервера и агента из [go-autotests](https://github.com/Yandex-Practicum/go-autotests); ветка должна называться `iter<N>`, тогда прогоняются инкременты с первого по `N`-й;
- **go vet test** — `go vet` со статическим анализатором Практикума.

Как запустить автотесты локально, описано в [README go-autotests](https://github.com/Yandex-Practicum/go-autotests).

## Производительность

Бенчмарки измеряют скорость важнейших компонентов, а профиль показывает, на что сервер тратит память под нагрузкой. Профили до и после оптимизации лежат в [`profiles/`](profiles).

### Итоги оптимизации

Одни и те же 100 тысяч запросов, по три прогона на каждую версию:

| Показатель                                  | До            | После         |
|---------------------------------------------|---------------|---------------|
| `alloc_space` — выделено памяти за прогон   | 77,7 ГБ       | 655–665 МБ    |
| `alloc_objects` — создано объектов          | 24,7–24,8 млн | 11,3 млн      |
| `inuse_space` — занято в куче под нагрузкой | 9,4–12,4 МБ   | 3,2–3,8 МБ    |
| `inuse_objects` — живых объектов в куче     | 5,2–8,1 тыс.  | 6,9–13,9 тыс. |
| запросов в секунду                          | 3–7 тыс.      | 11–14 тыс.    |

Живых объектов меньше не стало: это мелкие строки и указатели из разбора JSON в запросах, которые обрабатываются в момент снятия профиля. Занимают они всего 30–120 КБ. Запросы в секунду приведены для ориентира: генератор нагрузки работает на той же машине, а у исходной версии результат заметно менялся от серии к серии.

### Сравнение профилей

Вывод команды `pprof -top -diff_base=profiles/base.pprof profiles/result.pprof`, запущенной как `go tool pprof`. Отрицательные значения — память, которую сервер больше не занимает:

```
File: server.exe
Build ID: D:\Projects\GO_YANDEX\dev\mertic\cmd\server\server.exe2026-11-06 12:05:16.371591 +0500 +05
Type: inuse_space
Time: 2026-11-06 12:05:15 +05
Showing nodes accounting for -8951.98kB, 71.75% of 12476.51kB total
Dropped 295 nodes (cum <= 62.38kB)
      flat  flat%   sum%        cum   cum%
-7788.16kB 62.42% 62.42% -8774.76kB 70.33%  compress/flate.NewWriter (inline)
    -952kB  7.63% 70.05%     -952kB  7.63%  compress/flate.(*compressor).initDeflate (inline)
  322.23kB  2.58% 67.47%   322.23kB  2.58%  bytes.growSlice
 -288.10kB  2.31% 69.78%  -288.10kB  2.31%  compress/flate.(*dictDecoder).init (inline)
 -118.31kB  0.95% 70.73%  -118.31kB  0.95%  reflect.growslice
  -85.50kB  0.69% 71.41%   -89.59kB  0.72%  io.ReadAll
  -80.39kB  0.64% 72.06%   -80.39kB  0.64%  encoding/json.(*Decoder).refill
      64kB  0.51% 71.54%    96.01kB  0.77%  encoding/json.MarshalIndent
  -58.69kB  0.47% 72.01%  -346.79kB  2.78%  compress/flate.NewReader
   55.51kB  0.44% 71.57%    55.51kB  0.44%  io.init.func1
  -18.20kB  0.15% 71.72%   151.36kB  1.21%  main.newRouter.New.func6
  -13.06kB   0.1% 71.82%  -991.17kB  7.94%  compress/gzip.NewReader (inline)
    8.71kB  0.07% 71.75%    59.33kB  0.48%  github.com/Vortex-art-01/mertic/internal/middleware.init.func1
         0     0% 71.75%    45.64kB  0.37%  bufio.(*Writer).Flush
         0     0% 71.75%    86.27kB  0.69%  bytes.(*Buffer).ReadFrom
         0     0% 71.75%   239.96kB  1.92%  bytes.(*Buffer).Write
         0     0% 71.75%   322.23kB  2.58%  bytes.(*Buffer).grow
         0     0% 71.75%  -986.60kB  7.91%  compress/flate.(*compressor).init
         0     0% 71.75%  -452.09kB  3.62%  compress/gzip.(*Reader).Reset
         0     0% 71.75%  -346.79kB  2.78%  compress/gzip.(*Reader).readHeader
         0     0% 71.75% -8784.01kB 70.40%  compress/gzip.(*Writer).Write
         0     0% 71.75%  -234.84kB  1.88%  encoding/json.(*Decoder).Decode
         0     0% 71.75%   -80.39kB  0.64%  encoding/json.(*Decoder).readValue
         0     0% 71.75%  -122.34kB  0.98%  encoding/json.(*decodeState).array
         0     0% 71.75%  -122.34kB  0.98%  encoding/json.(*decodeState).unmarshal
         0     0% 71.75%  -122.34kB  0.98%  encoding/json.(*decodeState).value
         0     0% 71.75%    91.99kB  0.74%  github.com/Vortex-art-01/mertic/internal/dump.(*File).run
         0     0% 71.75%    91.99kB  0.74%  github.com/Vortex-art-01/mertic/internal/dump.(*File).save
         0     0% 71.75%   112.11kB   0.9%  github.com/Vortex-art-01/mertic/internal/httpbody.DecodeJSON
         0     0% 71.75% -8784.01kB 70.40%  github.com/Vortex-art-01/mertic/internal/middleware.(*compressWriter).Write
         0     0% 71.75%   256.26kB  2.05%  github.com/Vortex-art-01/mertic/internal/middleware.(*hashWriter).Write
         0     0% 71.75% -8763.29kB 70.24%  github.com/Vortex-art-01/mertic/internal/middleware.(*hashWriter).flush
         0     0% 71.75% -9178.47kB 73.57%  github.com/Vortex-art-01/mertic/internal/middleware.WithGzip.func1
         0     0% 71.75%   594.35kB  4.76%  github.com/Vortex-art-01/mertic/internal/middleware.newCompressReader
         0     0% 71.75% -9139.66kB 73.25%  github.com/go-chi/chi/v5.(*Mux).ServeHTTP
         0     0% 71.75%    81.64kB  0.65%  github.com/go-chi/chi/v5.(*Mux).routeHTTP
         0     0% 71.75%   -86.69kB  0.69%  html/template.(*Template).Execute
         0     0% 71.75%    50.57kB  0.41%  io.Copy (inline)
         0     0% 71.75%    50.57kB  0.41%  io.CopyN
         0     0% 71.75%    50.57kB  0.41%  io.copyBuffer
         0     0% 71.75%    50.57kB  0.41%  io.discard.ReadFrom
         0     0% 71.75%    74.52kB   0.6%  log/slog.(*Logger).LogAttrs
         0     0% 71.75%    74.52kB   0.6%  log/slog.(*Logger).logAttrs
         0     0% 71.75%   -87.64kB   0.7%  main.newRouter.New.func4
         0     0% 71.75% -8755.96kB 70.18%  main.newRouter.WithHash.func2.1
         0     0% 71.75% -9134.64kB 73.21%  main.newRouter.WithLogging.func1.1
         0     0% 71.75%    45.64kB  0.37%  net/http.(*chunkWriter).Write
         0     0% 71.75%    45.64kB  0.37%  net/http.(*chunkWriter).writeHeader
         0     0% 71.75% -9033.09kB 72.40%  net/http.(*conn).serve
         0     0% 71.75%    69.98kB  0.56%  net/http.(*response).finishRequest
         0     0% 71.75% -9130.46kB 73.18%  net/http.HandlerFunc.ServeHTTP
         0     0% 71.75% -9135.48kB 73.22%  net/http.serverHandler.ServeHTTP
         0     0% 71.75%  -118.31kB  0.95%  reflect.Value.Grow
         0     0% 71.75%  -118.31kB  0.95%  reflect.Value.grow
         0     0% 71.75%   -63.58kB  0.51%  runtime.malg
         0     0% 71.75%   -71.57kB  0.57%  runtime.newproc.func1
         0     0% 71.75%   -72.06kB  0.58%  runtime.newproc1
         0     0% 71.75%   -81.21kB  0.65%  runtime.systemstack
         0     0% 71.75%   214.88kB  1.72%  sync.(*Pool).Get
         0     0% 71.75%    46.50kB  0.37%  sync.(*Pool).Put
         0     0% 71.75%   -73.35kB  0.59%  text/template.(*Template).Execute (inline)
         0     0% 71.75%   -73.35kB  0.59%  text/template.(*Template).execute
         0     0% 71.75%   -73.35kB  0.59%  text/template.(*state).walk
         0     0% 71.75%   -73.35kB  0.59%  text/template.(*state).walkRange
         0     0% 71.75%   -73.35kB  0.59%  text/template.(*state).walkRange.func2
```

Положительные строки — плата за повторное использование: буферы (`bytes.growSlice`) и распаковщики (`newCompressReader`) теперь живут в пулах и не освобождаются вместе с запросом.

Команда сравнивает `inuse_space`: что занято в момент снятия. Сколько памяти сервер выделял за всё время, показывает `alloc_space` из тех же файлов:

```
$ go tool pprof -top -sample_index=alloc_space -nodecount=8 -diff_base=profiles/base.pprof profiles/result.pprof
File: server.exe
Build ID: D:\Projects\GO_YANDEX\dev\mertic\cmd\server\server.exe2026-11-06 12:05:16.371591 +0500 +05
Type: alloc_space
Time: 2026-11-06 12:05:15 +05
Showing nodes accounting for -75.63GB, 97.35% of 77.68GB total
Dropped 481 nodes (cum <= 0.39GB)
Showing top 8 nodes out of 39
      flat  flat%   sum%        cum   cum%
  -58.17GB 74.88% 74.88%   -70.73GB 91.04%  compress/flate.NewWriter
  -12.21GB 15.72% 90.60%   -12.21GB 15.72%  compress/flate.(*compressor).initDeflate (inline)
   -2.40GB  3.09% 93.69%    -2.40GB  3.09%  compress/flate.(*dictDecoder).init (inline)
   -0.69GB  0.89% 94.58%    -0.69GB  0.89%  reflect.growslice
   -0.61GB  0.78% 95.37%    -0.61GB  0.78%  compress/flate.(*huffmanEncoder).generate
   -0.57GB  0.73% 96.09%    -2.97GB  3.82%  compress/flate.NewReader
   -0.51GB  0.66% 96.76%    -0.53GB  0.69%  io.ReadAll
   -0.46GB   0.6% 97.35%    -0.46GB   0.6%  encoding/json.(*Decoder).refill
```

### Что показал профиль

1. **`compress/flate.NewWriter` — 91% выделенных байт.** Сервер создавал компрессор примерно на 800 КБ на каждый ответ клиенту с `Accept-Encoding: gzip`. Агент тоже такой клиент: стандартный `http.Client` добавляет этот заголовок сам. А отвечает ему сервер строкой `{"status":"ok"}`, которая в gzip занимает 39 байт, а без сжатия — 38. Теперь ответы до 256 байт уходят gzip-потоком из одного несжатого блока: для клиента это обычный gzip, а компрессор не нужен. Просто отказаться от gzip для маленьких ответов нельзя: заголовок `Content-Encoding` проверяют автотесты 8-го инкремента. Большие ответы сжимаются по-настоящему, через один переиспользуемый компрессор (`internal/gzippool`).
2. **`gzip.NewReader` — 4,4% байт.** На каждый сжатый запрос создавался распаковщик примерно на 45 КБ. Теперь они лежат в `sync.Pool` вместе с собственным `bufio.Reader`: получив обычный `io.Reader`, `gzip.Reader.Reset` каждый раз выделял бы новый буфер на 4 КБ.
3. **`reflect.growslice`, `io.ReadAll`, `json.Decoder` — 2,2% байт.** Тело запроса и слайс под пакет метрик наращивались на каждый запрос с нуля. Теперь буферы и слайсы берутся из пула (`internal/httpbody`, `updatesjson`).
4. **Страница со списком метрик — 27,5% всех объектов при 1% запросов.** Почти все они появлялись внутри `html/template`: шаблон делал на строку списка два десятка аллокаций через `reflect`. Теперь страница собирается вручную, а имена метрик экранируются через `html.EscapeString`.
5. **`hmac.New` — 3,7% объектов.** Вычислители HMAC переиспользуются через `hash.Signer`.
6. **Мелочи, заметные только после первых пунктов.** `slog.Info` упаковывал каждый атрибут лога в `any`, теперь вместо него `LogAttrs`. Имена метрик для аудита собирались, даже когда аудит выключен. В дампе значение каждой метрики занимало отдельную аллокацию. В `splitBatch` карта наращивалась с нуля, хотя размер пакета известен.

Те же правки в сжатии уменьшили и память агента на отправку пакета: `BenchmarkGzipped` выделяет 880 байт вместо 815 КБ.

Простое решение первого пункта — положить `gzip.Writer` в `sync.Pool` — было проверено и отвергнуто. Выделения памяти оно сокращает, но удерживаемая память растёт: `sync.Pool` хранит объекты отдельно на каждый P, поэтому в нём накапливается порядка `GOMAXPROCS` компрессоров, в замере на 12 логических ядрах их было 14. Вот как менялись показатели по шагам:

| Шаг                                                   | `inuse_space` | `alloc_space` | `alloc_objects` |
|-------------------------------------------------------|---------------|---------------|-----------------|
| исходная версия                                       | 9,4–12,4 МБ   | 77,7 ГБ       | 24,7–24,8 млн   |
| `sync.Pool` для `gzip.Writer` и `gzip.Reader`         | 13,9–14,5 МБ  | 3,8–4,0 ГБ    | 21,8–21,9 млн   |
| и маленькие ответы без компрессора                    | 3,1–5,2 МБ    | 4,0–4,1 ГБ    | 22,3–22,5 млн   |
| и остальные пункты, вместо пула один запасной компрессор | 3,2–3,8 МБ | 655–665 МБ    | 11,3 млн        |

### Что осталось и чем за это заплачено

Больше всего памяти теперь выделяют разбор запроса в `net/http` (24%) и `encoding/json` (17%). На пакет из 44 метрик JSON создаёт 132 объекта из 153: имя, тип и указатель на значение для каждой метрики. Убрать их можно только собственным парсером, а он не стоит такого риска.

У оптимизаций есть цена:

- после первого большого ответа в памяти остаётся один компрессор, около 800 КБ;
- буферы и распаковщики удерживаются в пулах, пока их не очистит сборщик мусора;
- ответ до 256 байт с большим числом повторов занимает больше, чем мог бы: например, 279 байт вместо 72. Обычных ответов API это не касается: метрика в JSON занимает около 50 байт, и несжатый поток для неё даже на байт короче;
- тело запроса теперь читается целиком и разбирается через `json.Unmarshal`, поэтому мусор после JSON-значения даёт `400`. Раньше `json.Decoder` такие данные молча игнорировал. Больше 1 МБ сервер не читает: предел ставит `WithBodyLimit` на весь роутер, а на тело крупнее он отвечает `413`.

### Бенчмарки

```bash
go test -run='^$' -bench=. -benchmem ./...
```

`BenchmarkRouter` гоняет запросы через весь сервер, кроме сети: роутер, мидлвари, хендлер и хранилище в памяти. Остальные бенчмарки измеряют хранилище, подпись, мидлвари, дамп, аудит и агента по отдельности. Записи в PostgreSQL среди них нет, потому что ей нужна живая база: без неё измеряется только разбор пакета (`BenchmarkSplitBatch`).

Медиана пяти запусков, до и после оптимизации:

| Бенчмарк                                                | ns/op            | B/op            | allocs/op   |
|---------------------------------------------------------|------------------|-----------------|-------------|
| `Router/updates_agent` — сжатый подписанный пакет       | 426246 → 47499   | 885553 → 2117   | 221 → 153   |
| `Router/updates_plain`                                  | 59177 → 36249    | 18403 → 1719    | 161 → 141   |
| `Router/update_json_gauge`                              | 7419 → 3788      | 1956 → 906      | 25 → 16     |
| `Router/update_json_counter_gzip`                       | 497689 → 4688    | 817877 → 1155   | 48 → 21     |
| `Router/value_json_gzip`                                | 501725 → 3800    | 817850 → 1107   | 46 → 19     |
| `Router/value_url`                                      | 2673 → 2164      | 1001 → 761      | 12 → 7      |
| `Router/index_gzip`                                     | 582475 → 49465   | 838677 → 6452   | 636 → 16    |
| `MiddlewareChain` — все мидлвари на запросе агента      | 579964 → 7248    | 867419 → 452    | 68 → 15     |
| `WithGzip/response`                                     | 240500 → 631     | 813957 → 192    | 22 → 7      |
| `WithGzip/request`                                      | 19140 → 1594     | 45576 → 32      | 10 → 2      |
| `WithHash/signed`                                       | 7822 → 2896      | 6800 → 256      | 32 → 9      |
| `WithLogging`                                           | 1625 → 1229      | 304 → 64        | 8 → 3       |
| `Gzipped` — сжатие пакета в агенте                      | 301328 → 34536   | 814738 → 880    | 21 → 4      |
| `ClientSendBatch` — отправка пакета агентом             | 703047 → 198115  | 855957 → 11182  | 132 → 95    |
| `File/save/1000` — дамп 1000 метрик                     | 4833031 → 3186805 | 332333 → 332918 | 1056 → 58  |
| `SplitBatch` — разбор пакета для PostgreSQL             | 4353 → 2610      | 3208 → 1832     | 7 → 3       |

Сравнивать время стоит, только когда оно изменилось многократно. Бенчмарки кода, который не менялся, разошлись между этими запусками почти вдвое: например, `Collector/PollSystem` — с 35477 до 17855 ns/op. Память и аллокации воспроизводятся точно.

Таблица снята до того, как появился предел размера тела. `WithBodyLimit` стоит каждому запросу с телом одну аллокацию на 64 байта — обёртку `http.MaxBytesReader`: у `Router/updates_agent` теперь около 2180 B/op и 154 allocs/op, у `MiddlewareChain` — около 517 B/op и 16 allocs/op. Запросы без тела, то есть GET и обновление через URL, предел не трогает. Зато `/update/` и `/update/{type}/{name}/{value}` при выключенном аудите больше не собирают событие и экономят на этом одну аллокацию, поэтому у `Router/update_json_gauge` их по-прежнему 16.

### Как снять профиль

Запустите сервер с профилировщиком:

```bash
go build -o cmd/server/server ./cmd/server
GODEBUG=memprofilerate=4096 ./cmd/server/server --pprof-addr localhost:6060 \
  -k secret -f /tmp/metrics-db.json -i 1 -r=false > /dev/null
```

В соседнем терминале дайте нагрузку и снимите профиль:

```bash
go run scripts/loadgen.go -key secret -out profiles/result.pprof
```

`profiles/base.pprof` снят таким же способом со сборки кода до оптимизации, в которую был добавлен только флаг `--pprof-addr`. В каждый из файлов попал средний по `inuse_space` из трёх прогонов.

Генератор шлёт смесь запросов от 32 клиентов и после 100 тысяч запросов снимает профиль, не останавливая нагрузку. Число запросов фиксировано, а не время, иначе более быстрая версия выделяла бы больше памяти только потому, что успела больше сделать.

| Доля  | Запрос                                                                        |
|-------|-------------------------------------------------------------------------------|
| 80%   | `POST /updates/` — пакет как у агента: 43 метрики, gzip, подпись              |
| 8%    | `POST /update/` — JSON                                                        |
| 2%    | `POST /update/gauge/{name}/{value}`                                           |
| 5%    | `POST /value/` — JSON                                                         |
| 4%    | `GET /value/gauge/{name}`                                                     |
| 1%    | `GET /` — страница примерно с 550 метриками                                   |

`memprofilerate=4096` делает выборку профиля намного чаще: по умолчанию одна запись в профиле представляет примерно 512 КБ, и три прогона одной и той же версии показали `inuse_space` от 11 до 21 МБ. С частой выборкой разброс заметно меньше.

Изучать профиль удобно так; метрика выбирается через `-sample_index`:

```bash
go tool pprof -top -sample_index=alloc_space profiles/base.pprof
go tool pprof -peek 'flate.NewWriter' -sample_index=alloc_space profiles/base.pprof
go tool pprof -list 'middleware.WithGzip' -sample_index=alloc_space profiles/base.pprof
go tool pprof -web -sample_index=alloc_objects profiles/base.pprof
```

Для `-web` нужен [Graphviz](https://graphviz.org/).

## Обновление шаблона

Репозиторий создан из шаблона [go-musthave-metrics-tpl](https://github.com/Yandex-Practicum/go-musthave-metrics-tpl). Чтобы подтянуть свежие автотесты:

```bash
git remote add -m v2 template https://github.com/Yandex-Practicum/go-musthave-metrics-tpl.git
git fetch template && git checkout template/v2 .github
```
