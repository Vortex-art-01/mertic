// Package library — не main: функция main в нём — обычная функция, а не точка
// входа в программу.
package library

import "os"

func main() {
	os.Exit(1)
}
