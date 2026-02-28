package chat

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/glamour"
	"github.com/kiosk404/echoryn/pkg/version"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

// ANSI color helpers using raw escape codes — no OSC queries, no termenv auto-detect.
var (
	colorReset      = "\033[0m"
	colorBold       = "\033[1m"
	colorDim        = "\033[2m"
	colorOrangeANSI = "\033[38;5;208m"
	colorBlueANSI   = "\033[38;5;39m"
	colorPinkANSI   = "\033[38;5;212m"
	colorGrayANSI   = "\033[38;5;241m"
	colorRedANSI    = "\033[38;5;196m"
)

func getTermWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

// printWelcomeBanner outputs the welcome banner once at startup.
func printWelcomeBanner(client *HivemindClient) {
	w := getTermWidth()

	sep := colorOrangeANSI + strings.Repeat("-", w) + colorReset
	fmt.Println(sep)
	fmt.Printf("%s%s Echoryn Chat %s %s\n", colorBold, colorOrangeANSI, version.GitVersion, colorReset)
	fmt.Println()
	fmt.Printf("  Model:   %s\n", client.Model)
	fmt.Printf("  Server:  %s\n", client.BaseURL)
	if client.SessionKey != "" {
		fmt.Printf("  Session: %s\n", client.SessionKey)
	}
	fmt.Println()
	fmt.Printf("%sTips:%s\n", colorOrangeANSI+colorBold, colorReset)
	fmt.Println("  Type a message and press Enter to send")
	fmt.Println("  /clear  - reset conversation")
	fmt.Println("  /quit   - exit")
	fmt.Println("  Ctrl+C  - exit")
	fmt.Println(sep)
	fmt.Println()
}

// printSeparator prints a dim horizontal rule.
func printSeparator() {
	w := getTermWidth()
	n := w - 2
	if n < 20 {
		n = 20
	}
	fmt.Printf("%s%s%s\n", colorGrayANSI, strings.Repeat("-", n), colorReset)
}

// printUserMessage displays the user's message.
func printUserMessage(msg string) {
	printSeparator()
	fmt.Printf("%s%syou%s\n", colorBold, colorBlueANSI, colorReset)
	fmt.Printf("%s%s%s\n", colorBlueANSI, msg, colorReset)
}

// printAssistantLabel outputs the assistant name label.
func printAssistantLabel() {
	printSeparator()
	fmt.Printf("%s%sechoryn%s\n", colorBold, colorPinkANSI, colorReset)
}

// printError outputs an error message.
func printError(msg string) {
	fmt.Printf("%s%sError: %s%s\n", colorBold, colorRedANSI, msg, colorReset)
}

// renderMarkdownToTerminal renders markdown content for terminal display.
func renderMarkdownToTerminal(content string, width int) string {
	if width <= 0 {
		width = 76
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithColorProfile(termenv.ANSI256),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return content
	}
	rendered, err := r.Render(content)
	if err != nil {
		return content
	}
	return strings.TrimRight(rendered, "\n")
}

// readLine reads one line of user input in raw mode with correct CJK/Unicode
// wide-character support for backspace.
//
// macOS tty line discipline (cooked mode) does NOT correctly handle backspace
// for CJK double-width characters — it only erases 1 column instead of 2,
// leaving half-character artifacts. To fix this we stay in raw mode and handle
// key-by-key input ourselves, using go-runewidth to determine each character's
// display width.
//
// Supported editing keys:
//   - Backspace/Delete: erase last character (correct width for CJK)
//   - Ctrl+U: clear entire line
//   - Ctrl+W: delete last word
//   - Ctrl+C: exit
//   - Ctrl+D on empty line: EOF
//   - Enter: submit line
func readLine() (string, bool) {
	// Print the coloured prompt.
	fmt.Print(colorOrangeANSI + colorBold + "> " + colorReset)

	var runes []rune // the line buffer as runes
	var buf [4]byte  // small buffer for reading stdin byte-by-byte

	for {
		// Read one byte at a time.
		n, err := os.Stdin.Read(buf[:1])
		if n == 0 || err != nil {
			return "", false // EOF
		}

		b := buf[0]

		switch {
		case b == '\r' || b == '\n':
			// Enter — submit line.
			// In raw mode \n doesn't do carriage-return, so emit \r\n.
			fmt.Print("\r\n")
			return string(runes), true

		case b == 3: // Ctrl+C
			fmt.Print("\r\n")
			return "", false

		case b == 4: // Ctrl+D
			if len(runes) == 0 {
				fmt.Print("\r\n")
				return "", false
			}
			// Non-empty line: ignore Ctrl+D (same as bash).

		case b == 21: // Ctrl+U — clear line
			// Erase the entire visible line content.
			eraseRunes(runes)
			runes = runes[:0]

		case b == 23: // Ctrl+W — delete last word
			// Delete trailing spaces, then non-space characters.
			i := len(runes)
			for i > 0 && runes[i-1] == ' ' {
				i--
			}
			for i > 0 && runes[i-1] != ' ' {
				i--
			}
			eraseRunes(runes[i:])
			runes = runes[:i]

		case b == 127 || b == 8: // Backspace (DEL) or Ctrl+H
			if len(runes) > 0 {
				last := runes[len(runes)-1]
				w := runewidth.RuneWidth(last)
				runes = runes[:len(runes)-1]
				// Move cursor back w columns, overwrite with spaces, move back again.
				for i := 0; i < w; i++ {
					fmt.Print("\b \b")
				}
			}

		case b == 27: // ESC — start of escape sequence
			// Read and discard escape sequences (arrow keys, etc.)
			// so they don't produce garbage. We don't implement arrow
			// key editing for simplicity.
			os.Stdin.Read(buf[:1]) // usually '[' for CSI
			if buf[0] == '[' {
				// Read until a letter byte (the final byte of CSI sequence).
				for {
					os.Stdin.Read(buf[:1])
					if buf[0] >= 0x40 && buf[0] <= 0x7E {
						break
					}
				}
			}

		default:
			// Regular character. Could be multi-byte UTF-8.
			// Determine how many bytes this UTF-8 character needs.
			var charBytes []byte
			if b < 0x80 {
				// ASCII
				charBytes = []byte{b}
			} else {
				// Multi-byte UTF-8: figure out total length from leading byte.
				var size int
				switch {
				case b&0xE0 == 0xC0:
					size = 2
				case b&0xF0 == 0xE0:
					size = 3
				case b&0xF8 == 0xF0:
					size = 4
				default:
					// Continuation byte or invalid — skip.
					continue
				}
				charBytes = make([]byte, size)
				charBytes[0] = b
				// Read remaining bytes.
				for i := 1; i < size; i++ {
					nn, err := os.Stdin.Read(buf[:1])
					if nn == 0 || err != nil {
						return "", false
					}
					charBytes[i] = buf[0]
				}
			}

			r, _ := utf8.DecodeRune(charBytes)
			if r == utf8.RuneError {
				continue
			}

			// Control chars other than the ones handled above — ignore.
			if r < 32 {
				continue
			}

			runes = append(runes, r)
			// Echo the character to terminal.
			fmt.Print(string(r))
		}
	}
}

// eraseRunes erases the given runes from the terminal display (moves cursor
// back by the correct display width for each rune, including CJK double-width).
func eraseRunes(runes []rune) {
	for i := len(runes) - 1; i >= 0; i-- {
		w := runewidth.RuneWidth(runes[i])
		for j := 0; j < w; j++ {
			fmt.Print("\b \b")
		}
	}
}

// RunTUI starts the interactive chat TUI using direct terminal output.
// This approach avoids alt-screen mode so that text can be freely selected and copied.
func RunTUI(client *HivemindClient) error {
	// Handle Ctrl+C gracefully
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	printWelcomeBanner(client)

	// Switch stdin to raw mode so we can handle key-by-key input with
	// correct CJK wide-character backspace support.
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("failed to set raw mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	// Restore terminal on Ctrl+C / SIGTERM before exiting.
	go func() {
		<-sigCh
		term.Restore(fd, oldState)
		fmt.Printf("\n\n%sGoodbye!%s\n\n", colorDim, colorReset)
		os.Exit(0)
	}()

	history := []ChatMessage{}

	for {
		input, ok := readLine()
		if !ok {
			// EOF (Ctrl+D) or Ctrl+C
			term.Restore(fd, oldState)
			fmt.Printf("\n%sGoodbye!%s\n\n", colorDim, colorReset)
			return nil
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		switch input {
		case "/quit", "/exit":
			term.Restore(fd, oldState)
			fmt.Printf("\n%sGoodbye!%s\n\n", colorDim, colorReset)
			return nil
		case "/clear":
			history = []ChatMessage{}
			fmt.Printf("%sConversation cleared.%s\r\n\r\n", colorGrayANSI, colorReset)
			continue
		}

		// Temporarily restore cooked mode for correct output (so \n works
		// as expected without needing \r\n everywhere).
		term.Restore(fd, oldState)

		// Display user message
		printUserMessage(input)

		// Add to history
		history = append(history, ChatMessage{Role: "user", Content: input})

		// Show assistant label and start streaming
		printAssistantLabel()

		// Spinner-like "thinking" indicator
		fmt.Printf("%sThinking...%s", colorGrayANSI, colorReset)

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)

		var firstDelta bool
		var fullContent strings.Builder
		var toolCallLines int // extra terminal lines occupied by tool call indicators

		_, streamErr := client.ChatStream(ctx, history, func(delta string) {
			if !firstDelta {
				fmt.Print("\r\033[K")
				firstDelta = true
			}
			fmt.Print(delta)
			fullContent.WriteString(delta)
		}, func(toolName string) {
			if !firstDelta {
				fmt.Print("\r\033[K")
				firstDelta = true
			}
			fmt.Printf("\n%s⚡ calling %s ...%s", colorGrayANSI, toolName, colorReset)
			toolCallLines++
		})
		cancel()

		if !firstDelta {
			fmt.Print("\r\033[K")
		}

		content := fullContent.String()

		if streamErr != nil {
			fmt.Println()
			if content != "" {
				history = append(history, ChatMessage{Role: "assistant", Content: content})
			}
			printError(streamErr.Error())
		} else {
			fmt.Println()
			history = append(history, ChatMessage{Role: "assistant", Content: content})

			// Re-render the assistant's complete reply with markdown formatting.
			w := getTermWidth() - 4
			rendered := renderMarkdownToTerminal(content, w)

			// Count lines of raw output to move cursor back.
			// Include both content lines and tool-call indicator lines.
			rawLines := strings.Count(content, "\n") + 1 + toolCallLines
			for i := 0; i < rawLines; i++ {
				fmt.Print("\033[A\033[K")
			}
			fmt.Println(rendered)
		}

		fmt.Println()

		// Re-enter raw mode for next input.
		term.MakeRaw(fd)
	}
}

// RunOnce performs a single chat request (non-interactive mode) with streaming output to stdout.
func RunOnce(client *HivemindClient, message string, out func(string)) error {
	messages := []ChatMessage{{Role: "user", Content: message}}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	_, err := client.ChatStream(ctx, messages, func(delta string) {
		if out != nil {
			out(delta)
		}
	}, nil)
	return err
}
