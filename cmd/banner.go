package cmd

import (
	"fmt"

	"github.com/postman/astro/apps/astro-cli/internal/theme"
)

const bannerArt = `
  ⠉⠛⠿⣿⣿⣿⣿⡿ ⣸⢿⡄  ⣀⣀⡀ ⣀⣷⣀⡀⢀⣀⡀⣀⡀ ⣀⣀⡀ ⣀⣀⣀⡀ ⢀⣀⡀  ⣀⣀⣿  ⣀⣀⡀
⡀⣤⣶⣿⣿⣿⣿⣿⣿⠁ ⣿ ⣿ ⢸⣏⠉⠿⠈⠉⣿⠉⠁⠈⢹⡟⠉ ⣿⠉⠉⣿ ⣿⠉⠙⣷⢠⡟⠉⢻⣆⣾⠋⠉⣿ ⢾⡏⠉⠿
     ⣿⡿⣿⡟ ⢸⡟⠛⢿⡄⣠⡉⠛⣿  ⣿   ⢸⡇  ⣿  ⣿ ⣿  ⣿⠸⣇ ⢰⡟⣿  ⣿ ⣤⡉⠛⣿
    ⣼⠟ ⣿  ⠛  ⠈⠛ ⠛⠛⠋ ⠙⠛⠃ ⠛⠛⠛  ⠈⠛⠛⠁ ⣿⠛⠛⠁ ⠙⠛⠋  ⠛⠛⠙  ⠛⠛⠋
    ⠃  ⠇                          `

func astroBanner() string {
	return theme.PrimaryANSI + bannerArt + "\033[0m"
}

func printBanner() {
	fmt.Println(astroBanner())
	fmt.Println()
}
