package cmd

import (
	"fmt"

	"github.com/kiosk404/echoryn/pkg/version"
)

const bannerText = `
  _____     _                            
 | ____|___| |__   ___  _ __ _   _ _ __  
 |  _| / __| '_ \ / _ \| '__| | | | '_ \ 
 | |__| (__| | | | (_) | |  | |_| | | | |
 |_____\___|_| |_|\___/|_|   \__, |_| |_|
                             |___/      
       Echoryn AI Agent System`

// Banner returns the CLI banner string.
func Banner() string {
	return fmt.Sprintf("%s\n  Version: %s\n", bannerText, version.Get().String())
}
