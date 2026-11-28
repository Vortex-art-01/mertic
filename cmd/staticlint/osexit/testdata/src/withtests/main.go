// У пакета есть тесты, поэтому go test создаёт для него ещё один пакет main —
// сгенерированный, с вызовом os.Exit в конце. О нём анализатор молчать должен,
// а о написанной вручную main — нет.
package main

import "os"

func run() int {
	return 0
}

func main() {
	os.Exit(run()) // want `direct call to os.Exit`
}
