// В этой main нет ни одного прямого вызова os.Exit, хотя на вид они есть.
package main

import (
	"log"
	"os"
)

type exiter struct{}

func (exiter) Exit(int) {}

func main() {
	// Косвенный вызов: у вызова через переменную нет статической цели.
	exit := os.Exit
	exit(0)

	// log.Fatal вызывает os.Exit сам — в main прямого вызова нет.
	if len(os.Args) > 1 {
		log.Fatal("слишком много аргументов")
	}

	// Локальная переменная заслоняет пакет: это метод exiter.Exit.
	{
		os := exiter{}
		os.Exit(1)
	}
}
