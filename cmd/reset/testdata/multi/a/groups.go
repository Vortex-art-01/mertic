package a

type (
	// G1 помечена внутри группы.
	//
	// generate:reset
	G1 struct{ s string }

	// G2 без метки.
	G2 struct{ s string }
)

// Метка над группой относится ко всем её типам.
//
// generate:reset
type (
	H1 struct{ n int }
	H2 struct{ list []string }
)

// Directive помечена директивой: go doc её не покажет.
//
//generate:reset
type Directive struct{ n int }
