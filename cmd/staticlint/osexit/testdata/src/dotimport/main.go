// Пакет os импортирован через точку: Exit без квалификатора — тот же os.Exit.
package main

import . "os"

func main() {
	Exit(1) // want `direct call to os.Exit`
}
