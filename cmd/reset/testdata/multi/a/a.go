// Package a хранит поля с типами из других пакетов.
package a

import (
	"example.com/multi/b"
	"example.com/multi/c"
)

// generate:reset
type A struct {
	item  b.Item    // помечена в другом пакете: Reset появится в этом же запуске
	pitem *b.Item   // то же по указателю
	wrap  b.Wrapper // встраивает b.Item и получает её Reset
	plain b.Plain   // без метки: обнуляется
	old   c.Old     // её Reset в старом reset.gen.go будет удалён: обнуляется
}
