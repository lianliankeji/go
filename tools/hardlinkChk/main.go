// test1 project main.go
package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"runtime/debug"
	"syscall"
)

type HardLinkCfg map[string][]string

func main() {
	defer func() {
		if err := recover(); err != nil {
			ErrorPrint("got exception: %s\n%s\n", err, string(debug.Stack()))
		}

		fmt.Printf("\n\npress 'Enter' key to exit.\n")
		os.Stdin.Read(make([]byte, 1))

		/* 如果要实现按任意键退出，可以调用c的getch函数。  纯go目前没有简单的方法实现getch，弱鸡？
		C.getch()
		#include<conio.h>
		import "C"
		*/
	}()

	data, err := ioutil.ReadFile("./hardlink.cfg")
	if err != nil {
		ErrorPrint("Read hardlink.cfg failed, err:%s.\n", err)
		return
	}

	var hlc2 HardLinkCfg
	err = json.Unmarshal(data, &hlc2)
	if err != nil {
		ErrorPrint("Unmarshal hardlink.cfg failed, err:%s.\n", err)
		return
	}

	//check
	NormalPrint("begin check the hardlink...\n")
	NormalPrint("check result:\n")
	NormalPrint("--------------------------------------------------\n")
	var hasErr = false
	for soureFile, dests := range hlc2 {
		if ok, _ := Exist(soureFile); !ok {
			ErrorPrint("soureFile '%s' in hardlink.cfg not exists.\n", soureFile)
			hasErr = true
			continue
		}

		info1, err := os.Stat(soureFile)
		if err != nil {
			ErrorPrint("get stat of '%s' failed, err:%s.\n", soureFile, err)
			hasErr = true
			continue
		}

		for _, dest := range dests {
			var destFile = path.Join(dest, soureFile)
			if ok, _ := Exist(destFile); !ok {
				ErrorPrint("destFile '%s' not exists.\n", destFile)
				hasErr = true
				continue
			}

			info2, err := os.Stat(destFile)
			if err != nil {
				ErrorPrint("get stat of '%s' failed, err:%s.\n", destFile, err)
				hasErr = true
				continue
			}

			if os.SameFile(info1, info2) {
				NormalPrint("check pass(√), '%s' is hardlink.\n", destFile)
			} else {
				ErrorPrint("check fail(×), '%s' is not hardlink.\n", destFile)
				hasErr = true
			}

		}
	}
	NormalPrint("--------------------------------------------------\n")

	if hasErr {
		ErrorPrint("Got some error, please check.\n")
	} else {
		NormalPrint("All successful.\n")
	}

}

func Exist(path string) (bool, error) {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	return true, err
}

//windows终端颜色
const (
	//前景
	FOREGROUND_BLUE      = 1
	FOREGROUND_GREEN     = 2
	FOREGROUND_RED       = 4
	FOREGROUND_INTENSITY = 8
	//背景
	BACKGROUND_BLUE      = 16
	BACKGROUND_GREEN     = 32
	BACKGROUND_RED       = 64
	BACKGROUND_INTENSITY = 128
)

func ColorPrint(s string, i int) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("SetConsoleTextAttribute")
	handle, _, _ := proc.Call(uintptr(syscall.Stdout), uintptr(i))
	fmt.Print(s)
	handle, _, _ = proc.Call(uintptr(syscall.Stdout), uintptr(7))
	CloseHandle := kernel32.NewProc("CloseHandle")
	CloseHandle.Call(handle)
}

func ErrorPrint(format string, args ...interface{}) {
	ColorPrint("Error: "+fmt.Sprintf(format, args...), FOREGROUND_INTENSITY|FOREGROUND_RED)
}
func NormalPrint(format string, args ...interface{}) {
	fmt.Printf(" Info: "+format, args...)
}
