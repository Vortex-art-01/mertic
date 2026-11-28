package buildinfo_test

import (
	"fmt"

	"github.com/Vortex-art-01/mertic/internal/buildinfo"
)

// Приложение печатает сведения о сборке первым делом при старте. Дату здесь
// не задали, поэтому вместо неё выводится N/A.
func ExampleInfo() {
	// В приложении поля заполняются из переменных buildVersion, buildDate и
	// buildCommit пакета main, которые задаёт линкер.
	info := buildinfo.Info{Version: "v1.0.0", Commit: "8c6ab07"}

	fmt.Println(info)

	// Output:
	// Build version: v1.0.0
	// Build date: N/A
	// Build commit: 8c6ab07
}
