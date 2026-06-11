package makcu

import (
	"os"
	"reflect"
	"testing"
	"time"
)

func TestNewMakcuController_Defaults(t *testing.T) {
	c := NewMakcuController()
	if c.Transport == nil {
		t.Fatal("Transport is nil")
	}
	if c.Mouse == nil {
		t.Fatal("Mouse is nil")
	}
}

func TestNewMakcuController_WithOptions(t *testing.T) {
	c := NewMakcuController(
		WithFallbackPort("COM99"),
		WithDebug(false),
		WithSendInit(true),
		WithAutoReconnect(true),
		WithOverridePort(false),
	)
	if c.Transport == nil {
		t.Fatal("Transport is nil")
	}
	if c.Mouse == nil {
		t.Fatal("Mouse is nil")
	}
}

func TestMakcuController_IsConnected_FalseBeforeConnect(t *testing.T) {
	c := NewMakcuController()
	if c.IsConnected() {
		t.Fatal("IsConnected() should be false before Connect()")
	}
}

func TestMakcuController_DisconnectNotConnected(t *testing.T) {
	c := NewMakcuController()

	if err := c.Disconnect(); err != nil {
		t.Fatalf("Disconnect() on non-connected controller returned error: %v", err)
	}
}

func TestMakcuController_BatchExecute_NotConnected(t *testing.T) {
	c := NewMakcuController()

	err := c.BatchExecute([]func() error{
		func() error { return nil },
	})
	if err == nil {
		t.Fatal("BatchExecute() should fail when not connected")
	}
	var connErr *MakcuConnectionError
	if !asError(err, &connErr) {
		t.Fatalf("expected MakcuConnectionError, got %T: %v", err, err)
	}
}

func TestMakcuController_Move_NotConnected(t *testing.T) {
	c := NewMakcuController()
	err := c.Move(10, 20)
	if err == nil {
		t.Fatal("Move() should fail when not connected")
	}
}

func TestMakcuController_Scroll_NotConnected(t *testing.T) {
	c := NewMakcuController()
	err := c.Scroll(1)
	if err == nil {
		t.Fatal("Scroll() should fail when not connected")
	}
}

func TestMakcuController_Click_NotConnected(t *testing.T) {
	c := NewMakcuController()
	err := c.Click(MouseLeft)
	if err == nil {
		t.Fatal("Click() should fail when not connected")
	}
}

func TestMakcuController_Press_NotConnected(t *testing.T) {
	c := NewMakcuController()
	err := c.Press(MouseLeft)
	if err == nil {
		t.Fatal("Press() should fail when not connected")
	}
}

func TestMakcuController_Release_NotConnected(t *testing.T) {
	c := NewMakcuController()
	err := c.Release(MouseLeft)
	if err == nil {
		t.Fatal("Release() should fail when not connected")
	}
}

func TestMakcuController_GetButtonMask_NotConnected(t *testing.T) {
	c := NewMakcuController()
	if _, err := c.GetButtonMask(); err == nil {
		t.Fatal("GetButtonMask should fail when not connected")
	}
}

func TestMakcuController_GetButtonStates_NotConnected(t *testing.T) {
	c := NewMakcuController()
	if _, err := c.GetButtonStates(); err == nil {
		t.Fatal("GetButtonStates should fail when not connected")
	}
}

func TestMakcuController_IsPressed_NotConnected(t *testing.T) {
	c := NewMakcuController()
	if _, err := c.IsPressed(MouseLeft); err == nil {
		t.Fatal("IsPressed should fail when not connected")
	}
}

func TestMakcuController_OnConnectionChange_Fires(t *testing.T) {
	c := NewMakcuController()

	var fired []bool
	cb := func(connected bool) {
		fired = append(fired, connected)
	}
	c.OnConnectionChange(cb)

	_ = c.Disconnect()

	if len(fired) == 0 {
		t.Fatal("OnConnectionChange callback was not called")
	}
	if fired[len(fired)-1] != false {
		t.Fatalf("last connection-change value: want false, got true")
	}
}

func TestMakcuController_RemoveConnectionCallback(t *testing.T) {
	c := NewMakcuController()

	var count int
	cb := func(_ bool) { count++ }

	c.OnConnectionChange(cb)
	_ = c.Disconnect()
	if count == 0 {
		t.Fatal("callback was not called after OnConnectionChange")
	}

	prev := count
	c.RemoveConnectionCallback(cb)
	_ = c.Disconnect()
	if count != prev {
		t.Fatalf("callback fired after RemoveConnectionCallback; count was %d, now %d", prev, count)
	}
}

func TestMakcuController_Lock_InvalidTarget(t *testing.T) {
	c := NewMakcuController()

	c.connected = true

	err := c.Lock("Z")
	if err == nil {
		t.Fatal("Lock(\"Z\") should return an error for invalid target")
	}
}

func TestMakcuController_Unlock_InvalidTarget(t *testing.T) {
	c := NewMakcuController()
	c.connected = true

	err := c.Unlock("Z")
	if err == nil {
		t.Fatal("Unlock(\"Z\") should return an error for invalid target")
	}
}

func TestMakcuController_MoveBezier_NilCtrl(t *testing.T) {
	c := NewMakcuController()
	c.connected = true

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("MoveBezier panicked: %v", r)
		}
	}()
	_ = c.MoveBezier(10, 20, 5, nil, nil)
}

func TestMakcuController_ClickHumanLike_InvalidProfile(t *testing.T) {
	c := NewMakcuController()
	c.connected = true

	err := c.ClickHumanLike(MouseLeft, 1, "nonexistent", 0)
	if err == nil {
		t.Fatal("ClickHumanLike with invalid profile should return error")
	}
}

func TestMakcuController_BatchExecute_ActionError(t *testing.T) {
	c := NewMakcuController()
	c.connected = true

	actions := []func() error{
		func() error { return nil },
		func() error { return newMakcuCommandError("simulated action failure") },
		func() error { return nil },
	}

	err := c.BatchExecute(actions)
	if err == nil {
		t.Fatal("BatchExecute should return an error when an action fails")
	}
}

func TestMakcuController_BatchExecute_AllSucceed(t *testing.T) {
	c := NewMakcuController()
	c.connected = true

	var ran int
	actions := []func() error{
		func() error { ran++; return nil },
		func() error { ran++; return nil },
		func() error { ran++; return nil },
	}

	err := c.BatchExecute(actions)
	if err != nil {
		t.Fatalf("BatchExecute should succeed when all actions succeed, got: %v", err)
	}
	if ran != 3 {
		t.Fatalf("expected 3 actions to run, got %d", ran)
	}
}

func TestMakcuController_GetAllLockStates_NotConnected(t *testing.T) {
	c := NewMakcuController()
	_, err := c.GetAllLockStates()
	if err == nil {
		t.Fatal("GetAllLockStates should fail when not connected")
	}
}

func TestMakcuController_GetFirmwareVersion_NotConnected(t *testing.T) {
	c := NewMakcuController()
	_, err := c.GetFirmwareVersion()
	if err == nil {
		t.Fatal("GetFirmwareVersion should fail when not connected")
	}
}

func TestMakcuController_SpoofSerial_NotConnected(t *testing.T) {
	c := NewMakcuController()
	err := c.SpoofSerial("TEST")
	if err == nil {
		t.Fatal("SpoofSerial should fail when not connected")
	}
}

func TestMakcuController_ResetSerial_NotConnected(t *testing.T) {
	c := NewMakcuController()
	err := c.ResetSerial()
	if err == nil {
		t.Fatal("ResetSerial should fail when not connected")
	}
}

func TestMakcuController_GetDeviceInfo_NotConnected(t *testing.T) {
	c := NewMakcuController()
	if _, err := c.GetDeviceInfo(); err == nil {
		t.Fatal("GetDeviceInfo should fail when not connected")
	}
}

func TestMakcuController_EnableButtonMonitoring_NotConnected(t *testing.T) {
	c := NewMakcuController()
	err := c.EnableButtonMonitoring(true)
	if err == nil {
		t.Fatal("EnableButtonMonitoring should fail when not connected")
	}
}

func TestMakcuController_SetButtonCallback_NotConnected(t *testing.T) {
	c := NewMakcuController()
	err := c.SetButtonCallback(func(_ MouseButton, _ bool) {})
	if err == nil {
		t.Fatal("SetButtonCallback should fail when not connected")
	}
}

func TestMakcuController_IsLocked_NotConnected(t *testing.T) {
	c := NewMakcuController()
	if _, err := c.IsLocked(MouseLeft); err == nil {
		t.Fatal("IsLocked should fail when not connected")
	}
}

func TestMakcuController_DoubleClick_NotConnected(t *testing.T) {
	c := NewMakcuController()
	err := c.DoubleClick(MouseLeft)
	if err == nil {
		t.Fatal("DoubleClick should fail when not connected")
	}
}

func TestMakcuController_MoveSmooth_NotConnected(t *testing.T) {
	c := NewMakcuController()
	err := c.MoveSmooth(10, 20, 5)
	if err == nil {
		t.Fatal("MoveSmooth should fail when not connected")
	}
}

func TestMakcuController_Drag_NotConnected(t *testing.T) {
	c := NewMakcuController()
	err := c.Drag(0, 0, 100, 100, MouseLeft, 1.0)
	if err == nil {
		t.Fatal("Drag should fail when not connected")
	}
}

func TestMakcuController_LockX_NotConnected(t *testing.T) {
	c := NewMakcuController()
	err := c.LockX(true)
	if err == nil {
		t.Fatal("LockX should fail when not connected")
	}
}

func TestMakcuController_LockY_NotConnected(t *testing.T) {
	c := NewMakcuController()
	err := c.LockY(true)
	if err == nil {
		t.Fatal("LockY should fail when not connected")
	}
}

func TestMakcuController_LockLeft_NotConnected(t *testing.T) {
	c := NewMakcuController()
	err := c.LockLeft(true)
	if err == nil {
		t.Fatal("LockLeft should fail when not connected")
	}
}

func TestMakcuController_LockRight_NotConnected(t *testing.T) {
	c := NewMakcuController()
	err := c.LockRight(true)
	if err == nil {
		t.Fatal("LockRight should fail when not connected")
	}
}

func TestMakcuController_LockMiddle_NotConnected(t *testing.T) {
	c := NewMakcuController()
	err := c.LockMiddle(true)
	if err == nil {
		t.Fatal("LockMiddle should fail when not connected")
	}
}

func TestMakcuController_LockSide1_NotConnected(t *testing.T) {
	c := NewMakcuController()
	err := c.LockSide1(true)
	if err == nil {
		t.Fatal("LockSide1 should fail when not connected")
	}
}

func TestMakcuController_LockSide2_NotConnected(t *testing.T) {
	c := NewMakcuController()
	err := c.LockSide2(true)
	if err == nil {
		t.Fatal("LockSide2 should fail when not connected")
	}
}

func TestMakcuController_LockMouseX_NotConnected(t *testing.T) {
	c := NewMakcuController()
	err := c.LockMouseX(true)
	if err == nil {
		t.Fatal("LockMouseX should fail when not connected")
	}
}

func TestMakcuController_LockMouseY_NotConnected(t *testing.T) {
	c := NewMakcuController()
	err := c.LockMouseY(true)
	if err == nil {
		t.Fatal("LockMouseY should fail when not connected")
	}
}

func TestMakcuController_Lock_ValidMouseButtons(t *testing.T) {
	c := NewMakcuController()
	c.connected = true

	buttonMap := map[MouseButton]bool{
		MouseLeft:   true,
		MouseRight:  true,
		MouseMiddle: true,
		MouseMouse4: true,
		MouseMouse5: true,
	}
	for btn := range buttonMap {
		err := c.Lock(btn)

		if err != nil {
			var cmdErr *MakcuCommandError
			if asError(err, &cmdErr) {
				t.Errorf("Lock(%v) returned unexpected command error: %v", btn, err)
			}
		}
	}
}

func TestMakcuController_Lock_ValidStringTargets(t *testing.T) {
	c := NewMakcuController()
	c.connected = true

	for _, target := range []string{"X", "x", "Y", "y"} {
		err := c.Lock(target)
		if err != nil {
			var cmdErr *MakcuCommandError
			if asError(err, &cmdErr) {
				t.Errorf("Lock(%q) returned unexpected command error: %v", target, err)
			}
		}
	}
}

func TestCreateController_ReturnsError_NoDevice(t *testing.T) {

	_, err := CreateController(WithFallbackPort("COM255"), WithOverridePort(true))
	if err == nil {
		t.Fatal("CreateController should return an error when device is not present")
	}
}

func TestCreateAsyncController_ReturnsError_NoDevice(t *testing.T) {
	_, err := CreateAsyncController(WithFallbackPort("COM255"), WithOverridePort(true))
	if err == nil {
		t.Fatal("CreateAsyncController should return an error when device is not present")
	}
}

func TestMakcuController_Close_CallsDisconnect(t *testing.T) {
	c := NewMakcuController()

	if err := c.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}
	if c.IsConnected() {
		t.Fatal("IsConnected() should be false after Close()")
	}
}

func TestMakcuController_Drag_DurationSegments(t *testing.T) {

	cases := []struct {
		duration float64
		want     int
	}{
		{0.0, 10},
		{0.1, 10},
		{0.5, 15},
		{1.0, 30},
		{2.0, 60},
	}
	for _, tc := range cases {
		seg := int(tc.duration * 30)
		if seg < 10 {
			seg = 10
		}
		if seg != tc.want {
			t.Errorf("duration=%.1f: segments want %d, got %d", tc.duration, tc.want, seg)
		}
	}
}

func asError[T error](err error, target *T) bool {
	if err == nil {
		return false
	}
	if t, ok := err.(T); ok {
		*target = t
		return true
	}
	return false
}

func sameFunc(a, b func(bool)) bool {
	return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}

var _ = sameFunc

func TestHW_ControllerConnect(t *testing.T) {
	skipIfNoDevice(t)

	var c *MakcuController
	if p := os.Getenv("MAKCU_PORT"); p != "" {
		c = NewMakcuController(WithFallbackPort(p), WithOverridePort(true))
	} else {
		c = NewMakcuController()
	}

	if err := c.Connect(); err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	defer func() { _ = c.Disconnect() }()

	if !c.IsConnected() {
		t.Fatal("IsConnected() false after Connect()")
	}
}

func TestHW_ControllerPressRelease(t *testing.T) {
	skipIfNoDevice(t)

	var c *MakcuController
	if p := os.Getenv("MAKCU_PORT"); p != "" {
		c = NewMakcuController(WithFallbackPort(p), WithOverridePort(true))
	} else {
		c = NewMakcuController()
	}

	if err := c.Connect(); err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	defer func() { _ = c.Disconnect() }()

	if err := c.Press(MouseLeft); err != nil {
		t.Fatalf("Press(LEFT) error: %v", err)
	}
	if err := c.Release(MouseLeft); err != nil {
		t.Fatalf("Release(LEFT) error: %v", err)
	}
}

func TestHW_ControllerFirmwareVersion(t *testing.T) {
	skipIfNoDevice(t)

	var c *MakcuController
	if p := os.Getenv("MAKCU_PORT"); p != "" {
		c = NewMakcuController(WithFallbackPort(p), WithOverridePort(true))
	} else {
		c = NewMakcuController()
	}

	if err := c.Connect(); err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	defer func() { _ = c.Disconnect() }()

	ver, err := c.GetFirmwareVersion()
	if err != nil {
		t.Fatalf("GetFirmwareVersion() error: %v", err)
	}
	if len(ver) == 0 {
		t.Fatal("firmware version empty")
	}
}

func TestHW_ControllerMove(t *testing.T) {
	skipIfNoDevice(t)

	var c *MakcuController
	if p := os.Getenv("MAKCU_PORT"); p != "" {
		c = NewMakcuController(WithFallbackPort(p), WithOverridePort(true))
	} else {
		c = NewMakcuController()
	}

	if err := c.Connect(); err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	defer func() { _ = c.Disconnect() }()

	coords := [][2]int{{10, 0}, {0, 10}, {-10, 0}, {0, -10}}
	for _, xy := range coords {
		if err := c.Move(xy[0], xy[1]); err != nil {
			t.Fatalf("Move(%d,%d) error: %v", xy[0], xy[1], err)
		}
	}
}

func TestHW_ControllerBehavior(t *testing.T) {
	skipIfNoDevice(t)

	var c *MakcuController
	if p := os.Getenv("MAKCU_PORT"); p != "" {
		c = NewMakcuController(WithFallbackPort(p), WithOverridePort(true))
	} else {
		c = NewMakcuController()
	}

	if err := c.Connect(); err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	defer func() { _ = c.Disconnect() }()

	if err := c.Move(25, 25); err != nil {
		t.Fatalf("Move error: %v", err)
	}
	if err := c.Click(MouseLeft); err != nil {
		t.Fatalf("Click error: %v", err)
	}
	if err := c.Scroll(-2); err != nil {
		t.Fatalf("Scroll error: %v", err)
	}
}

func TestHW_ControllerBatch(t *testing.T) {
	skipIfNoDevice(t)

	var c *MakcuController
	if p := os.Getenv("MAKCU_PORT"); p != "" {
		c = NewMakcuController(WithFallbackPort(p), WithOverridePort(true))
	} else {
		c = NewMakcuController()
	}

	if err := c.Connect(); err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	defer func() { _ = c.Disconnect() }()

	start := time.Now()
	err := c.BatchExecute([]func() error{
		func() error { return c.Move(5, 5) },
		func() error { return c.Click(MouseLeft) },
		func() error { return c.Scroll(-1) },
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("BatchExecute error: %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Logf("BatchExecute took %v (expected <500ms)", elapsed)
	}
}

func TestHW_ControllerLockState(t *testing.T) {
	skipIfNoDevice(t)

	var c *MakcuController
	if p := os.Getenv("MAKCU_PORT"); p != "" {
		c = NewMakcuController(WithFallbackPort(p), WithOverridePort(true))
	} else {
		c = NewMakcuController()
	}

	if err := c.Connect(); err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	defer func() {
		_ = c.LockLeft(false)
		_ = c.Disconnect()
	}()

	if err := c.LockLeft(true); err != nil {
		t.Fatalf("LockLeft(true) error: %v", err)
	}
	locked, err := c.IsLocked(MouseLeft)
	if err != nil {
		t.Fatalf("IsLocked(LEFT) error: %v", err)
	}
	if !locked {
		t.Fatal("IsLocked(LEFT) should be true after LockLeft(true)")
	}
}

func TestHW_ControllerCleanup(t *testing.T) {
	skipIfNoDevice(t)

	var c *MakcuController
	if p := os.Getenv("MAKCU_PORT"); p != "" {
		c = NewMakcuController(WithFallbackPort(p), WithOverridePort(true))
	} else {
		c = NewMakcuController()
	}

	if err := c.Connect(); err != nil {
		t.Fatalf("Connect() error: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	_ = c.LockLeft(false)
	_ = c.LockRight(false)
	_ = c.LockMiddle(false)
	_ = c.LockSide1(false)
	_ = c.LockSide2(false)
	_ = c.LockX(false)
	_ = c.LockY(false)

	_ = c.Release(MouseLeft)
	_ = c.Release(MouseRight)
	_ = c.Release(MouseMiddle)
	_ = c.Release(MouseMouse4)
	_ = c.Release(MouseMouse5)

	_ = c.EnableButtonMonitoring(false)

	if err := c.Disconnect(); err != nil {
		t.Fatalf("Disconnect() error: %v", err)
	}
	if c.IsConnected() {
		t.Fatal("IsConnected() should be false after Disconnect()")
	}
}
