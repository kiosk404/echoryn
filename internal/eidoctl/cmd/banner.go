package cmd

import (
	"fmt"

	"github.com/kiosk404/eidolon/pkg/version"
)

const bannerText = `
  _____ _     _       _             
 | ____(_) __| | ___ | | ___  _ __  
 |  _| | |/ _` + "`" + ` |/ _ \| |/ _ \| '_ \ 
 | |___| | (_| | (_) | | (_) | | | |
 |_____|_|\__,_|\___/|_|\___/|_| |_|

      Eidolon AI Agent System
`

// Banner returns the CLI banner string.
func Banner() string {
	return fmt.Sprintf("%s\n  Version: %s\n", bannerText, version.Get().String())
}
