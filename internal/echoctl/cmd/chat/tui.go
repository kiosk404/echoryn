package chat

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/kiosk404/echoryn/pkg/version"
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
	fmt.Printf("%s%s Eidolon Chat %s %s\n", colorBold, colorOrangeANSI, version.GitVersion, colorReset)
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
	fmt.Printf("%s%seidolon%s\n", colorBold, colorPinkANSI, colorReset)
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

// readLine reads a line of input from the user with a prompt.
// It handles Ctrl+C / Ctrl+D gracefully.
func readLine(prompt string) (string, bool) {
	fmt.Print(prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return scanner.Text(), true
	}
	// EOF or error (e.g. Ctrl+D)
	return "", false
}

// RunTUI starts the interactive chat TUI using direct terminal output.
// This approach avoids alt-screen mode so that text can be freely selected and copied.
func RunTUI(client *HivemindClient) error {
	// Handle Ctrl+C gracefully
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Printf("\n\n%sGoodbye!%s\n\n", colorDim, colorReset)
		os.Exit(0)
	}()

	printWelcomeBanner(client)

	history := []ChatMessage{}
	prompt := colorOrangeANSI + colorBold + "> " + colorReset

	for {
		input, ok := readLine(prompt)
		if !ok {
			// EOF (Ctrl+D)
			fmt.Printf("\n%sGoodbye!%s\n\n", colorDim, colorReset)
			return nil
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		switch input {
		case "/quit", "/exit":
			fmt.Printf("\n%sGoodbye!%s\n\n", colorDim, colorReset)
			return nil
		case "/clear":
			history = []ChatMessage{}
			fmt.Printf("%sConversation cleared.%s\n\n", colorGrayANSI, colorReset)
			continue
		}

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

		_, err := client.ChatStream(ctx, history, func(delta string) {
			if !firstDelta {
				// Clear "Thinking..." text
				fmt.Print("\r\033[K")
				firstDelta = true
			}
			// Write delta directly to stdout — this is the key difference from
			// alt-screen TUI: content flows naturally and can be selected/copied.
			fmt.Print(delta)
			fullContent.WriteString(delta)
		})
		cancel()

		if !firstDelta {
			// Clear "Thinking..." if no content arrived
			fmt.Print("\r\033[K")
		}

		content := fullContent.String()

		if err != nil {
			fmt.Println()
			if content != "" {
				history = append(history, ChatMessage{Role: "assistant", Content: content})
			}
			printError(err.Error())
		} else {
			fmt.Println()
			history = append(history, ChatMessage{Role: "assistant", Content: content})

			// Re-render the assistant's complete reply with markdown formatting.
			// We print it below the raw streamed text — use ANSI escape to
			// overwrite the raw output with the rendered version.
			w := getTermWidth() - 4
			rendered := renderMarkdownToTerminal(content, w)

			// Count lines of raw output to move cursor back
			rawLines := strings.Count(content, "\n") + 1
			// Move cursor up and clear
			for i := 0; i < rawLines; i++ {
				fmt.Print("\033[A\033[K")
			}
			fmt.Println(rendered)
		}

		fmt.Println()
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
	})
	return err
}
