package main

import (
	"fmt"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
)

var parmaters = map[string]string{
	"checkSiagnature": "0", //是否检查签名 0：不检查，1：检查
}

type SysCtrl struct {
}

var ctrlogger = shim.NewLogger("sysCtlcc_test")

func (b *SysCtrl) Init(stub shim.ChaincodeStubInterface) (pbResponse pb.Response) {
	ctrlogger.Debug("Enter Init")
	function, args := stub.GetFunctionAndParameters()
	ctrlogger.Debug("func =%s, args = %+v", function, args)

	defer func() {
		if excption := recover(); excption != nil {
			pbResponse = shim.Error(fmt.Sprintf("Init(%s) got excption:%s", function, excption))
		}
	}()

	//合约实例化时，默认会执行init函数，除非在调用合约实例化接口时指定了其它的函数
	//注意，只有在第一次部署时才能执行init函数，后续升级时如果执行了init函数，所有数据将会被清空
	if function == "init" {
		//do someting

		ctrlogger.Infof("parmaters=%+v", parmaters)

		return shim.Success(nil)

	} else if function == "upgrade" { //升级时默认会执行upgrade函数，除非在调用合约升级接口时指定了其它的函数
		//do someting,

		return shim.Success(nil)

	} else {

		return shim.Error(fmt.Sprintf("unknown function: %s", function))
	}
}

func (b *SysCtrl) Invoke(stub shim.ChaincodeStubInterface) (pbResponse pb.Response) {

	ctrlogger.Debug("Enter Invoke")
	function, args := stub.GetFunctionAndParameters()
	ctrlogger.Debug("func =%s, args = %+v", function, args)

	defer func() {
		if excption := recover(); excption != nil {
			pbResponse = shim.Error(fmt.Sprintf("Invoke(%s) got excption:%s", function, excption))
		}
	}()

	if function == "getParameter" {
		if len(args) < 1 {
			return shim.Error(fmt.Sprintf("Invoke(%s) miss args.", function))
		}

		var paraName = args[0]

		if _, ok := parmaters[paraName]; !ok {
			return shim.Error(fmt.Sprintf("Invoke(%s), can not find parameter name '%s'.", function, paraName))
		}

		return shim.Success([]byte(parmaters[paraName]))

	} else {

		return shim.Error(fmt.Sprintf("unknown function: %s", function))
	}
}

func main() {

	err := shim.Start(new(SysCtrl))
	if err != nil {
		ctrlogger.Error("Error starting  chaincode: %s", err)
	}
}
