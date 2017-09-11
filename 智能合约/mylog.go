package main

import (
	"bytes"
	"fmt"
	"time"
)

const (
	MYLOG_LVL_DEBUG int = iota
	MYLOG_LVL_INFO
	MYLOG_LVL_WARNING
	MYLOG_LVL_ERROR
	MYLOG_LVL_FATAL
	MYLOG_LVL_MAX
)

type MYLOG struct {
	module     string
	defaultLvl int
}

func InitMylog(module string) *MYLOG {
	return &MYLOG{module: module, defaultLvl: MYLOG_LVL_WARNING}
}

func (m *MYLOG) SetDefaultLvl(lvl int) {
	if lvl >= MYLOG_LVL_DEBUG && lvl < MYLOG_LVL_MAX {
		m.defaultLvl = lvl
	}
}

func (m *MYLOG) log(lvl int, format string, args ...interface{}) {

	if lvl < m.defaultLvl {
		return
	}

	var lvlStr string

	switch lvl {
	case MYLOG_LVL_DEBUG:
		lvlStr = "debug"
	case MYLOG_LVL_INFO:
		lvlStr = "info"
	case MYLOG_LVL_WARNING:
		lvlStr = "warning"
	case MYLOG_LVL_ERROR:
		lvlStr = "error"
	case MYLOG_LVL_FATAL:
		lvlStr = "fatal"
	}

	buf := bytes.NewBufferString(time.Now().Local().Format("20060102 15:04:05.000"))
	buf.WriteString(" [")
	buf.WriteString(m.module)
	buf.WriteString("]")
	buf.WriteString(lvlStr)
	buf.WriteString(": ")
	buf.WriteString(format)
	buf.WriteString("\n")

	fmt.Printf(buf.String(), args...)
}

func (m *MYLOG) Debug(format string, args ...interface{}) {
	m.log(MYLOG_LVL_DEBUG, format, args...)
}
func (m *MYLOG) Info(format string, args ...interface{}) {
	m.log(MYLOG_LVL_INFO, format, args...)
}
func (m *MYLOG) Warn(format string, args ...interface{}) {
	m.log(MYLOG_LVL_WARNING, format, args...)
}
func (m *MYLOG) Error(format string, args ...interface{}) {
	m.log(MYLOG_LVL_ERROR, format, args...)
}
func (m *MYLOG) Fatal(format string, args ...interface{}) {
	m.log(MYLOG_LVL_FATAL, format, args...)
}
