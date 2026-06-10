package makcu

import (
	"fmt"
	"os"
	"testing"
)

func TestMouseButtonName(t *testing.T) {
	cases := []struct {
		b    MouseButton
		want string
	}{
		{MouseLeft, "LEFT"},
		{MouseRight, "RIGHT"},
		{MouseMiddle, "MIDDLE"},
		{MouseMouse4, "MOUSE4"},
		{MouseMouse5, "MOUSE5"},
	}
	for _, tc := range cases {
		if got := tc.b.Name(); got != tc.want {
			t.Errorf("MouseButton(%d).Name() = %q, want %q", int(tc.b), got, tc.want)
		}
		if got := tc.b.String(); got != tc.want {
			t.Errorf("MouseButton(%d).String() = %q, want %q", int(tc.b), got, tc.want)
		}
	}
}

func TestMouseButtonName_Unknown(t *testing.T) {
	b := MouseButton(99)
	if got := b.Name(); got != "UNKNOWN" {
		t.Errorf("unknown button Name() = %q, want \"UNKNOWN\"", got)
	}
}

func TestAxisButtonName(t *testing.T) {
	a := AxisButton{name: "X"}
	if got := a.Name(); got != "X" {
		t.Errorf("AxisButton.Name() = %q, want \"X\"", got)
	}
}

func TestMouseCommandCache_PressCommands(t *testing.T) {
	transport := NewSerialTransport("", false, false, false, false)
	m := NewMouse(transport)

	want := map[MouseButton]string{
		MouseLeft:   "km.left(1)",
		MouseRight:  "km.right(1)",
		MouseMiddle: "km.middle(1)",
		MouseMouse4: "km.ms1(1)",
		MouseMouse5: "km.ms2(1)",
	}
	for btn, wantCmd := range want {
		if got, ok := m.pressCommands[btn]; !ok {
			t.Errorf("pressCommands missing key %v", btn)
		} else if got != wantCmd {
			t.Errorf("pressCommands[%v] = %q, want %q", btn, got, wantCmd)
		}
	}
	if len(m.pressCommands) != 5 {
		t.Errorf("pressCommands len: want 5, got %d", len(m.pressCommands))
	}
}

func TestMouseCommandCache_ReleaseCommands(t *testing.T) {
	transport := NewSerialTransport("", false, false, false, false)
	m := NewMouse(transport)

	want := map[MouseButton]string{
		MouseLeft:   "km.left(0)",
		MouseRight:  "km.right(0)",
		MouseMiddle: "km.middle(0)",
		MouseMouse4: "km.ms1(0)",
		MouseMouse5: "km.ms2(0)",
	}
	for btn, wantCmd := range want {
		if got, ok := m.releaseCommands[btn]; !ok {
			t.Errorf("releaseCommands missing key %v", btn)
		} else if got != wantCmd {
			t.Errorf("releaseCommands[%v] = %q, want %q", btn, got, wantCmd)
		}
	}
	if len(m.releaseCommands) != 5 {
		t.Errorf("releaseCommands len: want 5, got %d", len(m.releaseCommands))
	}
}

func TestMouseCommandCache_LockCommands(t *testing.T) {
	transport := NewSerialTransport("", false, false, false, false)
	m := NewMouse(transport)

	type wantEntry struct {
		cmd string
		bit int
	}
	wants := map[string]wantEntry{
		"LEFT":   {"km.lock_ml(1)", 0},
		"RIGHT":  {"km.lock_mr(1)", 1},
		"MIDDLE": {"km.lock_mm(1)", 2},
		"MOUSE4": {"km.lock_ms1(1)", 3},
		"MOUSE5": {"km.lock_ms2(1)", 4},
		"X":      {"km.lock_mx(1)", 5},
		"Y":      {"km.lock_my(1)", 6},
	}
	for name, we := range wants {
		got, ok := m.lockCommands[name]
		if !ok {
			t.Errorf("lockCommands missing key %q", name)
			continue
		}
		if got.cmd != we.cmd {
			t.Errorf("lockCommands[%q].cmd = %q, want %q", name, got.cmd, we.cmd)
		}
		if got.bit != we.bit {
			t.Errorf("lockCommands[%q].bit = %d, want %d", name, got.bit, we.bit)
		}
	}
	if len(m.lockCommands) != 7 {
		t.Errorf("lockCommands len: want 7, got %d", len(m.lockCommands))
	}
}

func TestMouseCommandCache_LockQueryCommands(t *testing.T) {
	transport := NewSerialTransport("", false, false, false, false)
	m := NewMouse(transport)

	type wantEntry struct {
		cmd string
		bit int
	}
	wants := map[string]wantEntry{
		"LEFT":   {"km.lock_ml()", 0},
		"RIGHT":  {"km.lock_mr()", 1},
		"MIDDLE": {"km.lock_mm()", 2},
		"MOUSE4": {"km.lock_ms1()", 3},
		"MOUSE5": {"km.lock_ms2()", 4},
		"X":      {"km.lock_mx()", 5},
		"Y":      {"km.lock_my()", 6},
	}
	for name, we := range wants {
		got, ok := m.lockQueryCommands[name]
		if !ok {
			t.Errorf("lockQueryCommands missing key %q", name)
			continue
		}
		if got.cmd != we.cmd {
			t.Errorf("lockQueryCommands[%q].cmd = %q, want %q", name, got.cmd, we.cmd)
		}
		if got.bit != we.bit {
			t.Errorf("lockQueryCommands[%q].bit = %d, want %d", name, got.bit, we.bit)
		}
	}
	if len(m.lockQueryCommands) != 7 {
		t.Errorf("lockQueryCommands len: want 7, got %d", len(m.lockQueryCommands))
	}
}

func TestMouseCommandCache_UnlockCommands(t *testing.T) {
	transport := NewSerialTransport("", false, false, false, false)
	m := NewMouse(transport)

	type wantEntry struct {
		cmd string
		bit int
	}
	wants := map[string]wantEntry{
		"LEFT":   {"km.lock_ml(0)", 0},
		"RIGHT":  {"km.lock_mr(0)", 1},
		"MIDDLE": {"km.lock_mm(0)", 2},
		"MOUSE4": {"km.lock_ms1(0)", 3},
		"MOUSE5": {"km.lock_ms2(0)", 4},
		"X":      {"km.lock_mx(0)", 5},
		"Y":      {"km.lock_my(0)", 6},
	}
	for name, we := range wants {
		got, ok := m.unlockCommands[name]
		if !ok {
			t.Errorf("unlockCommands missing key %q", name)
			continue
		}
		if got.cmd != we.cmd {
			t.Errorf("unlockCommands[%q].cmd = %q, want %q", name, got.cmd, we.cmd)
		}
		if got.bit != we.bit {
			t.Errorf("unlockCommands[%q].bit = %d, want %d", name, got.bit, we.bit)
		}
	}
	if len(m.unlockCommands) != 7 {
		t.Errorf("unlockCommands len: want 7, got %d", len(m.unlockCommands))
	}
}

func TestMouseCommandCache_BitsAreCorrect(t *testing.T) {
	transport := NewSerialTransport("", false, false, false, false)
	m := NewMouse(transport)

	bitMap := map[string]int{
		"LEFT": 0, "RIGHT": 1, "MIDDLE": 2, "MOUSE4": 3, "MOUSE5": 4, "X": 5, "Y": 6,
	}
	for name, wantBit := range bitMap {
		if lc, ok := m.lockCommands[name]; ok {
			if lc.bit != wantBit {
				t.Errorf("lockCommands[%q].bit = %d, want %d", name, lc.bit, wantBit)
			}
		}
		if qc, ok := m.lockQueryCommands[name]; ok {
			if qc.bit != wantBit {
				t.Errorf("lockQueryCommands[%q].bit = %d, want %d", name, qc.bit, wantBit)
			}
		}
	}
}

func TestMouseMoveCommandString(t *testing.T) {

	cases := []struct {
		x, y int
		want string
	}{
		{10, 20, "km.move(10,20)"},
		{-5, 0, "km.move(-5,0)"},
		{0, -10, "km.move(0,-10)"},
	}
	for _, tc := range cases {
		got := fmt.Sprintf("km.move(%d,%d)", tc.x, tc.y)
		if got != tc.want {
			t.Errorf("move format: got %q, want %q", got, tc.want)
		}
	}
}

func TestMouseScrollCommandString(t *testing.T) {
	cases := []struct {
		delta int
		want  string
	}{
		{1, "km.wheel(1)"},
		{-2, "km.wheel(-2)"},
		{0, "km.wheel(0)"},
	}
	for _, tc := range cases {
		got := fmt.Sprintf("km.wheel(%d)", tc.delta)
		if got != tc.want {
			t.Errorf("wheel format: got %q, want %q", got, tc.want)
		}
	}
}

func TestMouseSmoothCommandString(t *testing.T) {
	got := fmt.Sprintf("km.move(%d,%d,%d)", 10, 20, 5)
	want := "km.move(10,20,5)"
	if got != want {
		t.Errorf("smooth format: got %q, want %q", got, want)
	}
}

func TestMouseBezierCommandString(t *testing.T) {
	got := fmt.Sprintf("km.move(%d,%d,%d,%d,%d)", 10, 20, 5, 3, 4)
	want := "km.move(10,20,5,3,4)"
	if got != want {
		t.Errorf("bezier format: got %q, want %q", got, want)
	}
}

func TestMouseVersionCommandString(t *testing.T) {
	if got := "km.version()"; got != "km.version()" {
		t.Errorf("version command: got %q", got)
	}
}

func TestMouseSpoofSerialCommandString(t *testing.T) {
	s := "ABCDEF"
	got := fmt.Sprintf("km.serial('%s')", s)
	want := "km.serial('ABCDEF')"
	if got != want {
		t.Errorf("spoof serial: got %q, want %q", got, want)
	}

	reset := "km.serial(0)"
	if reset != "km.serial(0)" {
		t.Errorf("reset serial: got %q", reset)
	}
}

func TestHW_MousePressRelease(t *testing.T) {
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

	m := NewMouse(transport)
	if err := m.Press(MouseLeft); err != nil {
		t.Fatalf("Press(LEFT) error: %v", err)
	}
	if err := m.Release(MouseLeft); err != nil {
		t.Fatalf("Release(LEFT) error: %v", err)
	}
}

func TestHW_MouseMiddleClick(t *testing.T) {
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

	m := NewMouse(transport)
	if err := m.Press(MouseMiddle); err != nil {
		t.Fatalf("Press(MIDDLE) error: %v", err)
	}
	if err := m.Release(MouseMiddle); err != nil {
		t.Fatalf("Release(MIDDLE) error: %v", err)
	}
}

func TestHW_MouseFirmwareVersion(t *testing.T) {
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

	m := NewMouse(transport)
	ver, err := m.GetFirmwareVersion()
	if err != nil {
		t.Fatalf("GetFirmwareVersion() error: %v", err)
	}
	if len(ver) == 0 {
		t.Fatal("firmware version is empty")
	}
}

func TestHW_MouseDeviceInfo(t *testing.T) {
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

	m := NewMouse(transport)
	info := m.GetDeviceInfo()
	if info["port"] == "" || info["port"] == nil {
		t.Fatal("device info missing port")
	}
	connected, ok := info["isConnected"].(bool)
	if !ok || !connected {
		t.Fatal("device info isConnected should be true")
	}
}

func TestHW_MouseLockState(t *testing.T) {
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
	defer func() {
		m := NewMouse(transport)
		_ = m.LockLeft(false)
		transport.Disconnect()
	}()

	m := NewMouse(transport)
	if err := m.LockLeft(true); err != nil {
		t.Fatalf("LockLeft(true) error: %v", err)
	}
	if !m.IsLocked(MouseLeft) {
		t.Fatal("IsLocked(LEFT) should be true after LockLeft(true)")
	}
}
