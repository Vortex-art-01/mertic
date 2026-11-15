// Пакет os импортирован под другим именем: вызов всё равно прямой.
package main

import stdos "os"

func main() {
	stdos.Exit(1) // want `direct call to os.Exit`
}
