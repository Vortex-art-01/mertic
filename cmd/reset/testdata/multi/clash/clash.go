// Package clash проверяет, что имена в reset.gen.go ничего не перекрывают.
package clash

import (
	htmltemplate "html/template"
	"text/template"
	stdtime "time"
)

// time занимает имя стандартного пакета: в reset.gen.go его придётся
// импортировать под другим именем.
var time = stdtime.Second

// p совпадает с первой буквой имени Profile: получатель должен называться иначе.
type p struct{ n int }

// i совпадает с именем переменной цикла.
type i struct{ n int }

// generate:reset
type Profile struct {
	at   stdtime.Time
	cfg  p
	text template.Template
	html htmltemplate.Template
	rows [2]struct {
		id   i
		tags []string
	}
}
