package makcu

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"go.bug.st/serial/enumerator"
)

type AxisButton struct{ name string }

func NewAxisButton(name string) AxisButton { return AxisButton{name: name} }

func (a AxisButton) Name() string { return a.name }

type lockCmd struct {
	cmd string
	bit int
}

type Mouse struct {
	transport         *SerialTransport
	lockStatesCache   int
	cacheValid        bool
	pressCommands     map[MouseButton]string
	releaseCommands   map[MouseButton]string
	lockCommands      map[string]lockCmd
	unlockCommands    map[string]lockCmd
	lockQueryCommands map[string]lockCmd
}

var buttonCommands = map[MouseButton]string{
	MouseLeft:   "left",
	MouseRight:  "right",
	MouseMiddle: "middle",
	MouseMouse4: "ms1",
	MouseMouse5: "ms2",
}

func NewMouse(t *SerialTransport) *Mouse {
	m := &Mouse{
		transport:         t,
		lockStatesCache:   0,
		cacheValid:        false,
		pressCommands:     make(map[MouseButton]string),
		releaseCommands:   make(map[MouseButton]string),
		lockCommands:      make(map[string]lockCmd),
		unlockCommands:    make(map[string]lockCmd),
		lockQueryCommands: make(map[string]lockCmd),
	}
	m.initCommandCache()
	return m
}

func (m *Mouse) initCommandCache() {
	for button, cmd := range buttonCommands {
		m.pressCommands[button] = fmt.Sprintf("km.%s(1)", cmd)
		m.releaseCommands[button] = fmt.Sprintf("km.%s(0)", cmd)
	}

	type lockTarget struct {
		name string
		cmd  string
		bit  int
	}
	lockTargets := []lockTarget{
		{"LEFT", "ml", 0},
		{"RIGHT", "mr", 1},
		{"MIDDLE", "mm", 2},
		{"MOUSE4", "ms1", 3},
		{"MOUSE5", "ms2", 4},
		{"X", "mx", 5},
		{"Y", "my", 6},
	}

	for _, lt := range lockTargets {
		m.lockCommands[lt.name] = lockCmd{
			cmd: fmt.Sprintf("km.lock_%s(1)", lt.cmd),
			bit: lt.bit,
		}
		m.unlockCommands[lt.name] = lockCmd{
			cmd: fmt.Sprintf("km.lock_%s(0)", lt.cmd),
			bit: lt.bit,
		}
		m.lockQueryCommands[lt.name] = lockCmd{
			cmd: fmt.Sprintf("km.lock_%s()", lt.cmd),
			bit: lt.bit,
		}
	}
}

func (m *Mouse) Press(b MouseButton) error {
	_, err := m.transport.SendCommand(m.pressCommands[b], false, 0)
	return err
}

func (m *Mouse) Release(b MouseButton) error {
	_, err := m.transport.SendCommand(m.releaseCommands[b], false, 0)
	return err
}

func (m *Mouse) Move(x, y int) error {
	_, err := m.transport.SendCommand(fmt.Sprintf("km.move(%d,%d)", x, y), false, 0)
	return err
}

func (m *Mouse) MoveAbs(target [2]int, speed, waitMs int) error {
	if runtime.GOOS != "windows" {
		return newMakcuCommandError("move_abs is only supported on Windows")
	}

	multiplier := getMouseSpeedMultiplier()
	endX, endY := target[0], target[1]

	if speed < 1 {
		speed = 1
	}
	if speed > 14 {
		speed = 14
	}

	for {
		cx, cy := getCursorPos()
		dx := endX - cx
		dy := endY - cy

		if abs(dx) <= 1 && abs(dy) <= 1 {
			break
		}

		moveX := clampInt(-speed, speed, int(float64(dx)/multiplier))
		moveY := clampInt(-speed, speed, int(float64(dy)/multiplier))

		_, err := m.transport.SendCommand(fmt.Sprintf("km.move(%d,%d)", moveX, moveY), false, 0)
		if err != nil {
			return err
		}
		time.Sleep(time.Duration(waitMs) * time.Millisecond)
	}
	return nil
}

func (m *Mouse) Click(b MouseButton) error {
	if _, ok := buttonCommands[b]; !ok {
		return newMakcuCommandError("Unsupported button: %s", b)
	}
	if _, err := m.transport.SendCommand(m.pressCommands[b], false, 0); err != nil {
		return err
	}
	_, err := m.transport.SendCommand(m.releaseCommands[b], false, 0)
	return err
}

func (m *Mouse) MoveSmooth(x, y, segments int) error {
	_, err := m.transport.SendCommand(fmt.Sprintf("km.move(%d,%d,%d)", x, y, segments), false, 0)
	return err
}

func (m *Mouse) MoveBezier(x, y, segments, ctrlX, ctrlY int) error {
	_, err := m.transport.SendCommand(fmt.Sprintf("km.move(%d,%d,%d,%d,%d)", x, y, segments, ctrlX, ctrlY), false, 0)
	return err
}

func (m *Mouse) Scroll(delta int) error {
	_, err := m.transport.SendCommand(fmt.Sprintf("km.wheel(%d)", delta), false, 0)
	return err
}

func (m *Mouse) setLock(name string, lock bool) error {
	var lc lockCmd
	if lock {
		lc = m.lockCommands[name]
	} else {
		lc = m.unlockCommands[name]
	}

	_, err := m.transport.SendCommand(lc.cmd, false, 0)
	if err != nil {
		return err
	}

	if lock {
		m.lockStatesCache |= 1 << lc.bit
	} else {
		m.lockStatesCache &^= 1 << lc.bit
	}
	m.cacheValid = true
	return nil
}

func (m *Mouse) LockLeft(lock bool) error { return m.setLock("LEFT", lock) }

func (m *Mouse) LockMiddle(lock bool) error { return m.setLock("MIDDLE", lock) }

func (m *Mouse) LockRight(lock bool) error { return m.setLock("RIGHT", lock) }

func (m *Mouse) LockSide1(lock bool) error { return m.setLock("MOUSE4", lock) }

func (m *Mouse) LockSide2(lock bool) error { return m.setLock("MOUSE5", lock) }

func (m *Mouse) LockX(lock bool) error { return m.setLock("X", lock) }

func (m *Mouse) LockY(lock bool) error { return m.setLock("Y", lock) }

func (m *Mouse) SpoofSerial(s string) error {
	_, err := m.transport.SendCommand(fmt.Sprintf("km.serial('%s')", s), false, 0)
	return err
}

func (m *Mouse) ResetSerial() error {
	_, err := m.transport.SendCommand("km.serial(0)", false, 0)
	return err
}

func (m *Mouse) GetDeviceInfo() map[string]any {
	portName := m.transport.port
	isConnected := m.transport.IsConnected()

	if !isConnected || portName == "" {
		return map[string]any{
			"port": func() string {
				if portName != "" {
					return portName
				}
				return "Unknown"
			}(),
			"description": "Disconnected",
			"vid":         "Unknown",
			"pid":         "Unknown",
			"isConnected": false,
		}
	}

	info := map[string]any{
		"port":        portName,
		"description": "Connected Device",
		"vid":         "Unknown",
		"pid":         "Unknown",
		"isConnected": true,
	}

	func() {
		defer func() { _ = recover() }()
		ports, err := enumerator.GetDetailedPortsList()
		if err != nil {
			return
		}
		for _, p := range ports {
			if p.Name == portName {
				desc := p.Product
				if desc == "" {
					desc = "Connected Device"
				}
				info["description"] = desc
				if p.IsUSB {
					info["vid"] = "0x" + strings.ToLower(p.VID)
					info["pid"] = "0x" + strings.ToLower(p.PID)
				}
				break
			}
		}
	}()

	return info
}

func (m *Mouse) GetFirmwareVersion() (string, error) {
	res, err := m.transport.SendCommand("km.version()", true, 100*time.Millisecond)
	if err != nil {
		return "", err
	}
	return res, nil
}

func (m *Mouse) GetAllLockStates() map[string]bool {
	if m.cacheValid {
		return map[string]bool{
			"X":      (m.lockStatesCache & (1 << 5)) != 0,
			"Y":      (m.lockStatesCache & (1 << 6)) != 0,
			"LEFT":   (m.lockStatesCache & (1 << 0)) != 0,
			"RIGHT":  (m.lockStatesCache & (1 << 1)) != 0,
			"MIDDLE": (m.lockStatesCache & (1 << 2)) != 0,
			"MOUSE4": (m.lockStatesCache & (1 << 3)) != 0,
			"MOUSE5": (m.lockStatesCache & (1 << 4)) != 0,
		}
	}

	states := make(map[string]bool)
	targets := []string{"X", "Y", "LEFT", "RIGHT", "MIDDLE", "MOUSE4", "MOUSE5"}

	for _, target := range targets {
		lc := m.lockQueryCommands[target]
		func() {
			defer func() {
				if r := recover(); r != nil {
					states[target] = false
				}
			}()
			result, err := m.transport.SendCommand(lc.cmd, true, 50*time.Millisecond)
			if err != nil {
				states[target] = false
				return
			}
			r := strings.TrimSpace(result)
			if r == "0" || r == "1" {
				isLocked := r == "1"
				states[target] = isLocked
				if isLocked {
					m.lockStatesCache |= 1 << lc.bit
				} else {
					m.lockStatesCache &^= 1 << lc.bit
				}
			} else {
				states[target] = false
			}
		}()
	}

	m.cacheValid = true
	return states
}

func (m *Mouse) IsLocked(target any) bool {
	defer func() { _ = recover() }()

	var targetName string
	switch v := target.(type) {
	case MouseButton:
		targetName = v.Name()
	case AxisButton:
		targetName = v.Name()
	default:
		return false
	}

	lc, ok := m.lockQueryCommands[targetName]
	if !ok {
		return false
	}

	if m.cacheValid {
		return (m.lockStatesCache & (1 << lc.bit)) != 0
	}

	result, err := m.transport.SendCommand(lc.cmd, true, 50*time.Millisecond)
	if err != nil {
		return false
	}

	result = strings.TrimSpace(result)
	isLocked := result == "1"

	if isLocked {
		m.lockStatesCache |= 1 << lc.bit
	} else {
		m.lockStatesCache &^= 1 << lc.bit
	}

	return isLocked
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func clampInt(lo, hi, v int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
