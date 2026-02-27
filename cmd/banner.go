package cmd

import "fmt"

const astroBanner = "\033[36m" + `
  ⠉⠛⠿⣿⣿⣿⣿⡿ ⣸⢿⡄  ⣀⣀⡀ ⣀⣷⣀⡀⢀⣀⡀⣀⡀ ⣀⣀⡀ ⣀⣀⣀⡀ ⢀⣀⡀  ⣀⣀⣿  ⣀⣀⡀
⡀⣤⣶⣿⣿⣿⣿⣿⣿⠁ ⣿ ⣿ ⢸⣏⠉⠿⠈⠉⣿⠉⠁⠈⢹⡟⠉ ⣿⠉⠉⣿ ⣿⠉⠙⣷⢠⡟⠉⢻⣆⣾⠋⠉⣿ ⢾⡏⠉⠿
     ⣿⡿⣿⡟ ⢸⡟⠛⢿⡄⣠⡉⠛⣿  ⣿   ⢸⡇  ⣿  ⣿ ⣿  ⣿⠸⣇ ⢰⡟⣿  ⣿ ⣤⡉⠛⣿
    ⣼⠟ ⣿  ⠛  ⠈⠛ ⠛⠛⠋ ⠙⠛⠃ ⠛⠛⠛  ⠈⠛⠛⠁ ⣿⠛⠛⠁ ⠙⠛⠋  ⠛⠛⠙  ⠛⠛⠋
    ⠃  ⠇                          ` + "\033[0m"

func printBanner() {
	fmt.Println(astroBanner)
	fmt.Println()
}
