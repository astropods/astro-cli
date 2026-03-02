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
	s := theme.PrimaryANSI + bannerArt + "\033[0m"
	if theme.IsPreview {
		s += "\n" + theme.PrimaryANSI + "You are using the preview version of the astropods CLI." + "\033[0m"
	}
	return s
}

func printBanner() {
	fmt.Println(astroBanner())
	fmt.Println()
}
