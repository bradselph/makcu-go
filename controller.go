package makcu

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"sync"
	"time"
)

type Option func(*controllerConfig)

type controllerConfig struct {
	fallbackComPort string
	debug           bool
	sendInit        bool
	autoReconnect   bool
	overridePort    bool
}

func WithFallbackPort(port string) Option {
	return func(c *controllerConfig) { c.fallbackComPort = port }
}

func WithDebug(debug bool) Option {
	return func(c *controllerConfig) { c.debug = debug }
}

func WithSendInit(sendInit bool) Option {
	return func(c *controllerConfig) { c.sendInit = sendInit }
}

func WithAutoReconnect(autoReconnect bool) Option {
	return func(c *controllerConfig) { c.autoReconnect = autoReconnect }
}

func WithOverridePort(overridePort bool) Option {
	return func(c *controllerConfig) { c.overridePort = overridePort }
}

type MakcuController struct {
	Transport           *SerialTransport
	Mouse               *Mouse
	connectionCallbacks []func(bool)
	callbackMu          sync.Mutex
	connected           bool
}

func NewMakcuController(opts ...Option) *MakcuController {
	cfg := &controllerConfig{
		sendInit:      true,
		autoReconnect: true,
	}
	for _, o := range opts {
		o(cfg)
	}
	t := NewSerialTransport(cfg.fallbackComPort, cfg.debug, cfg.sendInit, cfg.autoReconnect, cfg.overridePort)
	return &MakcuController{
		Transport: t,
		Mouse:     NewMouse(t),
	}
}

func (c *MakcuController) checkConnection() error {
	if !c.connected {
		return newMakcuConnectionError("Not connected")
	}
	return nil
}

func (c *MakcuController) notifyConnectionChange(connected bool) {
	c.callbackMu.Lock()
	cbs := make([]func(bool), len(c.connectionCallbacks))
	copy(cbs, c.connectionCallbacks)
	c.callbackMu.Unlock()

	for _, cb := range cbs {
		func() {
			defer func() { _ = recover() }()
			cb(connected)
		}()
	}
}

func (c *MakcuController) Connect() error {
	if err := c.Transport.Connect(); err != nil {
		return err
	}
	c.connected = true
	c.notifyConnectionChange(true)
	return nil
}

func (c *MakcuController) Disconnect() error {
	c.Transport.Disconnect()
	c.connected = false
	c.notifyConnectionChange(false)
	return nil
}

func (c *MakcuController) Close() error {
	return c.Disconnect()
}

func (c *MakcuController) IsConnected() bool {
	return c.connected && c.Transport.IsConnected()
}

func (c *MakcuController) Click(b MouseButton) error {
	if err := c.checkConnection(); err != nil {
		return err
	}
	if err := c.Mouse.Press(b); err != nil {
		return err
	}
	return c.Mouse.Release(b)
}

func (c *MakcuController) DoubleClick(b MouseButton) error {
	if err := c.checkConnection(); err != nil {
		return err
	}
	if err := c.Mouse.Press(b); err != nil {
		return err
	}
	if err := c.Mouse.Release(b); err != nil {
		return err
	}
	time.Sleep(1 * time.Millisecond)
	if err := c.Mouse.Press(b); err != nil {
		return err
	}
	return c.Mouse.Release(b)
}

func (c *MakcuController) Move(dx, dy int) error {
	if err := c.checkConnection(); err != nil {
		return err
	}
	return c.Mouse.Move(dx, dy)
}

func (c *MakcuController) MoveAbs(target [2]int, speed, waitMs int, debug bool) error {
	if err := c.checkConnection(); err != nil {
		return err
	}

	if err := c.Mouse.MoveAbs(target, speed, waitMs); err != nil {
		return err
	}
	if debug {
		fmt.Printf("[DEBUG] Moving mouse to %v with speed=%d, wait_ms=%d\n", target, speed, waitMs)
	}
	return nil
}

func (c *MakcuController) BatchExecute(actions []func() error) error {
	if err := c.checkConnection(); err != nil {
		return err
	}
	for _, action := range actions {
		if err := action(); err != nil {
			return fmt.Errorf("batch execution failed at action: %w", err)
		}
	}
	return nil
}

func (c *MakcuController) Scroll(delta int) error {
	if err := c.checkConnection(); err != nil {
		return err
	}
	return c.Mouse.Scroll(delta)
}

func (c *MakcuController) Press(b MouseButton) error {
	if err := c.checkConnection(); err != nil {
		return err
	}
	return c.Mouse.Press(b)
}

func (c *MakcuController) Release(b MouseButton) error {
	if err := c.checkConnection(); err != nil {
		return err
	}
	return c.Mouse.Release(b)
}

func (c *MakcuController) MoveSmooth(dx, dy, segments int) error {
	if err := c.checkConnection(); err != nil {
		return err
	}
	return c.Mouse.MoveSmooth(dx, dy, segments)
}

func (c *MakcuController) MoveBezier(dx, dy, segments int, ctrlX, ctrlY *int) error {
	if err := c.checkConnection(); err != nil {
		return err
	}
	cx := dx / 2
	if ctrlX != nil {
		cx = *ctrlX
	}
	cy := dy / 2
	if ctrlY != nil {
		cy = *ctrlY
	}
	return c.Mouse.MoveBezier(dx, dy, segments, cx, cy)
}

func (c *MakcuController) Lock(target any) error {
	if err := c.checkConnection(); err != nil {
		return err
	}
	switch v := target.(type) {
	case MouseButton:
		switch v {
		case MouseLeft:
			return c.Mouse.LockLeft(true)
		case MouseRight:
			return c.Mouse.LockRight(true)
		case MouseMiddle:
			return c.Mouse.LockMiddle(true)
		case MouseMouse4:
			return c.Mouse.LockSide1(true)
		case MouseMouse5:
			return c.Mouse.LockSide2(true)
		default:
			return newMakcuCommandError("Unsupported button: %v", v)
		}
	case string:
		switch strings.ToUpper(v) {
		case "X":
			return c.Mouse.LockX(true)
		case "Y":
			return c.Mouse.LockY(true)
		default:
			return newMakcuCommandError("Invalid lock target: %v", v)
		}
	default:
		return newMakcuCommandError("Invalid lock target: %v", v)
	}
}

func (c *MakcuController) Unlock(target any) error {
	if err := c.checkConnection(); err != nil {
		return err
	}
	switch v := target.(type) {
	case MouseButton:
		switch v {
		case MouseLeft:
			return c.Mouse.LockLeft(false)
		case MouseRight:
			return c.Mouse.LockRight(false)
		case MouseMiddle:
			return c.Mouse.LockMiddle(false)
		case MouseMouse4:
			return c.Mouse.LockSide1(false)
		case MouseMouse5:
			return c.Mouse.LockSide2(false)
		default:
			return newMakcuCommandError("Unsupported button: %v", v)
		}
	case string:
		switch strings.ToUpper(v) {
		case "X":
			return c.Mouse.LockX(false)
		case "Y":
			return c.Mouse.LockY(false)
		default:
			return newMakcuCommandError("Invalid unlock target: %v", v)
		}
	default:
		return newMakcuCommandError("Invalid unlock target: %v", v)
	}
}

func (c *MakcuController) LockLeft(lock bool) error {
	if err := c.checkConnection(); err != nil {
		return err
	}
	return c.Mouse.LockLeft(lock)
}

func (c *MakcuController) LockMiddle(lock bool) error {
	if err := c.checkConnection(); err != nil {
		return err
	}
	return c.Mouse.LockMiddle(lock)
}

func (c *MakcuController) LockRight(lock bool) error {
	if err := c.checkConnection(); err != nil {
		return err
	}
	return c.Mouse.LockRight(lock)
}

func (c *MakcuController) LockSide1(lock bool) error {
	if err := c.checkConnection(); err != nil {
		return err
	}
	return c.Mouse.LockSide1(lock)
}

func (c *MakcuController) LockSide2(lock bool) error {
	if err := c.checkConnection(); err != nil {
		return err
	}
	return c.Mouse.LockSide2(lock)
}

func (c *MakcuController) LockX(lock bool) error {
	if err := c.checkConnection(); err != nil {
		return err
	}
	return c.Mouse.LockX(lock)
}

func (c *MakcuController) LockY(lock bool) error {
	if err := c.checkConnection(); err != nil {
		return err
	}
	return c.Mouse.LockY(lock)
}

func (c *MakcuController) LockMouseX(lock bool) error {
	return c.LockX(lock)
}

func (c *MakcuController) LockMouseY(lock bool) error {
	return c.LockY(lock)
}

func (c *MakcuController) IsLocked(b MouseButton) (bool, error) {
	if err := c.checkConnection(); err != nil {
		return false, err
	}
	return c.Mouse.IsLocked(b), nil
}

func (c *MakcuController) GetAllLockStates() (map[string]bool, error) {
	if err := c.checkConnection(); err != nil {
		return nil, err
	}
	return c.Mouse.GetAllLockStates(), nil
}

func (c *MakcuController) SpoofSerial(s string) error {
	if err := c.checkConnection(); err != nil {
		return err
	}
	return c.Mouse.SpoofSerial(s)
}

func (c *MakcuController) ResetSerial() error {
	if err := c.checkConnection(); err != nil {
		return err
	}
	return c.Mouse.ResetSerial()
}

func (c *MakcuController) GetDeviceInfo() (map[string]any, error) {
	if err := c.checkConnection(); err != nil {
		return nil, err
	}
	return c.Mouse.GetDeviceInfo(), nil
}

func (c *MakcuController) GetFirmwareVersion() (string, error) {
	if err := c.checkConnection(); err != nil {
		return "", err
	}
	return c.Mouse.GetFirmwareVersion()
}

func (c *MakcuController) GetButtonMask() (int, error) {
	if err := c.checkConnection(); err != nil {
		return 0, err
	}
	return c.Transport.GetButtonMask(), nil
}

func (c *MakcuController) GetButtonStates() (map[string]bool, error) {
	if err := c.checkConnection(); err != nil {
		return nil, err
	}
	return c.Transport.GetButtonStates(), nil
}

func (c *MakcuController) IsPressed(b MouseButton) (bool, error) {
	if err := c.checkConnection(); err != nil {
		return false, err
	}
	states := c.Transport.GetButtonStates()
	return states[strings.ToLower(b.Name())], nil
}

func (c *MakcuController) EnableButtonMonitoring(enable bool) error {
	if err := c.checkConnection(); err != nil {
		return err
	}
	return c.Transport.EnableButtonMonitoring(enable)
}

func (c *MakcuController) SetButtonCallback(cb func(MouseButton, bool)) error {
	if err := c.checkConnection(); err != nil {
		return err
	}
	c.Transport.SetButtonCallback(cb)
	return nil
}

func (c *MakcuController) OnConnectionChange(cb func(bool)) {
	c.callbackMu.Lock()
	c.connectionCallbacks = append(c.connectionCallbacks, cb)
	c.callbackMu.Unlock()
}

func (c *MakcuController) RemoveConnectionCallback(cb func(bool)) {
	c.callbackMu.Lock()
	defer c.callbackMu.Unlock()

	target := reflect.ValueOf(cb).Pointer()
	for i, existing := range c.connectionCallbacks {
		if reflect.ValueOf(existing).Pointer() == target {
			c.connectionCallbacks = append(c.connectionCallbacks[:i], c.connectionCallbacks[i+1:]...)
			return
		}
	}
}

type timingProfile struct {
	minDown, maxDown float64
	minWait, maxWait float64
}

var timingProfiles = map[string]timingProfile{
	"normal":   {60, 120, 100, 180},
	"fast":     {30, 60, 50, 100},
	"slow":     {100, 180, 150, 300},
	"variable": {40, 200, 80, 250},
	"gaming":   {20, 40, 30, 60},
}

func (c *MakcuController) ClickHumanLike(b MouseButton, count int, profile string, jitter int) error {
	if err := c.checkConnection(); err != nil {
		return err
	}

	tp, ok := timingProfiles[profile]
	if !ok {
		return newMakcuCommandError("Invalid profile: %s", profile)
	}

	for i := 0; i < count; i++ {
		if jitter > 0 {
			dx := rand.Intn(2*jitter+1) - jitter
			dy := rand.Intn(2*jitter+1) - jitter
			if err := c.Mouse.Move(dx, dy); err != nil {
				return err
			}
		}

		if err := c.Mouse.Press(b); err != nil {
			return err
		}
		downMs := tp.minDown + rand.Float64()*(tp.maxDown-tp.minDown)
		time.Sleep(time.Duration(downMs) * time.Millisecond)
		if err := c.Mouse.Release(b); err != nil {
			return err
		}

		if i < count-1 {
			waitMs := tp.minWait + rand.Float64()*(tp.maxWait-tp.minWait)
			time.Sleep(time.Duration(waitMs) * time.Millisecond)
		}
	}
	return nil
}

func (c *MakcuController) Drag(startX, startY, endX, endY int, button MouseButton, duration float64) error {
	if err := c.checkConnection(); err != nil {
		return err
	}

	if err := c.Move(startX, startY); err != nil {
		return err
	}
	time.Sleep(20 * time.Millisecond)

	if err := c.Press(button); err != nil {
		return err
	}
	time.Sleep(20 * time.Millisecond)

	segments := int(duration * 30)
	if segments < 10 {
		segments = 10
	}
	if err := c.MoveSmooth(endX-startX, endY-startY, segments); err != nil {
		return err
	}

	time.Sleep(20 * time.Millisecond)
	return c.Release(button)
}

func CreateController(opts ...Option) (*MakcuController, error) {
	ctrl := NewMakcuController(opts...)
	if err := ctrl.Connect(); err != nil {
		return nil, err
	}
	return ctrl, nil
}

func CreateAsyncController(opts ...Option) (*MakcuController, error) {
	return CreateController(opts...)
}
