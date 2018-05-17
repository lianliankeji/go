package main

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math"
	"os"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
)

const (
	TRANS_PAY    = 0 //交易支出
	TRANS_INCOME = 1 //交易收入

	ATTR_USRROLE = "usrrole"
	ATTR_USRNAME = "usrname"
	ATTR_USRTYPE = "usrtype"

	/*********************************************/
	/****** 适配新模块请修改此常量，一般用模块名命名******/
	/*********************************************/
	EXTEND_MODULE_NAME = "acsys"

	//每个key都加上前缀，便于区分，也便于以后在线升级时处理方便
	TRANSSEQ_PREFIX      = "!" + EXTEND_MODULE_NAME + "@txSeqPre~"          //序列号生成器的key的前缀。使用的是worldState存储
	TRANSINFO_PREFIX     = "!" + EXTEND_MODULE_NAME + "@txInfoPre~"         //全局交易信息的key的前缀。使用的是worldState存储
	ONE_ACC_TRANS_PREFIX = "!" + EXTEND_MODULE_NAME + "@oneAccTxPre~"       //存储单个账户的交易的key前缀
	USR_ENTITY_PREFIX    = "!" + EXTEND_MODULE_NAME + "@usrEntPre~"         //存储某个用户的用户信息的key前缀。
	ACC_ENTITY_PREFIX    = "!" + EXTEND_MODULE_NAME + "@accEntPre~"         //存储某个账户的账户信息的key前缀。
	USR_INFOS_PREFIX     = "!" + EXTEND_MODULE_NAME + "@usrInfoPre~"        //存储某个用户的信息的key前缀。
	CENTERBANK_ACC_KEY   = "!" + EXTEND_MODULE_NAME + "@centerBankAccKey@!" //央行账户的key。使用的是worldState存储
	ALL_ACC_INFO_KEY     = "!" + EXTEND_MODULE_NAME + "@allAccInfoKey@!"    //存储所有账户名的key。使用的是worldState存储
	ACC_STATIC_INFO_KEY  = "!" + EXTEND_MODULE_NAME + "@accStatcInfoKey@!"  //存储所有账户统计信息的key。
	ACC_AMTLOCK_PREFIX   = "!" + EXTEND_MODULE_NAME + "@accAmtLockPre~"     //账户金额锁定key前缀
	APP_INFO_PREFIX      = "!" + EXTEND_MODULE_NAME + "@appInfoKeyPre~"     //应用信息

	WORLDSTATE_FILE_PREFIX = "/home/" + EXTEND_MODULE_NAME + "_worldstate_"

	MULTI_STRING_DELIM        = ':' //多个string的分隔符
	INVALID_MD5_VALUE         = "-"
	INVALID_PUBKEY_HASH_VALUE = "-"

	ACC_INVALID_CHAR_SET = ",;:/\\"                  //账户中不能包含的字符
	COIN_ISSUE_ACC_ENTID = "issueCoinVirtualAccount" //发行货币的账户id

	CONTROL_CC_NAME              = "sysctrlcc"
	CONTROL_CC_GETPARA_FUNC_NAME = "getParameter"

	CROSSCCCALL_PREFIX = "^_^~"
)

type UserEntity struct {
	EntID       string   `json:"id"`  //ID
	AuthAccList []string `json:"aal"` //此user被授权给了哪些账户
	AccList     []string `json:"al"`  //此user的自己的账户列表
}

//账户信息Entity
// 一系列ID（或账户）都定义为字符串类型。因为putStat函数的第一个参数为字符串类型，这些ID（或账户）都作为putStat的第一个参数；另外从SDK传过来的参数也都是字符串类型。
type AccountEntity struct {
	EntID             string              `json:"id"`   //银行/企业/项目/个人ID
	EntType           int                 `json:"etp"`  //类型 中央银行:1, 企业:2, 项目:3, 个人:4
	TotalAmount       int64               `json:"tamt"` //货币总数额(发行或接收)
	RestAmount        int64               `json:"ramt"` //账户余额
	Time              int64               `json:"time"` //开户时间
	Owner             string              `json:"own"`  //该实例所属的用户
	OwnerPubKeyHash   string              `json:"opbk"` //公钥hash
	OwnerIdentityHash string              `json:"oidt"` //身份hash
	AuthUserHashMap   map[string][]string `json:"auhm"` //授权用户的pubkey和indentity的hash
}

type UserInfo struct {
	EntID          string `json:"id"`
	ProfilePicture string `json:"pic"`
	Nickname       string `json:"nnm"`
}

type UserAttrs struct {
	UserRole string `json:"role"`
	UserName string `json:"name"`
	UserType string `json:"type"`
}

//查询的交易记录结果格式
type QueryTransResult struct {
	NextSerial   int64            `json:"nextser"` //因为是批量返回结果，表示下次要请求的序列号
	MaxSerial    int64            `json:"maxser"`
	TransRecords []QueryTransRecd `json:"records"`
}

//供查询的交易记录内容
type QueryTransRecd struct {
	Serial int64 `json:"ser"` //交易序列号，返回给查询结果用，储存时
	PubTrans
}

type AppInfo struct {
	AppID       string `json:"app"`
	Description string `json:"desc"`
	Company     string `json:"corp"`
	Creater     string `json:"crtr"`
}

type PubTrans struct {
	FromID       string `json:"fid"`  //发送方ID
	TransFlag    int    `json:"tsf"`  //交易标志，收入还是支出
	Amount       int64  `json:"amt"`  //交易数额
	ToID         string `json:"tid"`  //接收方ID
	TransType    string `json:"tstp"` //交易类型，前端传入，透传
	Description  string `json:"desc"` //交易描述
	TxID         string `json:"txid"` //交易ID
	Time         int64  `json:"time"` //交易时间
	GlobalSerial int64  `json:"gser"` //全局交易序列号
	AppID        string `json:"app"`  //应用ID  目前一条链一个账户体系，但是可能会有多种应用，所以交易信息记录一下应用id，可以按应用来过滤交易信息
}

//交易内容  注意，Transaction中的字段名（包括json字段名）不能和PubTrans中的字段名重复，否则解析会出问题
type Transaction struct {
	PubTrans
	FromType int    `json:"ftp"`  //发送方角色 centerBank:1, 企业:2, 项目:3
	ToType   int    `json:"ttp"`  //接收方角色 企业:2, 项目:3
	TransLvl uint64 `json:"tlvl"` //交易级别
}

//查询的对账信息
type QueryBalance struct {
	IssueAmount  int64  `json:"issueAmount"`  //市面上发行货币的总量
	AccCount     int64  `json:"accCount"`     //所有账户的总量
	AccSumAmount int64  `json:"accSumAmount"` //所有账户的货币的总量
	Message      string `json:"message"`      //对账附件信息
}

type AccountStatisticInfo struct {
	AccountCount int64 `json:"ac"`
}

//账户金额锁定期   因为只有少数的账户有锁定期，所以这些配置不放在账户结构体中
type CoinLockCfg struct {
	LockEndTime int64 `json:"let"`
	LockAmount  int64 `json:"la"`
}
type AccountCoinLockInfo struct {
	AccName  string        `json:"acc"`
	LockList []CoinLockCfg `json:"ll"`
}

type QueryBalanceAndLocked struct {
	Balance      int64         `json:"balance"`
	LockedAmount int64         `json:"lockedamt"`
	LockCfg      []CoinLockCfg `json:"lockcfg"`
}

type TopNData struct {
	Ranking            int    `json:"rank"`
	UserProfilePicture string `json:"picture"`
	UserNickname       string `json:"nickname"`
	AccountName        string `json:"acc"`
	RestAmount         int64  `json:"restamt"`
}

type QueryAccAmtRankAndTopN struct {
	AccoutName string     `json:"acc"`
	RestAmount int64      `json:"restamt"`
	Ranking    string     `json:"rank"`
	TopN       []TopNData `json:"topn"`
}

type BaseInitArgs struct {
	FixedArgCount int
	InitTime      int64
}
type BaseInvokeArgs struct {
	FixedArgCount int
	UserName      string
	AccountName   string
	InvokeTime    int64
	AccountEnt    *AccountEntity
}

//扩展包的回调函数
var InitHook func(shim.ChaincodeStubInterface, *BaseInitArgs) ([]byte, *ErrorCodeMsg)
var InvokeHook func(shim.ChaincodeStubInterface, *BaseInvokeArgs) ([]byte, *ErrorCodeMsg)
var DateConvertWhenLoadHook func(stub shim.ChaincodeStubInterface, srcCcid, key string, valueB []byte) (string, []byte, *ErrorCodeMsg)
var DateUpdateAfterLoadHook func(stub shim.ChaincodeStubInterface, srcCcid string) *ErrorCodeMsg

var baselogger = NewMylogger(EXTEND_MODULE_NAME + "base")
var baseCrypto = MyCryptoNew()

var ErrcmNilEntity = NewErrorCodeMsg(ERRCODE_COMMON_INNER_ERROR, "nil entity.")
var ErrcmUnregistedFun = NewErrorCodeMsg(ERRCODE_COMMON_INNER_ERROR, "unregisted function.")

var stateCache StateWorldCache

type BASE struct {
}

var Base BASE

//包初始化函数
func init() {
	baselogger.SetDefaultLvl(shim.LogDebug)
}

func (b *BASE) Init(stub shim.ChaincodeStubInterface) (pbResponse pb.Response) {
	function, _ := stub.GetFunctionAndParameters()
	defer func() {
		if excption := recover(); excption != nil {
			pbResponse = shim.Error(baselogger.SError("Init(%s) got an unexpect error:%s", function, excption))
			baselogger.Critical("Init got exception, stack:\n%s", string(debug.Stack()))
		}
	}()

	payload, errcm := b.__Init(stub)
	if errcm != nil {
		return shim.Error(errcm.toJson())
	}

	return shim.Success(payload)
}

func (b *BASE) __Init(stub shim.ChaincodeStubInterface) ([]byte, *ErrorCodeMsg) {
	baselogger.Debug("Enter Init")
	function, args := stub.GetFunctionAndParameters()
	baselogger.Info("func =%s, args = %+v", function, args)

	stateCache.Create(stub)
	defer func() {
		stateCache.Destroy(stub)
	}()

	LogCtrlParameter(baselogger)

	/*
		//base中这里目前只用一个参数，扩展模块中如果需要更多参数，请自行检查
		var fixedArgCount = 1
		if len(args) < fixedArgCount {
			return shim.Error(baselogger.SError("Init miss arg, got %d, at least need %d(initTime).", len(args), fixedArgCount))
		}
	*/
	timestamp, err := stub.GetTxTimestamp()
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Init: GetTxTimestamp failed, error=(%s)", err)
	}

	var initTime = timestamp.Seconds*1000 + int64(timestamp.Nanos/1000000) //精确到毫秒

	var initFixArgs BaseInitArgs
	initFixArgs.FixedArgCount = 0
	initFixArgs.InitTime = initTime

	if function == "init" { //合约实例化时，默认会执行init函数，除非在调用合约实例化接口时指定了其它的函数
		baselogger.Debug("enter init")
		//do someting

		//虚拟一个超级账户，设置货币发行总额，给央行发行货币。
		errcm := b.setIssueAmountTotal(stub, 10000000000, initTime)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "Init setIssueAmountTotal error, error=(%s).", errcm)
		}

		if InitHook == nil {
			return nil, nil
		}

	} else if function == "upgrade" { //升级时默认会执行upgrade函数，除非在调用合约升级接口时指定了其它的函数
		baselogger.Debug("enter upgrade")
		//do someting

		if InitHook == nil {
			return nil, nil
		}
	}

	//这个判断不能放在上面的else分支， 因为执行了base的init，还需要执行InitHook里的init
	if InitHook != nil {
		retBytes, errcm := InitHook(stub, &initFixArgs)
		if errcm != nil {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "InitHook failed, error=(%s).", errcm)
		}
		return retBytes, nil
	}

	return nil, nil
}

var sysFunc = []string{"account", "transefer", "transefer3", "registerApp", "updateUserInfo", "recharge",
	"getBalance", "getBalanceAndLocked", "getTransInfo", "isAccExists", "getAppInfo", "getStatisticInfo", "getRankingAndTopN", "getUserInfo"}

// Transaction makes payment of X units from A to B
func (b *BASE) Invoke(stub shim.ChaincodeStubInterface) (pbResponse pb.Response) {
	function, _ := stub.GetFunctionAndParameters()
	defer func() {
		if excption := recover(); excption != nil {
			pbResponse = shim.Error(baselogger.SError("Invoke(%s) got an unexpect error:%s", function, excption))
			baselogger.Critical("Invoke got exception, stack:\n%s", string(debug.Stack()))
		}
	}()

	payload, errcm := b.__Invoke(stub)
	if errcm != nil {
		return shim.Error(errcm.toJson())
	}

	return shim.Success(payload)
}

func (b *BASE) __Invoke(stub shim.ChaincodeStubInterface) ([]byte, *ErrorCodeMsg) {
	function, args := stub.GetFunctionAndParameters()

	baselogger.Debug("Enter Invoke")
	baselogger.Debug("func =%s, args = %+v", function, args)

	stateCache.Create(stub)
	defer func() {
		stateCache.Destroy(stub)
	}()

	var err error
	var errcm *ErrorCodeMsg

	var crossCallChaincodeName = ""
	var crossCallFlag = args[len(args)-1]
	if b.isCrossChaincodeCallFlag(crossCallFlag) {
		crossCallChaincodeName = b.getCrossChaincodeName(crossCallFlag)
		//去掉最后一个参数，该参数是自动添加用来区分是不是跨合约调用的
		args = args[:len(args)-1]
		baselogger.Debug("func =%s, args = %+v", function, args)
	}

	var fixedArgCount = 2
	//最后一个参数为签名，所以参数必须大于fixedArgCount个
	if len(args) < fixedArgCount {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Invoke miss arg, got %d, at least need %d(use, acc).", len(args), fixedArgCount)
	}

	var userName = args[0]
	var accName = args[1]
	timestamp, err := stub.GetTxTimestamp()
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Init: GetTxTimestamp failed, error=(%s)", err)
	}

	var invokeTime = timestamp.Seconds*1000 + int64(timestamp.Nanos/1000000) //精确到毫秒

	var invokeFixArgs BaseInvokeArgs
	invokeFixArgs.FixedArgCount = fixedArgCount
	invokeFixArgs.UserName = userName
	invokeFixArgs.AccountName = accName
	invokeFixArgs.InvokeTime = invokeTime

	var accountEnt *AccountEntity = nil

	var sign, signMsg []byte

	//开户时验证在客户函数中做
	if function != "account" && function != "accountCB" {

		accountEnt, errcm = b.getAccountEntity(stub, accName)
		if errcm != nil {
			//查询函数和invoke函数走的一个入口， 所以查询接口的几个特殊处理放在这里
			if errcm == ErrcmNilEntity {
				if function == "isAccExists" { //如果是查询账户是否存在，如果是空，返回不存在
					return []byte("0"), nil
				} else if function == "getBalance" { //如果是查询余额，如果账户不存，返回0
					return []byte("0"), nil
				} else if function == "transPreCheck" { //如果是转账预检查，返回付款账户不存在
					return []byte(strconv.FormatInt(ERRCODE_TRANS_PAY_ACCOUNT_NOT_EXIST, 10)), nil
				}
			}
			return nil, baselogger.ErrorECM(errcm.Code, "Invoke getAccountEntity(%s) failed. error=(%s)", accName, errcm)
		}

		//非account时签名为最后一个参数
		sign, signMsg, errcm = b.getSignAndMsg(function, args, len(args)-1)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "Invoke: getSignAndMsg(%s) failed, error=(%s).", accName, errcm)
		}

		//校验修改Entity的用户身份，只有Entity的所有者才能修改自己的Entity
		if errcm = b.verifyIdentity(stub, userName, sign, signMsg, accountEnt, "", ""); errcm != nil {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_IDENTITY_VERIFY_FAILED, "Invoke: verifyIdentity(%s) failed.", accName)
		}

		//去除签名参数
		args = args[:len(args)-1]
	}

	invokeFixArgs.AccountEnt = accountEnt

	if len(crossCallChaincodeName) > 0 && !b.isAccountSysFunc(function) {
		var calledArgs = stub.GetArgs()
		//这里获取的是原始参数，所以要去掉后两个参数，最后一个参数是自动添加用来区分是不是跨合约调用的，倒数第二个为签名
		payload, errcm := b.corssChaincodeCall(stub, calledArgs[:len(calledArgs)-2], crossCallChaincodeName, userName, accName, sign, signMsg)
		if errcm != nil {
			baselogger.Error("Invoke: invoke chaincode '%s' failed, error=(%s)", crossCallChaincodeName, errcm)
			return nil, errcm //直接返回被调用者的错误信息
		}

		return payload, nil
	}

	if function == "account" {
		baselogger.Debug("Enter account")
		var usrType int

		//args:[usrname, accname, pubkey,..., signature, userIdentity]
		//因为userIdentity是平台自动添加的，而签名是在客户端做的，所以把userIdentity放在最后

		var argCount = fixedArgCount + 3
		if len(args) < argCount {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(account) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		//先取出固定参数 signature 和 userIdentity，因为这两个参数是平台自动加入args中的，所以一定有
		var userIdHash = args[len(args)-1] //base64

		//签名为倒数第二个参数
		sig, msg, errcm := b.getSignAndMsg(function, args, len(args)-2)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "Invoke(account): getSignAndMsg(%s) failed, error=(%s).", accName, errcm)
		}

		//然后去掉最后两个参数 signature 和 userIdentity ， 方便后续的可选参数处理
		args = args[:len(args)-2]
		argCount -= 2

		var userPubKeyHash = args[fixedArgCount] //base64

		//校验修改Entity的用户身份，只有Entity的所有者才能修改自己的Entity
		if errcm = b.verifyIdentity(stub, userName, sig, msg, nil, userPubKeyHash, userIdHash); errcm != nil {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_IDENTITY_VERIFY_FAILED, "Invoke(account) verifyIdentity(%s) failed.", accName)
		}

		_, errcm = b.newAccount(stub, accName, usrType, userName, userIdHash, userPubKeyHash, invokeTime, false)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "Invoke(account) newAccount failed. error=(%s)", errcm)
		}

		//可选参数
		var userPicture string
		var userNickname string
		if len(args) > argCount {
			userPicture = args[argCount]
		}
		if len(args) > argCount+1 {
			userNickname = args[argCount+1]
		}

		if len(userPicture) > 0 || len(userNickname) > 0 {
			errcm = b.updateUserInfo(stub, userName, userPicture, userNickname)
			if errcm != nil {
				return nil, baselogger.ErrorECM(errcm.Code, "Invoke(updateUserInfo) updateUserInfo failed, error=(%s).", errcm)
			}
		}

		return nil, nil

	} else if function == "accountCB" {
		baselogger.Debug("Enter accountCB")
		var usrType int = 0

		//args:[usrname, accname, pubkey, signature, userIdentity]
		//因为userIdentity是平台自动添加的，而签名是在客户端做的，所以把userIdentity放在最后

		var argCount = fixedArgCount + 3
		if len(args) < argCount {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(account) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		var userPubKeyHash = args[fixedArgCount] //base64
		var userIdHash = args[len(args)-1]       //base64

		//签名为倒数第二个参数
		sig, msg, errcm := b.getSignAndMsg(function, args, len(args)-2)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "Invoke(account): getSignAndMsg(%s) failed, error=(%s).", accName, errcm)
		}

		//校验修改Entity的用户身份，只有Entity的所有者才能修改自己的Entity
		if errcm = b.verifyIdentity(stub, userName, sig, msg, nil, userPubKeyHash, userIdHash); errcm != nil {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_IDENTITY_VERIFY_FAILED, "Invoke(accountCB) verifyIdentity(%s) failed.", accName)
		}

		tmpByte, errcm := b.getCenterBankAcc(stub)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "Invoke(accountCB) getCenterBankAcc failed. error=(%s)", errcm)
		}

		//如果央行账户已存在，报错
		if tmpByte != nil {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "Invoke(accountCB) CBaccount(%s) exists.", string(tmpByte))
		}

		_, errcm = b.newAccount(stub, accName, usrType, userName, userIdHash, userPubKeyHash, invokeTime, true)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "Invoke(accountCB) openAccount failed. error=(%s)", errcm)
		}

		errcm = b.setCenterBankAcc(stub, accName)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "Invoke(accountCB) setCenterBankAcc failed. error=(%s)", errcm)
		}

		return nil, nil

	} else if function == "issue" {
		baselogger.Debug("Enter issue")

		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(issue) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		var issueAmount int64
		issueAmount, err = strconv.ParseInt(args[fixedArgCount], 0, 64)
		if err != nil {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Invoke(issue) convert issueAmount(%s) failed. error=(%s)", args[fixedArgCount], err)
		}
		baselogger.Debug("issueAmount= %+v", issueAmount)

		tmpByte, errcm := b.getCenterBankAcc(stub)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "Invoke(issue) getCenterBankAcc failed. error=(%s)", errcm)
		}
		//如果没有央行账户，报错。否则校验账户是否一致。
		if tmpByte == nil {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "Invoke(issue) getCenterBankAcc nil.")
		} else {
			if accName != string(tmpByte) {
				return nil, baselogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "Invoke(issue) centerBank account is %s, can't issue to %s.", string(tmpByte), accName)
			}
		}

		_, errcm = b.issueCoin(stub, accName, issueAmount, invokeTime)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "Invoke(issue) issueCoin failed. error=(%s)", errcm)
		}
		return nil, nil

	} else if function == "transefer" {
		var argCount = fixedArgCount + 3
		if len(args) < argCount {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(transefer) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		var toAcc = args[fixedArgCount]

		var transAmount int64
		transAmount, err = strconv.ParseInt(args[fixedArgCount+1], 0, 64)
		if err != nil {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Invoke(transefer): convert issueAmount(%s) failed. error=(%s)", args[fixedArgCount+1], err)
		}
		baselogger.Debug("transAmount= %+v", transAmount)

		var appid = args[fixedArgCount+2]

		//以下为可选参数
		var description string
		var transType string
		var sameEntSaveTransFlag bool = true

		if len(args) > argCount {
			description = args[argCount]
		}
		if len(args) > argCount+1 {
			transType = args[argCount+1]
		}
		if len(args) > argCount+2 {
			var sameEntSaveTrans = args[argCount+2] //如果转出和转入账户相同，是否记录交易 0表示不记录 1表示记录
			if sameEntSaveTrans != "1" {
				sameEntSaveTransFlag = false
			}
		}

		//测试链不检查 appid 是否存在
		if !Ctrl_isTestChain {
			idExist, errcm := b.isAppExists(stub, appid)
			if errcm != nil {
				return nil, baselogger.ErrorECM(errcm.Code, "Invoke(transefer) failed, error=(%s).", errcm)
			}
			if !idExist {
				return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(transefer) appid(%s) not exist, please register it first.", appid)
			}
		}

		_, errcm = b.transferCoin(stub, accName, toAcc, transType, description, transAmount, invokeTime, sameEntSaveTransFlag, appid)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "Invoke(transefer) transferCoin failed. error=(%s)", errcm)
		}
		return nil, nil

	} else if function == "transefer3" { //带锁定期功能
		var argCount = fixedArgCount + 4
		if len(args) < argCount {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(transeferLockAmt) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		var toAcc = args[fixedArgCount]

		var transAmount int64
		transAmount, err = strconv.ParseInt(args[fixedArgCount+1], 0, 64)
		if err != nil {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Invoke(transeferLockAmt): convert issueAmount(%s) failed. error=(%s)", args[fixedArgCount+1], err)
		}

		var lockCfgs = args[fixedArgCount+2]
		var appid = args[fixedArgCount+3]

		//以下为可选参数
		var description string
		var transType string
		var sameEntSaveTransFlag bool = true

		if len(args) > argCount {
			description = args[argCount]
		}
		if len(args) > argCount+1 {
			transType = args[argCount+1]
		}
		if len(args) > argCount+2 {
			var sameEntSaveTrans = args[argCount+2] //如果转出和转入账户相同，是否记录交易 0表示不记录 1表示记录
			if sameEntSaveTrans != "1" {
				sameEntSaveTransFlag = false
			}
		}

		var lockedThistime int64 = 0
		lockedThistime, _, errcm = b.setAccountLockAmountCfg(stub, toAcc, lockCfgs, false)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "Invoke(transeferLockAmt): setAccountLockAmountCfg failed. error=(%s)", errcm)
		}

		if lockedThistime > transAmount {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "Invoke(transeferLockAmt): lockAmt(%d) > transAmount(%d).", lockedThistime, transAmount)
		}

		//测试链不检查 appid 是否存在
		if !Ctrl_isTestChain {
			idExist, errcm := b.isAppExists(stub, appid)
			if errcm != nil {
				return nil, baselogger.ErrorECM(errcm.Code, "Invoke(transeferLockAmt) failed, error=(%s).", errcm)
			}
			if !idExist {
				return nil, baselogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "Invoke(transeferLockAmt) appid(%s) not exist, please register it first.", appid)
			}
		}

		_, errcm = b.transferCoin(stub, accName, toAcc, transType, description, transAmount, invokeTime, sameEntSaveTransFlag, appid)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "Invoke(transeferLockAmt) transferCoin3 failed. error=(%s)", errcm)
		}
		return nil, nil

	} else if function == "updateEnv" {
		//更新环境变量
		if !b.isAdmin(stub, accName) {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "Invoke(updateEnv) can't exec updateEnv by %s.", accName)
		}

		var argCount = fixedArgCount + 2
		if len(args) < argCount {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(updateEnv) miss arg, got %d, at least need %d.", len(args), argCount)
		}
		key := args[fixedArgCount]
		value := args[fixedArgCount+1]

		if key == "logLevel" {
			lvl, _ := strconv.Atoi(value)
			// debug=5, info=4, notice=3, warning=2, error=1, critical=0
			var loggerSet = baselogger.GetLoggers()
			for _, l := range loggerSet {
				l.SetDefaultLvl(shim.LoggingLevel(lvl))
			}

			baselogger.Info("set logLevel to %d.", lvl)
		}

		return nil, nil

	} else if function == "updateState" {
		if !b.isAdmin(stub, accName) {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "Invoke(setWorldState) can't exec by %s.", accName)
		}

		var argCount = fixedArgCount + 4
		if len(args) < argCount {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "setWorldState miss arg, got %d, need %d.", len(args), argCount)
		}

		var fileName = args[fixedArgCount]
		var needHash = false
		if args[fixedArgCount+1] == "1" {
			needHash = true
		}
		var sameKeyOverwrite = false //相同的key是否覆盖
		if args[fixedArgCount+2] == "1" {
			sameKeyOverwrite = true
		}

		var srcCcid = args[fixedArgCount+3]

		_, errcm = b.loadWorldState(stub, fileName, needHash, sameKeyOverwrite, srcCcid)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "Invoke(setWorldState) setWorldState failed. error=(%s)", errcm)
		}
		return nil, nil

	} else if function == "lockAccAmt" {
		if !b.isAdmin(stub, accName) {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "Invoke(lockAccAmt) can't exec by %s.", accName)
		}

		var argCount = fixedArgCount + 4
		if len(args) < argCount {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(lockAccAmt) miss arg, got %d, need %d.", len(args), argCount)
		}

		var lockedAccName = args[fixedArgCount]
		var lockCfgs = args[fixedArgCount+1]

		var overwriteOld = false //是否覆盖已有记录
		if args[fixedArgCount+2] == "1" {
			overwriteOld = true
		}

		var canLockMoreThanRest = false //是否可以锁定比剩余额度多的额度
		if args[fixedArgCount+3] == "1" {
			canLockMoreThanRest = true
		}

		lockEnt, errcm := b.getAccountEntity(stub, lockedAccName)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "Invoke(lockAccAmt): getAccountEntity failed. error=(%s)", errcm)
		}

		_, lockedTotal, errcm := b.setAccountLockAmountCfg(stub, lockedAccName, lockCfgs, overwriteOld)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "Invoke(lockAccAmt): setAccountLockAmountCfg failed. error=(%s)", errcm)
		}

		if !canLockMoreThanRest && lockedTotal > lockEnt.RestAmount {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "Invoke(lockAccAmt): lock amount > account rest(%d,%d).", lockedTotal, lockEnt.RestAmount)
		}

		return nil, nil

	} else if function == "registerApp" {
		var argCount = fixedArgCount + 3
		if len(args) < argCount {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(RegisterApp) miss arg, got %d, need %d.", len(args), argCount)
		}

		var ai AppInfo
		ai.AppID = args[fixedArgCount]
		ai.Description = args[fixedArgCount+1]
		ai.Company = args[fixedArgCount+2]
		ai.Creater = accName

		if len(strings.TrimSpace(ai.AppID)) == 0 {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(RegisterApp) appid is empty.")
		}

		ok, errcm := b.isAppExists(stub, ai.AppID)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "Invoke(RegisterApp) isAppExists error=(%s).", errcm)
		}
		if ok {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "Invoke(RegisterApp) appid(%s) exists.", ai.AppID)
		}

		errcm = b.setAppInfo(stub, &ai)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "Invoke(RegisterApp) setAppInfo error=(%s).", errcm)
		}

		return nil, nil

	} else if function == "recharge" { //测试链才有
		if Ctrl_isTestChain {
			var argCount = fixedArgCount + 1
			if len(args) < argCount {
				return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(lockAccAmt) miss arg, got %d, need %d.", len(args), argCount)
			}

			var rechargeAmount int64
			rechargeAmount, err = strconv.ParseInt(args[fixedArgCount], 0, 64)
			if err != nil {
				return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Invoke(recharge): convert rechargeAmount(%s) failed. error=(%s)", args[fixedArgCount], err)
			}

			accountEnt.TotalAmount = rechargeAmount
			accountEnt.RestAmount = rechargeAmount

			errcm = b.setAccountEntity(stub, accountEnt)
			if errcm != nil {
				return nil, baselogger.ErrorECM(errcm.Code, "Invoke(recharge): save account failed. error=(%s)", errcm)
			}

			return nil, nil
		} else {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "Invoke(recharge): can not run this function.")
		}

	} else if function == "authAccountManager" { //授权本账户的其它管理者， 即多个用户可以操作同一个账户
		var argCount = fixedArgCount + 2
		if len(args) < argCount {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(authAccountManager) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		var manager = args[fixedArgCount+0]
		var operate = args[fixedArgCount+1] // add or delete

		var addFlag = false
		if operate == "add" {
			addFlag = true
		} else if operate != "delete" {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(authAccountManager) operate must be 'add' or 'delete'.")
		}

		//manager获取失败，报错
		managerEnt, errcm := b.getUserEntity(stub, manager)
		if errcm != nil && errcm != ErrcmNilEntity {
			return nil, baselogger.ErrorECM(errcm.Code, "Invoke(authAccountManager) getUserEntity failed. error=(%s), entname=%s", errcm, manager)
		}

		if addFlag {
			//添加时，manager 必存在
			if managerEnt == nil {
				return nil, baselogger.ErrorECM(errcm.Code, "Invoke(authAccountManager) manager(%s) not exists. ", manager)
			}

			//获取manager 的hash值 （从账户中取）
			var mngAccName = managerEnt.AccList[0]
			mngAccEnt, errcm := b.getAccountEntity(stub, mngAccName)
			if errcm != nil {
				return nil, baselogger.ErrorECM(errcm.Code, "Invoke(authAccountManager) getAccountEntity failed. error=(%s), entname=%s", errcm, mngAccName)
			}

			//在当前账户中加入新的用户
			if accountEnt.AuthUserHashMap == nil {
				accountEnt.AuthUserHashMap = make(map[string][]string)
			}
			//第一个元素为身份hash，第二个为pubkey的hash
			accountEnt.AuthUserHashMap[manager] = []string{mngAccEnt.OwnerIdentityHash, mngAccEnt.OwnerPubKeyHash}

			if !strSliceContains(managerEnt.AuthAccList, accName) {
				managerEnt.AuthAccList = append(managerEnt.AuthAccList, accName)
			}

		} else {
			delete(accountEnt.AuthUserHashMap, manager)
			//删除时，如果manager 不存在则不处理
			if managerEnt == nil {
				baselogger.Warn("manager(%s) not exists.", manager)
			} else {
				managerEnt.AuthAccList = strSliceDelete(managerEnt.AuthAccList, accName)
			}
		}

		baselogger.Debug("Invoke(authAccountManager) AccountEntity before =%+v.", *accountEnt)

		errcm = b.setAccountEntity(stub, accountEnt)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "Invoke(authAccountManager) setAccountEntity failed. error=(%s), entname=%s", errcm, accountEnt.EntID)
		}

		baselogger.Debug("Invoke(authAccountManager) AccountEntity  after =%+v.", *accountEnt)

		baselogger.Debug("Invoke(authAccountManager):  UserEntity before %+v", *managerEnt)

		errcm = b.setUserEntity(stub, managerEnt)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "Invoke(authAccountManager) setUserEntity failed. error=(%s), entname=%s", errcm, accountEnt.EntID)
		}
		baselogger.Debug("Invoke(authAccountManager):  UserEntity after %+v", *managerEnt)

		return nil, nil
	} else if function == "updateUserInfo" {
		var argCount = fixedArgCount + 2
		if len(args) < argCount {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(updateUserInfo) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		var picture = args[fixedArgCount+0]
		var nickname = args[fixedArgCount+1]

		errcm = b.updateUserInfo(stub, userName, picture, nickname)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "Invoke(updateUserInfo) updateUserInfo failed, error=(%s).", errcm)
		}

		return nil, nil
	} else if function == "deleteState" {
		if Ctrl_isTestChain {
			var argCount = fixedArgCount + 1
			if len(args) < argCount {
				return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(deleteState) miss arg, got %d, at least need %d.", len(args), argCount)
			}

			var key = args[fixedArgCount]

			var keyType string //可选参数
			if len(args) > argCount {
				keyType = args[argCount]
			}

			if key == "*" {
				keysIter, err := stub.GetStateByRange("", "")
				if err != nil {
					return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Invoke(deleteState): keys operation failed. Error accessing state: %s", err)
				}
				defer keysIter.Close()

				for keysIter.HasNext() {
					kv, iterErr := keysIter.Next()
					if iterErr != nil {
						baselogger.Error("Invoke(deleteState): getNext failed, %s", iterErr)
						continue
					}
					stub.DelState(kv.GetKey())
				}
			} else {
				if keyType == "account" {
					key = b.getAccountEntityKey(key)
				}
				err = stub.DelState(key)
				if err != nil {
					return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Invoke(deleteState) DelState failed, error=(%s).", err)
				}
			}
		} else {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "Invoke(deleteState) can not run deleteState here.")
		}

		return nil, nil
	} else {

		retValue, errcm := b.__Query(stub, &invokeFixArgs, function, args)

		if errcm != nil {
			//如果是因为没找到处理函数，尝试在扩展模块中查找
			if errcm == ErrcmUnregistedFun {
				//如果没有扩展处理模块，直接返回错误
				if InvokeHook == nil {
					return nil, baselogger.ErrorECM(errcm.Code, "unknown function:%s.", function)
				}

				retBytes, errcm := InvokeHook(stub, &invokeFixArgs)
				if errcm != nil {
					return nil, errcm
				}

				return retBytes, nil
			}

			return nil, errcm
		}

		return retValue, nil
	}
}

// Query callback representing the query of a chaincode
func (b *BASE) __Query(stub shim.ChaincodeStubInterface, ifas *BaseInvokeArgs, function string, args []string) ([]byte, *ErrorCodeMsg) {
	baselogger.Debug("Enter Query")
	baselogger.Debug("func =%s, args = %+v", function, args)

	var err error
	var errcm *ErrorCodeMsg

	var fixedArgCount = ifas.FixedArgCount
	var userName = ifas.UserName
	var accName = ifas.AccountName
	var queryTime = ifas.InvokeTime

	var accountEnt = ifas.AccountEnt

	if accountEnt == nil {
		accountEnt, errcm = b.getAccountEntity(stub, accName)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "Query getAccountEntity failed. error=(%s)", errcm)
		}

		ifas.AccountEnt = accountEnt
	}

	//校验用户身份
	//if ok, _ := b.verifyIdentity(stub, userName, accountEnt); !ok {
	//	return nil, baselogger.ErrorECM(ERRCODE_COMMON_IDENTITY_VERIFY_FAILED,"Query user and account check failed."))
	//}

	if function == "getBalance" { //查询余额

		var queryEntity *AccountEntity = accountEnt

		baselogger.Debug("queryEntity=%+v", queryEntity)

		retValue := []byte(strconv.FormatInt(queryEntity.RestAmount, 10))

		return (retValue), nil

	} else if function == "getBalanceAndLocked" { //查询余额及锁定金额
		var qbal QueryBalanceAndLocked

		var queryEntity *AccountEntity = accountEnt
		baselogger.Debug("queryEntity=%+v", queryEntity)

		qbal.Balance = queryEntity.RestAmount

		qbal.LockedAmount, qbal.LockCfg, errcm = b.getAccountLockedAmount(stub, accName, queryTime)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "getBalanceAndLocked: getAccountLockedAmount(id=%s) failed. error=(%s)", accName, errcm)
		}

		if qbal.LockCfg == nil {
			qbal.LockCfg = []CoinLockCfg{} //初始化为空，即使没查到数据也会返回'[]'
		}

		qbalB, err := json.Marshal(qbal)
		if err != nil {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getBalanceAndLocked: Marshal(id=%s) failed. error=(%s)", accName, err)
		}

		return (qbalB), nil

	} else if function == "getTransInfo" { //查询交易记录
		var argCount = fixedArgCount + 3
		if len(args) < argCount {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "queryTx miss arg, got %d, need %d.", len(args), argCount)
		}

		var appid string
		var begSeq int64
		var txCount int64

		begSeq, err = strconv.ParseInt(args[fixedArgCount], 0, 64)
		if err != nil {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "queryTx ParseInt for begSeq(%s) failed. error=(%s)", args[fixedArgCount], err)
		}
		txCount, err = strconv.ParseInt(args[fixedArgCount+1], 0, 64)
		if err != nil {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "queryTx ParseInt for endSeq(%s) failed. error=(%s)", args[fixedArgCount+1], err)
		}

		appid = args[fixedArgCount+2]

		var txAcc string //查询指定用户
		var transLvl uint64 = 2

		var begTime int64 = 0
		var endTime int64 = -1
		var queryOrder string = "desc" //升序 降序
		//本次查询的最大序列号，如果倒序查询时， 比如是从最新的第10条开始查的，查询过程中，又产生了新的记录，最新的为11条，那么第二次查询时，就会从第11条倒序查询，返回重复数据。
		//在第一次查询返回时，会返回此参数，如果前端后续查询时，把这个参数带下来，还从第10条开始查就不会出这样的问题了。
		var queryMaxSeq int64 = -1

		if len(args) > argCount {
			txAcc = args[argCount]
		}
		if len(args) > argCount+1 {
			transLvl, err = strconv.ParseUint(args[argCount+1], 0, 64)
			if err != nil {
				return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "queryTx ParseInt for transLvl(%s) failed. error=(%s)", args[argCount+1], err)
			}
		}

		if len(args) > argCount+2 {
			begTime, err = strconv.ParseInt(args[argCount+2], 0, 64)
			if err != nil {
				return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "queryTx ParseInt for begTime(%s) failed. error=(%s)", args[argCount+2], err)
			}
		}
		if len(args) > argCount+3 {
			endTime, err = strconv.ParseInt(args[argCount+3], 0, 64)
			if err != nil {
				return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "queryTx ParseInt for endTime(%s) failed. error=(%s)", args[argCount+3], err)
			}
		}
		if len(args) > argCount+4 {
			queryOrder = args[argCount+4]
		}
		if len(args) > argCount+5 {
			queryMaxSeq, err = strconv.ParseInt(args[argCount+5], 0, 64)
			if err != nil {
				return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "queryTx ParseInt for queryMaxSeq(%s) failed. error=(%s)", args[argCount+5], err)
			}
		}

		var isAsc = false
		if queryOrder == "asc" {
			isAsc = true
		}

		if b.isAdmin(stub, accName) {
			//管理员账户时，如果不传入txAcc，则查询所有交易记录；否则查询指定账户交易记录
			//transLvl 只能管理员有权设置
			if len(txAcc) == 0 {
				retValue, errcm := b.queryTransInfos(stub, transLvl, begSeq, txCount, begTime, endTime, queryMaxSeq, isAsc, appid)
				if errcm != nil {
					return nil, baselogger.ErrorECM(errcm.Code, "queryTx queryTransInfos failed. error=(%s)", errcm)
				}
				return (retValue), nil
			} else {
				retValue, errcm := b.queryAccTransInfos(stub, txAcc, begSeq, txCount, begTime, endTime, queryMaxSeq, isAsc, appid)
				if errcm != nil {
					return nil, baselogger.ErrorECM(errcm.Code, "queryTx queryAccTransInfos failed. error=(%s)", errcm)
				}
				return (retValue), nil
			}
		} else {
			//非管理员账户，只能查询自己的交易记录，忽略txAcc参数
			retValue, errcm := b.queryAccTransInfos(stub, accName, begSeq, txCount, begTime, endTime, queryMaxSeq, isAsc, appid)
			if errcm != nil {
				return nil, baselogger.ErrorECM(errcm.Code, "queryTx queryAccTransInfos2 failed. error=(%s)", errcm)
			}
			return (retValue), nil
		}

		return (nil), nil

	} else if function == "getAllAccAmt" { //所有账户中钱是否正确
		//是否是管理员帐户，管理员用户才可以查
		if !b.isAdmin(stub, accName) {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "%s can't query balance.", accName)
		}

		retValue, errcm := b.getAllAccAmt(stub)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "getAllAccAmt failed. error=(%s)", errcm)
		}
		return (retValue), nil

	} else if function == "queryState" { //某个state的值
		//是否是管理员帐户，管理员用户才可以查
		if !b.isAdmin(stub, accName) {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "%s can't query state.", accName)
		}

		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "queryState miss arg, got %d, need %d.", len(args), argCount)
		}

		key := args[fixedArgCount]

		retValue, err := stateCache.GetState_Ex(stub, key)
		if err != nil {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "queryState GetState failed. error=(%s)", err)
		}

		return (retValue), nil

	} else if function == "isAccExists" { //账户是否存在
		accExist, errcm := b.isAccEntityExists(stub, accName)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "accExists: isEntityExists (id=%s) failed. error=(%s)", accName, errcm)
		}

		var retValue []byte
		if accExist {
			retValue = []byte("1")
		} else {
			retValue = []byte("0")
		}

		return (retValue), nil

	} else if function == "getDataState" {
		if !b.isAdmin(stub, accName) {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "getWorldState: %s can't query.", accName)
		}

		var argCount = fixedArgCount + 3
		if len(args) < argCount {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "getWorldState miss arg, got %d, need %d.", len(args), argCount)
		}

		var needHash = false
		if args[fixedArgCount] == "1" {
			needHash = true
		}

		var flushLimit int
		flushLimit, err = strconv.Atoi(args[fixedArgCount+1])
		if err != nil {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getWorldState: convert flushLimit(%s) failed. error=(%s)", args[fixedArgCount+1], err)
		}
		if flushLimit < 0 {
			flushLimit = 4096
		}

		var currCcid = args[fixedArgCount+2]

		retValue, errcm := b.dumpWorldState(stub, queryTime, flushLimit, needHash, currCcid)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "getWorldState: getWorldState failed. error=(%s)", errcm)
		}
		return (retValue), nil

	} else if function == "getStatisticInfo" {
		//是否是管理员帐户，管理员用户才可以查
		if !b.isAdmin(stub, accName) {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "%s can't query InfoForWeb.", accName)
		}

		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "getStatisticInfo miss arg, got %d, need %d.", len(args), argCount)
		}

		//计算货币流通量的账户
		var circulateAmtAccName = args[fixedArgCount]

		retValue, errcm := b.getSysStatisticInfo(stub, circulateAmtAccName)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "getStatisticInfo: getInfo4Web failed. error=(%s)", errcm)
		}
		return (retValue), nil

	} else if function == "transPreCheck" {
		var argCount = fixedArgCount + 3
		if len(args) < argCount {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "transPreCheck miss arg, got %d, need %d.", len(args), argCount)
		}

		toAcc := args[fixedArgCount]
		//pwd := args[fixedArgCount+1]
		transAmount, err := strconv.ParseInt(args[fixedArgCount+2], 0, 64)
		if err != nil {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "transPreCheck: convert transAmount(%s) failed. error=(%s)", args[fixedArgCount+2], err)
		}

		baselogger.Debug("transPreCheck: accountEnt=%+v", accountEnt)

		//余额是否足够
		if transAmount < 0 { //如果是内部接口调用，可能会转账金额为0， 这里不检查0
			return []byte(strconv.FormatInt(ERRCODE_TRANS_AMOUNT_INVALID, 10)), nil
		}
		//看是否有锁定金额
		lockAmt, _, errcm := Base.getAccountLockedAmount(stub, accName, queryTime)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "transPreCheck: getAccountLockedAmount(id=%s) failed. error=(%s)", accName, errcm)
		}

		if transAmount > accountEnt.RestAmount {
			return []byte(strconv.FormatInt(ERRCODE_TRANS_BALANCE_NOT_ENOUGH, 10)), nil
		}
		//错误码丰富一点，这里再判断是否是因为锁定导致余额不足
		if lockAmt > 0 && transAmount > accountEnt.RestAmount-lockAmt {
			return []byte(strconv.FormatInt(ERRCODE_TRANS_BALANCE_NOT_ENOUGH_BYLOCK, 10)), nil
		}

		//收款账户是否存在  这个检查放到最后执行
		exists, errcm := Base.isAccEntityExists(stub, toAcc)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "transPreCheck: isEntityExists(%s) failed. error=(%s)", toAcc, errcm)
		}
		if !exists {
			return []byte(strconv.FormatInt(ERRCODE_TRANS_PAYEE_ACCOUNT_NOT_EXIST, 10)), nil
		}

		//通过返回0，表示检查通过
		return []byte(strconv.FormatInt(0, 10)), nil

	} else if function == "getAppInfo" {

		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "getAppInfo miss arg, got %d, need %d.", len(args), argCount)
		}

		var appList []AppInfo = []AppInfo{}

		var appid = args[fixedArgCount]

		//appid为空，则查询所有的，后续再实现
		if len(appid) != 0 {
			app, errcm := b.getAppInfo(stub, appid)
			if errcm != nil {
				return nil, baselogger.ErrorECM(errcm.Code, "getAppInfo: getAppInfo(%s) failed. error=(%s)", appid, errcm)
			}

			appList = append(appList, *app)
		}

		returnValue, err := json.Marshal(appList)
		if err != nil {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getAppInfo: Marshal failed. error=(%s)", err)
		}

		return returnValue, nil

	} else if function == "getRankingAndTopN" {

		var argCount = fixedArgCount + 3
		if len(args) < argCount {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "getRankingAndTopN miss arg, got %d, need %d.", len(args), argCount)
		}

		var topN int
		topN, err = strconv.Atoi(args[fixedArgCount])
		if err != nil {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "convert topN(%s) failed. error=(%s)", args[fixedArgCount], err)
		}

		var excludeAccStr = args[fixedArgCount+1]
		var excludeAcc = strings.Split(excludeAccStr, ",")

		var appid = args[fixedArgCount+2]

		rankAndTopN, errcm := b.getAccoutAmountRankingOrTopN(stub, userName, accName, topN, excludeAcc, appid)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "get ranking or topN failed. error=(%s)", errcm)
		}

		returnValue, err := json.Marshal(*rankAndTopN)
		if err != nil {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Marshal failed. error=(%s)", err)
		}

		return returnValue, nil

	} else if function == "getUserInfo" {
		pUser, errcm := b.getUserInfo(stub, userName)
		if errcm != nil && errcm != ErrcmNilEntity {
			return nil, baselogger.ErrorECM(errcm.Code, "getUserInfo failed. error=(%s)", errcm)
		}

		var userInfo UserInfo = UserInfo{}
		if pUser != nil {
			userInfo = *pUser
		}

		returnValue, err := json.Marshal(userInfo)
		if err != nil {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Marshal failed. error=(%s)", err)
		}

		return returnValue, nil

	} else {
		//如果没有匹配到处理函数，一定要返回ErrUnregistedFun
		return nil, ErrcmUnregistedFun
	}
}

func (b *BASE) queryTransInfos(stub shim.ChaincodeStubInterface, transLvl uint64, begIdx, count, begTime, endTime, queryMaxSeq int64, isAsc bool, appid string) ([]byte, *ErrorCodeMsg) {
	var maxSeq int64
	var err error
	var errcm *ErrorCodeMsg

	var retTransInfo []byte
	var queryResult QueryTransResult
	queryResult.NextSerial = -1
	queryResult.MaxSerial = -1
	queryResult.TransRecords = []QueryTransRecd{} //初始化为空，即使下面没查到数据也会返回'[]'

	retTransInfo, err = json.Marshal(queryResult)
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "queryTransInfos Marshal failed.error=(%s)", err)
	}

	//begIdx从1开始， 因为保存交易时，从1开始编号
	if begIdx < 1 {
		begIdx = 1
	}
	//endTime为负数，查询到最新时间
	if endTime < 0 {
		endTime = math.MaxInt64
	}

	if count == 0 {
		baselogger.Warn("queryTransInfos nothing to do(%d).", count)
		return retTransInfo, nil
	}

	//先判断是否存在交易序列号了，如果不存在，说明还没有交易发生。 这里做这个判断是因为在 getTransSeq 里如果没有设置过序列号的key会自动设置一次，但是在query中无法执行PutStat，会报错
	var seqKey = b.getGlobalTransSeqKey(stub)
	test, err := stateCache.GetState_Ex(stub, seqKey)
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "queryTransInfos GetState failed. error=(%s)", err)
	}
	if test == nil {
		baselogger.Info("no trans saved.")
		return retTransInfo, nil
	}

	//先获取当前最大的序列号
	if queryMaxSeq != -1 {
		maxSeq = queryMaxSeq
	} else {
		maxSeq, errcm = b.getTransSeq(stub, seqKey)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "queryTransInfos getTransSeq failed. error=(%s)", errcm)
		}
	}

	if begIdx > maxSeq {
		baselogger.Warn("queryTransInfos nothing to do(%d,%d).", begIdx, maxSeq)
		return retTransInfo, nil
	}

	if count < 0 {
		count = maxSeq - begIdx + 1
	}

	var loopCnt int64 = 0
	var trans *Transaction
	if isAsc { //升序

		/*
		   var retTransInfo = bytes.NewBuffer([]byte("["))
		   for i := begIdx; i <= endIdx; i++ {
		       key, _ := b.getTransInfoKey(stub, i)

		       tmpState, err := stateCache.GetState_Ex(stub,key)
		       if err != nil {
		           base_logger.Error("getTransInfo GetState(idx=%d) failed.error=(%s)", i, err)
		           //return nil, err
		           continue
		       }
		       if tmpState == nil {
		           //return nil, base_logger.Errorf("getTransInfo GetState nil(idx=%d).", i)
		           continue
		       }
		       //获取的TransInfo已经是JSON格式的了，这里直接给拼接为JSON数组，以提高效率。
		       retTransInfo.Write(tmpState)
		       retTransInfo.WriteByte(',')
		   }
		   retTransInfo.Truncate(retTransInfo.Len() - 1) //去掉最后的那个','
		   retTransInfo.WriteByte(']')
		*/
		for loop := begIdx; loop <= maxSeq; loop++ {
			//处理了count条时，不再处理
			if loopCnt >= count {
				break
			}

			trans, errcm = b.getOnceTransInfo(stub, b.getTransInfoKey(stub, loop))
			if errcm != nil {
				baselogger.Error("getTransInfo getQueryTransInfo(idx=%d) failed.error=(%s)", loop, errcm)
				continue
			}
			//取匹配的transLvl
			var qTrans QueryTransRecd
			if trans.TransLvl&transLvl != 0 && trans.Time >= begTime && trans.Time <= endTime {
				if len(appid) > 0 && appid != trans.AppID {
					continue
				}
				qTrans.Serial = trans.GlobalSerial
				qTrans.PubTrans = trans.PubTrans
				queryResult.TransRecords = append(queryResult.TransRecords, qTrans)
				queryResult.NextSerial = qTrans.Serial + 1
				queryResult.MaxSerial = maxSeq
				loopCnt++
			}
		}
	} else { //降序
		for loop := maxSeq - begIdx + 1; loop >= 1; loop-- { //序列号从1开始的
			//处理了count条时，不再处理
			if loopCnt >= count {
				break
			}

			trans, errcm = b.getOnceTransInfo(stub, b.getTransInfoKey(stub, loop))
			if errcm != nil {
				baselogger.Error("getTransInfo getQueryTransInfo(idx=%d) failed.error=(%s)", loop, errcm)
				continue
			}
			//取匹配的transLvl
			var qTrans QueryTransRecd
			if trans.TransLvl&transLvl != 0 && trans.Time >= begTime && trans.Time <= endTime {
				if len(appid) > 0 && appid != trans.AppID {
					continue
				}
				qTrans.Serial = maxSeq - trans.GlobalSerial + 1
				qTrans.PubTrans = trans.PubTrans
				queryResult.TransRecords = append(queryResult.TransRecords, qTrans)
				queryResult.NextSerial = qTrans.Serial + 1
				queryResult.MaxSerial = maxSeq
				loopCnt++
			}
		}
	}

	retTransInfo, err = json.Marshal(queryResult)
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getTransInfo Marshal failed.error=(%s)", err)
	}

	return retTransInfo, nil
}

func (b *BASE) queryAccTransInfos(stub shim.ChaincodeStubInterface, accName string, begIdx, count, begTime, endTime, queryMaxSeq int64, isAsc bool, appid string) ([]byte, *ErrorCodeMsg) {
	var maxSeq int64
	var err error
	var errcm *ErrorCodeMsg

	var retTransInfo []byte
	var queryResult QueryTransResult
	queryResult.NextSerial = -1
	queryResult.MaxSerial = -1
	queryResult.TransRecords = []QueryTransRecd{} //初始化为空，即使下面没查到数据也会返回'[]'

	retTransInfo, err = json.Marshal(queryResult)
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "queryAccTransInfos Marshal failed.error=(%s)", err)
	}

	//begIdx统一从1开始，防止调用者有的以0开始，有的以1开始
	if begIdx < 1 {
		begIdx = 1
	}
	//endTime为负数，查询到最新时间
	if endTime < 0 {
		endTime = math.MaxInt64
	}

	if count == 0 {
		baselogger.Warn("queryAccTransInfos nothing to do(%d).", count)
		return retTransInfo, nil
	}

	//先判断是否存在交易序列号了，如果不存在，说明还没有交易发生。 这里做这个判断是因为在 getTransSeq 里如果没有设置过序列号的key会自动设置一次，但是在query中无法执行PutStat，会报错
	var seqKey = b.getAccTransSeqKey(accName)
	test, err := stateCache.GetState_Ex(stub, seqKey)
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "queryAccTransInfos GetState failed. error=(%s)", err)
	}
	if test == nil {
		baselogger.Info("queryAccTransInfos no trans saved.")
		return retTransInfo, nil
	}

	//先获取当前最大的序列号
	if queryMaxSeq != -1 {
		maxSeq = queryMaxSeq
	} else {
		maxSeq, errcm = b.getTransSeq(stub, seqKey)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "queryAccTransInfos getTransSeq failed. error=(%s)", errcm)
		}
	}

	if begIdx > maxSeq {
		baselogger.Warn("queryAccTransInfos nothing to do(%d,%d).", begIdx, maxSeq)
		return retTransInfo, nil
	}

	if count < 0 {
		count = maxSeq - begIdx + 1
	}
	/*
	   infoB, err := stateCache.GetState_Ex(stub,t.getOneAccTransKey(accName))
	   if err != nil {
	       return nil, base_logger.Errorf("queryAccTransInfos(%s) GetState failed.error=(%s)", accName, err)
	   }
	   if infoB == nil {
	       return retTransInfo, nil
	   }

	   var transArr []QueryTransRecd = []QueryTransRecd{} //初始化为空数组，即使下面没查到数据也会返回'[]'
	   var loopCnt int64 = 0
	   var trans *QueryTransRecd
	   var buf = bytes.NewBuffer(infoB)
	   var oneStringB []byte
	   var oneString string
	   var loop int64 = 0
	   for {
	       if loopCnt >= count {
	           break
	       }
	       oneStringB, err = buf.ReadBytes(MULTI_STRING_DELIM)
	       if err != nil {
	           if err == io.EOF {
	               base_logger.Debug("queryAccTransInfos proc %d recds, end.", loop)
	               break
	           }
	           return nil, base_logger.Errorf("queryAccTransInfos ReadBytes failed. last=%s, error=(%s)", string(oneStringB), err)
	       }
	       loop++
	       if begIdx > loop {
	           continue
	       }

	       oneString = string(oneStringB[:len(oneStringB)-1]) //去掉末尾的分隔符

	       trans, err = b.getQueryTransInfo(stub, oneString)
	       if err != nil {
	           base_logger.Error("queryAccTransInfos(%s) getQueryTransInfo failed, error=(%s).", accName, err)
	           continue
	       }
	       var qTrans QueryTransRecd
	       if trans.Time >= begTime && trans.Time <= endTime {
	           qTrans.Serial = loop
	           qTrans.PubTrans = trans.PubTrans
	           transArr = append(transArr, qTrans)
	           loopCnt++
	       }
	   }
	*/

	var globTxKeyB []byte
	var trans *Transaction
	var loopCnt int64 = 0
	if isAsc { //升序
		for loop := begIdx; loop <= maxSeq; loop++ {
			//处理了count条时，不再处理
			if loopCnt >= count {
				break
			}

			globTxKeyB, err = stateCache.GetState_Ex(stub, b.getOneAccTransInfoKey(accName, loop))
			if err != nil {
				baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "queryAccTransInfos GetState(globTxKeyB,acc=%s,idx=%d) failed.error=(%s)", accName, loop, err)
				continue
			}

			trans, errcm = b.getOnceTransInfo(stub, string(globTxKeyB))
			if errcm != nil {
				baselogger.Error("queryAccTransInfos getQueryTransInfo(idx=%d) failed.error=(%s)", loop, errcm)
				continue
			}

			//记录有错误？
			if trans.FromID != accName {
				baselogger.Warn("queryAccTransInfos accName not match.(%s,%s)", trans.FromID, accName)
				continue
			}

			var qTrans QueryTransRecd
			if trans.Time >= begTime && trans.Time <= endTime {
				if len(appid) > 0 && appid != trans.AppID {
					continue
				}
				qTrans.Serial = loop
				qTrans.PubTrans = trans.PubTrans
				queryResult.TransRecords = append(queryResult.TransRecords, qTrans)
				queryResult.NextSerial = qTrans.Serial + 1
				queryResult.MaxSerial = maxSeq
				loopCnt++
			}
		}
	} else { //降序
		for loop := maxSeq - begIdx + 1; loop >= 1; loop-- { //序列号从1开始的
			//处理了count条时，不再处理
			if loopCnt >= count {
				break
			}

			globTxKeyB, err = stateCache.GetState_Ex(stub, b.getOneAccTransInfoKey(accName, loop))
			if err != nil {
				baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "queryAccTransInfos GetState(globTxKeyB,acc=%s,idx=%d) failed.error=(%s)", accName, loop, err)
				continue
			}

			trans, err := b.getOnceTransInfo(stub, string(globTxKeyB))
			if err != nil {
				baselogger.Error("queryAccTransInfos getQueryTransInfo(idx=%d) failed.error=(%s)", loop, err)
				continue
			}

			//记录有错误？
			if trans.FromID != accName {
				baselogger.Warn("queryAccTransInfos accName not match.(%s,%s)", trans.FromID, accName)
				continue
			}

			var qTrans QueryTransRecd
			if trans.Time >= begTime && trans.Time <= endTime {
				if len(appid) > 0 && appid != trans.AppID {
					continue
				}
				qTrans.Serial = maxSeq - loop + 1
				qTrans.PubTrans = trans.PubTrans
				queryResult.TransRecords = append(queryResult.TransRecords, qTrans)
				queryResult.NextSerial = qTrans.Serial + 1
				queryResult.MaxSerial = maxSeq
				loopCnt++
			}
		}
	}
	retTransInfo, err = json.Marshal(queryResult)
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "queryAccTransInfos Marshal failed.error=(%s)", err)
	}

	return retTransInfo, nil
}

func (b *BASE) getAllAccAmt(stub shim.ChaincodeStubInterface) ([]byte, *ErrorCodeMsg) {
	var qb QueryBalance
	qb.IssueAmount = 0
	qb.AccSumAmount = 0
	qb.AccCount = 0

	accsB, err := stateCache.GetState_Ex(stub, ALL_ACC_INFO_KEY)
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getAllAccAmt GetState failed. error=(%s)", err)
	}
	if accsB != nil {
		cbAccB, errcm := b.getCenterBankAcc(stub)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "getAllAccAmt getCenterBankAcc failed. error=(%s)", errcm)
		}
		if cbAccB == nil {
			qb.Message += "none centerBank;"
		} else {
			cbEnt, errcm := b.getAccountEntity(stub, string(cbAccB))
			if errcm != nil {
				return nil, baselogger.ErrorECM(errcm.Code, "getAllAccAmt getCenterBankAcc failed. error=(%s)", errcm)
			}
			qb.IssueAmount = cbEnt.TotalAmount - cbEnt.RestAmount
		}

		var allAccs = bytes.NewBuffer(accsB)
		var acc []byte
		var ent *AccountEntity
		for {
			acc, err = allAccs.ReadBytes(MULTI_STRING_DELIM)
			if err != nil {
				if err == io.EOF {
					break
				} else {
					baselogger.Error("getAllAccAmt ReadBytes failed. error=(%s)", err)
					continue
				}
			}
			qb.AccCount++
			acc = acc[:len(acc)-1] //去掉末尾的分隔符

			ent, errcm = b.getAccountEntity(stub, string(acc))
			if errcm != nil {
				baselogger.Error("getAllAccAmt getAccountEntity(%s) failed. error=(%s)", string(acc), errcm)
				qb.Message += fmt.Sprintf("get account(%s) info failed;", string(acc))
				continue
			}
			qb.AccSumAmount += ent.RestAmount
		}
	}

	retValue, err := json.Marshal(qb)
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getAllAccAmt Marshal failed. error=(%s)", err)
	}

	return retValue, nil
}

func (b *BASE) getSysStatisticInfo(stub shim.ChaincodeStubInterface, circulateAmtAccName string) ([]byte, *ErrorCodeMsg) {

	type SysStatisticInfo struct {
		AccountNum       int64 `json:"accountcount"`  //账户数量
		IssueTotalAmount int64 `json:"issuetotalamt"` //预计发行总量
		IssueAmount      int64 `json:"issueamt"`      //已发行数量
		CirculateAmount  int64 `json:"circulateamt"`  //流通数量
	}

	var qwi SysStatisticInfo
	qwi.AccountNum = 0
	qwi.IssueTotalAmount = 0
	qwi.IssueAmount = 0
	qwi.CirculateAmount = 0

	issueEntity, errcm := b.getAccountEntity(stub, COIN_ISSUE_ACC_ENTID)
	if errcm != nil {
		return nil, baselogger.ErrorECM(errcm.Code, "getInfo4Web: getIssueEntity failed. error=(%s)", errcm)
	}
	qwi.IssueTotalAmount = issueEntity.TotalAmount
	qwi.IssueAmount = issueEntity.TotalAmount - issueEntity.RestAmount

	var asi AccountStatisticInfo
	asiB, err := stateCache.GetState_Ex(stub, ACC_STATIC_INFO_KEY)
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getInfo4Web: GetState failed. error=(%s)", err)
	}
	if asiB != nil {
		err = json.Unmarshal(asiB, &asi)
		if err != nil {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getInfo4Web: Unmarshal failed. error=(%s)", err)
		}
		qwi.AccountNum = asi.AccountCount
	}

	//如果传入了计算流通货币量的账户，用此账户；否则用央行账户
	if len(circulateAmtAccName) > 0 {
		ent, errcm := b.getAccountEntity(stub, circulateAmtAccName)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "getInfo4Web: getAccountEntity failed. error=(%s)", errcm)
		}
		qwi.CirculateAmount = ent.TotalAmount - ent.RestAmount
	} else {
		cbAccB, errcm := b.getCenterBankAcc(stub)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "getInfo4Web: getCenterBankAcc failed. error=(%s)", errcm)
		}
		if cbAccB != nil {
			cbEnt, errcm := b.getAccountEntity(stub, string(cbAccB))
			if errcm != nil {
				return nil, baselogger.ErrorECM(errcm.Code, "getInfo4Web getAccountEntity failed. error=(%s)", errcm)
			}
			qwi.CirculateAmount = cbEnt.TotalAmount - cbEnt.RestAmount
		}
	}

	retValue, err := json.Marshal(qwi)
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getInfo4Web Marshal failed. error=(%s)", err)
	}

	return retValue, nil
}

func (b *BASE) needCheckSign(stub shim.ChaincodeStubInterface) bool {
	/*
		//默认返回true，除非读取到指定参数

		var args = util.ToChaincodeArgs(CONTROL_CC_GETPARA_FUNC_NAME, "checkSiagnature")

		response := stub.InvokeChaincode(CONTROL_CC_NAME, args, "")
		if response.Status != shim.OK {
			baselogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR,"needCheckSign: InvokeChaincode failed, response=%+v.", response)
			return true
		}

		paraValue := string(response.Payload)
		if paraValue == "0" {
			return false
		}

		return true
	*/
	return Ctrl_needCheckSign
}

var secp256k1 = NewSecp256k1()

func (b *BASE) verifySign(stub shim.ChaincodeStubInterface, ownerPubKeyHash string, sign, signMsg []byte) *ErrorCodeMsg {

	//没有保存pubkey，不验证
	if len(ownerPubKeyHash) == 0 {
		baselogger.Debug("verifySign: pubkey is nil, do not check signature.")
		return nil
	}

	if chk := b.needCheckSign(stub); !chk {
		baselogger.Debug("verifySign: do not need check signature.")
		return nil
	}

	baselogger.Debug("verifySign: sign = %v", sign)
	baselogger.Debug("verifySign: signMsg = %v", signMsg)

	if code := secp256k1.VerifySignatureValidity(sign); code != 1 {
		return baselogger.ErrorECM(ERRCODE_COMMON_IDENTITY_VERIFY_FAILED, "verifySign: sign invalid, code=%v.", code)
	}

	pubKey, err := secp256k1.RecoverPubkey(signMsg, sign)
	if err != nil {
		return baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "verifySign: RecoverPubkey failed,error=(%s)", err)
	}
	baselogger.Debug("verifySign: pubKey = %v", pubKey)

	hash, err := RipemdHash160(pubKey)
	if err != nil {
		return baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "verifySign: Hash160 error, error=(%s).", err)
	}
	var userPubKeyHash = base64.StdEncoding.EncodeToString(hash)
	baselogger.Debug("verifySign: userPubKeyHash = %s", userPubKeyHash)
	baselogger.Debug("verifySign: OwnerPubKeyHash = %s", ownerPubKeyHash)

	if userPubKeyHash != ownerPubKeyHash {
		return baselogger.ErrorECM(ERRCODE_COMMON_IDENTITY_VERIFY_FAILED, "verifySign: sign invalid.")
	}

	return nil
}

func (b *BASE) verifyIdentity(stub shim.ChaincodeStubInterface, userName string, sign, signMsg []byte, accountEnt *AccountEntity, ownerPubKeyHash, ownerIdentityHash string) *ErrorCodeMsg {

	var comparedPubKeyHash = ownerPubKeyHash
	var comparedIndentityHash = ownerIdentityHash

	baselogger.Debug("verifyIdentity: accountEnt = %+v", accountEnt)

	if accountEnt != nil {
		comparedPubKeyHash = accountEnt.OwnerPubKeyHash
		comparedIndentityHash = accountEnt.OwnerIdentityHash
	}

	if Ctrl_needCheckIndentity {
		if accountEnt != nil && accountEnt.Owner != userName {
			if _, ok := accountEnt.AuthUserHashMap[userName]; !ok {
				return baselogger.ErrorECM(ERRCODE_COMMON_IDENTITY_VERIFY_FAILED, "verifyIdentity: username not match, user=%s", userName)
			}
			var hashs = accountEnt.AuthUserHashMap[userName]
			if len(hashs) < 2 {
				return baselogger.ErrorECM(ERRCODE_COMMON_IDENTITY_VERIFY_FAILED, "verifyIdentity: hash  illegal(%d).", len(hashs))
			}
			//第一个元素为身份hash，第二个为pubkey的hash
			comparedIndentityHash = hashs[0]
			comparedPubKeyHash = hashs[1]
		}

		creatorByte, err := stub.GetCreator()
		if err != nil {
			return baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "verifyIdentity: GetCreator error, user=%s error=(%s).", userName, err)
		}
		baselogger.Debug("verifyIdentity: creatorByte = %s", string(creatorByte))

		certStart := bytes.IndexAny(creatorByte, "-----BEGIN")
		if certStart == -1 {
			return baselogger.ErrorECM(ERRCODE_COMMON_IDENTITY_VERIFY_FAILED, "verifyIdentity: No certificate found, user=%s.", userName)
		}
		certText := creatorByte[certStart:]

		block, _ := pem.Decode(certText)
		if block == nil {
			return baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "verifyIdentity: Decode failed, user=%s.", userName)
		}
		baselogger.Debug("verifyIdentity: block = %+v", *block)

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "verifyIdentity: ParseCertificate failed, user=%s, error=(%s).", userName, err)
		}
		baselogger.Debug("verifyIdentity: cert = %+v", *cert)

		nameInCert := cert.Subject.CommonName
		baselogger.Debug("verifyIdentity: nameInCert = %s", nameInCert)

		//传入的用户名是否是登录的用户
		if userName != nameInCert {
			return baselogger.ErrorECM(ERRCODE_COMMON_IDENTITY_VERIFY_FAILED, "verifyIdentity: username not match the cert(%s.%s).", userName, nameInCert)
		}

		var userId = string(certText)
		hash, err := RipemdHash160(certText)
		if err != nil {
			return baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "verifyIdentity: Hash160 error, user=%s error=(%s).", userName, err)
		}
		var userIdHash = base64.StdEncoding.EncodeToString(hash)

		baselogger.Debug("verifyIdentity: userId = %s", userId)
		baselogger.Debug("verifyIdentity: userIdHash = %s", userIdHash)
		baselogger.Debug("verifyIdentity: entIdHash = %s", comparedIndentityHash)

		if userIdHash != comparedIndentityHash {
			return baselogger.ErrorECM(ERRCODE_COMMON_IDENTITY_VERIFY_FAILED, "verifyIdentity: indentity invalid.")
		}
	}

	return b.verifySign(stub, comparedPubKeyHash, sign, signMsg)
}

func (b *BASE) getAccountEntityKey(accName string) string {
	return ACC_ENTITY_PREFIX + accName
}

func (b *BASE) getAccountLockInfoKey(accName string) string {
	return ACC_AMTLOCK_PREFIX + accName
}

func (b *BASE) getAccountEntity(stub shim.ChaincodeStubInterface, entName string) (*AccountEntity, *ErrorCodeMsg) {
	var entB []byte
	var cb AccountEntity
	var err error

	entB, err = stateCache.GetState_Ex(stub, b.getAccountEntityKey(entName))
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getAccountEntity: GetState_Ex failed, error=(%s).", err)
	}

	if entB == nil {
		return nil, ErrcmNilEntity
	}

	if err = json.Unmarshal(entB, &cb); err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getAccountEntity: Unmarshal failed, error=(%s).", err)
	}

	return &cb, nil
}

func (b *BASE) getAccountLockedAmount(stub shim.ChaincodeStubInterface, accName string, currTime int64) (int64, []CoinLockCfg, *ErrorCodeMsg) {
	var acli AccountCoinLockInfo

	var lockinfoKey = b.getAccountLockInfoKey(accName)
	acliB, err := stateCache.GetState_Ex(stub, lockinfoKey)
	if err != nil {
		return math.MaxInt64, nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getAccountLockedAmount: GetState  failed. error=(%s)", err)
	}

	var lockAmt int64 = 0
	if acliB == nil {
		lockAmt = 0
	} else {

		err = json.Unmarshal(acliB, &acli)
		if err != nil {
			return math.MaxInt64, nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getAccountLockedAmount: Unmarshal  failed. error=(%s)", err)
		}

		for _, lockCfg := range acli.LockList {
			if lockCfg.LockEndTime > currTime {
				lockAmt += lockCfg.LockAmount
			}
		}
	}

	baselogger.Debug("getAccountLockedAmount: amount is %d for %s", lockAmt, accName)

	return lockAmt, acli.LockList, nil

}

func (b *BASE) isAccEntityExists(stub shim.ChaincodeStubInterface, entName string) (bool, *ErrorCodeMsg) {
	var entB []byte
	var err error

	entB, err = stateCache.GetState_Ex(stub, b.getAccountEntityKey(entName))
	if err != nil {
		return false, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "marshal cb failed. error=(%s)", err)
	}

	if entB == nil {
		return false, nil
	}

	return true, nil
}

//央行数据写入
func (b *BASE) setAccountEntity(stub shim.ChaincodeStubInterface, cb *AccountEntity) *ErrorCodeMsg {

	jsons, err := json.Marshal(cb)

	if err != nil {
		return baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "marshal cb failed. error=(%s)", err)
	}

	err = stateCache.PutState_Ex(stub, b.getAccountEntityKey(cb.EntID), jsons)

	if err != nil {
		return baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "PutState cb failed. error=(%s)", err)
	}
	return nil
}

//发行
func (b *BASE) issueCoin(stub shim.ChaincodeStubInterface, cbID string, issueAmount, issueTime int64) ([]byte, *ErrorCodeMsg) {
	baselogger.Debug("Enter issueCoin")

	var errcm *ErrorCodeMsg

	if issueAmount < 0 {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "issueCoin issueAmount < 0.")
	}
	if issueAmount == 0 {
		return nil, nil
	}

	var cb *AccountEntity
	cb, errcm = b.getAccountEntity(stub, cbID)
	if errcm != nil {
		return nil, baselogger.ErrorECM(errcm.Code, "getCenterBank failed. error=(%s)", errcm)
	}

	issueEntity, errcm := b.getAccountEntity(stub, COIN_ISSUE_ACC_ENTID)
	if errcm != nil {
		return nil, baselogger.ErrorECM(errcm.Code, "issue: getIssueEntity failed. error=(%s)", errcm)
	}

	baselogger.Debug("issue before:cb=%+v, issue=%+v", cb, issueEntity)

	if issueAmount > issueEntity.RestAmount {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "issue amount not enougth(%v,%v), reject.", issueEntity.RestAmount, issueAmount)
	}

	issueEntity.RestAmount -= issueAmount
	cb.TotalAmount += issueAmount
	cb.RestAmount += issueAmount

	errcm = b.setAccountEntity(stub, cb)
	if errcm != nil {
		return nil, baselogger.ErrorECM(errcm.Code, "issue: setCenterBank failed. error=(%s)", errcm)
	}

	errcm = b.setAccountEntity(stub, issueEntity)
	if errcm != nil {
		return nil, baselogger.ErrorECM(errcm.Code, "issue: setIssueEntity failed. error=(%s)", errcm)
	}

	baselogger.Debug("issue after:cb=%+v, issue=%+v", cb, issueEntity)

	//这里只记录一下央行的收入，不记录支出
	errcm = b.recordTranse(stub, cb, issueEntity, TRANS_INCOME, "issue", "center bank issue coin.", issueAmount, issueTime, "")
	if errcm != nil {
		return nil, baselogger.ErrorECM(errcm.Code, "issue: recordTranse failed. error=(%s)", errcm)
	}

	return nil, nil
}

func (b *BASE) setIssueAmountTotal(stub shim.ChaincodeStubInterface, issueAmt, initTime int64) *ErrorCodeMsg {

	//虚拟一个超级账户，设置货币发行总额，给央行发行货币。
	var issueEntity AccountEntity
	issueEntity.EntID = COIN_ISSUE_ACC_ENTID
	issueEntity.EntType = -1
	issueEntity.TotalAmount = issueAmt
	issueEntity.RestAmount = issueAmt
	issueEntity.Time = initTime
	issueEntity.Owner = "system"

	errcm := b.setAccountEntity(stub, &issueEntity)
	if errcm != nil {
		return baselogger.ErrorECM(errcm.Code, "setIssueCoinTotal: setIssueEntity failed. error=(%s)", errcm)
	}

	return nil
}

//转账
func (b *BASE) transferCoin(stub shim.ChaincodeStubInterface, from, to, transType, description string, amount, transeTime int64, sameEntSaveTrans bool, appid string) ([]byte, *ErrorCodeMsg) {
	baselogger.Debug("Enter transferCoin")

	var errcm *ErrorCodeMsg

	if amount < 0 {
		return nil, baselogger.ErrorECM(ERRCODE_TRANS_AMOUNT_INVALID, "transferCoin failed. invalid amount(%d)", amount)
	}

	//有时前端后台调用这个接口时，可能会传0
	if amount == 0 {
		return nil, nil
	}

	//如果账户相同，并且账户相同时不需要记录交易，直接返回
	if from == to && !sameEntSaveTrans {
		baselogger.Warn("transferCoin from equals to.")
		return nil, nil
	}

	var fromEntity, toEntity *AccountEntity
	fromEntity, errcm = b.getAccountEntity(stub, from)
	if errcm != nil {
		var errCode int32
		if errcm == ErrcmNilEntity {
			errCode = ERRCODE_TRANS_PAY_ACCOUNT_NOT_EXIST
		} else {
			errCode = errcm.Code
		}
		return nil, baselogger.ErrorECM(errCode, "transferCoin: getAccountEntity(id=%s) failed. error=(%s)", from, errcm)
	}
	toEntity, errcm = b.getAccountEntity(stub, to)
	if errcm != nil {
		var errCode int32
		if errcm == ErrcmNilEntity {
			errCode = ERRCODE_TRANS_PAYEE_ACCOUNT_NOT_EXIST
		} else {
			errCode = errcm.Code
		}
		return nil, baselogger.ErrorECM(errCode, "transferCoin: getAccountEntity(id=%s) failed. error=(%s)", to, errcm)
	}

	//判断是否有锁定金额
	lockAmt, _, errcm := b.getAccountLockedAmount(stub, from, transeTime)
	if errcm != nil {
		return nil, baselogger.ErrorECM(errcm.Code, "transferCoin: getAccountLockedAmount(id=%s) failed. error=(%s)", from, errcm)
	}

	if fromEntity.RestAmount-lockAmt < amount {
		var errCode int32
		if lockAmt > 0 {
			errCode = ERRCODE_TRANS_BALANCE_NOT_ENOUGH_BYLOCK
		} else {
			errCode = ERRCODE_TRANS_BALANCE_NOT_ENOUGH
		}
		return nil, baselogger.ErrorECM(errCode, "transferCoin: fromEntity(id=%s) restAmount not enough(%d,%d,%d).", from, fromEntity.RestAmount, lockAmt, amount)
	}

	//如果账户相同，并且账户相同时需要记录交易，记录并返回
	if from == to && sameEntSaveTrans {
		errcm = b.recordTranse(stub, fromEntity, toEntity, TRANS_PAY, transType, description, amount, transeTime, appid)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "transferCoin: setAccountEntity recordTranse fromEntity(id=%s) failed. error=(%s)", from, errcm)
		}

		errcm = b.recordTranse(stub, toEntity, fromEntity, TRANS_INCOME, transType, description, amount, transeTime, appid)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "transferCoin: setAccountEntity recordTranse fromEntity(id=%s) failed. error=(%s)", from, errcm)
		}
		return nil, nil
	}

	//账户相同时为什么单独处理？  因为如果走了下面的流程，setAccountEntity两次同一个账户，会导致账户余额变化。 除非在计算并设置完fromEntity之后，再获取一下toEntity，再计算toEntity，这样感觉太呆了

	baselogger.Debug("transferCoin: fromEntity before= %+v", fromEntity)
	baselogger.Debug("transferCoin: toEntity before= %+v", toEntity)

	fromEntity.RestAmount -= amount

	toEntity.RestAmount += amount
	toEntity.TotalAmount += amount

	baselogger.Debug("transferCoin: fromEntity after= %+v", fromEntity)
	baselogger.Debug("transferCoin: toEntity after= %+v", toEntity)

	errcm = b.setAccountEntity(stub, fromEntity)
	if errcm != nil {
		return nil, baselogger.ErrorECM(errcm.Code, "transferCoin: setAccountEntity of fromEntity(id=%s) failed. error=(%s)", from, errcm)
	}

	errcm = b.recordTranse(stub, fromEntity, toEntity, TRANS_PAY, transType, description, amount, transeTime, appid)
	if errcm != nil {
		return nil, baselogger.ErrorECM(errcm.Code, "transferCoin: setAccountEntity recordTranse fromEntity(id=%s) failed. error=(%s)", from, errcm)
	}

	errcm = b.setAccountEntity(stub, toEntity)
	if errcm != nil {
		return nil, baselogger.ErrorECM(errcm.Code, "transferCoin: setAccountEntity of toEntity(id=%s) failed. error=(%s)", to, errcm)
	}

	//两个账户的收入支出都记录交易
	errcm = b.recordTranse(stub, toEntity, fromEntity, TRANS_INCOME, transType, description, amount, transeTime, appid)
	if errcm != nil {
		return nil, baselogger.ErrorECM(errcm.Code, "transferCoin: setAccountEntity recordTranse fromEntity(id=%s) failed. error=(%s)", from, errcm)
	}

	return nil, errcm
}

const (
	TRANS_LVL_CB   = 1
	TRANS_LVL_COMM = 2
)

//记录交易。目前交易分为两种：一种是和央行打交道的，包括央行发行货币、央行给项目或企业转帐，此类交易普通用户不能查询；另一种是项目、企业、个人间互相转账，此类交易普通用户能查询
func (b *BASE) recordTranse(stub shim.ChaincodeStubInterface, fromEnt, toEnt *AccountEntity, incomePayFlag int, transType, description string, amount, times int64, appid string) *ErrorCodeMsg {
	var transInfo Transaction
	//var now = time.Now()

	transInfo.AppID = appid
	transInfo.FromID = fromEnt.EntID
	transInfo.FromType = fromEnt.EntType
	transInfo.ToID = toEnt.EntID
	transInfo.TransFlag = incomePayFlag
	transInfo.ToType = toEnt.EntType
	//transInfo.Time = now.Unix()*1000 + int64(now.Nanosecond()/1000000) //单位毫秒
	transInfo.Time = times
	transInfo.Amount = amount
	transInfo.TxID = stub.GetTxID()
	transInfo.TransType = transType
	transInfo.Description = description

	var transLevel uint64 = TRANS_LVL_COMM
	accCB, errcm := b.getCenterBankAcc(stub)
	if errcm != nil {
		return baselogger.ErrorECM(errcm.Code, "recordTranse call getCenterBankAcc failed. error=(%s)", errcm)
	}
	if (accCB != nil) && (string(accCB) == transInfo.FromID || string(accCB) == transInfo.ToID) {
		transLevel = TRANS_LVL_CB
	}

	transInfo.TransLvl = transLevel

	errcm = b.setTransInfo(stub, &transInfo)
	if errcm != nil {
		return baselogger.ErrorECM(errcm.Code, "recordTranse call setTransInfo failed. error=(%s)", errcm)
	}

	return nil
}

func (b *BASE) checkAccountName(accName string) *ErrorCodeMsg {
	if strings.ContainsAny(accName, ACC_INVALID_CHAR_SET) {
		return baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "accName '%s' can not contains any of '%s'.", accName, ACC_INVALID_CHAR_SET)
	}
	return nil
}

func (b *BASE) saveAccountName(stub shim.ChaincodeStubInterface, accName string) *ErrorCodeMsg {
	accB, err := stateCache.GetState_Ex(stub, ALL_ACC_INFO_KEY)
	if err != nil {
		return baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "saveAccountName GetState failed.error=(%s)", err)
	}

	var accs []byte
	if accB == nil {
		accs = append([]byte(accName), MULTI_STRING_DELIM) //第一次添加accName，最后也要加上分隔符
	} else {
		accs = append(accB, []byte(accName)...)
		accs = append(accs, MULTI_STRING_DELIM)
	}

	err = stateCache.PutState_Ex(stub, ALL_ACC_INFO_KEY, accs)
	if err != nil {
		return baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "saveAccountName PutState(accs) failed.error=(%s)", err)
	}

	var asi AccountStatisticInfo
	asiB, err := stateCache.GetState_Ex(stub, ACC_STATIC_INFO_KEY)
	if asiB == nil {
		asi.AccountCount = 1
	} else {
		err = json.Unmarshal(asiB, &asi)
		if err != nil {
			return baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "saveAccountName Unmarshal failed.error=(%s)", err)
		}
		asi.AccountCount++
	}

	asiB, err = json.Marshal(asi)
	if err != nil {
		return baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "saveAccountName Marshal failed.error=(%s)", err)
	}

	err = stateCache.PutState_Ex(stub, ACC_STATIC_INFO_KEY, asiB)
	if err != nil {
		return baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "saveAccountName PutState(asiB) failed.error=(%s)", err)
	}

	return nil
}

func (b *BASE) newAccount(stub shim.ChaincodeStubInterface, accName string, accType int, userName, userIdHash, userPubKeyHash string, times int64, isCBAcc bool) ([]byte, *ErrorCodeMsg) {
	baselogger.Debug("Enter openAccount")

	var errcm *ErrorCodeMsg
	var accExist bool

	if errcm = b.checkAccountName(accName); errcm != nil {
		return nil, errcm
	}

	accExist, errcm = b.isAccEntityExists(stub, accName)
	if errcm != nil {
		return nil, baselogger.ErrorECM(errcm.Code, "isEntityExists (id=%s) failed. error=(%s)", accName, errcm)
	}

	if accExist {
		/*
			//兼容kd老版本，没有userIdHash和userPubKeyHash的情况，如果这两个字段为空，只写这两个字段，后续可以删除
			accEnt, errcm := b.getAccountEntity(stub, accName)
			if errcm != nil {
				return nil, baselogger.ErrorECM(errcm.Code,"getAccountEntity (id=%s) failed. error=(%s)", accName, errcm)
			}
			if (len(accEnt.OwnerIdentityHash) == 0 && len(userIdHash) > 0) ||
				(len(accEnt.OwnerPubKeyHash) == 0 && len(userPubKeyHash) > 0) {
				accEnt.OwnerIdentityHash = userIdHash
				accEnt.OwnerPubKeyHash = userPubKeyHash

				errcm = b.setAccountEntity(stub, accEnt)
				if errcm != nil {
					return nil, baselogger.ErrorECM(errcm.Code,"setAccountEntity (id=%s) failed. error=(%s)", accName, errcm)
				}
				return nil, nil
			}
			//兼容kd老版本，没有userIdHash和userPubKeyHash的情况，如果这两个字段为空，只写这两个字段，后续可以删除
		*/

		return nil, baselogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "account (id=%s) failed, already exists.", accName)
	}

	var ent AccountEntity
	//var now = time.Now()

	ent.EntID = accName
	ent.EntType = accType
	ent.RestAmount = 0
	ent.TotalAmount = 0
	//ent.Time = now.Unix()*1000 + int64(now.Nanosecond()/1000000) //单位毫秒
	ent.Time = times
	ent.Owner = userName
	ent.OwnerIdentityHash = userIdHash
	ent.OwnerPubKeyHash = userPubKeyHash

	errcm = b.setAccountEntity(stub, &ent)
	if errcm != nil {
		return nil, baselogger.ErrorECM(errcm.Code, "openAccount setAccountEntity (id=%s) failed. error=(%s)", accName, errcm)
	}

	baselogger.Debug("openAccount success: %+v", ent)

	puserEnt, errcm := b.getUserEntity(stub, userName)
	if errcm != nil && errcm != ErrcmNilEntity {
		return nil, baselogger.ErrorECM(errcm.Code, "openAccount getUserEntity (id=%s) failed. error=(%s)", userName, errcm)
	}

	var userEnt UserEntity
	if puserEnt == nil {
		userEnt.EntID = userName
	} else {
		userEnt = *puserEnt
	}
	userEnt.AccList = append(userEnt.AccList, accName)

	errcm = b.setUserEntity(stub, &userEnt)
	if errcm != nil {
		return nil, baselogger.ErrorECM(errcm.Code, "openAccount setUserEntity (id=%s) failed. error=(%s)", userName, errcm)
	}

	baselogger.Debug("setUserEntity success: %+v", userEnt)

	//央行账户此处不保存
	if !isCBAcc {
		errcm = b.saveAccountName(stub, accName)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "openAccount saveAccountName (id=%s) failed. error=(%s)", accName, errcm)
		}
	}

	return nil, nil
}

var centerBankAccCache []byte = nil

func (b *BASE) setCenterBankAcc(stub shim.ChaincodeStubInterface, acc string) *ErrorCodeMsg {
	err := stateCache.PutState_Ex(stub, CENTERBANK_ACC_KEY, []byte(acc))
	if err != nil {
		return baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setCenterBankAcc PutState failed.error=(%s)", err)
	}

	centerBankAccCache = []byte(acc)

	return nil
}
func (b *BASE) getCenterBankAcc(stub shim.ChaincodeStubInterface) ([]byte, *ErrorCodeMsg) {
	if centerBankAccCache != nil {
		return centerBankAccCache, nil
	}

	bankB, err := stateCache.GetState_Ex(stub, CENTERBANK_ACC_KEY)
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getCenterBankAcc GetState failed.error=(%s)", err)
	}

	centerBankAccCache = bankB

	return bankB, nil
}

func (b *BASE) getTransSeq(stub shim.ChaincodeStubInterface, transSeqKey string) (int64, *ErrorCodeMsg) {
	seqB, err := stateCache.GetState_Ex(stub, transSeqKey)
	if err != nil {
		return -1, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getTransSeq GetState failed.error=(%s)", err)
	}
	//如果不存在则创建
	if seqB == nil {
		err = stateCache.PutState_Ex(stub, transSeqKey, []byte("0"))
		if err != nil {
			return -1, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "initTransSeq PutState failed.error=(%s)", err)
		}
		return 0, nil
	}

	seq, err := strconv.ParseInt(string(seqB), 10, 64)
	if err != nil {
		return -1, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getTransSeq ParseInt failed.seq=%+v, error=(%s)", seqB, err)
	}

	return seq, nil
}
func (b *BASE) setTransSeq(stub shim.ChaincodeStubInterface, transSeqKey string, seq int64) *ErrorCodeMsg {
	err := stateCache.PutState_Ex(stub, transSeqKey, []byte(strconv.FormatInt(seq, 10)))
	if err != nil {
		return baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setTransSeq PutState failed.error=(%s)", err)
	}

	return nil
}

func (b *BASE) getTransInfoKey(stub shim.ChaincodeStubInterface, seq int64) string {
	var buf = bytes.NewBufferString(TRANSINFO_PREFIX)
	buf.WriteString(strconv.FormatInt(seq, 10))
	return buf.String()
}

func (b *BASE) getGlobalTransSeqKey(stub shim.ChaincodeStubInterface) string {
	return TRANSSEQ_PREFIX + "global"
}

//获取某个账户的trans seq key
func (b *BASE) getAccTransSeqKey(accName string) string {
	return TRANSSEQ_PREFIX + "acc_" + accName
}

func (b *BASE) setTransInfo(stub shim.ChaincodeStubInterface, info *Transaction) *ErrorCodeMsg {
	//先获取全局seq
	seqGlob, errcm := b.getTransSeq(stub, b.getGlobalTransSeqKey(stub))
	if errcm != nil {
		baselogger.Error("setTransInfo getTransSeq failed.error=(%s)", errcm)
		return errcm
	}
	seqGlob++

	/*
	   //再获取当前交易级别的seq
	   seqLvl, err := b.getTransSeq(stub, b.getTransSeqKey(stub, info.TransLvl))
	   if err != nil {
	       base_logger.Error("setTransInfo getTransSeq failed.error=(%s)", err)
	       return err
	   }
	   seqLvl++
	*/

	info.GlobalSerial = seqGlob
	//info.Serial = seqLvl
	transJson, err := json.Marshal(info)
	if err != nil {
		return baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setTransInfo marshal failed. error=(%s)", err)
	}

	putKey := b.getTransInfoKey(stub, seqGlob)
	err = stateCache.PutState_Ex(stub, putKey, transJson)
	if err != nil {
		return baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setTransInfo PutState failed. error=(%s)", err)
	}

	/*
	   //from和to账户都记录一次，因为两个账户的交易记录只有一条
	   err = b.setOneAccTransInfo(stub, info.FromID, putKey)
	   if err != nil {
	       return base_logger.Errorf("setTransInfo setOneAccTransInfo(%s) failed. error=(%s)", info.FromID, err)
	   }
	   err = b.setOneAccTransInfo(stub, info.ToID, putKey)
	   if err != nil {
	       return base_logger.Errorf("setTransInfo setOneAccTransInfo(%s) failed. error=(%s)", info.ToID, err)
	   }
	*/
	//目前交易记录收入和支出都记录了，所以这里只用记录一次
	errcm = b.setOneAccTransInfo(stub, info.FromID, putKey)
	if errcm != nil {
		return baselogger.ErrorECM(errcm.Code, "setTransInfo setOneAccTransInfo(%s) failed. error=(%s)", info.FromID, errcm)
	}

	//交易信息设置成功后，保存序列号
	errcm = b.setTransSeq(stub, b.getGlobalTransSeqKey(stub), seqGlob)
	if errcm != nil {
		return baselogger.ErrorECM(errcm.Code, "setTransInfo setTransSeq failed. error=(%s)", errcm)
	}
	/*
	   err = b.setTransSeq(stub, b.getTransSeqKey(stub, info.TransLvl), seqLvl)
	   if err != nil {
	       base_logger.Error("setTransInfo setTransSeq failed. error=(%s)", err)
	       return errors.New("setTransInfo setTransSeq failed.")
	   }
	*/

	baselogger.Debug("setTransInfo OK, info=%+v", info)

	return nil
}

func (b *BASE) getOneAccTransInfoKey(accName string, seq int64) string {
	return ONE_ACC_TRANS_PREFIX + accName + "_" + strconv.FormatInt(seq, 10)
}

func (b *BASE) setOneAccTransInfo(stub shim.ChaincodeStubInterface, accName, GlobalTransKey string) *ErrorCodeMsg {

	seq, errcm := b.getTransSeq(stub, b.getAccTransSeqKey(accName))
	if errcm != nil {
		return baselogger.ErrorECM(errcm.Code, "setOneAccTransInfo getTransSeq failed.error=(%s)", errcm)
	}
	seq++

	var key = b.getOneAccTransInfoKey(accName, seq)
	err := stateCache.PutState_Ex(stub, key, []byte(GlobalTransKey))
	if err != nil {
		return baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setOneAccTransInfo PutState failed. error=(%s)", err)
	}

	baselogger.Debug("setOneAccTransInfo key=%s, v=%s", key, GlobalTransKey)

	//交易信息设置成功后，保存序列号
	errcm = b.setTransSeq(stub, b.getAccTransSeqKey(accName), seq)
	if errcm != nil {
		return baselogger.ErrorECM(errcm.Code, "setOneAccTransInfo setTransSeq failed. error=(%s)", errcm)
	}

	return nil
}

func (b *BASE) getOnceTransInfo(stub shim.ChaincodeStubInterface, key string) (*Transaction, *ErrorCodeMsg) {
	var err error
	var trans Transaction

	tmpState, err := stateCache.GetState_Ex(stub, key)
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getTransInfo GetState failed.error=(%s)", err)
	}
	if tmpState == nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "getTransInfo GetState nil.")
	}

	err = json.Unmarshal(tmpState, &trans)
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getTransInfo Unmarshal failed. error=(%s)", err)
	}

	baselogger.Debug("getTransInfo OK, info=%+v", trans)

	return &trans, nil
}
func (b *BASE) getQueryTransInfo(stub shim.ChaincodeStubInterface, key string) (*QueryTransRecd, *ErrorCodeMsg) {
	var err error
	var trans QueryTransRecd

	tmpState, err := stateCache.GetState_Ex(stub, key)
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getQueryTransInfo GetState failed.error=(%s)", err)
	}
	if tmpState == nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "getQueryTransInfo GetState nil.")
	}

	err = json.Unmarshal(tmpState, &trans)
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getQueryTransInfo Unmarshal failed. error=(%s)", err)
	}

	baselogger.Debug("getQueryTransInfo OK, info=%+v", trans)

	return &trans, nil
}

func (b *BASE) dumpWorldState(stub shim.ChaincodeStubInterface, queryTime int64, flushLimit int, needHash bool, currCcid string) ([]byte, *ErrorCodeMsg) {
	//queryTime单位是毫秒
	var timestamp = time.Unix(queryTime/1000, (queryTime-(queryTime/1000*1000))*1000000)
	var outFile = WORLDSTATE_FILE_PREFIX + timestamp.Format("20060102_150405.000") + "_" + currCcid[:8]
	fHandle, err := os.OpenFile(outFile, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getWorldState: OpenFile failed. error=(%s)", err)
	}
	//defer fHandle.Close()  手工close，因为后面要重命名这个文件

	type QueryWorldState struct {
		KeyCount   int64    `json:"keyCount"`
		ErrKeyList []string `json:"errKeyList"`
		GetNextErr bool     `json:"getNextErr"`
		FileName   string   `json:"fileName"`
		FileLine   int64    `json:"fileLine"`
		FileSize   int64    `json:"fileSize"`
		RunTime    string   `json:"runTime"`
	}

	var writer = bufio.NewWriter(fHandle)
	var qws QueryWorldState
	qws.GetNextErr = false

	var begTime = time.Now()
	var flushSize = 0
	keysIter, err := stub.GetStateByRange("", "")
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getWorldState: keys operation failed. Error accessing state: %s", err)
	}
	defer keysIter.Close()

	for keysIter.HasNext() {
		qws.KeyCount++
		kv, iterErr := keysIter.Next()
		if iterErr != nil {
			baselogger.Error("getWorldState: getNext failed, %s", iterErr)
			qws.GetNextErr = true
			continue
		}
		var key = kv.GetKey()
		var valB = kv.GetValue()
		var oneRecd []string

		var valStr = base64.StdEncoding.EncodeToString(valB)

		oneRecd = append(oneRecd, key)
		oneRecd = append(oneRecd, valStr)

		if needHash {
			//对key和value做md5校验
			var hash = md5.New()
			_, err = io.WriteString(hash, key+valStr)
			if err != nil {
				oneRecd = append(oneRecd, INVALID_MD5_VALUE) //计算hash出错，写入INVALID_MD5_VALUE
			} else {
				oneRecd = append(oneRecd, hex.EncodeToString(hash.Sum(nil)))
			}
		}

		jsonRecd, err := json.Marshal(oneRecd)
		if err != nil {
			baselogger.Error("getWorldState: Marshal failed. key=%s, error=(%s)", key, err)
			qws.ErrKeyList = append(qws.ErrKeyList, key)
			continue
		}
		jsonRecd = append(jsonRecd, '\n') //每一个行一个keyValue

		_, err = writer.Write(jsonRecd)
		if err != nil {
			baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getWorldState: Write failed. key=%s, error=(%s)", key, err)
			qws.ErrKeyList = append(qws.ErrKeyList, key)
			continue
		}

		var writeLen = len(jsonRecd)
		flushSize += writeLen

		if flushSize >= flushLimit {
			writer.Flush()
			flushSize = 0
		}

		qws.FileLine++
		qws.FileSize += int64(writeLen)
	}

	writer.Flush()
	fHandle.Close() //注意关闭文件句柄

	var newOutFile = fmt.Sprintf("%s_%d_%d", outFile, qws.FileLine, qws.FileSize)
	os.Rename(outFile, newOutFile)

	var endTime = time.Now()
	var runTime = endTime.Sub(begTime)
	qws.RunTime = runTime.String()
	qws.FileName = newOutFile

	baselogger.Info("getWorldState: result=%+v.", qws)

	retJson, err := json.Marshal(qws)
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getWorldState: Marshal failed. error=(%s)", err)
	}

	return retJson, nil
}

func (b *BASE) loadWorldState(stub shim.ChaincodeStubInterface, fileName string, needHash, sameKeyOverwrite bool, srcCcid string) ([]byte, *ErrorCodeMsg) {
	var inFile = fmt.Sprintf("/home/%s", fileName)
	fHandle, err := os.OpenFile(inFile, os.O_RDONLY, 0755)
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setWorldState: OpenFile failed. error=(%s)", err)
	}
	defer fHandle.Close()

	type SetWorldStateResult struct {
		KeyCount int64  `json:"keyCount"`
		ReadErr  bool   `json:"readErr"`
		FileLine int64  `json:"fileLine"`
		FileSize int64  `json:"fileSize"`
		RunTime  string `json:"runTime"`
	}

	var swsr SetWorldStateResult
	swsr.ReadErr = false

	var reader = bufio.NewReader(fHandle)

	var begTime = time.Now()

	for {
		lineB, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				baselogger.Debug("setWorldState: reader end.")
				break
			}

			swsr.ReadErr = true
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setWorldState: ReadBytes failed. error=(%s)", err)
		}

		swsr.FileLine++
		swsr.FileSize += int64(len(lineB))

		var oneRecd []string
		err = json.Unmarshal(lineB, &oneRecd)
		if err != nil {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setWorldState: Unmarshal failed. line=%s error=(%s)", string(lineB), err)
		}
		if len(oneRecd) < 2 {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "setWorldState: oneRecd format error. oneRecd=%v", oneRecd)
		}
		var key = oneRecd[0]
		var value = oneRecd[1]

		if !sameKeyOverwrite {
			testB, err := stateCache.GetState_Ex(stub, key)
			if err != nil {
				return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setWorldState: GetState failed. key=%s error=(%s)", key, err)
			}
			if testB != nil {
				baselogger.Debug("setWorldState: has key '%s', not Overwrite.", key)
				continue
			}
		}

		if needHash {
			if len(oneRecd) < 3 {
				baselogger.Debug("setWorldState: no hash value, no check.")
			} else {
				var md5val = oneRecd[2]
				if md5val == INVALID_MD5_VALUE {
					baselogger.Debug("setWorldState: hash value is invalid, no check.")
				} else {
					var hash = md5.New()
					_, err = io.WriteString(hash, key+value)
					if err != nil {
						return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setWorldState: md5 create failed. key=%s, error=(%s).", key, err)
					} else {
						var newMd5 = hex.EncodeToString(hash.Sum(nil))
						if md5val != newMd5 {
							return nil, baselogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "setWorldState: md5 check failed. key=%s.", key)
						}
					}
				}
			}
		}

		valueB, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setWorldState: DecodeString failed. value=%s error=(%s)", value, err)
		}

		newKey, newValB, errcm := b.dateConvertWhenLoad(stub, srcCcid, key, valueB)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "setWorldState: dateConvertWhenUpdate failed.  error=(%s)", errcm)
		}

		err = stateCache.PutState_Ex(stub, newKey, newValB)
		if err != nil {
			return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setWorldState: PutState_Ex failed. key=%s error=(%s)", key, err)
		}

		swsr.KeyCount++

		baselogger.Debug("setWorldState: PutState_Ex Ok, key=%s.", key)
	}

	errcm := b.loadAfter(stub, srcCcid)
	if errcm != nil {
		return nil, baselogger.ErrorECM(errcm.Code, "setWorldState: updateAfter failed.  error=(%s)", errcm)
	}

	var endTime = time.Now()
	var runTime = endTime.Sub(begTime)
	swsr.RunTime = runTime.String()

	baselogger.Info("setWorldState: result=%+v.", swsr)

	return nil, nil
}
func (b *BASE) dateConvertWhenLoad(stub shim.ChaincodeStubInterface, srcCcid, key string, valueB []byte) (string, []byte, *ErrorCodeMsg) {
	var errcm *ErrorCodeMsg
	var newKey = key
	var newValB = valueB

	if DateConvertWhenLoadHook != nil {
		newKey, newValB, errcm = DateConvertWhenLoadHook(stub, srcCcid, key, valueB)
		if errcm != nil {
			return "", nil, baselogger.ErrorECM(errcm.Code, "dateConvertWhenUpdate: hook failed. error=(%s)", errcm)
		}
	}

	return newKey, newValB, nil
}
func (b *BASE) loadAfter(stub shim.ChaincodeStubInterface, srcCcid string) *ErrorCodeMsg {

	if DateUpdateAfterLoadHook != nil {
		errcm := DateUpdateAfterLoadHook(stub, srcCcid)
		if errcm != nil {
			return baselogger.ErrorECM(errcm.Code, "loadAfter: hook failed. error=(%s)", errcm)
		}
	}

	return nil
}

func (b *BASE) setAccountLockAmountCfg(stub shim.ChaincodeStubInterface, accName, cfgStr string, overwriteOld bool) (int64, int64, *ErrorCodeMsg) {
	//配置格式如下 "2000:1518407999000;3000:1518407999000..."，防止输入错误，先去除两边的空格，然后再去除两边的';'（防止split出来空字符串）
	var newCfg = strings.Trim(strings.TrimSpace(cfgStr), ";")

	var err error
	var amount int64
	var endtime int64
	var lockedThisTime int64 = 0
	var lockedTotal int64 = 0

	var endtimeAmtList []CoinLockCfg

	//含有";"，表示有多条配置，没有则说明只有一条配置
	var amtEndtimeArr = strings.Split(newCfg, ";")

	for _, ele := range amtEndtimeArr {
		var pair = strings.Split(ele, ":")
		if len(pair) != 2 {
			return 0, 0, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "setAccountLockAmountCfg parse error, '%s' format error 1.", ele)
		}

		amount, err = strconv.ParseInt(pair[0], 0, 64)
		if err != nil {
			return 0, 0, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "setAccountLockAmountCfg parse error, '%s' format error 2.", ele)
		}

		endtime, err = strconv.ParseInt(pair[1], 0, 64)
		if err != nil {
			return 0, 0, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "setAccountLockAmountCfg parse error, '%s' format error 3.", ele)
		}

		lockedThisTime += amount

		//这里要用list来存储，不能用map。map遍历时为随机顺序，会导致下面存储时各个节点的数据不一致
		endtimeAmtList = append(endtimeAmtList, CoinLockCfg{LockEndTime: endtime, LockAmount: amount})
	}

	var acli AccountCoinLockInfo
	var lockinfoKey = b.getAccountLockInfoKey(accName)

	if overwriteOld {
		acli.AccName = accName
	} else {
		acliB, err := stateCache.GetState_Ex(stub, lockinfoKey)
		if err != nil {
			return 0, 0, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setAccountLockAmountCfg: GetState  failed. error=(%s)", err)
		}
		if acliB == nil {
			acli.AccName = accName
		} else {
			err = json.Unmarshal(acliB, &acli)
			if err != nil {
				return 0, 0, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setAccountLockAmountCfg: Unmarshal failed. error=(%s)", err)
			}
		}
	}

	acli.LockList = append(acli.LockList, endtimeAmtList...)

	for _, ele := range acli.LockList {
		lockedTotal += ele.LockAmount
	}

	acliB, err := json.Marshal(acli)
	if err != nil {
		return 0, 0, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setAccountLockAmountCfg: Marshal  failed. error=(%s)", err)
	}
	err = stateCache.PutState_Ex(stub, lockinfoKey, acliB)
	if err != nil {
		return 0, 0, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setAccountLockAmountCfg: putState_Ex  failed. error=(%s)", err)
	}

	baselogger.Debug("setAccountLockAmountCfg: acliB=%s", string(acliB))

	return lockedThisTime, lockedTotal, nil
}

func (b *BASE) getUserEntityKey(userName string) string {
	return USR_ENTITY_PREFIX + userName
}

func (b *BASE) getUserEntity(stub shim.ChaincodeStubInterface, userName string) (*UserEntity, *ErrorCodeMsg) {
	var entB []byte
	var ue UserEntity
	var err error

	entB, err = stateCache.GetState_Ex(stub, b.getUserEntityKey(userName))
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getUserEntity GetState failed. error=(%s)", err)
	}

	if entB == nil {
		return nil, ErrcmNilEntity
	}

	if err = json.Unmarshal(entB, &ue); err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getUserEntity Unmarshal failed. error=(%s)", err)
	}

	return &ue, nil
}

func (b *BASE) setUserEntity(stub shim.ChaincodeStubInterface, ue *UserEntity) *ErrorCodeMsg {
	jsons, err := json.Marshal(ue)
	if err != nil {
		return baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setUserEntity: Marshal failed. error=(%s)", err)
	}

	err = stateCache.PutState_Ex(stub, b.getUserEntityKey(ue.EntID), jsons)
	if err != nil {
		return baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setUserEntity: PutState cb failed. error=(%s)", err)
	}
	return nil
}

func (b *BASE) getSignAndMsg(function string, args []string, signIdx int) ([]byte, []byte, *ErrorCodeMsg) {
	var err error

	var signBase64 = args[signIdx]

	var sign []byte
	sign, err = base64.StdEncoding.DecodeString(signBase64)
	if err != nil {
		return nil, nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getSignAndMsg: convert sign(%s) failed. error=(%s)", signBase64, err)
	}

	//客户端签名的生成： 把函数名和输入的参数用","拼接为字符串，然后计算其Sha256作为msg，然后用私钥对msg做签名。所以这里用同样的方法生成msg
	var allArgsString = function + "," + strings.Join(args[:signIdx], ",") //不包括签名本身，对签名参数以前的参数做签名
	msg := util.ComputeSHA256([]byte(allArgsString))

	baselogger.Debug("allArgsString =%s", allArgsString)
	baselogger.Debug("sign-msg =%v", msg)

	return sign, msg, nil
}

func (b *BASE) getAppRecdKey(appid string) string {
	return APP_INFO_PREFIX + appid
}

func (b *BASE) setAppInfo(stub shim.ChaincodeStubInterface, app *AppInfo) *ErrorCodeMsg {

	appB, err := json.Marshal(app)
	if err != nil {
		return baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Marshal failed. error=(%s)", err)
	}

	err = stateCache.PutState_Ex(stub, b.getAppRecdKey(app.AppID), appB)
	if err != nil {
		return baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "PutState failed. error=(%s)", err)
	}

	return nil
}

func (b *BASE) getAppInfo(stub shim.ChaincodeStubInterface, appid string) (*AppInfo, *ErrorCodeMsg) {
	appB, err := stateCache.GetState_Ex(stub, b.getAppRecdKey(appid))
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "GetState failed. error=(%s)", err)
	}

	if appB == nil {
		return nil, ErrcmNilEntity
	}

	var ai AppInfo
	err = json.Unmarshal(appB, &ai)
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Unmarshal failed. error=(%s)", err)
	}

	return &ai, nil
}

func (b *BASE) isCrossChaincodeCallFlag(flag string) bool {
	return strings.HasPrefix(flag, CROSSCCCALL_PREFIX)
}
func (b *BASE) getCrossChaincodeName(flag string) string {
	return flag[len(CROSSCCCALL_PREFIX):]
}

func (b *BASE) isAppExists(stub shim.ChaincodeStubInterface, appid string) (bool, *ErrorCodeMsg) {
	appB, err := stateCache.GetState_Ex(stub, b.getAppRecdKey(appid))
	if err != nil {
		return false, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "GetState failed. error=(%s)", err)
	}

	if appB != nil {
		return true, nil
	}

	return false, nil
}

func (b *BASE) isAccountSysFunc(function string) bool {
	return strSliceContains(sysFunc, function)
}

func (b *BASE) corssChaincodeCall(stub shim.ChaincodeStubInterface, args [][]byte, chaincodeName, currUserName, currAccountName string, sign, signMsg []byte) ([]byte, *ErrorCodeMsg) {
	baselogger.Debug("before invoke")
	response := stub.InvokeChaincode(chaincodeName, args, "")
	if response.Status != shim.OK {
		baselogger.Error("InvokeChaincode failed, response=%+v.", response)
		//尝试将返回错误信息转为 ErrorCodeMsg
		errcm, err := NewErrorCodeMsgFromString(response.Message)
		if err != nil {
			baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "convert response.Message failed, error=(%s).", err)
			return nil, NewErrorCodeMsg(ERRCODE_COMMON_INNER_ERROR, response.Message)
		}

		return nil, errcm
	}
	baselogger.Debug(" after invoke, payload=%s, payload len=%v", string(response.Payload), len(response.Payload))

	var invokeRslt InvokeResult
	err := json.Unmarshal(response.Payload, &invokeRslt)
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "InvokeChaincode(%s) Unmarshal error, error=(%s).", chaincodeName, err)
	}

	if len(invokeRslt.TransInfos) > 0 {

		var noCurrAccMap = make(map[string]int)
		for _, tx := range invokeRslt.TransInfos {
			//转出账户不是当前账户
			if tx.FromID != currAccountName {
				noCurrAccMap[tx.FromID] = 0
			}
		}
		//转出账户不是当前账户， 校验用户身份，看当前用户是否能操作该账户。  如果是当前账户，已经校验过了
		for noCurrAcc, _ := range noCurrAccMap {
			fromAcc, errcm := b.getAccountEntity(stub, noCurrAcc)
			if errcm != nil {
				return nil, baselogger.ErrorECM(errcm.Code, "InvokeChaincode(%s) transfer failed, get transfer-from account failed, error=(%s).", chaincodeName, errcm)
			}
			if errcm := b.verifyIdentity(stub, currUserName, sign, signMsg, fromAcc, "", ""); errcm != nil {
				return nil, baselogger.ErrorECM(errcm.Code, "InvokeChaincode(%s) transfer failed, verify user and transfer-from account failed, error=(%s).", chaincodeName, errcm)
			}
		}
		for _, tx := range invokeRslt.TransInfos {
			_, errcm := b.transferCoin(stub, tx.FromID, tx.ToID, tx.TransType, tx.Description, tx.Amount, tx.Time, true, tx.AppID)
			if errcm != nil {
				return nil, baselogger.ErrorECM(errcm.Code, "InvokeChaincode(%s) transferCoin error, error=(%s).", chaincodeName, errcm)
			}
		}
	}

	return invokeRslt.Payload, nil
}

type AccountAmount struct {
	UserName   string `json:"user"`
	AccoutName string `json:"acc"`
	Amount     int64  `json:"amt"`
}
type AccountAmountList []AccountAmount

func (c AccountAmountList) Len() int {
	return len(c)
}
func (c AccountAmountList) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}
func (c AccountAmountList) Less(i, j int) bool {
	return c[i].Amount > c[j].Amount
}

func (b *BASE) getAccoutAmountRankingOrTopN(stub shim.ChaincodeStubInterface, userName, accName string, topN int, excludeAcc []string, appid string) (*QueryAccAmtRankAndTopN, *ErrorCodeMsg) {
	var qaart QueryAccAmtRankAndTopN
	qaart.AccoutName = accName
	qaart.RestAmount = 0
	qaart.Ranking = "-"
	qaart.TopN = []TopNData{}

	if len(accName) == 0 && topN <= 0 {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "nothing to do(%s,%d).", accName, topN)
	}

	var errcm *ErrorCodeMsg
	var accEnt *AccountEntity = nil
	if len(accName) > 0 {
		accEnt, errcm = b.getAccountEntity(stub, accName)
		if errcm != nil {
			return nil, baselogger.ErrorECM(errcm.Code, "getAccountEntity(%s) failed. error=(%s)", accName, errcm)
		}
	}

	accsB, err := stateCache.GetState_Ex(stub, ALL_ACC_INFO_KEY)
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "GetState failed. error=(%s)", err)
	}

	var aal AccountAmountList
	var accRanking int64 = 1
	if accsB != nil {
		var allAccs = bytes.NewBuffer(accsB)
		var acc []byte
		var accS string
		var tmpEnt *AccountEntity
		for {
			acc, err = allAccs.ReadBytes(MULTI_STRING_DELIM)
			if err != nil {
				if err == io.EOF {
					break
				} else {
					baselogger.Error("ReadBytes failed. error=(%s)", err)
					continue
				}
			}
			acc = acc[:len(acc)-1] //去掉末尾的分隔符
			accS = string(acc)
			if strSliceContains(excludeAcc, accS) {
				continue
			}

			tmpEnt, errcm = b.getAccountEntity(stub, accS)
			if errcm != nil {
				baselogger.Error("getAccountEntity(%s) failed. error=(%s)", string(acc), errcm)
				continue
			}
			if accEnt != nil && tmpEnt.RestAmount > accEnt.RestAmount {
				accRanking++
			}

			if topN >= 1 {
				aal = append(aal, AccountAmount{UserName: tmpEnt.Owner, AccoutName: accS, Amount: tmpEnt.RestAmount})
			}
		}
	}

	if accEnt != nil {
		qaart.Ranking = strconv.FormatInt(accRanking, 10)
		qaart.RestAmount = accEnt.RestAmount
	}

	if topN >= 1 {
		sort.Sort(aal)
		for idx, aa := range aal {
			var tnd TopNData
			tnd.AccountName = aa.AccoutName
			tnd.RestAmount = aa.Amount
			tnd.Ranking = idx + 1
			userInfo, _ := b.getUserInfo(stub, aa.UserName)
			if userInfo != nil {
				tnd.UserProfilePicture = userInfo.ProfilePicture
				tnd.UserNickname = userInfo.Nickname
			}
			qaart.TopN = append(qaart.TopN, tnd)
			if len(qaart.TopN) >= topN {
				break
			}
		}
	}

	return &qaart, nil
}

func (b *BASE) getUserInfoKey(userName string) string {
	return USR_INFOS_PREFIX + userName
}

func (b *BASE) setUserInfo(stub shim.ChaincodeStubInterface, user *UserInfo) *ErrorCodeMsg {

	userB, err := json.Marshal(user)
	if err != nil {
		return baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Marshal failed. error=(%s)", err)
	}

	err = stateCache.PutState_Ex(stub, b.getUserInfoKey(user.EntID), userB)
	if err != nil {
		return baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "PutState failed. error=(%s)", err)
	}

	return nil
}

func (b *BASE) getUserInfo(stub shim.ChaincodeStubInterface, userName string) (*UserInfo, *ErrorCodeMsg) {
	userB, err := stateCache.GetState_Ex(stub, b.getUserInfoKey(userName))
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "GetState failed. error=(%s)", err)
	}

	if userB == nil {
		return nil, ErrcmNilEntity
	}

	var ui UserInfo
	err = json.Unmarshal(userB, &ui)
	if err != nil {
		return nil, baselogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Unmarshal failed. error=(%s)", err)
	}

	return &ui, nil
}

func (b *BASE) updateUserInfo(stub shim.ChaincodeStubInterface, userName, picture, nickname string) *ErrorCodeMsg {

	var userInfo UserInfo
	pUser, errcm := b.getUserInfo(stub, userName)
	if errcm != nil && errcm != ErrcmNilEntity {
		return baselogger.ErrorECM(errcm.Code, "getUserInfo failed, error=(%s).", errcm)
	}

	if pUser == nil {
		userInfo.EntID = userName
	} else {
		userInfo = *pUser
	}

	userInfo.Nickname = nickname
	userInfo.ProfilePicture = picture

	errcm = b.setUserInfo(stub, &userInfo)
	if errcm != nil {
		return baselogger.ErrorECM(errcm.Code, "setUserInfo failed, error=(%s).", errcm)
	}

	return nil
}

func (b *BASE) isAdmin(stub shim.ChaincodeStubInterface, accName string) bool {
	//获取管理员帐号(央行账户作为管理员帐户)
	tmpByte, err := b.getCenterBankAcc(stub)
	if err != nil {
		baselogger.Error("Query getCenterBankAcc failed. error=(%s)", err)
		return false
	}
	//如果没有央行账户
	if tmpByte == nil {
		baselogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "Query getCenterBankAcc nil.")
		return false
	}

	return string(tmpByte) == accName
}

func main() {
	// for debug
	baselogger.SetDefaultLvl(shim.LogDebug)

	//primitives.SetSecurityLevel("SHA3", 256)

	err := shim.Start(new(BASE))
	if err != nil {
		baselogger.Error("Error starting EventSender chaincode: %s", err)
	}
}
