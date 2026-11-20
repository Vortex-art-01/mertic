// Package b объявляет помеченную структуру, которую хранят другие пакеты.
package b

import "time"

// generate:reset
type Item struct {
	ID   int
	Tags []string
	At   time.Time
}

// Wrapper встраивает Item, поэтому у неё есть метод Reset — от Item.
type Wrapper struct {
	Item
	Extra int
}

// Plain — структура без метки.
type Plain struct{ N int }
