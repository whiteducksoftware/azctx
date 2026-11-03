// Command pickercapture emits a static SVG of the real picker with synthetic data.
package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/ui"
)

func main() {
	lipgloss.SetColorProfile(termenv.Ascii)
	rows := []profile.Subscription{
		{Id: "11111111-1111-1111-1111-111111111111", Name: "Production", TenantName: "Example Production", Tenant: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"},
		{Id: "22222222-2222-2222-2222-222222222222", Name: "Development", TenantName: "Example Engineering", Tenant: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"},
	}
	view := ui.NewPicker(rows, 160, 15).View()
	var svg bytes.Buffer
	svg.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="1480" height="360" viewBox="0 0 1480 360" role="img" aria-labelledby="title desc">
<title id="title">azctx built-in subscription picker</title>
<desc id="desc">Static capture of the actual 160-column picker renderer using synthetic subscription and tenant identities.</desc>
<rect width="1480" height="360" rx="12" fill="#15202b"/>
<g fill="#e6edf3" font-family="monospace" font-size="14" xml:space="preserve">
`)
	for index, line := range strings.Split(view, "\n") {
		fmt.Fprintf(&svg, `<text x="24" y="%d">`, 32+index*21)
		if err := xml.EscapeText(&svg, []byte(strings.TrimRight(line, " "))); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		svg.WriteString("</text>\n")
	}
	svg.WriteString("</g>\n</svg>\n")
	if _, err := os.Stdout.Write(svg.Bytes()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
