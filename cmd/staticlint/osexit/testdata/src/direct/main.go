// Прямые вызовы os.Exit в main: во всех положениях, где они могут стоять.
package main

import (
	"fmt"
	"os"
)

type app struct{}

// Метод с именем main — не точка входа в программу.
func (app) main() {
	os.Exit(1)
}

// Из других функций вызывать os.Exit можно: правило касается только main.
func run() int {
	if len(os.Args) > 2 {
		os.Exit(2)
	}

	return 0
}

func main() {
	defer fmt.Println("при выходе через os.Exit не напечатается")

	if len(os.Args) > 1 {
		os.Exit(1) // want `^direct call to os.Exit in function main of package main$`
	}

	for i := 0; i < 3; i++ {
		switch run() {
		case 0:
			os.Exit(0) // want `direct call to os.Exit`
		}
	}

	func() {
		os.Exit(3) // want `direct call to os.Exit`
	}()

	go os.Exit(4)    // want `direct call to os.Exit`
	defer os.Exit(5) // want `direct call to os.Exit`

	(os.Exit)(6) // want `direct call to os.Exit`

	app{}.main()

	os.Exit(run()) // want `direct call to os.Exit`
}
