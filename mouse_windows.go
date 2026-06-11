//go:build windows

package makcu

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

type point struct {
	X int32
	Y int32
}

var (
	user32            = windows.NewLazySystemDLL("user32.dll")
	procGetCursorPos  = user32.NewProc("GetCursorPos")
	procSysParamsInfo = user32.NewProc("SystemParametersInfoW")
)

func getCursorPos() (int, int) {
	var pt point
	_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	return int(pt.X), int(pt.Y)
}

func getMouseSpeedMultiplier() float64 {
	const SPI_GETMOUSESPEED = 0x0070
	var value uint32
	_, _, _ = procSysParamsInfo.Call(
		uintptr(SPI_GETMOUSESPEED),
		0,
		uintptr(unsafe.Pointer(&value)),
		0,
	)
	return float64(value) / 10.0
}
