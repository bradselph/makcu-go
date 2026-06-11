package makcu

import (
	"os"
	"sync"
	"testing"
	"time"

	"go.bug.st/serial"
)

func skipIfNoDevice(t *testing.T) {
	t.Helper()
	if p := os.Getenv("MAKCU_PORT"); p != "" {
		return
	}
	transport := NewSerialTransport("", false, false, false, false)
	_, ok := transport.FindComPort()
	if !ok {
		t.Skip("no makcu device present (set MAKCU_PORT or attach device)")
	}
}

func TestGenerateCommandID_FaithfulMaskQuirk(t *testing.T) {
	transport := NewSerialTransport("", false, false, false, false)

	for call := 1; call <= 5; call++ {
		got := transport.generateCommandID()
		if got != 0 {
			t.Fatalf("fresh transport call %d: want 0 (the &0x2710 quirk), got %d", call, got)
		}
	}

	cases := []struct{ start, want int }{
		{0x0000, 0},
		{0x270F, 0x2710},
		{0x2710, 0x2710},
		{0x0001, 0},
		{0x000F, 0x10},
	}
	for _, c := range cases {
		transport.commandCounter = c.start
		got := transport.generateCommandID()
		want := (c.start + 1) & 0x2710
		if got != want {
			t.Fatalf("start=0x%X: formula want %d, got %d", c.start, want, got)
		}
		if got != c.want {
			t.Fatalf("start=0x%X: documented want %d, got %d", c.start, c.want, got)
		}
	}
}

func TestGenerateCommandID_RangeBounded(t *testing.T) {
	transport := NewSerialTransport("", false, false, false, false)
	for start := 0; start <= 0x2720; start++ {
		transport.commandCounter = start
		id := transport.generateCommandID()
		if id < 0 || id > 0x2710 {
			t.Fatalf("start=%d produced id %d out of range [0, 0x2710]", start, id)
		}
		if id != (start+1)&0x2710 {
			t.Fatalf("start=%d: id %d != (start+1)&0x2710 %d", start, id, (start+1)&0x2710)
		}
	}
}

func TestParseResponseLine_Prefix(t *testing.T) {
	transport := NewSerialTransport("", false, false, false, false)

	cases := []struct {
		input []byte
		want  string
	}{
		{[]byte(">>> foo "), "foo"},
		{[]byte(">>> foo"), "foo"},
		{[]byte(">>>foo"), ">>>foo"},
		{[]byte("bar"), "bar"},
		{[]byte("  bar  "), "bar"},
		{[]byte(">>> "), ""},
		{[]byte(">>> 1.2.3"), "1.2.3"},
		{[]byte("km.version()"), "km.version()"},
		{[]byte(">>> result#42"), "result#42"},

		{[]byte(">>> ab\xC3cd"), "abcd"},
		{[]byte("\xFFkm.version()\x80"), "km.version()"},
	}

	for _, tc := range cases {
		got := transport.parseResponseLine(tc.input)
		if got != tc.want {
			t.Errorf("parseResponseLine(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestProcessPendingCommands_EchoIgnored_ThenResponse(t *testing.T) {
	transport := NewSerialTransport("", false, false, false, false)

	resultCh := make(chan string, 1)
	const cmdStr = "km.version()"
	const cmdID = 7
	pc := &pendingCommand{
		commandID:      cmdID,
		command:        cmdStr,
		result:         resultCh,
		timestamp:      time.Now(),
		expectResponse: true,
		timeout:        defaultTimeout,
	}

	transport.cmdMu.Lock()
	transport.pending[cmdID] = pc
	transport.pendingOrder = append(transport.pendingOrder, cmdID)
	transport.cmdMu.Unlock()

	transport.processPendingCommands(cmdStr)

	transport.cmdMu.Lock()
	_, stillThere := transport.pending[cmdID]
	transport.cmdMu.Unlock()
	if !stillThere {
		t.Fatal("pending entry was removed on echo; expected it to stay")
	}

	select {
	case v := <-resultCh:
		t.Fatalf("echo incorrectly delivered result %q", v)
	default:
	}

	const resp = "1.0.5"
	transport.processPendingCommands(resp)

	transport.cmdMu.Lock()
	_, goneNow := transport.pending[cmdID]
	orderLen := len(transport.pendingOrder)
	transport.cmdMu.Unlock()
	if goneNow {
		t.Fatal("pending entry not removed after real response")
	}
	if orderLen != 0 {
		t.Fatalf("pendingOrder not empty after removal; len=%d", orderLen)
	}

	select {
	case got := <-resultCh:
		if got != resp {
			t.Fatalf("result chan: want %q, got %q", resp, got)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("result chan did not receive within 100ms")
	}
}

func TestProcessPendingCommands_NoExpectResponse_EchoResolvesImmediately(t *testing.T) {
	transport := NewSerialTransport("", false, false, false, false)

	resultCh := make(chan string, 1)
	const cmdStr = "km.left(1)"
	const cmdID = 3
	pc := &pendingCommand{
		commandID:      cmdID,
		command:        cmdStr,
		result:         resultCh,
		timestamp:      time.Now(),
		expectResponse: false,
		timeout:        defaultTimeout,
	}

	transport.cmdMu.Lock()
	transport.pending[cmdID] = pc
	transport.pendingOrder = append(transport.pendingOrder, cmdID)
	transport.cmdMu.Unlock()

	transport.processPendingCommands(cmdStr)

	transport.cmdMu.Lock()
	_, goneNow := transport.pending[cmdID]
	transport.cmdMu.Unlock()
	if goneNow {
		t.Fatal("pending entry should have been removed after echo with expectResponse=false")
	}

	select {
	case got := <-resultCh:
		if got != cmdStr {
			t.Fatalf("result: want %q, got %q", cmdStr, got)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("result chan did not receive within 100ms")
	}
}

func TestProcessPendingCommands_EmptyContent(t *testing.T) {
	transport := NewSerialTransport("", false, false, false, false)

	resultCh := make(chan string, 1)
	const cmdID = 1
	pc := &pendingCommand{
		commandID:      cmdID,
		command:        "km.version()",
		result:         resultCh,
		timestamp:      time.Now(),
		expectResponse: true,
		timeout:        defaultTimeout,
	}
	transport.cmdMu.Lock()
	transport.pending[cmdID] = pc
	transport.pendingOrder = append(transport.pendingOrder, cmdID)
	transport.cmdMu.Unlock()

	transport.processPendingCommands("")

	transport.cmdMu.Lock()
	_, still := transport.pending[cmdID]
	transport.cmdMu.Unlock()
	if !still {
		t.Fatal("entry removed on empty content")
	}
}

func TestCleanupTimedOutCommands_NoBogusSignal(t *testing.T) {
	transport := NewSerialTransport("", false, false, false, false)
	resultCh := make(chan string, 1)
	const cmdID = 11
	transport.cmdMu.Lock()
	transport.pending[cmdID] = &pendingCommand{
		commandID:      cmdID,
		command:        "km.version()",
		result:         resultCh,
		timestamp:      time.Now().Add(-1 * time.Second),
		expectResponse: true,
		timeout:        defaultTimeout,
	}
	transport.pendingOrder = append(transport.pendingOrder, cmdID)
	transport.cmdMu.Unlock()

	transport.cleanupTimedOutCommands()

	transport.cmdMu.Lock()
	_, present := transport.pending[cmdID]
	orderLen := len(transport.pendingOrder)
	transport.cmdMu.Unlock()
	if present {
		t.Fatal("stale entry not removed by cleanup")
	}
	if orderLen != 0 {
		t.Fatalf("pendingOrder not cleared; len=%d", orderLen)
	}

	select {
	case v := <-resultCh:
		t.Fatalf("cleanup delivered a bogus result %q; channel must stay empty", v)
	default:
	}
}

func TestDisconnect_ClearsPendingNoBogusSignal(t *testing.T) {
	transport := NewSerialTransport("", false, false, false, false)
	resultCh := make(chan string, 1)
	const cmdID = 21
	transport.cmdMu.Lock()
	transport.pending[cmdID] = &pendingCommand{
		commandID:      cmdID,
		command:        "km.version()",
		result:         resultCh,
		timestamp:      time.Now(),
		expectResponse: true,
		timeout:        defaultTimeout,
	}
	transport.pendingOrder = append(transport.pendingOrder, cmdID)
	transport.cmdMu.Unlock()

	transport.Disconnect()

	transport.cmdMu.Lock()
	n := len(transport.pending)
	transport.cmdMu.Unlock()
	if n != 0 {
		t.Fatalf("Disconnect did not clear pending; len=%d", n)
	}
	select {
	case v := <-resultCh:
		t.Fatalf("Disconnect delivered a bogus result %q; channel must stay empty", v)
	default:
	}
}

func TestHandleButtonData_PressAndRelease(t *testing.T) {
	transport := NewSerialTransport("", false, false, false, false)

	type callRecord struct {
		button  MouseButton
		pressed bool
	}
	var calls []callRecord
	transport.SetButtonCallback(func(b MouseButton, pressed bool) {
		calls = append(calls, callRecord{b, pressed})
	})

	transport.handleButtonData(0x01)

	if m := transport.GetButtonMask(); m != 0x01 {
		t.Fatalf("mask after press: want 0x01, got 0x%02X", m)
	}

	states := transport.GetButtonStates()
	if !states["left"] {
		t.Fatal("left should be true in GetButtonStates after press")
	}
	for _, k := range []string{"right", "middle", "mouse4", "mouse5"} {
		if states[k] {
			t.Fatalf("button %q should be false, got true", k)
		}
	}

	if len(calls) != 1 {
		t.Fatalf("callback fires: want 1, got %d", len(calls))
	}
	if calls[0].button != MouseLeft || !calls[0].pressed {
		t.Fatalf("callback call[0]: want (LEFT, true), got (%v, %v)", calls[0].button, calls[0].pressed)
	}

	transport.handleButtonData(0x00)

	if m := transport.GetButtonMask(); m != 0x00 {
		t.Fatalf("mask after release: want 0x00, got 0x%02X", m)
	}
	states = transport.GetButtonStates()
	if states["left"] {
		t.Fatal("left should be false after release")
	}

	if len(calls) != 2 {
		t.Fatalf("callback fires: want 2, got %d", len(calls))
	}
	if calls[1].button != MouseLeft || calls[1].pressed {
		t.Fatalf("callback call[1]: want (LEFT, false), got (%v, %v)", calls[1].button, calls[1].pressed)
	}
}

func TestHandleButtonData_SameMaskNoCallback(t *testing.T) {
	transport := NewSerialTransport("", false, false, false, false)

	var callCount int
	transport.SetButtonCallback(func(_ MouseButton, _ bool) {
		callCount++
	})

	transport.handleButtonData(0x01)
	callCount = 0

	transport.handleButtonData(0x01)
	if callCount != 0 {
		t.Fatalf("callback should not fire on same mask; fired %d times", callCount)
	}
}

func TestHandleButtonData_MultipleButtonsChanged(t *testing.T) {
	transport := NewSerialTransport("", false, false, false, false)

	type record struct {
		button  MouseButton
		pressed bool
	}
	var calls []record
	transport.SetButtonCallback(func(b MouseButton, p bool) {
		calls = append(calls, record{b, p})
	})

	transport.handleButtonData(0x03)

	if len(calls) != 2 {
		t.Fatalf("want 2 callbacks for 2-bit change, got %d", len(calls))
	}

	states := transport.GetButtonStates()
	if !states["left"] || !states["right"] {
		t.Fatal("left and right should both be pressed")
	}
}

func TestHandleButtonData_CallbackSeesPythonFaithfulState(t *testing.T) {
	transport := NewSerialTransport("", false, false, false, false)

	type obs struct {
		button MouseButton
		mask   int
		left   bool
		right  bool
	}
	var seen []obs
	transport.SetButtonCallback(func(b MouseButton, _ bool) {
		st := transport.GetButtonStates()
		seen = append(seen, obs{
			button: b,
			mask:   transport.GetButtonMask(),
			left:   st["left"],
			right:  st["right"],
		})
	})

	transport.handleButtonData(0x03)

	if len(seen) != 2 {
		t.Fatalf("want 2 callback observations, got %d", len(seen))
	}

	if seen[0].button != MouseLeft {
		t.Fatalf("first callback should be LEFT (bit 0), got %v", seen[0].button)
	}
	if seen[0].mask != 0x00 {
		t.Errorf("mask during LEFT callback: want 0x00 (old), got 0x%02X", seen[0].mask)
	}
	if !seen[0].left || seen[0].right {
		t.Errorf("states during LEFT callback: want left=true right=false, got left=%v right=%v", seen[0].left, seen[0].right)
	}

	if seen[1].button != MouseRight {
		t.Fatalf("second callback should be RIGHT (bit 1), got %v", seen[1].button)
	}
	if seen[1].mask != 0x00 {
		t.Errorf("mask during RIGHT callback: want 0x00 (old), got 0x%02X", seen[1].mask)
	}
	if !seen[1].left || !seen[1].right {
		t.Errorf("states during RIGHT callback: want left=true right=true, got left=%v right=%v", seen[1].left, seen[1].right)
	}

	if m := transport.GetButtonMask(); m != 0x03 {
		t.Errorf("mask after handleButtonData: want 0x03, got 0x%02X", m)
	}
}

func TestHandleButtonData_CallbackPanicRecovered(t *testing.T) {
	transport := NewSerialTransport("", false, false, false, false)

	transport.SetButtonCallback(func(_ MouseButton, _ bool) {
		panic("deliberate panic")
	})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic escaped handleButtonData: %v", r)
		}
	}()

	transport.handleButtonData(0x01)
}

type fakePort struct{}

func (fakePort) SetMode(*serial.Mode) error  { return nil }
func (fakePort) Read([]byte) (int, error)    { return 0, nil }
func (fakePort) Write(p []byte) (int, error) { return len(p), nil }
func (fakePort) Drain() error                { return nil }
func (fakePort) ResetInputBuffer() error     { return nil }
func (fakePort) ResetOutputBuffer() error    { return nil }
func (fakePort) SetDTR(bool) error           { return nil }
func (fakePort) SetRTS(bool) error           { return nil }
func (fakePort) GetModemStatusBits() (*serial.ModemStatusBits, error) {
	return &serial.ModemStatusBits{}, nil
}
func (fakePort) SetReadTimeout(time.Duration) error { return nil }
func (fakePort) Close() error                       { return nil }
func (fakePort) Break(time.Duration) error          { return nil }

func TestSendCommand_ConcurrentNoCounterRace(t *testing.T) {
	transport := NewSerialTransport("", false, false, false, false)
	transport.serialMu.Lock()
	transport.serial = fakePort{}
	transport.serialMu.Unlock()
	transport.connected.Store(true)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			_, _ = transport.SendCommand("km.move(1,1)", true, time.Millisecond)
		}()
	}
	wg.Wait()

	transport.cmdMu.Lock()
	pendingLeft := len(transport.pending)
	orderLeft := len(transport.pendingOrder)
	transport.cmdMu.Unlock()
	if pendingLeft != 0 || orderLeft != 0 {
		t.Fatalf("pending not fully cleaned up after concurrent SendCommand: pending=%d order=%d", pendingLeft, orderLeft)
	}
}

func TestGetButtonStates_AllKeys(t *testing.T) {
	transport := NewSerialTransport("", false, false, false, false)
	states := transport.GetButtonStates()
	wantKeys := []string{"left", "right", "middle", "mouse4", "mouse5"}
	for _, k := range wantKeys {
		if _, ok := states[k]; !ok {
			t.Errorf("GetButtonStates missing key %q", k)
		}
	}
	if len(states) != 5 {
		t.Errorf("GetButtonStates len: want 5, got %d", len(states))
	}
}

func TestGetButtonMask_InitialZero(t *testing.T) {
	transport := NewSerialTransport("", false, false, false, false)
	if m := transport.GetButtonMask(); m != 0 {
		t.Fatalf("initial mask: want 0, got %d", m)
	}
}

func TestHW_Connect(t *testing.T) {
	skipIfNoDevice(t)

	var transport *SerialTransport
	if p := os.Getenv("MAKCU_PORT"); p != "" {
		transport = NewSerialTransport(p, false, true, false, true)
	} else {
		transport = NewSerialTransport("", false, true, false, false)
	}

	if err := transport.Connect(); err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	defer transport.Disconnect()

	if !transport.IsConnected() {
		t.Fatal("IsConnected() returned false after Connect()")
	}
}

func TestHW_GetButtonStates(t *testing.T) {
	skipIfNoDevice(t)

	var transport *SerialTransport
	if p := os.Getenv("MAKCU_PORT"); p != "" {
		transport = NewSerialTransport(p, false, true, false, true)
	} else {
		transport = NewSerialTransport("", false, true, false, false)
	}

	if err := transport.Connect(); err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	defer transport.Disconnect()

	states := transport.GetButtonStates()
	wantKeys := []string{"left", "right", "middle", "mouse4", "mouse5"}
	for _, k := range wantKeys {
		if _, ok := states[k]; !ok {
			t.Errorf("GetButtonStates missing key %q", k)
		}
	}
}

func TestHW_GetButtonMask(t *testing.T) {
	skipIfNoDevice(t)

	var transport *SerialTransport
	if p := os.Getenv("MAKCU_PORT"); p != "" {
		transport = NewSerialTransport(p, false, true, false, true)
	} else {
		transport = NewSerialTransport("", false, true, false, false)
	}

	if err := transport.Connect(); err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	defer transport.Disconnect()

	mask := transport.GetButtonMask()
	_ = mask
}

func TestHW_Disconnect(t *testing.T) {
	skipIfNoDevice(t)

	var transport *SerialTransport
	if p := os.Getenv("MAKCU_PORT"); p != "" {
		transport = NewSerialTransport(p, false, true, false, true)
	} else {
		transport = NewSerialTransport("", false, true, false, false)
	}

	if err := transport.Connect(); err != nil {
		t.Fatalf("Connect() error: %v", err)
	}

	transport.Disconnect()

	if transport.IsConnected() {
		t.Fatal("IsConnected() should return false after Disconnect()")
	}
}
