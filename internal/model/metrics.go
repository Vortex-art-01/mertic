// Package model описывает метрику в том виде, в котором её передают по HTTP и
// хранят в файле дампа.
package model

// Типы метрик — допустимые значения [Metrics.MType].
const (
	// Counter — счётчик: сервер складывает присланные приращения.
	Counter = "counter"

	// Gauge — датчик: новое значение заменяет прежнее.
	Gauge = "gauge"
)

// Metrics — метрика в JSON API сервера: тело запросов POST /update/ и
// POST /value/, элемент пакета POST /updates/ и запись в файле дампа.
//
// Значение лежит в одном из двух полей в зависимости от типа: у gauge — в Value,
// у counter — в Delta. Оба поля — указатели, чтобы отличать ноль от незаданного
// значения и не выводить незаданное поле в JSON:
//
//	{"id":"Alloc","type":"gauge","value":1024.5}
//	{"id":"PollCount","type":"counter","delta":5}
type Metrics struct {
	// ID — имя метрики.
	ID string `json:"id"`

	// MType — тип метрики: [Gauge] или [Counter].
	MType string `json:"type"`

	// Delta — приращение counter. В ответах сервера и в дампе — накопленная сумма.
	Delta *int64 `json:"delta,omitempty"`

	// Value — значение gauge.
	Value *float64 `json:"value,omitempty"`

	// Hash остался от шаблона проекта и не используется: подпись считается по
	// всему телу запроса и передаётся в заголовке HashSHA256 (см. пакет hash).
	Hash string `json:"hash,omitempty"`
}
