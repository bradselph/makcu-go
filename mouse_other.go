//go:build !windows

package makcu

func getCursorPos() (int, int) {
	return 0, 0
}

func getMouseSpeedMultiplier() float64 {
	return 1.0
}
