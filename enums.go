package makcu

type MouseButton int

const (
	MouseLeft   MouseButton = 0
	MouseRight  MouseButton = 1
	MouseMiddle MouseButton = 2
	MouseMouse4 MouseButton = 3
	MouseMouse5 MouseButton = 4
)

func (b MouseButton) Name() string {
	switch b {
	case MouseLeft:
		return "LEFT"
	case MouseRight:
		return "RIGHT"
	case MouseMiddle:
		return "MIDDLE"
	case MouseMouse4:
		return "MOUSE4"
	case MouseMouse5:
		return "MOUSE5"
	default:
		return "UNKNOWN"
	}
}

func (b MouseButton) String() string { return b.Name() }
