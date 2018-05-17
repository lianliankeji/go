// test1 project main.go
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"runtime/debug"
	"strings"
	"syscall"
)

type HardLinkCfg map[string][]string

func main() {
	var isWait = true
	defer func() {
		if err := recover(); err != nil {
			ErrorPrint("got exception: %s\n%s\n", err, string(debug.Stack()))
		}

		if isWait {
			fmt.Printf("\n\npress 'Enter' key to exit.\n")
			os.Stdin.Read(make([]byte, 1))
		}

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
	NormalPrint("check hardlink.cfg...\n")
	var oldLinkList []string
	var newLinkList []string
	for soureFile, dests := range hlc2 {
		if ok, _ := Exist(soureFile); !ok {
			ErrorPrint("soureFile '%s' in hardlink.cfg not exists.\n", soureFile)
			return
		}

		info1, err := os.Stat(soureFile)
		if err != nil {
			ErrorPrint("get stat of '%s' failed, err:%s.\n", soureFile, err)
			return
		}

		for _, dest := range dests {
			if ok, _ := Exist(dest); !ok {
				ErrorPrint("destPath '%s' in hardlink.cfg not exists.\n", dest)
				return
			}

			var destFile = path.Join(dest, soureFile)
			var links = fmt.Sprintf("'%s' => '%s'", soureFile, destFile)
			//目的文件已存在，看是否已经是硬链接
			if ok, _ := Exist(destFile); ok {
				info2, err := os.Stat(destFile)
				if err != nil {
					ErrorPrint("get stat of '%s' failed, err:%s.\n", destFile, err)
					return
				}

				if os.SameFile(info1, info2) {
					oldLinkList = append(oldLinkList, links)
				} else {
					newLinkList = append(newLinkList, links)
				}
			} else {
				newLinkList = append(newLinkList, links)
			}

		}
	}

	if len(oldLinkList) > 0 {
		NormalPrint("hardlinks already exist as follow:\n")
		for _, link := range oldLinkList {
			fmt.Printf("    - %s\n", link)
		}
	}

	if len(newLinkList) == 0 {
		NormalPrint("no more new hardlinks need to make.\n")
		return
	}

	NormalPrint("will make new hardlinks as follow:\n")
	for _, link := range newLinkList {
		fmt.Printf("    - %s\n", link)
	}

waitInput:
	fmt.Printf("Do you want to continue? (yes/no) ")
	var inputReader = bufio.NewReader(os.Stdin)
	input, _ := inputReader.ReadString('\n')
	input = strings.ToUpper(strings.TrimSpace(input))
	if input != "YES" && input != "NO" {
		goto waitInput
	} else if input == "NO" {
		isWait = false //输入no时直接退出，不用等待
		return
	}

	NormalPrint("begin make hardlinks...\n")
	NormalPrint("make result:\n")
	NormalPrint("--------------------------------------------------\n")
	var hasErr = false
	for soureFile, dests := range hlc2 {

		info1, err := os.Stat(soureFile)
		if err != nil {
			ErrorPrint("get stat of '%s' failed, err:%s.\n", soureFile, err)
			hasErr = true
			continue
		}

		for _, dest := range dests {
			var destFile = path.Join(dest, soureFile)
			//文件已存在则备份
			if ok, _ := Exist(destFile); ok {

				info2, err := os.Stat(destFile)
				if err != nil {
					ErrorPrint("get stat of '%s' failed, err:%s.\n", destFile, err)
					hasErr = true
					continue
				}
				//已是硬链接，不处理
				if os.SameFile(info1, info2) {
					continue
				}

				var bakFile = destFile + ".hdlk.bak"
				err = os.Rename(destFile, bakFile)
				if err != nil {
					ErrorPrint("bakup for '%s' failed, err=%s.\n", destFile, err)
					hasErr = true
				} else {
					//NormalPrint("bakup file '%s' => '%s'.\n", destFile, bakFile)
				}
			}

			err := os.Link(soureFile, destFile)
			if err != nil {
				ErrorPrint("hardlink '%s' => '%s' failed, err=%s.\n", soureFile, destFile, err)
				hasErr = true
			} else {
				NormalPrint("hardlink '%s' => '%s' success.\n", soureFile, destFile)
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

func copyFile(source, dest string) error {
	if source == "" || dest == "" {
		return fmt.Errorf("source or dest file is nil.")
	}
	source_open, err := os.Open(source)
	if err != nil {
		return err
	}
	defer source_open.Close()
	//只写模式打开文件 如果文件不存在进行创建 并赋予 644的权限。详情查看linux 权限解释
	dest_open, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY, 644)
	if err != nil {
		return err
	}
	defer dest_open.Close()

	_, copy_err := io.Copy(dest_open, source_open)
	if copy_err != nil {
		return copy_err
	}

	return nil
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
