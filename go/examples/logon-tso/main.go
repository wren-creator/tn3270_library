package main

// examples/logon-tso/main.go
// ─────────────────────────────────────────────────────────────────────────────
// Example: automate a TSO logon by detecting the logon screen,
// filling in the userid field, and pressing Enter.
//
// Usage:
//
//	go run . <host> <userid> [password]
// ─────────────────────────────────────────────────────────────────────────────

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/wren-creator/tn3270_library/go"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: logon-tso <host> <userid> [password]")
		os.Exit(1)
	}

	host   := os.Args[1]
	userid := os.Args[2]
	password := ""
	if len(os.Args) > 3 {
		password = os.Args[3]
	}

	sess := tn3270e.NewSession(tn3270e.SessionOptions{
		Host:       host,
		Port:       23,
		Model:      "3278-2",
		UseTN3270E: true,
		Codepage:   37,
	})

	if err := sess.Connect(); err != nil {
		log.Fatal("connect:", err)
	}
	defer sess.Disconnect("done")

	state := "await-logon"

	for {
		select {
		case screen := <-sess.Screen():
			text := sess.GetScreenText()

			switch state {

			case "await-logon":
				if strings.Contains(text, "ENTER USERID") ||
					strings.Contains(text, "TSO/E LOGON") {
					fmt.Println("✓ Logon screen detected")
					state = "logging-on"

					// Find first unprotected field — that's the userid field
					var useridField *tn3270e.Field
					for _, f := range screen.Fields {
						if !f.Protected {
							f := f
							useridField = &f
							break
						}
					}

					if useridField == nil {
						log.Fatal("could not find userid field")
					}

					fmt.Printf("  Typing userid into field addr=%d\n",
						useridField.StartAddr)

					err := sess.SendAidWithFields(tn3270e.AIDEnter,
						[]tn3270e.FieldData{
							{Addr: useridField.StartAddr + 1, Value: userid},
						},
					)
					if err != nil {
						log.Fatal("sendAid:", err)
					}
				}

			case "logging-on":
				if strings.Contains(text, "ENTER PASSWORD") ||
					strings.Contains(text, "PASSWORD") {
					fmt.Println("✓ Password prompt detected")
					if password == "" {
						fmt.Println("  (no password provided — leaving for operator)")
						state = "done"
						continue
					}

					var pwdField *tn3270e.Field
					for _, f := range screen.Fields {
						if !f.Protected {
							f := f
							pwdField = &f
							break
						}
					}
					if pwdField != nil {
						sess.SendAidWithFields(tn3270e.AIDEnter,
							[]tn3270e.FieldData{
								{Addr: pwdField.StartAddr + 1, Value: password},
							},
						)
						state = "done"
					}

				} else if strings.Contains(text, "READY") {
					fmt.Println("✓ TSO READY — logon successful")
					fmt.Println(text)
					return
				}
			}

		case ev := <-sess.Errors():
			log.Fatal("error:", ev.Err)

		case ev := <-sess.Disconnected():
			fmt.Println("Disconnected:", ev.Reason)
			return
		}
	}
}
