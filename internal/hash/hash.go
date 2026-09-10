package hash

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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

func Sign(data []byte, key string) string {
	return hex.EncodeToString(sum(data, key))
}

func Valid(data []byte, key, want string) bool {
	got, err := hex.DecodeString(want)
	if err != nil {
		return false
	}

	return hmac.Equal(got, sum(data, key))
}

func sum(data []byte, key string) []byte {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(data)

	return mac.Sum(nil)
}
