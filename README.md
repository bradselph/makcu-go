#  makcu-go — Go port of the Makcu library

[![Go Reference](https://pkg.go.dev/badge/github.com/bradselph/makcu-go.svg)](https://pkg.go.dev/github.com/bradselph/makcu-go)

A high-performance Go library for controlling Makcu devices

---

## Installation

```bash
go get github.com/bradselph/makcu-go
```

Requires Go 1.26+. Pulls in [`go.bug.st/serial`](https://github.com/bugst/go-serial) for
cross-platform serial I/O and `golang.org/x/sys` for the Windows cursor calls used by
`MoveAbs`.

---

## Quick start

```go
package main

import (
	"fmt"
	"log"

	makcu "github.com/bradselph/makcu-go"
)

func main() {
	// Create and connect (auto-discovers the device, or pass WithFallbackPort).
	c, err := makcu.CreateController(makcu.WithDebug(false), makcu.WithAutoReconnect(true))
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close() // == Disconnect()

	// Basic operations
	_ = c.Click(makcu.MouseLeft)
	_ = c.Move(100, 50)
	_ = c.Scroll(-1)

	// Human-like interaction
	_ = c.ClickHumanLike(makcu.MouseLeft, 2, "gaming", 3)

	v, _ := c.GetFirmwareVersion()
	fmt.Println("firmware:", v)
}
```
### Concurrency

All methods are synchronous, and the transport is safe to call from multiple goroutines. To run operations concurrently, use normal goroutines:

```go
var wg sync.WaitGroup

for _, fn := range []func() error{
func() error { return c.Move(100, 0) },
func() error { return c.Click(makcu.MouseLeft) },
func() error { return c.Scroll(-1) },
} {
wg.Add(1)

go func(f func() error) {
defer wg.Done()
_ = f()
}(fn)
}

wg.Wait()
```

---

## API overview

### Construction

```go
makcu.NewMakcuController(opts ...Option) *MakcuController // build only
makcu.CreateController(opts ...Option) (*MakcuController, error) // build + Connect
makcu.CreateAsyncController(opts ...Option) (*MakcuController, error)

// Options (defaults: SendInit=true, AutoReconnect=true)
makcu.WithFallbackPort("COM3")
makcu.WithDebug(true)
makcu.WithSendInit(false)
makcu.WithAutoReconnect(false)
makcu.WithOverridePort(true)
```

### Buttons & movement

```go
c.Click(b)             c.DoubleClick(b)       c.Press(b)        c.Release(b)
c.Move(dx, dy)         c.MoveSmooth(dx, dy, segments)
c.MoveBezier(dx, dy, segments, ctrlX, ctrlY) // *int; pass nil for dx/2, dy/2
c.MoveAbs(target, speed, waitMs, debug)        // Windows only
c.Scroll(delta)
c.Drag(startX, startY, endX, endY, button, durationSeconds)
c.ClickHumanLike(b, count, profile, jitter) // profile: fast|normal|slow|variable|gaming
c.BatchExecute([]func () error{ ... })
```

### Locking

```go
c.Lock(makcu.MouseLeft)   c.Unlock(makcu.MouseRight) // MouseButton
c.Lock("X")               c.Unlock("Y") // axis, case-insensitive
c.LockLeft(true)  c.LockRight(true)  c.LockMiddle(true)
c.LockSide1(true) c.LockSide2(true)  c.LockX(true)  c.LockY(true)
c.LockMouseX(true) c.LockMouseY(true)
locked, err := c.IsLocked(makcu.MouseLeft)
states, err := c.GetAllLockStates() // map[string]bool keyed "LEFT","X",...
```

### Button monitoring

```go
_ = c.SetButtonCallback(func (b makcu.MouseButton, pressed bool) {
fmt.Printf("%s %v\n", b.Name(), pressed)
})
_ = c.EnableButtonMonitoring(true)
mask, err := c.GetButtonMask()
states, err := c.GetButtonStates() // map[string]bool keyed "left","right",...
down, err := c.IsPressed(makcu.MouseRight)
```

### Device & connection

```go
info, err := c.GetDeviceInfo() // map[string]any: port, description, vid, pid, isConnected
v, err := c.GetFirmwareVersion()
_ = c.SpoofSerial("CUSTOM123456")
_ = c.ResetSerial()
c.OnConnectionChange(func (connected bool) { /* ... */ })
ok := c.IsConnected()
```

### Low-level transport

`c.Transport` is the `*SerialTransport`; `c.Mouse` is the `*Mouse`. You can issue raw
commands:

```go
resp, err := c.Transport.SendCommand("km.version()", true, 100*time.Millisecond)
```

### Errors

```go
var connErr *makcu.MakcuConnectionError
if errors.As(err, &connErr) { /* ... */ }
// Also: *MakcuCommandError, *MakcuTimeoutError, *MakcuResponseError, base *MakcuError.
```

---

## Command-line tool

```bash
go run ./cmd/makcu --debug          # interactive raw-command console
go run ./cmd/makcu --testPort COM3  # test a specific port
go run ./cmd/makcu --runtest        # runs `go test ./...`
```

## Tests

```bash
go test ./...           # unit tests (no hardware needed; HW tests self-skip)
go test -race ./...     # with the race detector
MAKCU_PORT=COM3 go test ./...   # also run the hardware-dependent tests
```

Hardware tests skip automatically when no Makcu device is detected (or set `MAKCU_PORT`).

---

## License

GPL License