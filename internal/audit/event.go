// Package audit сообщает приёмникам о метриках, принятых сервером. Он устроен
// по паттерну «Наблюдатель»: хендлеры уведомляют субъект (Subject) и не знают,
// кто на него подписан — файл, удалённый сервер или вообще никто.
package audit

import (
	"net"
	"net/http"
	"time"
)

// Event — событие аудита: какие метрики, когда и с какого адреса принял сервер.
type Event struct {
	TS        int64    `json:"ts"`
	Metrics   []string `json:"metrics"`
	IPAddress string   `json:"ip_address"`
}

// NewEvent формирует событие о метриках, принятых в запросе r.
func NewEvent(r *http.Request, metrics ...string) Event {
	return Event{
		TS:        time.Now().Unix(),
		Metrics:   metrics,
		IPAddress: remoteIP(r),
	}
}

// remoteIP берёт адрес из соединения, а не из X-Forwarded-For и подобных
// заголовков: их клиент подставляет сам, и запись аудита ничего бы не значила.
func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return host
}
