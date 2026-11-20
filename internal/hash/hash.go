// Package hash подписывает тела запросов и ответов по HMAC-SHA256. Подпись в hex
// передаётся в заголовке [Header]: агент подписывает отправляемые пакеты, а
// сервер проверяет присланные подписи и подписывает свои ответы.
package hash

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"hash"

	"github.com/Vortex-art-01/mertic/internal/pool"
)

// Контракт протокола подписи.
const (
	// Header — заголовок с HMAC-SHA256 от тела запроса или ответа.
	Header = "HashSHA256"

	// None — значение, которым отправитель явно сообщает, что тело не подписано.
	//
	// Сервер с ключом принимает и такие запросы, и запросы вовсе без
	// заголовка — это осознанное решение, а не недосмотр. Мидлварь стоит на
	// всём роутере, поэтому строгая проверка отвергала бы любой GET (/ping,
	// /value/...), которые никто не подписывает, а автотесты Практикума в
	// инкременте 14 шлют серверу с ключом и None, и неподписанные запросы.
	//
	// Цена компромисса: подпись ловит искажение тела, но не обязывает клиента
	// её присылать — кто заголовок не прислал, тот проверку прошёл. Если
	// понадобится строгий режим, править надо здесь, в middleware.WithHash и
	// в описании флага -k у сервера.
	None = "none"
)

// Signer считает и проверяет подписи одним ключом. hmac.New выделяет память под два
// состояния SHA-256 и блоки ключа, поэтому сервер, который проверяет каждый запрос и
// подписывает каждый ответ, переиспользует их через [pool.Pool]. Signer можно
// использовать из нескольких горутин.
type Signer struct {
	macs *pool.Pool[*mac]
}

// mac — вычислитель HMAC вместе с буфером под результат: через интерфейс hash.Hash
// буфер со стека всё равно ушёл бы в кучу. Метод Reset создаёт утилита cmd/reset, а
// вызывает пул, когда вычислитель возвращают. Сброс при возврате, а не перед
// расчётом, выгоден и здесь: первый Reset у hmac дорогой, а разовым подписям он
// вообще не нужен.
//
// generate:reset
type mac struct {
	hash hash.Hash
	sum  [sha256.Size]byte
}

func newMAC(key string) *mac {
	return &mac{hash: hmac.New(sha256.New, []byte(key))}
}

// compute возвращает срез буфера m. Вызывать его можно только на чистом вычислителе:
// только что созданном или сброшенном при возврате в пул.
func (m *mac) compute(data []byte) []byte {
	m.hash.Write(data)

	return m.hash.Sum(m.sum[:0])
}

func (m *mac) sign(data []byte) string {
	var encoded [sha256.Size * 2]byte

	hex.Encode(encoded[:], m.compute(data))

	return string(encoded[:])
}

func (m *mac) valid(data []byte, want string) bool {
	var decoded [sha256.Size]byte

	if len(want) != len(decoded)*2 {
		return false
	}
	if _, err := hex.Decode(decoded[:], []byte(want)); err != nil {
		return false
	}

	return hmac.Equal(decoded[:], m.compute(data))
}

// NewSigner возвращает Signer с ключом key.
func NewSigner(key string) *Signer {
	return &Signer{macs: pool.New(func() *mac { return newMAC(key) })}
}

// Sign возвращает подпись data в hex.
func (s *Signer) Sign(data []byte) string {
	m := s.macs.Get()
	defer s.macs.Put(m)

	return m.sign(data)
}

// Valid сообщает, совпадает ли подпись want в hex с подписью data.
func (s *Signer) Valid(data []byte, want string) bool {
	m := s.macs.Get()
	defer s.macs.Put(m)

	return m.valid(data, want)
}

// Sign подписывает data разово: для редких подписей, когда держать Signer незачем.
func Sign(data []byte, key string) string {
	return newMAC(key).sign(data)
}

// Valid проверяет подпись разово: для редких проверок, когда держать Signer незачем.
func Valid(data []byte, key, want string) bool {
	return newMAC(key).valid(data, want)
}
