package idle

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32             = windows.NewLazySystemDLL("user32.dll")
	kernel32           = windows.NewLazySystemDLL("kernel32.dll")
	procGetLastInput   = user32.NewProc("GetLastInputInfo")
	procGetTickCount32 = kernel32.NewProc("GetTickCount")
)

type lastInputInfo struct {
	size uint32
	time uint32
}

// windowsSource uses GetLastInputInfo, which reports the tick count of the
// session's last input. Both counts are 32 bit milliseconds that wrap after
// 49 days; unsigned subtraction handles the wrap.
type windowsSource struct{}

func systemSource() Source { return windowsSource{} }

func (windowsSource) Idle() (time.Duration, error) {
	info := lastInputInfo{size: uint32(unsafe.Sizeof(lastInputInfo{}))}
	if ok, _, _ := procGetLastInput.Call(uintptr(unsafe.Pointer(&info))); ok == 0 {
		return 0, ErrUnsupported
	}
	now, _, _ := procGetTickCount32.Call()
	return time.Duration(uint32(now)-info.time) * time.Millisecond, nil
}
