package main

// examples/basic/main.go
// ─────────────────────────────────────────────────────────────────────────────
// Minimal example: connect to a TN3270E host, wait for the first screen,
// print it as plain text, then disconnect.
//
// Usage:
//
//	go run . <host> [port]
//
// Example:
//
//	go run . 10.1.1.1 23
// ─────────────────────────────────────────────────────────────────────────────

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/wren-creator/tn3270_library/go"
)

func main() {
	host := "localhost"
	port := 23

	if len(os.Args) > 1 {
		host = os.Args[1]
	}
	if len(os.Args) > 2 {
		p, err := strconv.Atoi(os.Args[2])
		if err != nil {
			log.Fatalf("invalid port: %s", os.Args[2])
		}
		port = p
	}

	sess := tn3270e.NewSession(tn3270e.SessionOptions{
		Host:       host,
		Port:       port,
		Model:      "3278-2",
		UseTN3270E: true,
		Codepage:   37,
		Logger:     log.Default(),
	})

	if err := sess.Connect(); err != nil {
		log.Fatal("connect:", err)
	}
	defer sess.Disconnect("done")

	screenCount := 0

	for {
		select {
		case ev := <-sess.Connected():
			fmt.Printf("✓ TCP connected to %s:%d\n", ev.Host, ev.Port)

		case ev := <-sess.Ready():
			fmt.Printf("✓ Session ready  LU=%s  Model=%s\n", ev.LU, ev.Model)

		case screen := <-sess.Screen():
			screenCount++
			divider := strings.Repeat("─", screen.Cols)
			fmt.Printf("\n%s\n", divider)
			fmt.Printf("SCREEN %d  cursor=%d  %d×%d\n",
				screenCount, screen.Cursor, screen.Rows, screen.Cols)
			fmt.Printf("%s\n", divider)

			// Print each row
			for r := 0; r < screen.Rows; r++ {
				for c := 0; c < screen.Cols; c++ {
					fmt.Printf("%c", screen.Buffer[r*screen.Cols+c].Char)
				}
				fmt.Println()
			}

			fmt.Printf("%s\n", divider)
			fmt.Printf("Fields: %d total\n", len(screen.Fields))

			if screenCount == 1 {
				fmt.Println("\nFirst screen received — disconnecting.")
				return
			}

		case ev := <-sess.Errors():
			log.Fatal("session error:", ev.Err)

		case ev := <-sess.Disconnected():
			fmt.Println("Disconnected:", ev.Reason)
			return
		}
	}
}
