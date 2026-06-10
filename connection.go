package makcu

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

const (
	Version = "2.3.0"
	Author  = "bradselph"
)

var baudChangeCommand = []byte{0xDE, 0xAD, 0x05, 0x00, 0xA5, 0x00, 0x09, 0x3D, 0x00}

const (
	defaultTimeout       = 100 * time.Millisecond
	maxReconnectAttempts = 3
	reconnectDelay       = 100 * time.Millisecond
)

var buttonMap = [5]string{"left", "right", "middle", "mouse4", "mouse5"}
var buttonEnumMap = [5]MouseButton{MouseLeft, MouseRight, MouseMiddle, MouseMouse4, MouseMouse5}

type pendingCommand struct {
	commandID      int
	command        string
	result         chan string
	timestamp      time.Time
	expectResponse bool
	timeout        time.Duration
}

type SerialTransport struct {
	fallbackComPort string
	debug           bool
	sendInit        bool
	autoReconnect   bool
	overridePort    bool

	connected         atomic.Bool
	stopRequested     atomic.Bool
	reconnectAttempts int
	port              string
	baudrate          int
	currentBaud       int

	serial   serial.Port
	serialMu sync.Mutex
	writeMu  sync.Mutex

	commandCounter int
	pending        map[int]*pendingCommand
	pendingOrder   []int
	cmdMu          sync.Mutex

	buttonCallback func(MouseButton, bool)
	lastButtonMask int
	buttonStates   int
	stateMu        sync.Mutex

	listenerDone chan struct{}
}

func NewSerialTransport(fallback string, debug, sendInit, autoReconnect, overridePort bool) *SerialTransport {
	t := &SerialTransport{
		fallbackComPort: fallback,
		debug:           debug,
		sendInit:        sendInit,
		autoReconnect:   autoReconnect,
		overridePort:    overridePort,
		baudrate:        115200,
		pending:         make(map[int]*pendingCommand),
		listenerDone:    make(chan struct{}),
	}

	t.log("Makcu version: "+Version, "INFO")
	t.log(fmt.Sprintf("Initializing SerialTransport with params: fallback='%s', debug=%v, send_init=%v, auto_reconnect=%v, override_port=%v",
		fallback, debug, sendInit, autoReconnect, overridePort), "INFO")
	t.log("SerialTransport initialization completed", "INFO")
	return t
}

func (t *SerialTransport) log(message, level string) {
	if !t.debug {
		return
	}
	timestamp := time.Now().Format("15:04:05")
	fmt.Printf("[%s] [%s] %s\n", timestamp, level, message)
}

func (t *SerialTransport) generateCommandID() int {
	t.commandCounter = (t.commandCounter + 1) & 0x2710
	return t.commandCounter
}

func (t *SerialTransport) FindComPort() (string, bool) {
	t.log("Starting COM port discovery", "INFO")

	if t.overridePort {
		t.log("Override port enabled, using: "+t.fallbackComPort, "INFO")
		if t.fallbackComPort == "" {
			return "", false
		}
		return t.fallbackComPort, true
	}

	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		t.log("Error listing ports: "+err.Error(), "ERROR")
	} else {
		t.log(fmt.Sprintf("Found %d COM ports total", len(ports)), "INFO")
		for i, p := range ports {
			t.log(fmt.Sprintf("Port %d: %s - IsUSB: %v VID: %s PID: %s", i, p.Name, p.IsUSB, p.VID, p.PID), "INFO")
			if p.IsUSB && strings.EqualFold(p.VID, "1A86") && strings.EqualFold(p.PID, "55D3") {
				t.log("Target device found on port: "+p.Name, "INFO")
				return p.Name, true
			}
		}
	}

	t.log("Target device not found in COM port scan", "INFO")

	if t.fallbackComPort != "" {
		t.log("Using fallback COM port: "+t.fallbackComPort, "INFO")
		return t.fallbackComPort, true
	}

	t.log("No fallback port specified, returning None", "INFO")
	return "", false
}

func (t *SerialTransport) parseResponseLine(line []byte) (content string) {
	body := line
	if len(line) >= 4 && string(line[:4]) == ">>> " {
		body = line[4:]
	}

	ascii := make([]byte, 0, len(body))
	for _, b := range body {
		if b < 0x80 {
			ascii = append(ascii, b)
		}
	}
	content = strings.TrimSpace(string(ascii))
	return content
}

func (t *SerialTransport) handleButtonData(byteVal int) {
	t.stateMu.Lock()
	if byteVal == t.lastButtonMask {
		t.stateMu.Unlock()
		return
	}

	changedBits := byteVal ^ t.lastButtonMask
	fmt.Print("\n")
	t.log(fmt.Sprintf("Button state changed: 0x%02X -> 0x%02X", t.lastButtonMask, byteVal), "INFO")

	for bit := 0; bit < 8; bit++ {
		if changedBits&(1<<bit) != 0 {
			isPressed := (byteVal & (1 << bit)) != 0
			var buttonName string
			if bit < len(buttonMap) {
				buttonName = buttonMap[bit]
			} else {
				buttonName = fmt.Sprintf("bit%d", bit)
			}

			t.log(fmt.Sprintf("Button %s: %s", buttonName, func() string {
				if isPressed {
					return "PRESSED"
				}
				return "RELEASED"
			}()), "INFO")
			fmt.Print(">>> ")
			if isPressed {
				t.buttonStates |= 1 << bit
			} else {
				t.buttonStates &^= 1 << bit
			}

			if cb := t.buttonCallback; cb != nil && bit < len(buttonEnumMap) {
				btn := buttonEnumMap[bit]
				t.stateMu.Unlock()
				func() {
					defer func() {
						if r := recover(); r != nil {
							t.log(fmt.Sprintf("Button callback failed: %v", r), "ERROR")
						}
					}()
					cb(btn, isPressed)
				}()
				t.stateMu.Lock()
			}
		}
	}

	t.lastButtonMask = byteVal
	t.stateMu.Unlock()
}

func (t *SerialTransport) processPendingCommands(content string) {
	if content == "" {
		return
	}

	t.cmdMu.Lock()
	if len(t.pendingOrder) == 0 {
		t.cmdMu.Unlock()
		return
	}

	oldestID := t.pendingOrder[0]
	pending, ok := t.pending[oldestID]
	if !ok {
		t.cmdMu.Unlock()
		return
	}

	if content == pending.command {
		if !pending.expectResponse {
			pending.result <- pending.command

			delete(t.pending, oldestID)
			t.pendingOrder = t.pendingOrder[1:]
		}

	} else {
		pending.result <- content
		delete(t.pending, oldestID)
		t.pendingOrder = t.pendingOrder[1:]
	}
	t.cmdMu.Unlock()
}

func (t *SerialTransport) cleanupTimedOutCommands() {
	t.cmdMu.Lock()
	if len(t.pendingOrder) == 0 {
		t.cmdMu.Unlock()
		return
	}

	now := time.Now()
	var toRemove []int
	for _, cmdID := range t.pendingOrder {
		pending := t.pending[cmdID]
		if now.Sub(pending.timestamp) > pending.timeout {
			age := now.Sub(pending.timestamp)
			t.log(fmt.Sprintf("Command '%s' timed out after %.3fs", pending.command, age.Seconds()), "ERROR")
			toRemove = append(toRemove, cmdID)
		}
	}

	for _, cmdID := range toRemove {

		delete(t.pending, cmdID)
		for i, id := range t.pendingOrder {
			if id == cmdID {
				t.pendingOrder = append(t.pendingOrder[:i], t.pendingOrder[i+1:]...)
				break
			}
		}
	}
	t.cmdMu.Unlock()
}

func (t *SerialTransport) currentPort() serial.Port {
	t.serialMu.Lock()
	defer t.serialMu.Unlock()
	return t.serial
}

func (t *SerialTransport) listen() {
	defer close(t.listenerDone)
	t.log("Starting listener thread", "INFO")

	buf := make([]byte, 4096)
	lineBuffer := make([]byte, 256)
	linePos := 0

	lastCleanup := time.Now()
	cleanupInterval := 50 * time.Millisecond

	expectingTextMode := false
	lastByte := -1

	for t.connected.Load() && !t.stopRequested.Load() {

		t.serialMu.Lock()
		p := t.serial
		t.serialMu.Unlock()

		if p == nil {
			time.Sleep(time.Millisecond)
			continue
		}

		n, err := p.Read(buf)
		if err != nil {
			if !t.connected.Load() || t.stopRequested.Load() {
				break
			}
			t.log("Serial exception in listener: "+err.Error(), "ERROR")
			if t.autoReconnect {
				t.attemptReconnect()
			} else {
				break
			}
			continue
		}
		if n == 0 {

			if time.Since(lastCleanup) > cleanupInterval {
				t.cleanupTimedOutCommands()
				lastCleanup = time.Now()
			}
			continue
		}

		for _, b := range buf[:n] {
			byteVal := int(b)

			if lastByte == 0x0D && byteVal == 0x0A {
				if linePos > 0 {
					line := make([]byte, linePos)
					copy(line, lineBuffer[:linePos])
					linePos = 0
					content := t.parseResponseLine(line)
					if content != "" {
						t.processPendingCommands(content)
					}
				}
				expectingTextMode = false

			} else if byteVal >= 32 || byteVal == 0x09 {

				expectingTextMode = true
				if linePos < 256 {
					lineBuffer[linePos] = b
					linePos++
				}

			} else if byteVal == 0x0D {

				if expectingTextMode || linePos > 0 {
					expectingTextMode = true
				}

			} else if byteVal == 0x0A {

				_ = lastByte

				buttonCombinationDetected := false

				if (t.lastButtonMask != 0) ||
					(lastByte != -1 && lastByte < 32 && lastByte != 0x0D) ||
					(linePos > 0 && !expectingTextMode) {

					t.handleButtonData(byteVal)
					expectingTextMode = false
					buttonCombinationDetected = true
					linePos = 0
				}

				if !buttonCombinationDetected {
					if lastByte == 0x0D {

						t.log("LF: Completing CRLF sequence", "DEBUG")
						if linePos > 0 {
							line := make([]byte, linePos)
							copy(line, lineBuffer[:linePos])
							linePos = 0
							content := t.parseResponseLine(line)
							if content != "" {
								t.processPendingCommands(content)
							}
						}
						expectingTextMode = false
					} else if linePos > 0 && expectingTextMode {
						t.log("LF: Ending line with text in buffer", "DEBUG")
						line := make([]byte, linePos)
						copy(line, lineBuffer[:linePos])
						linePos = 0
						content := t.parseResponseLine(line)
						if content != "" {
							t.processPendingCommands(content)
						}
						expectingTextMode = false
					} else if expectingTextMode {
						t.log("LF: Empty line in text mode", "DEBUG")
						expectingTextMode = false
					} else {
						t.log("LF: Treating as button data (no text context)", "DEBUG")
						t.handleButtonData(byteVal)
						expectingTextMode = false
						linePos = 0
					}
				}

			} else if byteVal < 32 {

				if lastByte == 0x0D {
					t.log("Processing delayed CR as button data: 0x0D", "DEBUG")
					t.handleButtonData(0x0D)
				}
				t.handleButtonData(byteVal)
				expectingTextMode = false
				linePos = 0
			}

			lastByte = byteVal
		}

		if time.Since(lastCleanup) > cleanupInterval {
			t.cleanupTimedOutCommands()
			lastCleanup = time.Now()
		}
	}

	t.log("Listener thread ending", "INFO")
}

func (t *SerialTransport) attemptReconnect() {
	t.log(fmt.Sprintf("Attempting reconnect #%d/%d", t.reconnectAttempts+1, maxReconnectAttempts), "INFO")

	if t.reconnectAttempts >= maxReconnectAttempts {
		t.log("Max reconnect attempts reached, giving up", "ERROR")
		t.connected.Store(false)
		return
	}

	t.reconnectAttempts++

	defer func() {
		if r := recover(); r != nil {
			t.log(fmt.Sprintf("Reconnect attempt failed: %v", r), "ERROR")
			time.Sleep(reconnectDelay)
		}
	}()

	t.serialMu.Lock()
	p := t.serial
	t.serialMu.Unlock()

	if p != nil {
		t.log("Closing existing serial connection for reconnect", "INFO")
		_ = p.Close()
	}

	time.Sleep(reconnectDelay)

	portName, ok := t.FindComPort()
	if !ok {
		t.log("Device not found during reconnect", "ERROR")
		time.Sleep(reconnectDelay)
		return
	}
	t.port = portName

	t.log(fmt.Sprintf("Reconnecting to %s at %d baud", t.port, t.baudrate), "INFO")
	newPort, err := serial.Open(t.port, &serial.Mode{
		BaudRate: 115200,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	})
	if err != nil {
		t.log("Reconnect attempt failed: "+err.Error(), "ERROR")
		time.Sleep(reconnectDelay)
		return
	}
	if err2 := newPort.SetReadTimeout(10 * time.Millisecond); err2 != nil {
		t.log("SetReadTimeout failed during reconnect: "+err2.Error(), "ERROR")
	}

	t.serialMu.Lock()
	t.serial = newPort
	t.serialMu.Unlock()

	if !t.changeBaudTo4M() {
		t.log("Failed to change baud during reconnect", "ERROR")
		time.Sleep(reconnectDelay)
		return
	}

	if t.sendInit {
		t.log("Sending init command during reconnect", "INFO")
		t.writeMu.Lock()
		t.serialMu.Lock()
		cp := t.serial
		t.serialMu.Unlock()
		if cp != nil {
			_, _ = cp.Write([]byte("km.buttons(1)\r"))
			_ = cp.Drain()
		}
		t.writeMu.Unlock()
	}

	t.reconnectAttempts = 0
	t.log("Reconnect successful", "INFO")
}

func (t *SerialTransport) changeBaudTo4M() bool {
	t.log("Changing baud rate to 4M", "INFO")

	t.serialMu.Lock()
	p := t.serial
	t.serialMu.Unlock()

	if p == nil {
		t.log("Cannot change baud - serial not open", "ERROR")
		return false
	}

	t.writeMu.Lock()
	_, _ = p.Write(baudChangeCommand)
	_ = p.Drain()
	t.writeMu.Unlock()

	time.Sleep(20 * time.Millisecond)

	err := p.SetMode(&serial.Mode{
		BaudRate: 4000000,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	})
	if err != nil {
		t.log("SetMode 4M failed: "+err.Error(), "ERROR")
		return false
	}
	t.currentBaud = 4000000
	t.log(fmt.Sprintf("Baud rate changed: %d -> %d", 115200, 4000000), "INFO")
	return true
}

func (t *SerialTransport) Connect() error {
	connectionStart := time.Now()
	t.log("Starting connection process", "INFO")

	if t.connected.Load() {
		t.log("Already connected", "INFO")
		return nil
	}

	var portName string
	if t.overridePort {
		portName = t.fallbackComPort
	} else {
		var ok bool
		portName, ok = t.FindComPort()
		if !ok {
			portName = ""
		}
	}

	if portName == "" {
		return newMakcuConnectionError("Makcu device not found")
	}
	t.port = portName

	t.log("Connecting to "+t.port, "INFO")

	p, err := serial.Open(t.port, &serial.Mode{
		BaudRate: 115200,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	})
	if err != nil {
		t.log("Connection failed: "+err.Error(), "ERROR")
		return newMakcuConnectionError("Failed to connect: %v", err)
	}

	if err2 := p.SetReadTimeout(10 * time.Millisecond); err2 != nil {
		t.log("SetReadTimeout failed: "+err2.Error(), "ERROR")
		_ = p.Close()
		return newMakcuConnectionError("Failed to connect: %v", err2)
	}

	t.serialMu.Lock()
	t.serial = p
	t.serialMu.Unlock()

	if !t.changeBaudTo4M() {
		t.serialMu.Lock()
		t.serial = nil
		t.serialMu.Unlock()
		_ = p.Close()
		return newMakcuConnectionError("Failed to switch to 4M baud")
	}

	t.connected.Store(true)
	t.reconnectAttempts = 0
	t.log(fmt.Sprintf("Connection established in %.3fs", time.Since(connectionStart).Seconds()), "INFO")

	if t.sendInit {
		t.log("Sending initialization command", "INFO")
		t.writeMu.Lock()
		t.serialMu.Lock()
		cp := t.serial
		t.serialMu.Unlock()
		if cp != nil {
			_, _ = cp.Write([]byte("km.buttons(1)\r"))
			_ = cp.Drain()
		}
		t.writeMu.Unlock()
	}

	t.stopRequested.Store(false)
	t.listenerDone = make(chan struct{})
	go t.listen()
	t.log("Listener thread started", "INFO")

	return nil
}

func (t *SerialTransport) Disconnect() {
	t.log("Starting disconnection process", "INFO")

	t.connected.Store(false)

	if t.sendInit {
		t.stopRequested.Store(true)
		select {
		case <-t.listenerDone:
			t.log("Listener thread stopped", "INFO")
		case <-time.After(100 * time.Millisecond):
			t.log("Listener thread did not join within timeout", "INFO")
		}
	}

	t.cmdMu.Lock()
	if pendingCount := len(t.pending); pendingCount > 0 {
		t.log(fmt.Sprintf("Cancelling %d pending commands", pendingCount), "INFO")
	}

	t.pending = make(map[int]*pendingCommand)
	t.pendingOrder = nil
	t.cmdMu.Unlock()

	t.serialMu.Lock()
	p := t.serial
	t.serial = nil
	t.serialMu.Unlock()

	if p != nil {
		t.log("Closing serial port: "+t.port, "INFO")
		_ = p.Close()
	}

	t.log("Disconnection completed", "INFO")
}

func (t *SerialTransport) SendCommand(command string, expectResponse bool, timeout time.Duration) (string, error) {
	commandStart := time.Now()
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	if !t.connected.Load() || t.currentPort() == nil {
		return "", newMakcuConnectionError("Not connected")
	}

	if !expectResponse {
		t.writeMu.Lock()
		t.serialMu.Lock()
		p := t.serial
		t.serialMu.Unlock()
		if p == nil {
			t.writeMu.Unlock()
			return "", newMakcuConnectionError("Not connected")
		}
		_, _ = p.Write([]byte(command + "\r\n"))
		_ = p.Drain()
		t.writeMu.Unlock()
		t.log(fmt.Sprintf("Command '%s' sent in %.5fs (no response expected)", command, time.Since(commandStart).Seconds()), "INFO")
		return command, nil
	}

	ch := make(chan string, 1)

	t.cmdMu.Lock()
	cmdID := t.generateCommandID()
	t.pending[cmdID] = &pendingCommand{
		commandID:      cmdID,
		command:        command,
		result:         ch,
		timestamp:      time.Now(),
		expectResponse: expectResponse,
		timeout:        timeout,
	}
	t.pendingOrder = append(t.pendingOrder, cmdID)
	t.cmdMu.Unlock()

	tagged := command + "#" + strconv.Itoa(cmdID)

	t.writeMu.Lock()
	t.serialMu.Lock()
	p := t.serial
	t.serialMu.Unlock()
	if p == nil {
		t.writeMu.Unlock()
		t.cmdMu.Lock()
		delete(t.pending, cmdID)
		for i, id := range t.pendingOrder {
			if id == cmdID {
				t.pendingOrder = append(t.pendingOrder[:i], t.pendingOrder[i+1:]...)
				break
			}
		}
		t.cmdMu.Unlock()
		return "", newMakcuConnectionError("Not connected")
	}
	if _, werr := p.Write([]byte(tagged + "\r\n")); werr != nil {

		t.writeMu.Unlock()
		t.log(fmt.Sprintf("Command '%s' failed after %.3fs: %v", command, time.Since(commandStart).Seconds(), werr), "ERROR")
		t.cmdMu.Lock()
		delete(t.pending, cmdID)
		for i, id := range t.pendingOrder {
			if id == cmdID {
				t.pendingOrder = append(t.pendingOrder[:i], t.pendingOrder[i+1:]...)
				break
			}
		}
		t.cmdMu.Unlock()
		return "", newMakcuConnectionError("Failed to send command: %v", werr)
	}
	_ = p.Drain()
	t.writeMu.Unlock()

	select {
	case res := <-ch:
		response := res
		if i := strings.IndexByte(res, '#'); i >= 0 {
			response = res[:i]
		}
		t.log(fmt.Sprintf("Command '%s' completed in %.5fs total", command, time.Since(commandStart).Seconds()), "INFO")
		return response, nil
	case <-time.After(timeout):
		t.cmdMu.Lock()
		delete(t.pending, cmdID)
		for i, id := range t.pendingOrder {
			if id == cmdID {
				t.pendingOrder = append(t.pendingOrder[:i], t.pendingOrder[i+1:]...)
				break
			}
		}
		t.cmdMu.Unlock()
		t.log(fmt.Sprintf("Command '%s' timed out after %.3fs", command, time.Since(commandStart).Seconds()), "ERROR")
		return "", newMakcuTimeoutError("Command timed out: %s", command)
	}
}

func (t *SerialTransport) IsConnected() bool {
	return t.connected.Load() && t.currentPort() != nil
}

func (t *SerialTransport) SetButtonCallback(cb func(MouseButton, bool)) {
	t.stateMu.Lock()
	t.buttonCallback = cb
	t.stateMu.Unlock()
	t.log(fmt.Sprintf("Setting button callback: %v", cb != nil), "INFO")
}

func (t *SerialTransport) GetButtonStates() map[string]bool {
	t.stateMu.Lock()
	states := t.buttonStates
	t.stateMu.Unlock()

	m := make(map[string]bool, 5)
	for i := 0; i < 5; i++ {
		m[buttonMap[i]] = (states>>i)&1 == 1
	}
	return m
}

func (t *SerialTransport) GetButtonMask() int {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	return t.lastButtonMask
}

func (t *SerialTransport) EnableButtonMonitoring(enable bool) error {
	cmd := "km.buttons(0)"
	if enable {
		cmd = "km.buttons(1)"
	}
	t.log(fmt.Sprintf("%s button monitoring", func() string {
		if enable {
			return "Enabling"
		}
		return "Disabling"
	}()), "INFO")
	_, err := t.SendCommand(cmd, false, 0)
	return err
}
