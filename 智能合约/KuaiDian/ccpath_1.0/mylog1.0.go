package main

import (
	"fmt"

	"github.com/hyperledger/fabric/core/chaincode/shim"
)

type MYLOG struct {
	logger *shim.ChaincodeLogger
}

func InitMylog(module string) *MYLOG {
	var logger = shim.NewLogger(module)
	logger.SetLevel(shim.LogInfo)
	return &MYLOG{logger}
}

// debug=5, info=4, notice=3, warning=2, error=1, critical=0
func (m *MYLOG) SetDefaultLvl(lvl shim.LoggingLevel) {
	m.logger.SetLevel(lvl)
}

func (m *MYLOG) Debug(format string, args ...interface{}) {
	m.logger.Debugf(format, args...)
}
func (m *MYLOG) Info(format string, args ...interface{}) {
	m.logger.Infof(format, args...)
}
func (m *MYLOG) Notice(format string, args ...interface{}) {
	m.logger.Noticef(format, args...)
}

func (m *MYLOG) Warn(format string, args ...interface{}) {
	m.logger.Warningf(format, args...)
}
func (m *MYLOG) Error(format string, args ...interface{}) {
	m.logger.Errorf(format, args...)
}
func (m *MYLOG) Errorf(format string, args ...interface{}) error {
	var info = fmt.Sprintf(format, args...)
	m.logger.Errorf(info)
	return fmt.Errorf(info)
}
func (m *MYLOG) SError(format string, args ...interface{}) string {
	var info = fmt.Sprintf(format, args...)
	m.logger.Errorf(info)
	return info
}

func (m *MYLOG) Critical(format string, args ...interface{}) {
	m.logger.Criticalf(format, args...)
}
