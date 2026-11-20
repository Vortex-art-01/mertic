package main

import "golang.org/x/tools/go/analysis/multichecker"

func main() {
	// Main разбирает флаги, запускает анализаторы и сама завершает процесс с
	// нужным кодом возврата: управление из неё не возвращается.
	multichecker.Main(analyzers()...)
}
