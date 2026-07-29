# tn3270e (Go)

A complete TN3270/TN3270E protocol library for Go.

Handles the full mainframe terminal session lifecycle — TCP connection,
Telnet negotiation, LU binding, 3270 datastream parsing, and EBCDIC
conversion — so you can focus on your application, not the protocol.

```bash
go get github.com/YOUR_USERNAME/tn3270e@latest
```

> **Why this library exists:** TN3270E and VTAM are mature protocols powering
> thousands of mainframe installations. The engineers who built them are
> retiring. This library is both a working implementation and a documented
> reference — see [`docs/`](../docs/) for the companion glossary and protocol
> reference.

---

## Quick Start

```go
package main

import (
    "fmt"
    "log"
    "strings"

    "github.com/YOUR_USERNAME/tn3270e"
)

func main() {
    sess := tn3270e.NewSession(tn3270e.SessionOptions{
        Host:       "10.1.1.1",
        Port:       23,
        Model:      "3278-2",
        UseTN3270E: true,
        Logger:     log.Default(),
    })

    if err := sess.Connect(); err != nil {
        log.Fatal(err)
    }
    defer sess.Disconnect("done")

    for {
        select {
        case screen := <-sess.Screen():
            if strings.Contains(sess.GetScreenText(), "READY") {
                fmt.Println("TSO READY")
                return
            }

        case ev := <-sess.Errors():
            log.Fatal(ev.Err)

        case ev := <-sess.Disconnected():
            fmt.Println("Disconnected:", ev.Reason)
            return
        }
    }
}
```

---

## API

### SessionOptions

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `Host` | string | required | Mainframe hostname or IP |
| `Port` | int | required | TCP port (23, 992 TLS, 339) |
| `LUName` | string | `""` | Specific LU to request (empty = any) |
| `Model` | string | `"3278-2"` | Terminal model — determines screen size |
| `Codepage` | int | `37` | EBCDIC code page (37=US, 500=International) |
| `UseTN3270E` | bool | `true` | false = classic TN3270 (z/VM) |
| `UseTLS` | bool | `false` | Wrap in TLS |
| `TLSConfig` | `*tls.Config` | `nil` | TLS options |
| `SocketTimeoutSecs` | int | `120` | Idle timeout |
| `Logger` | `*log.Logger` | nil | Pass `log.Default()` for output |
| `ID` | string | auto | Session ID for log messages |

### Channels

```go
sess.Screen()       <-chan ScreenData      // new screen from host
sess.Errors()       <-chan ErrorEvent      // protocol/socket errors
sess.Connected()    <-chan ConnectedEvent  // TCP socket opened
sess.Ready()        <-chan ReadyEvent      // negotiation complete
sess.Disconnected() <-chan DisconnectedEvent // session ended
```

### Methods

```go
// Lifecycle
sess.Connect() error
sess.Disconnect(reason string)
sess.State() SessionState

// Sending
sess.SendAid(aid byte) error
sess.SendAidWithFields(aid byte, fields []FieldData) error
sess.SendAidAt(aid byte, cursorAddr int, fields []FieldData) error

// Reading
sess.GetScreen() []Cell
sess.GetScreenText() string
sess.GetFields() []Field
sess.GetFieldByIndex(i int) (Field, error)
sess.FindField(substr string) (Field, error)

// Cursor
sess.SetCursor(addr int)
sess.SetCursorRC(row, col int)
sess.CursorAddr() int

// Info
sess.LUName() string
sess.Model() string
sess.Rows() int
sess.Cols() int
```

### AID Constants

```go
tn3270e.AIDEnter   // 0x7D — Enter
tn3270e.AIDClear   // 0x6D — Clear
tn3270e.AIDPA1     // 0x6C — PA1
tn3270e.AIDPA2     // 0x6E — PA2
tn3270e.AIDPA3     // 0x6B — PA3
tn3270e.AIDPF1     // PF1 through PF24
// ... AIDPF2 through AIDPF24
```

### EBCDIC Utilities

```go
tn3270e.EBCDICToASCII(data []byte, cp int) string
tn3270e.ASCIIToEBCDIC(s string, cp int) []byte
tn3270e.ASCIIToEBCDICFixed(s string, n int, cp int) []byte
tn3270e.RegisterCodePage(num int, table [256]byte, name string) error
tn3270e.ListCodePages() []struct{ Number int; Name string }
```

---

## Terminal Models

| Model | Rows | Cols | Notes |
|-------|------|------|-------|
| `3278-2` | 24 | 80 | Standard — nearly universal |
| `3278-3` | 32 | 80 | Uncommon |
| `3278-4` | 43 | 80 | Uncommon |
| `3278-5` | 27 | 132 | Wide — SDSF, ISPF split-screen |
| `IBM-DYNAMIC` | negotiated | negotiated | Dynamic resize |

---

## Publishing

Tag a release and pkg.go.dev updates automatically:

```bash
git tag v1.0.0
git push origin v1.0.0
```

Users install with:
```bash
go get github.com/YOUR_USERNAME/tn3270e@v1.0.0
```

---

## See Also

- [`docs/GLOSSARY.md`](../docs/GLOSSARY.md) — TN3270E & VTAM terminology
- [`docs/PROTOCOL.md`](../docs/PROTOCOL.md) — Negotiation sequence
- [`docs/DATASTREAM.md`](../docs/DATASTREAM.md) — 3270 datastream reference
- [RFC 2355](https://www.rfc-editor.org/rfc/rfc2355) — TN3270E specification

---

## License

GPL-3.0-or-later
