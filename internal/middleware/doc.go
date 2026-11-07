// Package middleware содержит мидлвари HTTP-сервера: логирование запросов,
// сжатие gzip и подпись HMAC-SHA256. Сервер подключает их на весь роутер в таком
// порядке:
//
//	r.Use(middleware.WithLogging(l))
//	r.Use(middleware.WithGzip)
//	r.Use(middleware.WithHash(key))
//
// Порядок важен. WithGzip распаковывает запрос раньше, чем WithHash проверит
// подпись, а сжимает ответ уже после того, как WithHash его подписал, поэтому
// подпись в обе стороны считается по несжатому телу. WithLogging стоит снаружи и
// записывает итоговый код ответа и размер тела, ушедшего клиенту.
package middleware
