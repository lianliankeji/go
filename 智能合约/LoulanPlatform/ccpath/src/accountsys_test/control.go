package main

//流程控制
const (
	Ctrl_isTestChain        = true  //是否是测试链。
	Ctrl_needCheckSign      = false //是否检查签名。
	Ctrl_needCheckIndentity = false //是否检查用户身份。
)

func LogCtrlParameter(logger *MYLOG) {
	logger.Info("Ctrl_isTestChain        = %v", Ctrl_isTestChain)
	logger.Info("Ctrl_needCheckSign      = %v", Ctrl_needCheckSign)
	logger.Info("Ctrl_needCheckIndentity = %v", Ctrl_needCheckIndentity)
}
