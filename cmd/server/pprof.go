package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
)

// startPprof поднимает профилировщик на отдельном адресе, а не на общем роутере:
// профили раскрывают внутренности процесса, поэтому наружу вместе с API метрик им
// незачем, а мидлвари (лог, gzip, подпись) попали бы в замеры. Занятый адрес
// останавливает сервер на старте, как и другие ошибки конфигурации.
func startPprof(addr string, l *slog.Logger) (io.Closer, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen pprof: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	srv := &http.Server{Handler: mux}

	go func() {
		if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
			l.Error("pprof server failed", slog.Any("error", err))
		}
	}()

	return srv, nil
}
