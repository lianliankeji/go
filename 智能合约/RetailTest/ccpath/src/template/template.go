package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
)

const (
	CONTROL_CC_NAME              = "sysctrlcc"
	CONTROL_CC_GETPARA_FUNC_NAME = "getParameter"
)

type AccountEntity struct {
	EntID             string `json:"id"`   //ID
	TotalAmount       int64  `json:"tamt"` //货币总数额(发行或接收)
	RestAmount        int64  `json:"ramt"` //账户余额
	Time              int64  `json:"time"` //开户时间
	Owner             string `json:"own"`  //该实例所属的用户
	OwnerPubKeyHash   string `json:"opbk"` //公钥hash
	OwnerIdentityHash string `json:"oidt"` //身份hash
}

type Template struct {
}

//包初始化函数
func init() {

}

var tmpltLogger = shim.NewLogger("template")

var ErrNilEntity = errors.New("nil entity.")

//do not delete this hook
var verifyIdentityHook func(stub shim.ChaincodeStubInterface, userName string, sign, signMsg []byte, ownerPubKeyHash, ownerIdentityHash string) error

func (t *Template) Init(stub shim.ChaincodeStubInterface) (pbResponse pb.Response) {
	tmpltLogger.Debug("Enter Init")
	function, args := stub.GetFunctionAndParameters()
	tmpltLogger.Debug("func =%s, args = %+v", function, args)

	defer func() {
		if excption := recover(); excption != nil {
			pbResponse = shim.Error(fmt.Sprintf("Init(%s) got excption:%s", function, excption))
		}
	}()

	timestamp, err := stub.GetTxTimestamp()
	if err != nil {
		return shim.Error(fmt.Sprintf("Init: GetTxTimestamp failed, err=%s", err))
	}

	var initTime = timestamp.Seconds*1000 + int64(timestamp.Nanos/1000000) //精确到毫秒
	tmpltLogger.Debug("initTime= %+v", initTime)

	//合约实例化时，默认会执行init函数，除非在调用合约实例化接口时指定了其它的函数
	//注意，只有在第一次部署时才能执行init函数，后续升级时如果执行了init函数，所有数据将会被清空
	if function == "init" {
		//do someting
		return shim.Success(nil)

	} else if function == "upgrade" { //升级时默认会执行upgrade函数，除非在调用合约升级接口时指定了其它的函数
		//do someting,

		return shim.Success(nil)

	} else {

		return shim.Error(fmt.Sprintf("unknown function: %s", function))
	}
}

func (t *Template) Invoke(stub shim.ChaincodeStubInterface) (pbResponse pb.Response) {

	tmpltLogger.Debug("Enter Invoke")
	function, args := stub.GetFunctionAndParameters()
	tmpltLogger.Debug("func =%s, args = %+v", function, args)

	defer func() {
		if excption := recover(); excption != nil {
			pbResponse = shim.Error(fmt.Sprintf("Invoke(%s) got excption:%s", function, excption))
		}
	}()

	timestamp, err := stub.GetTxTimestamp()
	if err != nil {
		return shim.Error(fmt.Sprintf("Init: GetTxTimestamp failed, err=%s", err))
	}

	var invokeTime = timestamp.Seconds*1000 + int64(timestamp.Nanos/1000000) //精确到毫秒

	//第一个参数为用户名，第二个参数为账户名， 第三个...  最后一个元素是用户签名，实际情况中，可以根据业务需求调整这个最小参数个数。
	var fixedArgCount = 2
	//最后一个参数为签名，所以参数必须大于fixedArgCount个
	if len(args) < fixedArgCount+1 {
		return shim.Error(fmt.Sprintf("Invoke miss arg, got %d, at least need %d(use, acc, signature).", len(args), fixedArgCount+1))
	}

	var userName = args[0]
	var accName = args[1]

	//签名为最后一个参数
	var signBase64 = args[len(args)-1]

	var sign []byte
	sign, err = base64.StdEncoding.DecodeString(signBase64)
	if err != nil {
		return shim.Error(fmt.Sprintf("Invoke convert sign(%s) failed. err=%s", signBase64, err))
	}
	if len(sign) == 0 {
		return shim.Error(fmt.Sprintf("Invoke can not get signature."))
	}

	//客户端签名的生成： 把函数名和输入的参数用","拼接为字符串，然后计算其Sha256作为msg，然后用私钥对msg做签名。所以这里用同样的方法生成msg
	var allArgsString = function + "," + strings.Join(args[:len(args)-1], ",") //不包括签名本身
	msg := util.ComputeSHA256([]byte(allArgsString))

	tmpltLogger.Debug("allArgsString =%s", allArgsString)
	tmpltLogger.Debug("sign-msg =%v", msg)

	var accountEnt *AccountEntity = nil

	//开户之前还没有，账户中没有保存公钥，先不验证
	if function != "account" {

		accountEnt, err = t.getAccountEntity(stub, accName)
		if err != nil {
			return shim.Error(fmt.Sprintf("Invoke getAccountEntity failed. err=%s", err))
		}

		//校验修改Entity的用户身份，只有Entity的所有者才能修改自己的Entity
		if err = verifyIdentity(stub, userName, sign, msg, accountEnt.OwnerPubKeyHash, accountEnt.OwnerIdentityHash); err != nil {
			return shim.Error(fmt.Sprintf("Invoke: verifyIdentity(%s) failed.", accName))
		}

	}

	if function == "account" {
		tmpltLogger.Debug("Enter account")
		var usrType int

		var argCount = fixedArgCount + 2
		if len(args) < argCount {
			return shim.Error(fmt.Sprintf("Invoke(account) miss arg, got %d, at least need %d.", len(args), argCount))
		}

		var userIdHash = args[fixedArgCount]       //base64
		var userPubKeyHash = args[fixedArgCount+1] //base64

		//校验修改Entity的用户身份，只有Entity的所有者才能修改自己的Entity
		if err = verifyIdentity(stub, userName, sign, msg, userPubKeyHash, userIdHash); err != nil {
			return shim.Error(fmt.Sprintf("Invoke(account) verifyIdentity(%s) failed.", accName))
		}

		_, err = t.newAccount(stub, accName, usrType, userName, userIdHash, userPubKeyHash, invokeTime, false)
		if err != nil {
			return shim.Error(fmt.Sprintf("Invoke(account) newAccount failed. err=%s", err))
		}
		return shim.Success(nil)

	} else if function == "writeXXX" {

		err = stub.PutState("XXX", []byte(args[0]))
		if err != nil {
			return shim.Error(fmt.Sprintf("writeXXX: PutState failed, err=%s", err))
		}

		return shim.Success(nil)

	} else if function == "writeYYY" {
		err = stub.PutState("YYY", []byte(args[0]))
		if err != nil {
			return shim.Error(fmt.Sprintf("writeYYY: PutState failed, err=%s", err))
		}

		return shim.Success(nil)

	} else {

		//fabric1.0版本不再区分invoke和query调用只有一个Invoke接口（0.6版本是区分的，有Invoke和Query两个接口），这里仍然沿用两者分开的方式，不过要在Invoke中调用Query
		retValue, err := t.Query(stub, invokeTime)

		if err != nil {
			return shim.Error(err.Error())
		}

		return shim.Success(retValue)
	}
}

// Query callback representing the query of a chaincode
func (t *Template) Query(stub shim.ChaincodeStubInterface, invokeTime int64) ([]byte, error) {
	tmpltLogger.Debug("Enter Query")
	function, args := stub.GetFunctionAndParameters()
	tmpltLogger.Debug("func =%s, args = %+v", function, args)

	var queryTime = invokeTime
	tmpltLogger.Debug("queryTime= %+v", queryTime)

	if function == "queryXXX" {
		value, err := stub.GetState("XXX")
		if err != nil {
			return nil, fmt.Errorf("queryXXX: GetState failed, err=%s", err)
		}

		return value, nil

	} else if function == "queryYYY" {
		value, err := stub.GetState("YYY")
		if err != nil {
			return nil, fmt.Errorf("queryYYY: GetState failed, err=%s", err)
		}

		return (value), nil

	} else {
		return nil, fmt.Errorf("unknown function: %s", function)
	}
}

func (t *Template) newAccount(stub shim.ChaincodeStubInterface, accName string, accType int, userName, userIdHash, userPubKeyHash string, times int64, isCBAcc bool) ([]byte, error) {
	tmpltLogger.Debug("Enter openAccount")

	var err error
	var accExist bool

	accExist, err = t.isEntityExists(stub, accName)
	if err != nil {
		return nil, fmt.Errorf("isEntityExists (id=%s) failed. err=%s", accName, err)
	}

	if accExist {
		return nil, fmt.Errorf("account (id=%s) failed, already exists.", accName)
	}

	var ent AccountEntity

	ent.EntID = accName
	ent.RestAmount = 0
	ent.TotalAmount = 0
	ent.Time = times
	ent.Owner = userName
	ent.OwnerIdentityHash = userIdHash
	ent.OwnerPubKeyHash = userPubKeyHash

	err = t.setAccountEntity(stub, &ent)
	if err != nil {
		return nil, fmt.Errorf("openAccount setAccountEntity (id=%s) failed. err=%s", accName, err)
	}

	tmpltLogger.Debug("openAccount success: %+v", ent)

	return nil, nil
}

func (t *Template) setAccountEntity(stub shim.ChaincodeStubInterface, ent *AccountEntity) error {

	jsons, err := json.Marshal(ent)

	if err != nil {
		return fmt.Errorf("marshal ent failed. err=%s", err)
	}

	err = stub.PutState(ent.EntID, jsons)

	if err != nil {
		return fmt.Errorf("PutState ent failed. err=%s", err)
	}
	return nil
}

func (t *Template) isEntityExists(stub shim.ChaincodeStubInterface, entName string) (bool, error) {
	var entB []byte
	var err error

	entB, err = stub.GetState(entName)
	if err != nil {
		return false, err
	}

	if entB == nil {
		return false, nil
	}

	return true, nil
}

func (t *Template) getAccountEntity(stub shim.ChaincodeStubInterface, entName string) (*AccountEntity, error) {
	var entB []byte
	var cb AccountEntity
	var err error

	entB, err = stub.GetState(entName)
	if err != nil {
		return nil, err
	}

	if entB == nil {
		return nil, ErrNilEntity
	}

	if err = json.Unmarshal(entB, &cb); err != nil {
		return nil, fmt.Errorf("getAccountEntity: Unmarshal failed, err=%s.", err)
	}

	return &cb, nil
}

func verifyIdentity(stub shim.ChaincodeStubInterface, userName string, sign, signMsg []byte, ownerPubKeyHash, ownerIdentityHash string) error {

	if verifyIdentityHook != nil {
		return verifyIdentityHook(stub, userName, sign, signMsg, ownerPubKeyHash, ownerIdentityHash)
	}

	return nil
}

func main() {

	err := shim.Start(new(Template))
	if err != nil {
		tmpltLogger.Error("Error starting  chaincode: %s", err)
	}
}
