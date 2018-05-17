package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	//"errors"
	//"fmt"
	"io"
	"math"
	//"sort"
	"crypto/md5"
	"runtime/debug"
	"strconv"
	"strings"

	//"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	//"github.com/hyperledger/fabric/core/crypto/primitives"
	pb "github.com/hyperledger/fabric/protos/peer"
)

const (
	CROSSCHAINCODE_CALL_THIS = true //跨合约调用该合约。一般指从账户系统掉用该合约

	//销售分成相关
	RACK_GLOBAL_ALLOCRATE_KEY = "!kd@globalAllocRate@!" //全局的收入分成比例
	RACK_ALLOCRATE_PREFIX     = "!kd@allocRatePre~"     //每个货架的收入分成比例的key前缀
	RACK_ALLOCTXSEQ_PREFIX    = "!kd@allocTxSeqPre~"    //每个货架的分成记录的序列号的key前缀
	RACK_ALLOC_TX_PREFIX      = "!kd@alloctxPre__"      //每个货架收入分成交易记录
	RACK_ACC_ALLOC_TX_PREFIX  = "!kd@acc_alloctxPre__"  //某个账户收入分成交易记录

	//积分奖励相关
	RACK_SALE_ENC_SCORE_CFG_PREFIX = "!kd@rackSESCPre~" //货架销售奖励积分比例分配配置的key前缀 销售奖励积分，简称SES
	RACK_NEWRACK_ENC_SCORE_DEFAULT = 5000               //新开货架默认奖励的金额

	//货架融资相关
	RACK_FINANCE_CFG_PREFIX    = "!kd@rack_FinacCfgPre~"             //货架融资配置的key前缀
	FINACINFO_PREFIX           = "!kd@rack_FinacInfoPre~"            //理财发行信息的key的前缀。使用的是worldState存储
	RACKINFO_PREFIX            = "!kd@rack_RackInfoPre~"             //货架信息的key的前缀。使用的是worldState存储
	RACKFINACINFO_PREFIX       = "!kd@rack_RackFinacInfoPre~"        //货架融资信息的key的前缀。使用的是worldState存储
	RACKFINACHISTORY_KEY       = "!kd@rack_RackFinacHistoryKey@!"    //货架融资发行的历史信息
	RACKFINACISSUEFINISHID_KEY = "!kd@rack_RackFinacIssueFinIdKey@!" //货架融资发行完毕的期号
	RACK_ACCINVESTINFO_PREFIX  = "!kd@rack_AccInvestInfoPre~"        //账户货架融资信息

	//临时用一下
	ACCOUT_CIPHER_PREFIX = "!kd@accCip~" //每个货架的收入分成比例的key前缀

	RACKFINAC_INVEST = 0 //融资明细中的投资
	RACKFINAC_PROFIT = 1 //融资明细中的收益

	RACK_GLOBAL_CFG_RACK_ID = "_global__rack___" //货架全局配置的id

	MULTI_STRING_DELIM = ':' //多个string的分隔符

	RACK_ROLE_SELLER   = "slr"
	RACK_ROLE_FIELDER  = "fld"
	RACK_ROLE_DELIVERY = "dvy"
	RACK_ROLE_PLATFORM = "pfm"

	ACCOUNT_SYS_CC_NAME          = "accoutsys" //账户系统cc名
	IDENTITY_AUTH_FLAG           = "__identityAuth__"
	GET_ACCOUT_SYS_FCN_AUTH_FLAG = "__getAccountSysFcn__"

	APPID_KEY = "!kd@appid@!"
)

const (
	FINANC_STAGE_INIT         = iota
	FINANC_STAGE_ISSUE_BEGING //理财发行开始
	FINANC_STAGE_ISSUE_FINISH //理财发行结束，即不能再买该期理财
	FINANC_STAGE_BONUS_FINISH //理财分红结束
)

type RolesRate struct {
	SellerRate   int64 `json:"slr"` //经营者分成比例 因为要和int64参与运算，这里都定义为int64
	FielderRate  int64 `json:"fld"` //场地提供者分成比例
	DeliveryRate int64 `json:"dvy"` //送货人分成比例
	PlatformRate int64 `json:"pfm"` //平台分成比例
}

//每个角色分配的数额
type RolesAllocAmount struct {
	SellerAmount   int64 `json:"slrAmt"` //经营者分成比例 因为要和int64参与运算，这里都定义为int64
	FielderAmount  int64 `json:"fldAmt"` //场地提供者分成比例
	DeliveryAmount int64 `json:"dvyAmt"` //送货人分成比例
	PlatformAmount int64 `json:"pfmAmt"` //平台分成比例
}

//货架收入分成比例
type EarningAllocRate struct {
	Rackid     string `json:"rid"`
	UpdateTime int64  `json:"uptm"`
	RolesRate
}

type QueryEarningAllocTx struct {
	Serial int64 `json:"ser"`
	PubEarningAllocTx
}

type PubEarningAllocTx struct {
	Rackid       string                      `json:"rid"`
	AllocKey     string                      `json:"ak"`     //本次分成的key，因为目前invoke操作不能返回分成结果，所以执行分成时，设置这个key，然后在查询时使用这个key来查询
	TotalAmt     int64                       `json:"amt"`    //总金额
	AmountMap    map[string]map[string]int64 `json:"amtmap"` //分成结果 {seller:{usr1:20}, Fielder：{usr2:20} ...}
	GlobalSerial int64                       `json:"gser"`
	DateTime     int64                       `json:"dtm"`
	RolesRate
}

type EarningAllocTx struct {
	PubEarningAllocTx
}

type AllocAccs struct {
	SellerAcc   string `json:"slracc"`
	FielderAcc  string `json:"fldacc"`
	DeliveryAcc string `json:"dvyacc"`
	PlatformAcc string `json:"pfmacc"`
}

type QueryAccEarningAllocTx struct {
	Serail        int64            `json:"ser"`
	AccName       string           `json:"acc"`
	Rackid        string           `json:"rid"`
	RoleAmountMap map[string]int64 `json:"ramap"`
	DateTime      int64            `json:"dtm"`
	TotalAmt      int64            `json:"tamt"` //总金额
	GlobalSerial  int64            `json:"gser"`
	RolesRate
}

//积分奖励比例
type ScoreEncouragePercentCfg struct {
	Rackid      string  `json:"rid"`  //货架id
	UpdateTime  int64   `json:"uptm"` //更新时间
	RangeList   []int64 `json:"rl"`   //区间list
	PercentList []int   `json:"pl"`   //比例list
	/* 0.6的环境 json.Marshal时不支持map[int64]int类型 err=json: unsupported type: map[int64]int.  改为两个数组来存放
	   RangePercentMap map[int64]int `json:"rrm"`  //销售额区间奖励比例  百分比制 {2000:100, 2500:130, 3000:170, 99999999999:200} 表示小于2000奖励销售额的100%，2000-2500奖励130%，2500-3000奖励170%，大于3000小于999999999（一个极大值即可）奖励200%
	*/
}

type RackRolesSales struct {
	Rackid string `json:"rid"`  //货架id
	Sales  int64  `json:"sale"` //销售额
	AllocAccs
}

type RackRolesEncourageScores struct {
	Rackid string `json:"rid"`   //货架id
	Scores int64  `json:"score"` //奖励的积分
	AllocAccs
}

/*----------------- 货架融资 ---------------------------*/
//货架融资相关配置
type PubRackFinanceCfg struct {
	ProfitsPercent       int   `json:"prop"` //货架利润率。本来利润应该是根据实际销售额减去成本来计算，但是目前没有这么精确计算每件商品的销售，先用暂时使用利润率×销售额来计算利润。
	InvestProfitsPercent int   `json:"ivpp"` //投资货架的投资人，利润分成的比例。比如投资人投了1000块钱，货架赚了200，那么百分之几分给投资人，因为货架经营者也要拿一部分利润。
	InvestCapacity       int64 `json:"ivc"`  //货架支撑的投资容量。即货架能支持多少投资
}
type RackFinanceCfg struct {
	Rackid     string `json:"rid"`
	UpdateTime int64  `json:"uptm"`
	PubRackFinanceCfg
}
type FinancialInfo struct {
	FID       string   `json:"fid"`   //发行理财id，每期一个id。可以以年月日为id
	RackList  []string `json:"rlst"`  //本期有多少货架参与融资
	Time      int64    `json:"time"`  //创建时间
	SerialNum int64    `json:"serNo"` //序列号
}

//货架信息 保存在链上
type RackInfo struct {
	RackID    string   `json:"rid"`   //货架id
	FinacList []string `json:"flst"`  //货架参与过哪些融资
	Time      int64    `json:"time"`  //创建时间
	SerialNum int64    `json:"serNo"` //序列号
}

type CostEarnInfo struct {
	//WareCost        int64 `json:"wc"`  //商品成本
	//TransportCost   int64 `json:"tpc"` //运输成本
	//MaintenanceCost int64 `json:"mtc"` //维护成本
	//TraderCost      int64 `json:"tc"`  //零售商成本
	//WareEarning     int64 `json:"we"`  //卖出商品收益
	//BrandEarning    int64 `json:"be"`  //品牌收益
	WareSales int64 `json:"ws"` //商品销售额
}

//货架融资信息 保存在链上
type RackFinancInfo struct {
	RackID             string            `json:"rid"`  //货架id
	FID                string            `json:"fid"`  //发行理财id
	DataTime           int64             `json:"dt"`   //创建时间
	SerialNum          int64             `json:"ser"`  //序列号
	AmountFinca        int64             `json:"amtf"` //实际投资额度
	CEInfo             CostEarnInfo      `json:"cei"`  //成本及收益
	RFCfg              PubRackFinanceCfg `json:"rfc"`
	RolesAllocRate     RolesRate         `json:"rar"`
	UserAmountMap      map[string]int64  `json:"uamp"` //每个用户投资的金额（包括新买的和续期的）
	UserProfitMap      map[string]int64  `json:"upmp"` //每个用户收益的金额
	UserRenewalMap     map[string]int64  `json:"urmp"` //每个用户续期的金额
	Stage              int               `json:"stg"`  //处于什么阶段
	PayFinanceUserList []string          `json:"pful"` //退出投资的用户列表
	/*
	   如果用户本期未提取的投资，本金会自动转到下期（但是这个结构中的金额是不动的），所以每个用户的所有本金
	   需要从最新的的理财中获取， 而收益从历史的每一次投资获取。
	*/
}

type RackFinancHistory struct {
	PreCurrFID [2]string `json:"pcfid"` //前一次和本次的融资id  第一个位置为前一期融资id，第二个位置为本期融资id
}

//货架融资信息
type AccRackInvest struct {
	EntID       string         `json:"id"`   //银行/企业/项目/个人ID
	RFInfoMap   map[string]int `json:"rfim"` //用户参与投资的货架融资信息，保存RackFinancInfo的两个key，rackid和financeId。用map是因为容易删除某个元素，因为用户提取积分后，会删除这两个key。map的value无意义。
	LatestFid   string         `json:"lfid"` //用户购买的最新一期的理财
	PaidFidList []string       `json:"pfl"`  //用户已经赎回的理财期号。
}

type QueryFinac struct {
	FinancialInfo
	RFInfoList []RackFinancInfo `json:"rfList"`
}
type QueryRack struct {
	RackInfo
	RFInfoList []RackFinancInfo `json:"rfList"`
}

//查询用，不记入链
type QueryRackFinanceTx struct {
	NextSerial     int64             `json:"nextser"` //因为是批量返回结果，表示下次要请求的序列号
	FinanceRecords []RackFinanceRecd `json:"records"`
}

//查询用，不记入链
type RackFinanceRecd struct {
	RackId  string `json:"rid"`
	FId     string `json:"fid"`
	AccName string `json:"acc"`
	Amount  string `json:"amt"`
	Type    string `json:"type"` //投资、收益
}

type InvokeArgs struct {
	FixedArgCount int
	UserName      string
	AccountName   string
	InvokeTime    int64
}

var kdlogger = NewMylogger("kd")
var kdCrypto = MyCryptoNew()

var stateCache StateWorldCache

type KD struct {
}

//包初始化函数
func init() {
	/*
		var kd KD
		//注册base中的hook函数
		InitHook = kd.Init
		InvokeHook = kd.Invoke
		DateConvertWhenLoadHook = kd.dateConvertWhenLoad
		DateUpdateAfterLoadHook = kd.loadAfter
	*/
}

func (t *KD) Init(stub shim.ChaincodeStubInterface) (pbResponse pb.Response) {
	function, _ := stub.GetFunctionAndParameters()
	defer func() {
		if excption := recover(); excption != nil {
			pbResponse = shim.Error(kdlogger.SError("Init(%s) got an unexpect error:%s", function, excption))
			kdlogger.Critical("Init got exception, stack:\n%s", string(debug.Stack()))
		}
	}()

	retBytes, errcm := t.__Init(stub)
	if errcm != nil {
		return shim.Error(errcm.toJson())
	}

	return shim.Success(retBytes)
}

func (t *KD) __Init(stub shim.ChaincodeStubInterface) ([]byte, *ErrorCodeMsg) {
	kdlogger.Debug("Enter Init")
	function, args := stub.GetFunctionAndParameters()
	//init函数属于非常规操作，记录日志
	kdlogger.Info("func =%s, args = %+v", function, args)

	stateCache.Create(stub)
	defer func() {
		stateCache.Destroy(stub)
	}()

	/*
		defer func() {
			if excption := recover(); excption != nil {
				pbResponse = shim.Error(baselogger.SError("Init(%s) got an unexpect error=(%s)", function, excption))
				kdlogger.Critical("Init got exception, stack:\n%s", string(debug.Stack()))
			}
		}()
	*/

	//目前不需要参数
	var fixedArgCount = 0
	if len(args) < fixedArgCount {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Init miss arg, got %d, at least need %d.", len(args), fixedArgCount)
	}
	timestamp, err := stub.GetTxTimestamp()
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Init: GetTxTimestamp failed, error=(%s)", err)
	}

	var initTime = timestamp.Seconds*1000 + int64(timestamp.Nanos/1000000) //精确到毫秒

	if function == "init" {

		/* 这里不输入当前时间参数，因为fabic0.6版本，如果init输入了变量参数，每次deploy出来的chainCodeId不一致。
		   var argCount = 1
		   if len(args) < argCount {
		       return nil, mylog.Errorf("Init miss arg, got %d, at least need %d.", len(args), argCount)
		   }

		       times, err := strconv.ParseInt(args[0], 0, 64)
		       if err != nil {
		           return nil, mylog.Errorf("Invoke convert times(%s) failed. error=(%s)", args[0], err)
		       }
		*/
		//全局分成比例设置
		var eap EarningAllocRate
		eap.Rackid = RACK_GLOBAL_CFG_RACK_ID //全局比例
		eap.PlatformRate = 3                 //3%
		eap.FielderRate = 3                  //3%
		eap.DeliveryRate = 2                 //2%
		eap.SellerRate = 92                  //92%
		//eap.UpdateTime = times
		eap.UpdateTime = initTime

		eapJson, err := json.Marshal(eap)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Init Marshal error, error=(%s).", err)
		}

		err = stateCache.PutState_Ex(stub, t.getGlobalRackAllocRateKey(), eapJson)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Init PutState_Ex error, error=(%s).", err)
		}

		//全局销售额区间奖励积分设置
		var serc ScoreEncouragePercentCfg
		serc.Rackid = RACK_GLOBAL_CFG_RACK_ID //全局比例
		serc.UpdateTime = initTime
		/*
		   serc.RangePercentMap = make(map[int64]int)
		   serc.RangePercentMap[2000] = 100
		   serc.RangePercentMap[2500] = 130
		   serc.RangePercentMap[3000] = 170
		   serc.RangePercentMap[math.MaxInt64] = 200
		*/
		serc.RangeList = []int64{2000, 2500, 3000, math.MaxInt64}
		serc.PercentList = []int{100, 130, 170, 200}

		sercJson, err := json.Marshal(serc)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Init Marshal(serc) error, error=(%s).", err)
		}

		err = stateCache.PutState_Ex(stub, t.getGlobalRackEncourageScoreCfgKey(), sercJson)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Init PutState_Ex(serc) error, error=(%s).", err)
		}

		var rfc RackFinanceCfg
		rfc.Rackid = RACK_GLOBAL_CFG_RACK_ID //全局
		rfc.UpdateTime = initTime
		rfc.ProfitsPercent = 20       //20%的利润率
		rfc.InvestProfitsPercent = 90 //90%的利润分给投资人
		rfc.InvestCapacity = 2000     //目前是积分投资，单位为积分的单位

		rfcJson, err := json.Marshal(rfc)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Init Marshal(rfc) error, error=(%s).", err)
		}

		err = stateCache.PutState_Ex(stub, t.getGlobalRackFinancCfgKey(), rfcJson)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Init PutState_Ex(rfc) error, error=(%s).", err)
		}

		return nil, nil
	} else if function == "upgrade" {

		return nil, nil
	} else {

		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "unkonwn function '%s'", function)
	}
}

var transInfoCache = NewTransferInfoCache()

func (t *KD) Invoke(stub shim.ChaincodeStubInterface) (pbResponse pb.Response) {
	function, _ := stub.GetFunctionAndParameters()
	defer func() {
		if excption := recover(); excption != nil {
			pbResponse = shim.Error(kdlogger.SError("Invoke(%s) got an unexpect error:%s", function, excption))
			kdlogger.Critical("Invoke got exception, stack:\n%s", string(debug.Stack()))
		}
	}()

	//每次invoke必须初始化
	transInfoCache.Create(stub)
	defer func() {
		transInfoCache.Destroy(stub)
	}()

	payload, errcm := t.__Invoke(stub)
	if errcm != nil {
		return shim.Error(errcm.toJson())
	}

	if CROSSCHAINCODE_CALL_THIS {
		var invokeRslt InvokeResult
		invokeRslt.TransInfos = transInfoCache.Get(stub)
		invokeRslt.Payload = payload

		invokeRsltB, err := json.Marshal(invokeRslt)
		if err != nil {
			return shim.Error(kdlogger.SError("Invoke(%s) marshal invokeResult failed, error=(%s)", function, err))
		}

		kdlogger.Debug("invokeRsltB=%s len=%v", string(invokeRsltB), len(invokeRsltB))

		return shim.Success(invokeRsltB)
	}

	return shim.Success(payload)

}

// Transaction makes payment of X units from A to B
func (t *KD) __Invoke(stub shim.ChaincodeStubInterface) ([]byte, *ErrorCodeMsg) {
	kdlogger.Debug("Enter Invoke")
	function, args := stub.GetFunctionAndParameters()
	kdlogger.Debug("func =%s, args = %+v", function, args)

	stateCache.Create(stub)
	defer func() {
		stateCache.Destroy(stub)
	}()

	//第一个参数为用户名，第二个参数为账户名， 第三个...  最后一个元素是用户签名，实际情况中，可以根据业务需求调整这个最小参数个数。
	var fixedArgCount = 2
	//最后一个参数为签名，所以参数必须大于fixedArgCount个
	if len(args) < fixedArgCount+1 {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke miss arg, got %d, at least need %d.", len(args), fixedArgCount+1)
	}

	var userName = args[0]
	var accName = args[1]
	timestamp, err := stub.GetTxTimestamp()
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Init: GetTxTimestamp failed, error=(%s)", err)
	}

	var invokeTime = timestamp.Seconds*1000 + int64(timestamp.Nanos/1000000) //精确到毫秒

	var kia InvokeArgs
	kia.FixedArgCount = fixedArgCount
	kia.AccountName = accName
	kia.UserName = userName
	kia.InvokeTime = invokeTime

	//记录一下appid 方便后续使用
	if function == "saveAppid" {
		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(saveAppid) miss arg, got %d, need %d.", len(args), argCount)
		}

		var appid = args[fixedArgCount]
		kdlogger.Debug("saveAppid: appid=%s", appid)

		err = stateCache.PutState_Ex(stub, APPID_KEY, []byte(appid))
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Invoke(saveAppid) save appid failed, error=(%s).", err)
		}

		return nil, nil
	} else if function == "transefer2" { //带交易密码功能
		var argCount = fixedArgCount + 4
		if len(args) < argCount {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(transeferUsePwd) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		var toAcc = args[fixedArgCount]

		var transAmount int64
		transAmount, err = strconv.ParseInt(args[fixedArgCount+1], 0, 64)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Invoke(transeferUsePwd): convert issueAmount(%s) failed. error=(%s)", args[fixedArgCount+1], err)
		}
		kdlogger.Debug("transAmount= %+v", transAmount)

		var pwdBase64 = args[fixedArgCount+2]
		pwd, errcm := t.decodeAccountPasswd(pwdBase64)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "Invoke(resetAccPwd): decodeAccountPasswd (%s) failed. error=(%s)", pwdBase64, errcm)
		}
		kdlogger.Debug("Invoke(transeferUsePwd): pwd=%s", pwd)

		//var appid = args[fixedArgCount+3]

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

		//验证密码
		setPwd, errcm := t.isSetAccountPasswd(stub, accName)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "Invoke(transeferUsePwd): IsSetAccountPasswd failed. error=(%s), acc=%s", errcm, accName)
		}
		kdlogger.Debug("Invoke(transeferUsePwd): setPwd=%v", setPwd)
		if setPwd {
			ok, errcm := t.authAccountPasswd(stub, accName, pwd)
			if errcm != nil {
				return nil, kdlogger.ErrorECM(errcm.Code, "Invoke(transeferUsePwd): AuthAccountPasswd failed. error=(%s)", errcm)
			}
			if !ok {
				return nil, kdlogger.ErrorECM(ERRCODE_COMMON_IDENTITY_VERIFY_FAILED, "Invoke(transeferUsePwd): passwd invalid.")
			}
		} else {

			errcm := t.setAccountPasswd(stub, accName, pwd)
			if errcm != nil {
				return nil, kdlogger.ErrorECM(errcm.Code, "Invoke(transeferUsePwd): setAccountPasswd failed. error=(%s)", errcm)
			}
		}

		_, errcm = t.transferCoin(stub, accName, toAcc, transType, description, transAmount, invokeTime, sameEntSaveTransFlag)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "Invoke(transeferUsePwd) transferCoin2 failed. error=(%s)", errcm)
		}
		return nil, nil

	} else if function == "setAllocCfg" {
		if !t.isAdmin(stub, accName) {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "Invoke(setAllocCfg) can't exec by %s.", accName)
		}

		var argCount = fixedArgCount + 5
		if len(args) < argCount {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(setAllocCfg) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		rackid := args[fixedArgCount]

		seller, err := strconv.ParseInt(args[fixedArgCount+1], 0, 64)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Invoke(setAllocCfg) convert seller(%s) failed. error=(%s)", args[fixedArgCount+1], err)
		}
		fielder, err := strconv.ParseInt(args[fixedArgCount+2], 0, 64)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Invoke(setAllocCfg) convert fielder(%s) failed. error=(%s)", args[fixedArgCount+2], err)
		}
		delivery, err := strconv.ParseInt(args[fixedArgCount+3], 0, 64)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Invoke(setAllocCfg) convert delivery(%s) failed. error=(%s)", args[fixedArgCount+3], err)
		}
		platform, err := strconv.ParseInt(args[fixedArgCount+4], 0, 64)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Invoke(setAllocCfg) convert platform(%s) failed. error=(%s)", args[fixedArgCount+4], err)
		}

		var eap EarningAllocRate

		eap.Rackid = rackid
		if rackid == "*" {
			eap.Rackid = RACK_GLOBAL_CFG_RACK_ID
		}
		eap.SellerRate = seller
		eap.FielderRate = fielder
		eap.DeliveryRate = delivery
		eap.PlatformRate = platform
		eap.UpdateTime = invokeTime

		eapJson, err := json.Marshal(eap)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Invoke(setAllocCfg) Marshal error, error=(%s).", err)
		}

		var stateKey string
		if rackid == "*" {
			stateKey = t.getGlobalRackAllocRateKey()
		} else {
			stateKey = t.getRackAllocRateKey(rackid)
		}

		err = stateCache.PutState_Ex(stub, stateKey, eapJson)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Invoke(setAllocCfg) PutState_Ex error, error=(%s).", err)
		}

		return nil, nil

	} else if function == "allocEarning" {
		if !t.isAdmin(stub, accName) {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "Invoke(allocEarning) can't exec by %s.", accName)
		}

		var argCount = fixedArgCount + 7
		if len(args) < argCount {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(allocEarning) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		rackid := args[fixedArgCount]
		sellerAcc := args[fixedArgCount+1]
		fielderAcc := args[fixedArgCount+2]
		deliveryAcc := args[fixedArgCount+3]
		platformAcc := args[fixedArgCount+4]
		allocKey := args[fixedArgCount+5]

		var totalAmt int64
		totalAmt, err = strconv.ParseInt(args[fixedArgCount+6], 0, 64)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Invoke(allocEarning) convert totalAmt(%s) failed. error=(%s)", args[fixedArgCount+6], err)
		}

		var eap EarningAllocRate

		eapB, err := stateCache.GetState_Ex(stub, t.getRackAllocRateKey(rackid))
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Invoke(allocEarning) GetState(rackid=%s) failed. error=(%s)", rackid, err)
		}
		if eapB == nil {
			kdlogger.Warn("Invoke(allocEarning) GetState(rackid=%s) nil, try to get global.", rackid)
			//没有为该货架单独配置，返回global配置
			eapB, err = stateCache.GetState_Ex(stub, t.getGlobalRackAllocRateKey())
			if err != nil {
				return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Invoke(allocEarning) GetState(global, rackid=%s) failed. error=(%s)", rackid, err)
			}
			if eapB == nil {
				return nil, kdlogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "Invoke(allocEarning) GetState(global, rackid=%s) nil.", rackid)
			}
		}

		err = json.Unmarshal(eapB, &eap)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Invoke(allocEarning) Unmarshal failed. error=(%s)", err)
		}

		var accs AllocAccs
		accs.SellerAcc = sellerAcc
		accs.FielderAcc = fielderAcc
		accs.DeliveryAcc = deliveryAcc
		accs.PlatformAcc = platformAcc

		_, errcm := t.setAllocEarnTx(stub, rackid, allocKey, totalAmt, &accs, &eap, invokeTime)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "Invoke(allocEarning) setAllocEarnTx failed. error=(%s)", errcm)
		}
		return nil, nil

	} else if function == "setSESCfg" { //设置每个货架的销售额奖励区间比例
		if !t.isAdmin(stub, accName) {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "Invoke(setSESCfg) can't exec by %s.", accName)
		}

		var argCount = fixedArgCount + 2
		if len(args) < argCount {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(setSESCfg) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		var rackid = args[fixedArgCount]
		var cfgStr = args[fixedArgCount+1]

		_, errcm := t.setRackEncourageScoreCfg(stub, rackid, cfgStr, invokeTime)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "Invoke(setSESCfg) setRackEncourageScoreCfg failed. error=(%s)", errcm)
		}
		return nil, nil

	} else if function == "encourageScoreForSales" { //根据销售额奖励积分
		var argCount = fixedArgCount + 4
		if len(args) < argCount {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(encourageScoreForSales) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		var paraStr = args[fixedArgCount]
		var transType = args[fixedArgCount+1]
		var transDesc = args[fixedArgCount+2]
		var sameEntSaveTrans = args[fixedArgCount+3] //如果转出和转入账户相同，是否记录交易 0表示不记录 1表示记录
		var sameEntSaveTransFlag bool = true
		if sameEntSaveTrans != "1" {
			sameEntSaveTransFlag = false
		}

		//使用登录的账户进行转账
		_, errcm := t.allocEncourageScoreForSales(stub, paraStr, accName, transType, transDesc, invokeTime, sameEntSaveTransFlag)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "Invoke(encourageScoreForSales) allocEncourageScoreForSales failed. error=(%s)", errcm)
		}
		return nil, nil

	} else if function == "encourageScoreForNewRack" { //新开货架奖励积分
		var argCount = fixedArgCount + 4
		if len(args) < argCount {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(encourageScoreForNewRack) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		var paraStr = args[fixedArgCount]
		var transType = args[fixedArgCount+1]
		var transDesc = args[fixedArgCount+2]
		var sameEntSaveTrans = args[fixedArgCount+3] //如果转出和转入账户相同，是否记录交易 0表示不记录 1表示记录
		var sameEntSaveTransFlag bool = true
		if sameEntSaveTrans != "1" {
			sameEntSaveTransFlag = false
		}

		//使用登录的账户进行转账
		_, errcm := t.allocEncourageScoreForNewRack(stub, paraStr, accName, transType, transDesc, invokeTime, sameEntSaveTransFlag)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "Invoke(encourageScoreForNewRack) allocEncourageScoreForNewRack failed. error=(%s)", errcm)
		}
		return nil, nil

	} else if function == "setFinanceCfg" {
		if !t.isAdmin(stub, accName) {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "Invoke(setFinanceCfg) can't exec by %s.", accName)
		}

		var argCount = fixedArgCount + 4
		if len(args) < argCount {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(setFinanceCfg) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		var rackid = args[fixedArgCount]
		var profitsPercent int
		var investProfitsPercent int
		var investCapacity int64

		profitsPercent, err = strconv.Atoi(args[fixedArgCount+1])
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Invoke(setFinanceCfg) convert profitsPercent(%s) failed. error=(%s)", args[fixedArgCount+1], err)
		}
		investProfitsPercent, err = strconv.Atoi(args[fixedArgCount+2])
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Invoke(setFinanceCfg) convert investProfitsPercent(%s) failed. error=(%s)", args[fixedArgCount+2], err)
		}
		investCapacity, err = strconv.ParseInt(args[fixedArgCount+3], 0, 64)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Invoke(setFinanceCfg) convert investCapacity(%s) failed. error=(%s)", args[fixedArgCount+3], err)
		}

		var rfc RackFinanceCfg
		rfc.Rackid = rackid
		if rackid == "*" {
			rfc.Rackid = RACK_GLOBAL_CFG_RACK_ID
		}
		rfc.ProfitsPercent = profitsPercent             //x%的利润率
		rfc.InvestProfitsPercent = investProfitsPercent //x%的利润分给投资人
		rfc.InvestCapacity = investCapacity
		rfc.UpdateTime = invokeTime

		rfcJson, err := json.Marshal(rfc)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Invoke(setFinanceCfg) Marshal(rfc) error, error=(%s).", err)
		}

		var stateKey string
		if rackid == "*" {
			stateKey = t.getGlobalRackFinancCfgKey()
		} else {
			stateKey = t.getRackFinancCfgKey(rackid)
		}

		err = stateCache.PutState_Ex(stub, stateKey, rfcJson)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "IInvoke(setFinanceCfg) PutState_Ex(rfc) error, error=(%s).", err)
		}

		return nil, nil

	} else if function == "buyFinance" {
		var argCount = fixedArgCount + 7
		if len(args) < argCount {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(buyFinancial) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		var rackid = args[fixedArgCount]
		var financid = args[fixedArgCount+1]
		var payee = args[fixedArgCount+2]
		var amount int64
		amount, err = strconv.ParseInt(args[fixedArgCount+3], 0, 64)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "Invoke(buyFinancial) convert amount(%s) failed. error=(%s)", args[fixedArgCount+3], err)
		}

		var transType = args[fixedArgCount+4]
		var transDesc = args[fixedArgCount+5]
		var sameEntSaveTrans = args[fixedArgCount+6] //如果转出和转入账户相同，是否记录交易 0表示不记录 1表示记录
		var sameEntSaveTransFlag bool = true
		if sameEntSaveTrans != "1" {
			sameEntSaveTransFlag = false
		}

		//每次购买时，肯定是购买最新一期的理财，设置为当前的fid
		errcm := t.setCurrentFid(stub, financid)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "Invoke(buyFinancial) setCurrentFid failed, error=(%s).", errcm)
		}

		//使用登录的账户进行转账
		_, errcm = t.userBuyFinance(stub, accName, rackid, financid, payee, transType, transDesc, amount, invokeTime, sameEntSaveTransFlag, false)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "Invoke(buyFinancial) userBuyFinance failed. error=(%s)", errcm)
		}
		return nil, nil

	} else if function == "financeIssueFinish" {
		if !t.isAdmin(stub, accName) {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "Invoke(financeIssueFinish) can't exec by %s.", accName)
		}

		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(financeIssueFinish) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		var financid = args[fixedArgCount]

		//理财结束时，肯定是最新一期的理财，设置为当前的fid
		errcm := t.setCurrentFid(stub, financid)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "Invoke(financeIssueFinish) setCurrentFid failed, error=(%s).", errcm)
		}

		errcm = t.financeIssueFinishAfter(stub, financid, invokeTime)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "Invoke(financeIssueFinishAfter) financeRenewal failed, error=(%s).", errcm)
		}

		return nil, nil

	} else if function == "payFinance" {
		var argCount = fixedArgCount + 5
		if len(args) < argCount {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(payFinance) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		var rackid = args[fixedArgCount]
		var reacc = args[fixedArgCount+1]
		var transType = args[fixedArgCount+2]
		var transDesc = args[fixedArgCount+3]
		var sameEntSaveTrans = args[fixedArgCount+4] //如果转出和转入账户相同，是否记录交易 0表示不记录 1表示记录
		var sameEntSaveTransFlag bool = true
		if sameEntSaveTrans != "1" {
			sameEntSaveTransFlag = false
		}

		errcm := t.payUserFinance(stub, accName, reacc, rackid, invokeTime, transType, transDesc, sameEntSaveTransFlag)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "Invoke(payFinance) payUserFinance failed, error=(%s).", errcm)
		}

		return nil, nil

	} else if function == "financeBouns" {
		if !t.isAdmin(stub, accName) {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "Invoke(financeBouns) can't exec by %s.", accName)
		}

		var argCount = fixedArgCount + 2
		if len(args) < argCount {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(financeBouns) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		var fid = args[fixedArgCount]
		var rackSalesCfg = args[fixedArgCount+1]
		_, errcm := t.financeBonus(stub, fid, rackSalesCfg, invokeTime)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "Invoke(financeBouns) financeBonus failed. error=(%s)", errcm)
		}
		return nil, nil

	} else if function == "setAccCfg1" { //设置交易密码
		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(setAccPwd) miss arg, got %d, need %d.", len(args), argCount)
		}

		setPwd, errcm := t.isSetAccountPasswd(stub, accName)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "Invoke(setAccPwd): IsSetAccountPasswd failed. error=(%s), acc=%s", errcm, accName)
		}
		//如果已设置，则报错
		if setPwd {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "Invoke(setAccPwd): pwd is setted, do nothing, acc=%s", accName)
		}

		var pwdBase64 = args[fixedArgCount]
		pwd, errcm := t.decodeAccountPasswd(pwdBase64)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "Invoke(setAccPwd): decodeAccountPasswd (%s) failed. error=(%s)", pwdBase64, errcm)
		}
		kdlogger.Debug("Invoke(setAccPwd): pwd=%s", pwd)

		errcm = t.setAccountPasswd(stub, accName, pwd)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "Invoke(setAccPwd) setAccountPasswd failed. error=(%s)", errcm)
		}
		return nil, nil

	} else if function == "setAccCfg2" { //重置交易密码
		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(resetAccPwd) miss arg, got %d, need %d.", len(args), argCount)
		}

		var pwdBase64 = args[fixedArgCount]
		pwd, errcm := t.decodeAccountPasswd(pwdBase64)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "Invoke(resetAccPwd): decodeAccountPasswd (%s) failed. error=(%s)", pwdBase64, errcm)
		}
		kdlogger.Debug("Invoke(resetAccPwd): pwd=%s", pwd)

		errcm = t.setAccountPasswd(stub, accName, pwd)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "Invoke(resetAccPwd) setAccountPasswd failed. error=(%s)", errcm)
		}
		return nil, nil

	} else if function == "setAccCfg3" { //修改交易密码
		var argCount = fixedArgCount + 2
		if len(args) < argCount {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(chgAccPwd) miss arg, got %d, need %d.", len(args), argCount)
		}

		var errcm *ErrorCodeMsg
		var oldpwd, newpwd string
		var oldpwdBase64 = args[fixedArgCount]
		var newpwdBase64 = args[fixedArgCount+1]

		oldpwd, errcm = t.decodeAccountPasswd(oldpwdBase64)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "Invoke(chgAccPwd): decodeAccountPasswd o(%s) failed. error=(%s)", oldpwdBase64, errcm)
		}
		newpwd, errcm = t.decodeAccountPasswd(newpwdBase64)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "Invoke(chgAccPwd): decodeAccountPasswd n(%s) failed. error=(%s)", newpwdBase64, errcm)
		}

		kdlogger.Debug("Invoke(chgAccPwd): opwd=%s", oldpwd)
		kdlogger.Debug("Invoke(chgAccPwd): npwd=%s", newpwd)

		errcm = t.changeAccountPasswd(stub, accName, oldpwd, newpwd)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "Invoke(chgAccPwd) changeAccountPasswd failed. error=(%s)", errcm)
		}
		return nil, nil

	} else if function == "updateEnv" {
		var argCount = fixedArgCount + 2
		if len(args) < argCount {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Invoke(updateEnv) miss arg, got %d, at least need %d.", len(args), argCount)
		}
		key := args[fixedArgCount]
		value := args[fixedArgCount+1]

		if key == "logLevel" {
			lvl, _ := strconv.Atoi(value)
			// debug=5, info=4, notice=3, warning=2, error=1, critical=0
			var loggerSet = kdlogger.GetLoggers()
			for _, l := range loggerSet {
				l.SetDefaultLvl(shim.LoggingLevel(lvl))
			}

			kdlogger.Info("set logLevel to %d.", lvl)
		}

		return nil, nil

	} else {
		//其它函数看是否是query函数
		return t.__Query(stub, &kia)
	}
}

// Query callback representing the query of a chaincode
func (t *KD) __Query(stub shim.ChaincodeStubInterface, ifas *InvokeArgs) ([]byte, *ErrorCodeMsg) {
	kdlogger.Debug("Enter Query")
	function, args := stub.GetFunctionAndParameters()
	kdlogger.Debug("func =%s, args = %+v", function, args)


	var fixedArgCount = ifas.FixedArgCount
	if len(args) < fixedArgCount {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "Query miss arg, got %d, at least need %d.", len(args), fixedArgCount)
	}

	//var userName = ifas.UserName
	var accName = ifas.AccountName
	//var queryTime int64 = ifas.InvokeTime

	if function == "queryRackAlloc" {

		var argCount = fixedArgCount + 7
		if len(args) < argCount {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "queryRackAlloc miss arg, got %d, need %d.", len(args), argCount)
		}

		var rackid string
		var allocKey string
		var begSeq int64
		var txCount int64
		var begTime int64
		var endTime int64
		var txAcc string
        var err error

		rackid = args[fixedArgCount]
		allocKey = args[fixedArgCount+1]

		begSeq, err = strconv.ParseInt(args[fixedArgCount+2], 0, 64)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "queryRackAlloc ParseInt for begSeq(%s) failed. error=(%s)", args[fixedArgCount+2], err)
		}
		txCount, err = strconv.ParseInt(args[fixedArgCount+3], 0, 64)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "queryRackAlloc ParseInt for txCount(%s) failed. error=(%s)", args[fixedArgCount+3], err)
		}

		begTime, err = strconv.ParseInt(args[fixedArgCount+4], 0, 64)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "queryRackAlloc ParseInt for begTime(%s) failed. error=(%s)", args[fixedArgCount+4], err)
		}
		endTime, err = strconv.ParseInt(args[fixedArgCount+5], 0, 64)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "queryRackAlloc ParseInt for endTime(%s) failed. error=(%s)", args[fixedArgCount+5], err)
		}
		txAcc = args[fixedArgCount+6]

		if len(allocKey) > 0 {
			//是否是管理员帐户，管理员用户才可以查
			if !t.isAdmin(stub, accName) {
				return nil, kdlogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "queryRackAlloc: %s can't query allocKey.", accName)
			}

			//查询某一次的分配情况（由allocKey检索）
			retValue, errcm := t.getAllocTxRecdByKey(stub, rackid, allocKey)
			if errcm != nil {
				return nil, kdlogger.ErrorECM(errcm.Code, "queryRackAlloc: getAllocTxRecdByKey failed. error=(%s)", errcm)
			}
			return retValue, nil
		} else {
			if t.isAdmin(stub, accName) {
				if len(txAcc) > 0 {
					//查询某一个账户的分配情况
					retValue, errcm := t.getOneAccAllocTxRecds(stub, txAcc, begSeq, txCount, begTime, endTime)
					if errcm != nil {
						return nil, kdlogger.ErrorECM(errcm.Code, "queryRackAlloc: getOneAccAllocTxRecds failed. error=(%s)", errcm)
					}
					return retValue, nil
				} else {
					//查询某一个货架的分配情况
					retValue, errcm := t.getAllocTxRecds(stub, rackid, begSeq, txCount, begTime, endTime)
					if errcm != nil {
						return nil, kdlogger.ErrorECM(errcm.Code, "queryRackAlloc: getAllocTxRecds failed. error=(%s)", errcm)
					}
					return retValue, nil
				}
			} else {
				//非管理员账户，只能查询自己的交易记录，忽略txAcc参数
				retValue, errcm := t.getOneAccAllocTxRecds(stub, accName, begSeq, txCount, begTime, endTime)
				if errcm != nil {
					return nil, kdlogger.ErrorECM(errcm.Code, "queryRackAlloc: getOneAccAllocTxRecds2 failed. error=(%s)", errcm)
				}
				return retValue, nil
			}
		}

		return nil, nil

	} else if function == "getRackAllocCfg" {
		if !t.isAdmin(stub, accName) {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "getRackAllocCfg: %s can't query.", accName)
		}

		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "getRackAllocCfg miss arg, got %d, need %d.", len(args), argCount)
		}

		var rackid = args[fixedArgCount]

		eapB, errcm := t.getRackAllocCfg(stub, rackid, nil)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "getRackAllocCfg getRackAllocCfg(rackid=%s) failed. error=(%s)", rackid, errcm)
		}

		return eapB, nil

	} else if function == "getSESCfg" {
		if !t.isAdmin(stub, accName) {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "getSESCfg: %s can't query.", accName)
		}

		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "getSESCfg miss arg, got %d, need %d.", len(args), argCount)
		}

		var rackid = args[fixedArgCount]

		sercB, errcm := t.getRackEncourageScoreCfg(stub, rackid, nil)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "getSESCfg getRackEncourageScoreCfg(rackid=%s) failed. error=(%s)", rackid, errcm)
		}

		return sercB, nil

	} else if function == "getRackFinanceCfg" {
		if !t.isAdmin(stub, accName) {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "getRackFinanceCfg: %s can't query.", accName)
		}

		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "getRackFinanceCfg miss arg, got %d, need %d.", len(args), argCount)
		}

		var rackid = args[fixedArgCount]

		rfcB, errcm := t.getRackFinancCfg(stub, rackid, nil)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "getRackFinanceCfg getRackFinancCfg(rackid=%s) failed. error=(%s)", rackid, errcm)
		}

		return rfcB, nil

	} else if function == "getRackFinanceProfit" {
		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "getRackFinanceProfit miss arg, got %d, need %d.", len(args), argCount)
		}

		var rackid = args[fixedArgCount]

		profit, errcm := t.getUserFinanceProfit(stub, accName, rackid)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "getRackFinanceProfit getUserFinanceProfit(rackid=%s) failed. error=(%s)", rackid, errcm)
		}

		return []byte(strconv.FormatInt(profit, 10)), nil

	} else if function == "getRackRestFinanceCapacity" {
		if !t.isAdmin(stub, accName) {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "getRackFinanceCapacity: %s can't query.", accName)
		}

		var argCount = fixedArgCount + 2
		if len(args) < argCount {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "getRackFinanceCapacity miss arg, got %d, need %d.", len(args), argCount)
		}

		var rackid = args[fixedArgCount]
		var fid = args[fixedArgCount+1]

		//新理财发行后，用户购买理财时，前台会查询一下货架剩余的投资额度，传入的fid为最新期的理财id

		restCap, errcm := t.getRestFinanceCapacityForRack(stub, rackid, fid)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "getRackFinanceCapacity getFinanceCapacityForRack(rackid=%s) failed. error=(%s)", rackid, errcm)
		}

		return []byte(strconv.FormatInt(restCap, 10)), nil

	} else if function == "transPreCheck" {
		var argCount = fixedArgCount + 3
		if len(args) < argCount {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "transPreCheck miss arg, got %d, need %d.", len(args), argCount)
		}

		//toAcc := args[fixedArgCount]
		pwd := args[fixedArgCount+1]
		/*
			transAmount, err := strconv.ParseInt(args[fixedArgCount+2], 0, 64)
			if err != nil {
				return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "transPreCheck: convert transAmount(%s) failed. error=(%s)", args[fixedArgCount+2], err)
			}
		*/

		//先看密码是否正确
		if len(pwd) > 0 {
			setPwd, errcm := t.isSetAccountPasswd(stub, accName)
			if errcm != nil {
				return nil, kdlogger.ErrorECM(errcm.Code, "transPreCheck: isSetAccountPasswd(%s) failed. error=(%s)", accName, errcm)
			}

			if setPwd {
				ok, errcm := t.authAccountPasswd(stub, accName, pwd)
				if errcm != nil {
					return nil, kdlogger.ErrorECM(errcm.Code, "transPreCheck: AuthAccountPasswd(%s) failed. error=(%s)", accName, errcm)
				}
				if !ok {
					return []byte(strconv.FormatInt(ERRCODE_TRANS_PASSWD_INVALID, 10)), nil
				}
			}
		}

		//通过返回0，表示检查通过
		return []byte(strconv.FormatInt(0, 10)), nil

	} else if function == "isAccSetPwd" { //账户是否设置密码
		setPwd, errcm := t.isSetAccountPasswd(stub, accName)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "Query(isAccSetPwd): IsSetAccountPasswd failed. error=(%s), acc=%s", errcm, accName)
		}

		var retValues []byte
		if setPwd {
			retValues = []byte("1")
		} else {
			retValues = []byte("0")
		}

		return retValues, nil
	} else {

		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "unknown function: %s.", function)
	}
}

func (t *KD) setAllocEarnTx(stub shim.ChaincodeStubInterface, rackid, allocKey string, totalAmt int64,
	accs *AllocAccs, eap *EarningAllocRate, times int64) ([]byte, *ErrorCodeMsg) {

	var eat EarningAllocTx
	eat.Rackid = rackid
	eat.AllocKey = allocKey
	eat.RolesRate = eap.RolesRate
	eat.TotalAmt = totalAmt

	eat.AmountMap = make(map[string]map[string]int64)
	eat.AmountMap[RACK_ROLE_SELLER] = make(map[string]int64)
	eat.AmountMap[RACK_ROLE_FIELDER] = make(map[string]int64)
	eat.AmountMap[RACK_ROLE_DELIVERY] = make(map[string]int64)
	eat.AmountMap[RACK_ROLE_PLATFORM] = make(map[string]int64)

	rolesAllocAmt := t.getRackRolesAllocAmt(eap, totalAmt)

	errcm := t.getRolesAllocEarning(rolesAllocAmt.SellerAmount, accs.SellerAcc, eat.AmountMap[RACK_ROLE_SELLER])
	if errcm != nil {
		return nil, kdlogger.ErrorECM(errcm.Code, "setAllocEarnTx getRolesAllocEarning 1 failed.error=(%s)", errcm)
	}
	errcm = t.getRolesAllocEarning(rolesAllocAmt.FielderAmount, accs.FielderAcc, eat.AmountMap[RACK_ROLE_FIELDER])
	if errcm != nil {
		return nil, kdlogger.ErrorECM(errcm.Code, "setAllocEarnTx getRolesAllocEarning 2 failed.error=(%s)", errcm)
	}
	errcm = t.getRolesAllocEarning(rolesAllocAmt.DeliveryAmount, accs.DeliveryAcc, eat.AmountMap[RACK_ROLE_DELIVERY])
	if errcm != nil {
		return nil, kdlogger.ErrorECM(errcm.Code, "setAllocEarnTx getRolesAllocEarning 3 failed.error=(%s)", errcm)
	}
	errcm = t.getRolesAllocEarning(rolesAllocAmt.PlatformAmount, accs.PlatformAcc, eat.AmountMap[RACK_ROLE_PLATFORM])
	if errcm != nil {
		return nil, kdlogger.ErrorECM(errcm.Code, "setAllocEarnTx getRolesAllocEarning 4 failed.error=(%s)", errcm)
	}

	seqKey := t.getAllocTxSeqKey(stub, rackid)
	seq, errcm := t.getTransSeq(stub, seqKey)
	if errcm != nil {
		return nil, kdlogger.ErrorECM(errcm.Code, "setAllocEarnTx  getTransSeq failed.error=(%s)", errcm)
	}
	seq++

	eat.GlobalSerial = seq
	eat.DateTime = times

	eatJson, err := json.Marshal(eat)
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setAllocEarnTx Marshal failed. error=(%s)", err)
	}
	kdlogger.Debug("setAllocEarnTx return %s.", string(eatJson))

	var txKey = t.getAllocTxKey(stub, rackid, seq)
	err = stateCache.PutState_Ex(stub, txKey, eatJson)
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setAllocEarnTx  PutState_Ex failed.error=(%s)", err)
	}

	errcm = t.setTransSeq(stub, seqKey, seq)
	if errcm != nil {
		return nil, kdlogger.ErrorECM(errcm.Code, "setAllocEarnTx  setTransSeq failed.error=(%s)", errcm)
	}

	//记录每个账户的分成情况
	//四种角色有可能是同一个人，所以判断一下，如果已保存过key，则不再保存
	var checkMap = make(map[string]int)
	errcm = t.setOneAccAllocEarnTx(stub, accs.SellerAcc, txKey)
	if errcm != nil {
		return nil, kdlogger.ErrorECM(errcm.Code, "setAllocEarnTx  setOneAccAllocEarnTx(%s) failed.error=(%s)", accs.SellerAcc, errcm)
	}
	checkMap[accs.SellerAcc] = 0

	if _, ok := checkMap[accs.FielderAcc]; !ok {
		errcm = t.setOneAccAllocEarnTx(stub, accs.FielderAcc, txKey)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "setAllocEarnTx  setOneAccAllocEarnTx(%s) failed.error=(%s)", accs.FielderAcc, errcm)
		}
		checkMap[accs.FielderAcc] = 0
	}

	if _, ok := checkMap[accs.DeliveryAcc]; !ok {
		errcm = t.setOneAccAllocEarnTx(stub, accs.DeliveryAcc, txKey)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "setAllocEarnTx  setOneAccAllocEarnTx(%s) failed.error=(%s)", accs.DeliveryAcc, errcm)
		}
		checkMap[accs.DeliveryAcc] = 0
	}

	if _, ok := checkMap[accs.PlatformAcc]; !ok {
		errcm = t.setOneAccAllocEarnTx(stub, accs.PlatformAcc, txKey)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "setAllocEarnTx  setOneAccAllocEarnTx(%s) failed.error=(%s)", accs.PlatformAcc, errcm)
		}
		checkMap[accs.PlatformAcc] = 0
	}

	return nil, nil
}

func (t *KD) getRackRolesAllocAmt(eap *EarningAllocRate, totalAmt int64) *RolesAllocAmount {

	var raa RolesAllocAmount
	var base = eap.SellerRate + eap.FielderRate + eap.DeliveryRate + eap.PlatformRate

	raa.SellerAmount = totalAmt * eap.SellerRate / base
	raa.FielderAmount = totalAmt * eap.FielderRate / base
	raa.DeliveryAmount = totalAmt * eap.DeliveryRate / base
	//上面计算可能有四舍五入的情况，剩余的都放在平台账户
	raa.PlatformAmount = totalAmt - raa.SellerAmount - raa.FielderAmount - raa.DeliveryAmount

	return &raa
}

func (t *KD) setOneAccAllocEarnTx(stub shim.ChaincodeStubInterface, accName, txKey string) *ErrorCodeMsg {
	var accTxKey = t.getOneAccAllocTxKey(accName)

	txsB, err := stateCache.GetState_Ex(stub, accTxKey)
	if err != nil {
		return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setOneAccAllocEarnTx: GetState error=(%s)", err)
	}

	var newTxsB []byte
	if txsB == nil {
		newTxsB = append([]byte(txKey), MULTI_STRING_DELIM) //第一次添加accName，最后也要加上分隔符
	} else {
		newTxsB = append(txsB, []byte(txKey)...)
		newTxsB = append(newTxsB, MULTI_STRING_DELIM)
	}

	err = stateCache.PutState_Ex(stub, accTxKey, newTxsB)
	if err != nil {

		return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setOneAccAllocEarnTx PutState failed.error=(%s)", err)
	}

	return nil
}

func (t *KD) getRolesAllocEarning(totalAmt int64, accs string, result map[string]int64) *ErrorCodeMsg {

	//如果有多个子账户，格式如下 "a:20;b:20;c:60"，防止输入错误，先去除两边的空格，然后再去除两边的';'（防止split出来空字符串）
	var newAccs = strings.Trim(strings.TrimSpace(accs), ";")

	if strings.Contains(newAccs, ";") {
		var base = 0
		var accRatArr = strings.Split(newAccs, ";")
		var accCnt = len(accRatArr)
		var tmpAmt int64 = 0
		var sumAmt int64 = 0
		var err error
		var rat int
		var accArr []string
		var ratArr []int
		//检查输入格式并计算比例总和，用于做分母
		for _, acc := range accRatArr {
			if !strings.Contains(acc, ":") {
				return kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "getRolesAllocEarning  accs parse error, '%s' has no ':'.", acc)
			}
			var pair = strings.Split(acc, ":")
			if len(pair) != 2 {
				return kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "getRolesAllocEarning  accs parse error, '%s' format error 1.", acc)
			}
			rat, err = strconv.Atoi(pair[1])
			if err != nil {
				return kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "getRolesAllocEarning  accs parse error, '%s' format error 2.", acc)
			}
			base += rat
			accArr = append(accArr, pair[0])
			ratArr = append(ratArr, rat)
		}
		for i, acc := range accArr {
			if i == accCnt-1 {
				result[acc] = totalAmt - sumAmt
			} else {
				tmpAmt = totalAmt * int64(ratArr[i]) / int64(base)
				sumAmt += tmpAmt
				result[acc] = tmpAmt
			}
		}
	} else {
		//没有分号，有冒号，报错
		if strings.Contains(newAccs, ":") {
			return kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "getRolesAllocEarning  accs parse error, '%s' format error 3.", newAccs)
		}
		result[accs] = totalAmt
	}

	return nil
}

func (t *KD) getAllocTxSeqKey(stub shim.ChaincodeStubInterface, rackid string) string {
	return RACK_ALLOCTXSEQ_PREFIX + rackid + "_"
}

func (t *KD) getAllocTxKey(stub shim.ChaincodeStubInterface, rackid string, seq int64) string {
	var buf = bytes.NewBufferString(RACK_ALLOC_TX_PREFIX)
	buf.WriteString(rackid)
	buf.WriteString("_")
	buf.WriteString(strconv.FormatInt(seq, 10))
	return buf.String()
}

func (t *KD) getOneAccAllocTxKey(accName string) string {
	return RACK_ACC_ALLOC_TX_PREFIX + accName
}

func (t *KD) getRackAllocRateKey(rackid string) string {
	return RACK_ALLOCRATE_PREFIX + rackid
}
func (t *KD) getGlobalRackAllocRateKey() string {
	return RACK_GLOBAL_ALLOCRATE_KEY
}

func (t *KD) getAllocTxRecdByKey(stub shim.ChaincodeStubInterface, rackid, allocKey string) ([]byte, *ErrorCodeMsg) {

	var retTransInfo = []byte("[]") //默认为空数组。 因为和下面的查询所有记录使用同一个restful接口，所以这里也返回数组形式

	//先判断是否存在交易序列号了，如果不存在，说明还没有交易发生。 这里做这个判断是因为在 getTransSeq 里如果没有设置过序列号的key会自动设置一次，但是在query中无法执行PutStat，会报错
	var seqKey = t.getAllocTxSeqKey(stub, rackid)
	test, err := stateCache.GetState_Ex(stub, seqKey)
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getOneAllocRecd GetState(seqKey) failed. error=(%s)", err)
	}
	if test == nil {
		kdlogger.Info("getOneAllocRecd no trans saved.")
		return retTransInfo, nil
	}

	//先获取当前最大的序列号
	maxSeq, errcm := t.getTransSeq(stub, seqKey)
	if errcm != nil {
		return nil, kdlogger.ErrorECM(errcm.Code, "getOneAllocRecd getTransSeq failed. error=(%s)", errcm)
	}

	var txArray []QueryEarningAllocTx = []QueryEarningAllocTx{} //给个默认空值，即使没有数据，marshal之后也会为'[]'

	//从最后往前找，因为查找最新的可能性比较大
	for i := maxSeq; i > 0; i-- { //序列号生成器从1开始
		txkey := t.getAllocTxKey(stub, rackid, i)
		txB, err := stateCache.GetState_Ex(stub, txkey)
		if err != nil {
			kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getOneAllocRecd GetState(rackid=%s) failed. error=(%s)", rackid, err)
			continue
		}
		if txB == nil {
			kdlogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "getOneAllocRecd GetState(rackid=%s) nil.", rackid)
			continue
		}

		var eat EarningAllocTx
		err = json.Unmarshal(txB, &eat)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getOneAllocRecd Unmarshal(rackid=%s) failed. error=(%s)", rackid, err)
		}

		if eat.AllocKey == allocKey {
			var qeat QueryEarningAllocTx
			qeat.Serial = eat.GlobalSerial
			qeat.PubEarningAllocTx = eat.PubEarningAllocTx
			txArray = append(txArray, qeat)
			break
		}
	}

	retTransInfo, err = json.Marshal(txArray)
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getOneAllocRecd Marshal(rackid=%s) failed. error=(%s)", rackid, err)
	}

	return retTransInfo, nil
}
func (t *KD) getAllocTxRecds(stub shim.ChaincodeStubInterface, rackid string, begIdx, count, begTime, endTime int64) ([]byte, *ErrorCodeMsg) {
	var maxSeq int64
	var errcm *ErrorCodeMsg
	var retTransInfo = []byte("[]") //默认为空数组

	//begIdx从1开始， 因为保存交易时，从1开始编号
	if begIdx < 1 {
		begIdx = 1
	}
	//endTime为负数，查询到最新时间
	if endTime < 0 {
		endTime = math.MaxInt64
	}

	if count == 0 {
		kdlogger.Warn("getAllocTxRecds nothing to do(%d).", count)
		return retTransInfo, nil
	}

	//先判断是否存在交易序列号了，如果不存在，说明还没有交易发生。 这里做这个判断是因为在 getTransSeq 里如果没有设置过序列号的key会自动设置一次，但是在query中无法执行PutStat，会报错
	var seqKey = t.getAllocTxSeqKey(stub, rackid)
	test, err := stateCache.GetState_Ex(stub, seqKey)
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getAllocTxRecds GetState(seqKey) failed. error=(%s)", err)
	}
	if test == nil {
		kdlogger.Info("getAllocTxRecds no trans saved.")
		return retTransInfo, nil
	}

	//先获取当前最大的序列号
	maxSeq, errcm = t.getTransSeq(stub, seqKey)
	if errcm != nil {
		return nil, kdlogger.ErrorECM(errcm.Code, "getAllocTxRecds getTransSeq failed. error=(%s)", errcm)
	}

	if begIdx > maxSeq {
		kdlogger.Warn("getAllocTxRecds nothing to do(%d,%d).", begIdx, maxSeq)
		return retTransInfo, nil
	}

	if count < 0 {
		count = maxSeq - begIdx + 1
	}

	var txArray []QueryEarningAllocTx = []QueryEarningAllocTx{} //给个默认空值，即使没有数据，marshal之后也会为'[]'

	var loopCnt int64 = 0
	for loop := begIdx; loop <= maxSeq; loop++ {
		//处理了count条时，不再处理
		if loopCnt >= count {
			break
		}

		txkey := t.getAllocTxKey(stub, rackid, loop)
		txB, err := stateCache.GetState_Ex(stub, txkey)
		if err != nil {
			kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getAllocTxRecds GetState(rackid=%s) failed. error=(%s)", rackid, err)
			continue
		}
		if txB == nil {
			kdlogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "getAllocTxRecds GetState(rackid=%s) nil.", rackid)
			continue
		}

		var eat EarningAllocTx
		err = json.Unmarshal(txB, &eat)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getAllocTxRecds Unmarshal(rackid=%s) failed. error=(%s)", rackid, err)
		}

		if eat.DateTime >= begTime && eat.DateTime <= endTime {
			var qeat QueryEarningAllocTx
			qeat.Serial = eat.GlobalSerial
			qeat.PubEarningAllocTx = eat.PubEarningAllocTx
			txArray = append(txArray, qeat)
			loopCnt++
		}
	}

	retTransInfo, err = json.Marshal(txArray)
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getAllocTxRecds Marshal(rackid=%s) failed. error=(%s)", rackid, err)
	}

	return retTransInfo, nil
}

func (t *KD) getOneAccAllocTxRecds(stub shim.ChaincodeStubInterface, accName string, begIdx, count, begTime, endTime int64) ([]byte, *ErrorCodeMsg) {
	var resultJson = []byte("[]") //默认为空数组
	var accTxKey = t.getOneAccAllocTxKey(accName)

	//begIdx从1开始，下面处理注意
	if begIdx < 1 {
		begIdx = 1
	}
	//endTime为负数，查询到最新时间
	if endTime < 0 {
		endTime = math.MaxInt64
	}

	if count == 0 {
		kdlogger.Warn("getOneAccAllocTxRecds nothing to do(%d).", count)
		return resultJson, nil
	}
	//count为负数，查询到最后
	if count < 0 {
		count = math.MaxInt64
	}

	txsB, err := stateCache.GetState_Ex(stub, accTxKey)
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getOneAccAllocTxRecds: GetState(accName=%s) error=(%s)", accName, err)
	}
	if txsB == nil {
		return resultJson, nil
	}

	var qaeatArr []QueryAccEarningAllocTx = []QueryAccEarningAllocTx{}
	var buf = bytes.NewBuffer(txsB)
	var oneStringB []byte
	var oneString string
	var loop int64 = 0
	var cnt int64 = 0
	var errcm *ErrorCodeMsg
	for {
		if cnt >= count {
			break
		}
		oneStringB, err = buf.ReadBytes(MULTI_STRING_DELIM)
		if err != nil {
			if err == io.EOF {
				kdlogger.Debug("getOneAccAllocTxRecds proc %d recds, end.", loop)
				break
			}
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getOneAccAllocTxRecds ReadBytes failed. last=%s, error=(%s)", string(oneStringB), err)
		}
		loop++
		if begIdx > loop {
			continue
		}

		oneString = string(oneStringB[:len(oneStringB)-1]) //去掉末尾的分隔符
		var pqaeat *QueryAccEarningAllocTx
		pqaeat, errcm = t.getOneAccAllocTx(stub, oneString, accName)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "getOneAccAllocTxRecds walker failed. acc=%s, error=(%s)", accName, errcm)
		}
		if pqaeat.DateTime >= begTime && pqaeat.DateTime <= endTime {
			pqaeat.Serail = loop
			qaeatArr = append(qaeatArr, *pqaeat)
			cnt++
		}
	}

	resultJson, err = json.Marshal(qaeatArr)
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getOneAccAllocTxRecds Marshal failed. acc=%s, error=(%s)", accName, err)
	}

	return resultJson, nil
}

func (t *KD) getOneAccAllocTx(stub shim.ChaincodeStubInterface, txKey, accName string) (*QueryAccEarningAllocTx, *ErrorCodeMsg) {
	eat, errcm := t.getAllocTxRecdEntity(stub, txKey)
	if errcm != nil {
		return nil, kdlogger.ErrorECM(errcm.Code, "procOneAccAllocTx getAllocTxRecdEntity failed. txKey=%s, error=(%s)", txKey, errcm)
	}

	var qaeat QueryAccEarningAllocTx
	qaeat.AccName = accName
	qaeat.DateTime = eat.DateTime
	qaeat.Rackid = eat.Rackid
	qaeat.TotalAmt = eat.TotalAmt
	qaeat.GlobalSerial = eat.GlobalSerial
	qaeat.RolesRate = eat.RolesRate
	qaeat.RoleAmountMap = make(map[string]int64)
	for role, accAmtMap := range eat.AmountMap {
		for acc, amt := range accAmtMap {
			if acc == accName {
				qaeat.RoleAmountMap[role] += amt //防止每个角色的账户列表中含有同样的账户？
			}
		}
	}

	return &qaeat, nil
}

func (t *KD) getAllocTxRecdEntity(stub shim.ChaincodeStubInterface, txKey string) (*EarningAllocTx, *ErrorCodeMsg) {
	txB, err := stateCache.GetState_Ex(stub, txKey)
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getAllocTxRecdEntity GetState(txKey=%s) failed. error=(%s)", txKey, err)
	}
	if txB == nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "getAllocTxRecdEntity GetState(txKey=%s) nil.", txKey)
	}

	var eat EarningAllocTx
	err = json.Unmarshal(txB, &eat)
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getAllocTxRecdEntity Unmarshal(txKey=%s) failed. error=(%s)", txKey, err)
	}

	return &eat, nil
}

func (t *KD) getRackAllocCfg(stub shim.ChaincodeStubInterface, rackid string, peap *EarningAllocRate) ([]byte, *ErrorCodeMsg) {
	var eapB []byte = nil
	var err error

	if rackid != "*" {
		eapB, err = stateCache.GetState_Ex(stub, t.getRackAllocRateKey(rackid))
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getRackAllocCfg GetState(rackid=%s) failed. error=(%s)", rackid, err)
		}
	}

	if eapB == nil {
		kdlogger.Warn("getRackAllocCfg GetState(rackid=%s) nil, try to get global.", rackid)
		//没有为该货架单独配置，返回global配置
		eapB, err = stateCache.GetState_Ex(stub, t.getGlobalRackAllocRateKey())
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getRackAllocCfg GetState(global, rackid=%s) failed. error=(%s)", rackid, err)
		}
		if eapB == nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "getRackAllocCfg GetState(global, rackid=%s) nil.", rackid)
		}
	}

	if peap != nil {
		err = json.Unmarshal(eapB, peap)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getRackAllocCfg Unmarshal failed. error=(%s)", rackid, err)
		}
	}

	return eapB, nil
}

/* ----------------------- 积分奖励相关 ----------------------- */
func (t *KD) getGlobalRackEncourageScoreCfgKey() string {
	return RACK_SALE_ENC_SCORE_CFG_PREFIX + "global"
}
func (t *KD) getRackEncourageScoreCfgKey(rackid string) string {
	return RACK_SALE_ENC_SCORE_CFG_PREFIX + "rack_" + rackid
}

func (t *KD) setRackEncourageScoreCfg(stub shim.ChaincodeStubInterface, rackid, cfgStr string, invokeTime int64) ([]byte, *ErrorCodeMsg) {
	//配置格式如下 "2000:150;3000:170..."，防止输入错误，先去除两边的空格，然后再去除两边的';'（防止split出来空字符串）
	var newCfg = strings.Trim(strings.TrimSpace(cfgStr), ";")

	var sepc ScoreEncouragePercentCfg
	sepc.Rackid = rackid
	if rackid == "*" {
		sepc.Rackid = RACK_GLOBAL_CFG_RACK_ID
	}
	sepc.UpdateTime = invokeTime

	var rangeRatArr []string

	var err error
	var rang int64
	var percent int

	//含有";"，表示有多条配置，没有则说明只有一条配置
	if strings.Contains(newCfg, ";") {
		rangeRatArr = strings.Split(newCfg, ";")

	} else {
		rangeRatArr = append(rangeRatArr, newCfg)
	}

	var rangePercentMap = make(map[int64]int)
	for _, rangeRate := range rangeRatArr {
		if !strings.Contains(rangeRate, ":") {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "setRackEncourageScoreCfg  rangeRate parse error, '%s' has no ':'.", rangeRate)
		}
		var pair = strings.Split(rangeRate, ":")
		if len(pair) != 2 {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "setRackEncourageScoreCfg  rangeRate parse error, '%s' format error 1.", rangeRate)
		}
		//"-"表示正无穷
		if pair[0] == "-" {
			rang = math.MaxInt64
		} else {
			rang, err = strconv.ParseInt(pair[0], 0, 64)
			if err != nil {
				return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setRackEncourageScoreCfg  rangeRate parse error, '%s' format error 2.", rangeRate)
			}
		}
		percent, err = strconv.Atoi(pair[1])
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setRackEncourageScoreCfg  rangeRate parse error, '%s' format error 3.", rangeRate)
		}

		//sepc.RangePercentMap[rang] = percent
		rangePercentMap[rang] = percent
	}

	//注意，这里如果下面没有排序sepc.RangeList， 则不能使用 rangePercentMap 来临时存储数据，会导致各个节点上sepc.RangeList数据顺序不一致
	for rang, _ := range rangePercentMap {
		sepc.RangeList = append(sepc.RangeList, rang)
	}

	//升序排序
	var cnt = len(sepc.RangeList)
	for i := 0; i < cnt; i++ {
		for j := i + 1; j < cnt; j++ {
			if sepc.RangeList[i] > sepc.RangeList[j] {
				sepc.RangeList[j], sepc.RangeList[i] = sepc.RangeList[i], sepc.RangeList[j]
			}
		}
	}

	for i := 0; i < cnt; i++ {
		sepc.PercentList = append(sepc.PercentList, rangePercentMap[sepc.RangeList[i]])
	}

	sepcJson, err := json.Marshal(sepc)
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setRackEncourageScoreCfg Marshal failed. error=(%s)", err)
	}

	var stateKey string
	if rackid == "*" {
		stateKey = t.getGlobalRackEncourageScoreCfgKey()
	} else {
		stateKey = t.getRackEncourageScoreCfgKey(rackid)
	}

	err = stateCache.PutState_Ex(stub, stateKey, sepcJson)
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setRackEncourageScoreCfg PutState_Ex failed. error=(%s)", err)
	}

	return nil, nil
}

func (t *KD) getRackEncourageScoreCfg(stub shim.ChaincodeStubInterface, rackid string, psepc *ScoreEncouragePercentCfg) ([]byte, *ErrorCodeMsg) {

	var sepcB []byte = nil
	var err error

	if rackid != "*" {
		sepcB, err = stateCache.GetState_Ex(stub, t.getRackEncourageScoreCfgKey(rackid))
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getRackEncourageScoreCfg GetState failed.rackid=%s error=(%s)", rackid, err)
		}
	}

	if sepcB == nil {
		kdlogger.Warn("getRackEncourageScoreCfg: can not find cfg for %s, will use golobal.", rackid)
		sepcB, err = stateCache.GetState_Ex(stub, t.getGlobalRackEncourageScoreCfgKey())
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getRackEncourageScoreCfg GetState(global cfg) failed.rackid=%s error=(%s)", rackid, err)
		}
		if sepcB == nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "getRackEncourageScoreCfg GetState(global cfg) nil.rackid=%s", rackid)
		}
	}

	if psepc != nil {
		err = json.Unmarshal(sepcB, psepc)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getRackEncourageScoreCfg Unmarshal failed.rackid=%s error=(%s)", rackid, err)
		}
	}

	return sepcB, nil
}

func (t *KD) allocEncourageScoreForSales(stub shim.ChaincodeStubInterface, paraStr string, transFromAcc, transType, transDesc string, invokeTime int64, sameEntSaveTx bool) ([]byte, *ErrorCodeMsg) {
	//配置格式如下 "货架id1,销售额,货架经营者账户,场地提供者账户,送货人账户,平台账户;货架id2,销售额,货架经营者账户,场地提供者账户,送货人账户,平台账户;...."，
	//防止输入错误，先去除两边的空格，然后再去除两边的';'（防止split出来空字符串）
	var newStr = strings.Trim(strings.TrimSpace(paraStr), ";")

	var rrsList []RackRolesSales

	var rackRolesSalesArr []string

	var err error

	//含有";"，表示有多条配置，没有则说明只有一条配置
	if strings.Contains(newStr, ";") {
		rackRolesSalesArr = strings.Split(newStr, ";")
	} else {
		rackRolesSalesArr = append(rackRolesSalesArr, newStr)
	}

	var eleDelim = ","
	var rackRolesSales string
	var errList []string
	for _, v := range rackRolesSalesArr {
		rackRolesSales = strings.Trim(strings.TrimSpace(v), eleDelim)
		if !strings.Contains(rackRolesSales, eleDelim) {
			kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "encourageScoreBySales  rackRolesSales parse error, '%s' has no '%s'.", rackRolesSales, eleDelim)
			errList = append(errList, rackRolesSales)
			continue
		}
		var eles = strings.Split(rackRolesSales, eleDelim)
		if len(eles) != 6 {
			kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "encourageScoreBySales  rackRolesSales parse error, '%s' format error 1.", rackRolesSales)
			errList = append(errList, rackRolesSales)
			continue
		}

		var rrs RackRolesSales

		rrs.Rackid = eles[0]
		rrs.Sales, err = strconv.ParseInt(eles[1], 0, 64)
		if err != nil {
			kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "encourageScoreBySales  rackRolesSales parse error, '%s' format error 2.", rackRolesSales)
			errList = append(errList, rrs.Rackid)
			continue
		}

		if rrs.Sales <= 0 {
			kdlogger.Info("encourageScoreBySales sales is 0(rack=%s), do nothing.", rrs.Rackid)
			continue
		}

		rrs.Sales = rrs.Sales / 100 //输入的单位为分，这里计算以元为单位

		rrs.AllocAccs.SellerAcc = eles[2]
		rrs.AllocAccs.FielderAcc = eles[3]
		rrs.AllocAccs.DeliveryAcc = eles[4]
		rrs.AllocAccs.PlatformAcc = eles[5]

		rrsList = append(rrsList, rrs)
	}

	for _, rrs := range rrsList {
		encourageScore, errcm := t.getRackEncourgeScoreBySales(stub, rrs.Rackid, rrs.Sales)
		if errcm != nil {
			kdlogger.ErrorECM(errcm.Code, "encourageScoreBySales  getRackEncourgePercentBySales failed, error=%s.", errcm)
			errList = append(errList, rrs.Rackid)
			continue
		}

		var rres RackRolesEncourageScores
		rres.Rackid = rrs.Rackid
		rres.Scores = encourageScore
		rres.AllocAccs = rrs.AllocAccs

		//销售奖励积分时，货架经营者要补偿销售额同等的积分
		errcm = t.allocEncourageScore(stub, &rres, transFromAcc, transType, transDesc, invokeTime, sameEntSaveTx, rrs.Sales)
		if errcm != nil {
			kdlogger.ErrorECM(errcm.Code, "encourageScoreBySales allocEncourageScore failed, error=%s.", errcm)
			errList = append(errList, rrs.Rackid)
			continue
		}
	}

	if len(errList) > 0 {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "encourageScoreBySales: has some error:(%s).", strings.Join(errList, ";"))
	}

	return nil, nil
}

func (t *KD) getRackEncourgeScoreBySales(stub shim.ChaincodeStubInterface, rackid string, sales int64) (int64, *ErrorCodeMsg) {
	var sepc ScoreEncouragePercentCfg
	_, errcm := t.getRackEncourageScoreCfg(stub, rackid, &sepc)
	if errcm != nil {
		return 0, kdlogger.ErrorECM(errcm.Code, "getRackEncourgePercent getRackEncourageScoreCfg failed.rackid=%s error=(%s)", rackid, errcm)
	}

	/*
	   var sortedRange []int64
	   for rang, _ := range sepc.RangePercentMap {
	       sortedRange = append(sortedRange, rang)
	   }

	   //升序排序
	   var cnt = len(sortedRange)
	   for i := 0; i < cnt; i++ {
	       for j := i + 1; j < cnt; j++ {
	           if sortedRange[i] > sortedRange[j] {
	               sortedRange[j], sortedRange[i] = sortedRange[i], sortedRange[j]
	           }
	       }
	   }

	   for _, v := range sortedRange {
	       if sales <= v {
	           return int64(sepc.RangePercentMap[v]) * sales / 100, nil //营业额乘以百分比
	       }
	   }
	*/
	for i, v := range sepc.RangeList {
		if sales <= v {
			return int64(sepc.PercentList[i]) * sales / 100, nil //营业额乘以百分比
		}
	}

	return sales, nil
}

func (t *KD) allocEncourageScore(stub shim.ChaincodeStubInterface, rrs *RackRolesEncourageScores, transFromAcc, transType, transDesc string,
	invokeTime int64, sameEntSaveTx bool, sellerComps int64) *ErrorCodeMsg {
	var ear EarningAllocRate
	_, errcm := t.getRackAllocCfg(stub, rrs.Rackid, &ear)
	if errcm != nil {
		return kdlogger.ErrorECM(errcm.Code, "allocEncourageScore getRackAllocCfg failed,Rackid=%s,  error=%s.", rrs.Rackid, errcm)
	}

	var hasErr = false
	var failedAccList []string

	rolesAllocScore := t.getRackRolesAllocAmt(&ear, rrs.Scores)

	_, errcm = t.transferCoin(stub, transFromAcc, rrs.SellerAcc, transType, transDesc,
		rolesAllocScore.SellerAmount+sellerComps, invokeTime, sameEntSaveTx)
	if errcm != nil {
		kdlogger.ErrorECM(errcm.Code, "allocEncourageScore: transferCoin(SellerAcc=%s) failed, error=%s.", rrs.SellerAcc, errcm)
		hasErr = true
		failedAccList = append(failedAccList, rrs.SellerAcc)
	}

	_, errcm = t.transferCoin(stub, transFromAcc, rrs.FielderAcc, transType, transDesc,
		rolesAllocScore.FielderAmount, invokeTime, sameEntSaveTx)
	if errcm != nil {
		kdlogger.ErrorECM(errcm.Code, "allocEncourageScore: transferCoin(FielderAcc=%s) failed, error=%s.", rrs.FielderAcc, errcm)
		hasErr = true
		failedAccList = append(failedAccList, rrs.FielderAcc)
	}

	_, errcm = t.transferCoin(stub, transFromAcc, rrs.DeliveryAcc, transType, transDesc,
		rolesAllocScore.DeliveryAmount, invokeTime, sameEntSaveTx)
	if errcm != nil {
		kdlogger.ErrorECM(errcm.Code, "allocEncourageScore: transferCoin(DeliveryAcc=%s) failed, error=%s.", rrs.DeliveryAcc, errcm)
		hasErr = true
		failedAccList = append(failedAccList, rrs.DeliveryAcc)
	}

	_, errcm = t.transferCoin(stub, transFromAcc, rrs.PlatformAcc, transType, transDesc,
		rolesAllocScore.PlatformAmount, invokeTime, sameEntSaveTx)
	if errcm != nil {
		kdlogger.ErrorECM(errcm.Code, "allocEncourageScore: transferCoin(PlatformAcc=%s) failed, error=%s.", rrs.PlatformAcc, errcm)
		hasErr = true
		failedAccList = append(failedAccList, rrs.PlatformAcc)
	}

	if hasErr {
		return kdlogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "allocEncourageScore: transferCoin faied, acc=%s", strings.Join(failedAccList, ";"))
	}

	return nil
}

func (t *KD) allocEncourageScoreForNewRack(stub shim.ChaincodeStubInterface, paraStr string, transFromAcc, transType, transDesc string, invokeTime int64, sameEntSaveTx bool) ([]byte, *ErrorCodeMsg) {
	//配置格式如下 "货架1,货架经营者账户,场地提供者账户,送货人账户,平台账户,奖励金额(可省略);货架2,货架经营者账户,场地提供者账户,送货人账户,平台账户,奖励金额(可省略);...."，
	//防止输入错误，先去除两边的空格，然后再去除两边的';'（防止split出来空字符串）
	var newStr = strings.Trim(strings.TrimSpace(paraStr), ";")

	var rresList []RackRolesEncourageScores

	var rackRolesScoreArr []string

	var err error

	//含有";"，表示有多条配置，没有则说明只有一条配置
	if strings.Contains(newStr, ";") {
		rackRolesScoreArr = strings.Split(newStr, ";")

	} else {
		rackRolesScoreArr = append(rackRolesScoreArr, newStr)
	}

	var eleDelim = ","
	var rackRolesScore string
	var errList []string
	for _, v := range rackRolesScoreArr {
		rackRolesScore = strings.Trim(strings.TrimSpace(v), eleDelim)
		if !strings.Contains(rackRolesScore, eleDelim) {
			kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "allocEncourageScoreForNewRack  rackRolesSales parse error, '%s' has no '%s'.", rackRolesScore, eleDelim)
			errList = append(errList, rackRolesScore)
			continue
		}
		var eles = strings.Split(rackRolesScore, eleDelim)
		//至少包含货架id，四个角色
		if len(eles) < 5 {
			kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "allocEncourageScoreForNewRack  rackRolesSales parse error, '%s' format error 1.", rackRolesScore)
			errList = append(errList, rackRolesScore)
			continue
		}

		var rres RackRolesEncourageScores

		rres.Rackid = eles[0]
		rres.AllocAccs.SellerAcc = eles[1]
		rres.AllocAccs.FielderAcc = eles[2]
		rres.AllocAccs.DeliveryAcc = eles[3]
		rres.AllocAccs.PlatformAcc = eles[4]
		if len(eles) >= 6 {
			rres.Scores, err = strconv.ParseInt(eles[5], 0, 64)
			if err != nil {
				kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "allocEncourageScoreForNewRack  rackRolesSales parse error, '%s' format error 2.", rackRolesScore)
				errList = append(errList, rres.Rackid)
				continue
			}
		} else {
			rres.Scores = RACK_NEWRACK_ENC_SCORE_DEFAULT
		}

		rresList = append(rresList, rres)
	}

	for _, rres := range rresList {
		errcm := t.allocEncourageScore(stub, &rres, transFromAcc, transType, transDesc, invokeTime, sameEntSaveTx, 0)
		if errcm != nil {
			kdlogger.ErrorECM(errcm.Code, "allocEncourageScoreForNewRack allocEncourageScore failed, error=%s.", errcm)
			errList = append(errList, rres.Rackid)
			continue
		}
	}

	if len(errList) > 0 {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "allocEncourageScoreForNewRack: some err,[%s].", strings.Join(errList, ";"))
	}

	return nil, nil
}

/* ----------------------- 积分奖励相关 ----------------------- */

/* ----------------------- 货架融资相关 ----------------------- */
func (t *KD) getGlobalRackFinancCfgKey() string {
	return RACK_FINANCE_CFG_PREFIX + "global"
}
func (t *KD) getRackFinancCfgKey(rackid string) string {
	return RACK_FINANCE_CFG_PREFIX + "rack_" + rackid
}

func (t *KD) getRackFinancCfg(stub shim.ChaincodeStubInterface, rackid string, prfc *RackFinanceCfg) ([]byte, *ErrorCodeMsg) {

	var rfcB []byte = nil
	var err error

	if rackid != "*" { // "*"表示查询全局配置
		rfcB, err = stateCache.GetState_Ex(stub, t.getRackFinancCfgKey(rackid))
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getRackFinancCfg GetState failed.rackid=%s error=(%s)", rackid, err)
		}
	}

	if rfcB == nil {
		kdlogger.Warn("getRackFinancCfg: can not find cfg for %s, will use golobal.", rackid)
		rfcB, err = stateCache.GetState_Ex(stub, t.getGlobalRackFinancCfgKey())
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getRackFinancCfg GetState(global cfg) failed.rackid=%s error=(%s)", rackid, err)
		}
		if rfcB == nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "getRackFinancCfg GetState(global cfg) nil.rackid=%s", rackid)
		}
	}

	if prfc != nil {
		err = json.Unmarshal(rfcB, prfc)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getRackFinancCfg Unmarshal failed.rackid=%s error=(%s)", rackid, err)
		}
	}

	return rfcB, nil
}

func (t *KD) getFinacInfoKey(fiId string) string {
	return FINACINFO_PREFIX + fiId
}
func (t *KD) getRackInfoKey(rId string) string {
	return RACKINFO_PREFIX + rId
}
func (t *KD) getRackFinacInfoKey(rackId, finacId string) string {
	return RACKFINACINFO_PREFIX + rackId + "_" + finacId
}

//用户购买理财，包括自动续期
func (t *KD) userBuyFinance(stub shim.ChaincodeStubInterface, accName, rackid, fid, payee, transType, desc string, amount, invokeTime int64, sameEntSaveTx, isRenewal bool) ([]byte, *ErrorCodeMsg) {
	var fiacInfoKey = t.getFinacInfoKey(fid)
	fiB, err := stateCache.GetState_Ex(stub, fiacInfoKey)
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "userBuyFinance:  GetState(%s) failed. error=(%s).", fiacInfoKey, err)
	}
	var fi FinancialInfo
	if fiB == nil {
		fi.FID = fid
		fi.Time = invokeTime
	} else {
		err = json.Unmarshal(fiB, &fi)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "userBuyFinance:  Unmarshal(fib) failed. error=(%s).", err)
		}
		//一般不会出现此情况
		if fi.FID != fid {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "userBuyFinance:  fid missmatch(%s).", fi.FID)
		}
	}

	var rackInfoKey = t.getRackInfoKey(rackid)
	riB, err := stateCache.GetState_Ex(stub, rackInfoKey)
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "userBuyFinance:  GetState(%s) failed. error=(%s).", rackInfoKey, err)
	}
	var ri RackInfo
	if riB == nil {
		ri.RackID = rackid
		ri.Time = invokeTime
	} else {
		err = json.Unmarshal(riB, &ri)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "userBuyFinance:  Unmarshal(riB) failed. error=(%s).", err)
		}
		//一般不会出现此情况
		if ri.RackID != rackid {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "userBuyFinance:  rackid missmatch(%s).", ri.RackID)
		}
	}

	//写入货架融资信息
	rackFinacInfoKey := t.getRackFinacInfoKey(rackid, fid)
	rfiB, err := stateCache.GetState_Ex(stub, rackFinacInfoKey)
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "userBuyFinance:  GetState(%s) failed. error=(%s).", rackFinacInfoKey, err)
	}
	var rfi RackFinancInfo
	if rfiB == nil {
		rfi.RackID = rackid
		rfi.FID = fid
		rfi.DataTime = invokeTime
		rfi.SerialNum = 0 /////
		rfi.AmountFinca = amount
		rfi.UserAmountMap = make(map[string]int64)
		rfi.UserAmountMap[accName] = amount
		rfi.UserRenewalMap = make(map[string]int64)
		if isRenewal {
			rfi.UserRenewalMap[accName] = amount
		}
		rfi.Stage = FINANC_STAGE_ISSUE_BEGING //新购买理财时，初始为理财发行开始

		var rfc RackFinanceCfg
		_, errcm := t.getRackFinancCfg(stub, rackid, &rfc)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "userBuyFinance:  getRackFinancCfg failed. error=(%s).", errcm)
		}

		var ear EarningAllocRate
		_, errcm = t.getRackAllocCfg(stub, rackid, &ear)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "userBuyFinance:  getRackAllocCfg failed. error=(%s).", errcm)
		}

		rfi.RFCfg = rfc.PubRackFinanceCfg
		rfi.RolesAllocRate = ear.RolesRate
	} else {
		err = json.Unmarshal(rfiB, &rfi)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "userBuyFinance:  Unmarshal RackFinancInfo failed. error=(%s).", err)
		}

		//如果不是续期，且理财发行完毕，不能购买
		if !isRenewal && rfi.Stage >= FINANC_STAGE_ISSUE_FINISH {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "userBuyFinance:  finance finish, user can't buy.")
		}

		rfi.AmountFinca += amount
		_, ok := rfi.UserAmountMap[accName]
		if ok {
			//如果用户已提取了，又来买，那么从新记录投资额，不能累计，否则会把前一次的累计进来。
			if strSliceContains(rfi.PayFinanceUserList, accName) {
				rfi.AmountFinca -= rfi.UserAmountMap[accName] //实际投资额度要减去上一次的
				rfi.UserAmountMap[accName] = amount
				if isRenewal {
					rfi.UserRenewalMap[accName] = amount
				}
				rfi.PayFinanceUserList = strSliceDelete(rfi.PayFinanceUserList, accName)
			} else {
				rfi.UserAmountMap[accName] += amount
				if isRenewal {
					rfi.UserRenewalMap[accName] = amount
				}
			}
		} else {
			rfi.UserAmountMap[accName] = amount
			if isRenewal {
				rfi.UserRenewalMap[accName] = amount
			}
		}
	}

	//如果是续期，说明该期理财已经发行完毕了。因为发行完毕之后才会调用续期
	if isRenewal {
		rfi.Stage = FINANC_STAGE_ISSUE_FINISH
	}

	var rfc RackFinanceCfg
	_, errcm := t.getRackFinancCfg(stub, rackid, &rfc)
	if errcm != nil {
		return nil, kdlogger.ErrorECM(errcm.Code, "userBuyFinance:  getRackFinancCfg failed. error=(%s).", errcm)
	}

	//看该货架是否有历史投资，如果有的话，这些投资会自动转到当前融资，就会导致超额。
	var historyFinance int64 = 0
	if !isRenewal { //自动续期时，不需要计算历史投资，因为续期的金额就是历史投资额
		//调用购买理财的接口时，已经将最新的理财期号设置了（调用setCurrentFid），所以这里取前一期的期号
		pfid, errcm := t.getPreviousFid(stub)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "userBuyFinance: getPreviousFid failed. error=(%s).", errcm)
		}

		kdlogger.Debug("userBuyFinance: pfid=%s", pfid)

		//有前一期的fid时才计算。如果没有说明没有历史投资
		if len(pfid) > 0 {
			historyFinance, errcm = t.getRackFinanceAmount(stub, rackid, pfid)
			if errcm != nil {
				return nil, kdlogger.ErrorECM(errcm.Code, "userBuyFinance: getRackHistoryFinance failed. error=(%s).", errcm)
			}
		}
	}

	//融资额度超出货架支持能力
	if rfi.AmountFinca+historyFinance > rfc.InvestCapacity {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "userBuyFinance:  AmountFinca > rack's capacity. (%d,%d,%d)", rfi.AmountFinca, historyFinance, rfc.InvestCapacity)
	}

	//用户给融资方转账
	if !isRenewal {
		_, errcm := t.transferCoin(stub, accName, payee, transType, desc, amount, invokeTime, sameEntSaveTx)
		if errcm != nil {
			return nil, kdlogger.ErrorECM(errcm.Code, "userBuyFinance: transferCoin failed. error=(%s).", errcm)
		}
	}

	//转账成功后在用户entity中写入相应信息
	paccRFEnt, errcm := t.getAccountRackInvestInfo(stub, accName)
	if errcm != nil {
		return nil, kdlogger.ErrorECM(errcm.Code, "userBuyFinance: getAccountRackFinanceInfo failed. error=(%s).", errcm)
	}

	var arfi AccRackInvest
	if paccRFEnt != nil {
		arfi = *paccRFEnt
	} else {
		arfi.EntID = accName
	}

	if arfi.RFInfoMap == nil {
		arfi.RFInfoMap = make(map[string]int)
	}
	arfi.RFInfoMap[t.getMapKey4RackFinance(rackid, fid)] = 0
	arfi.LatestFid = fid

	errcm = t.setAccountRackInvestInfo(stub, &arfi)
	if errcm != nil {
		return nil, kdlogger.ErrorECM(errcm.Code, "userBuyFinance: setAccountRackInvestInfo failed. error=(%s).", errcm)
	}
	kdlogger.Debug("userBuyFinance: ent=%+v", arfi)

	if !strSliceContains(fi.RackList, ri.RackID) {
		fi.RackList = append(fi.RackList, ri.RackID)
	}
	fiJson, err := json.Marshal(fi)
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "userBuyFinance:  Marshal failed. error=(%s).", err)
	}

	if !strSliceContains(ri.FinacList, fi.FID) {
		ri.FinacList = append(ri.FinacList, fi.FID)
	}

	riJson, err := json.Marshal(ri)
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "userBuyFinance:  Marshal failed. error=(%s).", err)
	}
	rfiJson, err := json.Marshal(rfi)
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "userBuyFinance:  Marshal failed. error=(%s).", err)
	}

	err = stateCache.PutState_Ex(stub, rackFinacInfoKey, rfiJson)
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "userBuyFinance:  PutState failed. error=(%s).", err)
	}

	err = stateCache.PutState_Ex(stub, rackInfoKey, riJson)
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "userBuyFinance:  PutState failed. error=(%s).", err)
	}

	err = stateCache.PutState_Ex(stub, fiacInfoKey, fiJson)
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "userBuyFinance:  PutState failed. error=(%s).", err)
	}

	kdlogger.Debug("userBuyFinance: ri=%+v fi=%+v rfi=%+v", ri, fi, rfi)
	kdlogger.Debug("userBuyFinance: rfiJson=%s", string(rfiJson))

	return nil, nil
}

func (t *KD) financeBonus(stub shim.ChaincodeStubInterface, fid, rackales string, invokeTime int64) ([]byte, *ErrorCodeMsg) {
	//配置格式如下 "货架1:销售额;货架2:销售额"，
	//防止输入错误，先去除两边的空格，然后再去除两边的';'（防止split出来空字符串）
	var newStr = strings.Trim(strings.TrimSpace(rackales), ";")

	var rackSalesArr []string

	var err error

	//含有";"，表示有多条配置，没有则说明只有一条配置
	if strings.Contains(newStr, ";") {
		rackSalesArr = strings.Split(newStr, ";")
	} else {
		rackSalesArr = append(rackSalesArr, newStr)
	}

	var eleDelim = ":"
	var rackSales string
	var errRackList []string
	for _, v := range rackSalesArr {
		rackSales = strings.Trim(strings.TrimSpace(v), eleDelim)
		if !strings.Contains(rackSales, eleDelim) {
			kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "financeBonus: rackSales parse error, '%s' has no '%s'.", rackSales, eleDelim)
			errRackList = append(errRackList, rackSales)
			continue
		}
		var eles = strings.Split(rackSales, eleDelim)
		if len(eles) < 2 {
			kdlogger.ErrorECM(ERRCODE_COMMON_PARAM_INVALID, "financeBonus: rackSales parse error, '%s' format error 1.", rackSales)
			errRackList = append(errRackList, rackSales)
			continue
		}

		var rackid = eles[0]
		var sales int64
		sales, err = strconv.ParseInt(eles[1], 0, 64)
		if err != nil {
			kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "financeBonus: sales parse error, '%s' format error 2.", rackSales)
			errRackList = append(errRackList, rackid)
			continue
		}

		errcm := t.financeBonus4OneRack(stub, rackid, fid, sales, invokeTime)
		if errcm != nil {
			kdlogger.ErrorECM(errcm.Code, "financeBonus: financeBonus4OneRack failed, error=(%s)", errcm)
			errRackList = append(errRackList, rackid)
			continue
		}
	}

	if len(errRackList) > 0 {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "financeBonus: has some error:(%s)", strings.Join(errRackList, ";"))
	}

	return nil, nil
}

func (t *KD) financeBonus4OneRack(stub shim.ChaincodeStubInterface, rackid, fid string, sales, invokeTime int64) *ErrorCodeMsg {
	var rackFinacInfoKey = t.getRackFinacInfoKey(rackid, fid)

	rfiB, err := stateCache.GetState_Ex(stub, rackFinacInfoKey)
	if err != nil {
		return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "financeBonus4OneRack:  GetState(%s) failed. error=(%s).", rackFinacInfoKey, err)
	}
	if rfiB == nil {
		return kdlogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "financeBonus4OneRack:  rackFinacInfo not exists(%s,%s).", rackid, fid)
	}
	var rfi RackFinancInfo
	err = json.Unmarshal(rfiB, &rfi)
	if err != nil {
		return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "financeBonus4OneRack:  Unmarshal failed. error=(%s).", err)
	}

	//已分红过不能再分红
	if rfi.Stage >= FINANC_STAGE_BONUS_FINISH {
		return kdlogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "financeBonus4OneRack: rack(rid=%s fid=%s) has bonus, something wrong?", rackid, fid)
	}

	rfi.CEInfo.WareSales = sales

	//货架利润
	var rackProfit = rfi.CEInfo.WareSales * int64(rfi.RFCfg.ProfitsPercent) / 100
	//经营者获取的利润
	var sellerProfit = rackProfit * rfi.RolesAllocRate.SellerRate / (rfi.RolesAllocRate.SellerRate + rfi.RolesAllocRate.FielderRate + rfi.RolesAllocRate.DeliveryRate + rfi.RolesAllocRate.PlatformRate)
	//分给投资者的利润
	var profit = sellerProfit * int64(rfi.RFCfg.InvestProfitsPercent) / 100

	profit = profit / 100 //利润的单位为分，一块钱兑一积分

	kdlogger.Debug("financeBonus4OneRack: rfi.RFCfg=%+v, rfi.RolesAllocRate=%+v", rfi.RFCfg, rfi.RolesAllocRate)

	var amtCheck int64 = 0
	var profitCheck int64 = 0
	var accProfit int64
	if rfi.UserProfitMap == nil {
		rfi.UserProfitMap = make(map[string]int64)
	}

	var cost = rfi.CEInfo.WareSales * int64(100-rfi.RFCfg.ProfitsPercent) / 100 //成本

	kdlogger.Debug("financeBonus4OneRack:rackProfit=%d, sellerProfit=%d, profit=%d, cost=%d", rackProfit, sellerProfit, profit, cost)

	for acc, amt := range rfi.UserAmountMap {
		amtCheck += amt
		//accProfit = amt * profit / rfi.AmountFinca
		//accProfit = amt * profit / (cost / 100) //分母不使用投资总额，使用当期成本, cost的单位为分，所以要再除以100
		accProfit = amt * profit / rfi.RFCfg.InvestCapacity
		rfi.UserProfitMap[acc] = accProfit
		profitCheck += accProfit
	}
	if profitCheck > profit || amtCheck != rfi.AmountFinca {
		return kdlogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "financeBonus4OneRack:  bonus check(%d,%d,%d,%d) failed.", profitCheck, profit, amtCheck, rfi.AmountFinca)
	}

	rfi.Stage = FINANC_STAGE_BONUS_FINISH

	rfiJson, err := json.Marshal(rfi)
	if err != nil {
		return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "financeBonus4OneRack:  Marshal failed. error=(%s).", err)
	}

	err = stateCache.PutState_Ex(stub, rackFinacInfoKey, rfiJson)
	if err != nil {
		return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "financeBonus4OneRack:  PutState failed. error=(%s).", err)
	}

	kdlogger.Info("financeBonus4OneRack: statistic(%v,%v,%v,%v), rfi=%+v", rfi.CEInfo.WareSales, rackProfit, sellerProfit, profit, rfi)

	return nil
}

var currentFidCache string

func (t *KD) setCurrentFid(stub shim.ChaincodeStubInterface, currentFid string) *ErrorCodeMsg {
	//因为会调用多次，所以用cache加速一下
	if len(currentFidCache) > 0 && currentFidCache == currentFid {
		return nil
	}

	hisB, err := stateCache.GetState_Ex(stub, RACKFINACHISTORY_KEY)
	if err != nil {
		return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setCurrentFid: GetState failed. error=(%s).", err)
	}
	var his RackFinancHistory
	if hisB == nil {
		his.PreCurrFID[1] = currentFid
		currentFidCache = currentFid
	} else {
		err = json.Unmarshal(hisB, &his)
		if err != nil {
			return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setCurrentFid: Unmarshal failed. error=(%s).", err)
		}
		//该函数可能调用多次，如果和当前值相同，不用再设置
		if his.PreCurrFID[1] == currentFid {
			currentFidCache = currentFid
			return nil
		}

		his.PreCurrFID[0] = his.PreCurrFID[1]
		his.PreCurrFID[1] = currentFid
		currentFidCache = currentFid
	}

	hisB, err = json.Marshal(his)
	if err != nil {
		return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setCurrentFid: Marshal failed. error=(%s).", err)
	}

	err = stateCache.PutState_Ex(stub, RACKFINACHISTORY_KEY, hisB)
	if err != nil {
		return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setCurrentFid: PutState_Ex failed. error=(%s).", err)
	}

	kdlogger.Debug("setCurrentFid: his=%+v", his)

	return nil
}

func (t *KD) getPrevAndCurrFids(stub shim.ChaincodeStubInterface) (*RackFinancHistory, *ErrorCodeMsg) {
	hisB, err := stateCache.GetState_Ex(stub, RACKFINACHISTORY_KEY)
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getPrevAndCurrFids: GetState failed. error=(%s).", err)
	}
	if hisB == nil {
		//return "", mylog.Errorf("getPrevAndCurrFids: nil info.")
		return nil, nil //如果第一次执行，这个可能为空
	}

	var his RackFinancHistory
	err = json.Unmarshal(hisB, &his)
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getPrevAndCurrFids: Unmarshal failed. error=(%s).", err)
	}

	return &his, nil
}

func (t *KD) getRecentlyFid(stub shim.ChaincodeStubInterface, getCurrent bool) (string, *ErrorCodeMsg) {
	his, errcm := t.getPrevAndCurrFids(stub)
	if errcm != nil {
		return "", kdlogger.ErrorECM(errcm.Code, "getRecentlyFid: getPrevAndCurrFids failed. error=(%s).", errcm)
	}
	if his == nil {
		return "", nil //如果第一次执行，这个可能为空
	}

	if getCurrent {
		return his.PreCurrFID[1], nil
	} else {
		return his.PreCurrFID[0], nil
	}
}
func (t *KD) getPreviousFid(stub shim.ChaincodeStubInterface) (string, *ErrorCodeMsg) {
	return t.getRecentlyFid(stub, false)
}
func (t *KD) getLatestFid(stub shim.ChaincodeStubInterface) (string, *ErrorCodeMsg) {
	return t.getRecentlyFid(stub, true)
}

func (t *KD) getUserInvestAmount(stub shim.ChaincodeStubInterface, accName, rackid, fid string) (int64, *ErrorCodeMsg) {
	/*
	   fid, err := t.getLatestFid(stub)
	   if err != nil {
	       return 0, mylog.Errorf("getUserHistoryFinance: getLatestFid failed. error=(%s).", err)
	   }
	*/

	ent, errcm := t.getAccountRackInvestInfo(stub, accName)
	if errcm != nil {
		return 0, kdlogger.ErrorECM(errcm.Code, "getUserHistoryFinance: getAccountRackInvestInfo failed. error=(%s).", errcm)
	}

	var rfkey = t.getMapKey4RackFinance(rackid, fid)

	if ent == nil || ent.RFInfoMap == nil {
		kdlogger.Debug("getUserHistoryFinance: pair(%+v) not exists in %s's acc.", rfkey, accName)
		return 0, nil
	}

	if _, ok := ent.RFInfoMap[rfkey]; !ok {
		kdlogger.Debug("getUserHistoryFinance: pair(%+v) not exists in %s's acc.", rfkey, accName)
		return 0, nil
	}

	rfiB, err := stateCache.GetState_Ex(stub, t.getRackFinacInfoKey(rackid, fid))
	if err != nil {
		return 0, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getUserHistoryFinance:  GetState failed. error=(%s).", err)
	}
	//ent中记录了该条记录，肯定是有的，没有则报错
	if rfiB == nil {
		return 0, kdlogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "getUserHistoryFinance:  FinancialInfo not exists.")
	}
	var rfi RackFinancInfo
	err = json.Unmarshal(rfiB, &rfi)
	if err != nil {
		return 0, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getUserHistoryFinance:  Unmarshal failed. error=(%s).", err)
	}
	//投资记录没有该账户，报错
	if _, ok := rfi.UserAmountMap[accName]; !ok {
		return 0, kdlogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "getUserHistoryFinance: acc not exists in UserAmountMap.")
	}

	kdlogger.Debug("getUserHistoryFinance: rfi=%+v", rfi)

	return rfi.UserAmountMap[accName], nil
}

func (t *KD) getRackFinanceAmount(stub shim.ChaincodeStubInterface, rackid, fid string) (int64, *ErrorCodeMsg) {
	/*
	   fid, err := t.getLatestFid(stub)
	   if err != nil {
	       return 0, mylog.Errorf("getRackHistoryFinance: getLatestFid failed. error=(%s).", err)
	   }
	*/

	rfiB, err := stateCache.GetState_Ex(stub, t.getRackFinacInfoKey(rackid, fid))
	if err != nil {
		return 0, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getRackHistoryFinance:  GetState failed. error=(%s).", err)
	}
	if rfiB == nil {
		kdlogger.Debug("getRackHistoryFinance: rfiB is nil.")
		return 0, nil
	}
	var rfi RackFinancInfo
	err = json.Unmarshal(rfiB, &rfi)
	if err != nil {
		return 0, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getRackHistoryFinance:  Unmarshal failed. error=(%s).", err)
	}
	var totalAmt int64 = 0
	for acc, amt := range rfi.UserAmountMap {
		if !strSliceContains(rfi.PayFinanceUserList, acc) {
			totalAmt += amt
		}
	}

	return totalAmt, nil
}

func (t *KD) financeIssueFinishAfter(stub shim.ChaincodeStubInterface, currentFid string, invokeTime int64) *ErrorCodeMsg {
	//看是否已经处理过
	finishIdB, err := stateCache.GetState_Ex(stub, RACKFINACISSUEFINISHID_KEY)
	if err != nil {
		return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "financeIssueFinishAfter: GetState(finishId) failed. error=(%s).", err)
	}
	if finishIdB == nil {
		err = stateCache.PutState_Ex(stub, RACKFINACISSUEFINISHID_KEY, []byte(currentFid))
		if err != nil {
			return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "financeIssueFinishAfter: PutState_Ex(finishId) failed. error=(%s).", err)
		}
	} else {
		var finishId = string(finishIdB)

		if finishId == currentFid {
			return kdlogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "financeIssueFinishAfter: has finished already.")
		}
	}

	//给本期理财设置为"发行完毕"
	fiB, err := stateCache.GetState_Ex(stub, t.getFinacInfoKey(currentFid))
	if err != nil {
		return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "financeIssueFinishAfter: GetState(fi=%s) failed. error=(%s).", currentFid, err)
	}

	if fiB != nil {
		var fi FinancialInfo
		err = json.Unmarshal(fiB, &fi)
		if err != nil {
			return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "financeIssueFinishAfter: Unmarshal failed. error=(%s).", err)
		}

		for _, rackid := range fi.RackList {
			var rfiKey = t.getRackFinacInfoKey(rackid, currentFid)
			rfiB, err := stateCache.GetState_Ex(stub, rfiKey)
			if err != nil {
				return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "financeIssueFinishAfter: GetState(rfi=%s,%s) failed. error=(%s).", rackid, currentFid, err)
			}
			if rfiB == nil {
				continue
			}

			var rfi RackFinancInfo
			err = json.Unmarshal(rfiB, &rfi)
			if err != nil {
				return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "financeIssueFinishAfter: Unmarshal(rfi=%s,%s) failed. error=(%s).", rackid, currentFid, err)
			}

			kdlogger.Debug("financeIssueFinishAfter: rfi=%+v", rfi)

			if rfi.Stage >= FINANC_STAGE_ISSUE_FINISH {
				return kdlogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "financeIssueFinishAfter: (%s,%s) has finished already, something wrong?", rackid, currentFid)
			}

			rfi.Stage = FINANC_STAGE_ISSUE_FINISH

			rfiB, err = json.Marshal(rfi)
			if err != nil {
				return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "financeIssueFinishAfter: Marshal(rfi=%s,%s) failed. error=(%s).", rackid, currentFid, err)
			}

			err = stateCache.PutState_Ex(stub, rfiKey, rfiB)
			if err != nil {
				return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "financeIssueFinishAfter: PutState_Ex(rfi=%s,%s) failed. error=(%s).", rackid, currentFid, err)
			}
		}
	}

	//为上一期理财续期
	return t.financeRenewalPreviousFinance(stub, currentFid, invokeTime)
}

func (t *KD) financeRenewalPreviousFinance(stub shim.ChaincodeStubInterface, currentFid string, invokeTime int64) *ErrorCodeMsg {
	//看上期的理财中，哪些没有提取的自动续期
	//调用理财续期的接口时，已经将最新的理财期号设置了（调用setCurrentFid），所以这里取前一期的期号
	preFid, errcm := t.getPreviousFid(stub)
	if errcm != nil {
		return kdlogger.ErrorECM(errcm.Code, "financeRenewal: getPreviousFid failed. error=(%s).", errcm)
	}

	kdlogger.Debug("financeRenewal: preFid=%s", preFid)

	//没有上期理财，说明是第一次，退出
	if len(preFid) == 0 {
		kdlogger.Debug("financeRenewal: no preFid.")
		return nil
	}

	if preFid == currentFid {
		return kdlogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "financeRenewal: preFid == currentFid, error.")
	}

	fiB, err := stateCache.GetState_Ex(stub, t.getFinacInfoKey(preFid))
	if err != nil {
		return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "financeRenewal: GetState(fi=%s) failed. error=(%s).", preFid, err)
	}

	//上一期没人买过理财
	if fiB == nil {
		kdlogger.Debug("financeRenewal: no fiB.")
		return nil
	}

	var fi FinancialInfo
	err = json.Unmarshal(fiB, &fi)
	if err != nil {
		return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "financeRenewal: Unmarshal failed. error=(%s).", err)
	}

	for _, rackid := range fi.RackList {
		var rfiKey = t.getRackFinacInfoKey(rackid, preFid)
		rfiB, err := stateCache.GetState_Ex(stub, rfiKey)
		if err != nil {
			return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "financeRenewal: GetState(rfi=%s,%s) failed. error=(%s).", rackid, preFid, err)
		}
		if rfiB == nil {
			continue
		}

		var rfi RackFinancInfo
		err = json.Unmarshal(rfiB, &rfi)
		if err != nil {
			return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "financeRenewal: Unmarshal(rfi=%s,%s) failed. error=(%s).", rackid, preFid, err)
		}

		kdlogger.Debug("financeRenewal: rfi=%+v", rfi)

		for acc, amt := range rfi.UserAmountMap {
			//已赎回的用户不在续期
			if strSliceContains(rfi.PayFinanceUserList, acc) {
				continue
			}

			//使用info日志，后台可查
			kdlogger.Info("financeRenewal: renewal for %s,%s", rackid, currentFid)

			//续期，即内部给这些用户买新一期的理财
			_, errcm := t.userBuyFinance(stub, acc, rackid, currentFid, "", "", "", amt, invokeTime, true, true)
			if errcm != nil {
				return kdlogger.ErrorECM(errcm.Code, "financeRenewal: userBuyFinance(rfi=%s,%s,%s) failed. error=(%s).", rackid, preFid, acc, errcm)
			}
		}
	}

	return nil
}

func (t *KD) payUserFinance(stub shim.ChaincodeStubInterface, accName, reacc, rackid string, invokeTime int64, transType, desc string, sameEntSaveTx bool) *ErrorCodeMsg {
	reaccEnt, errcm := t.getAccountRackInvestInfo(stub, reacc)
	if errcm != nil {
		return kdlogger.ErrorECM(errcm.Code, "payUserFinance: getAccountEntity(acc=%s) failed. error=(%s).", reacc, errcm)
	}
	kdlogger.Debug("payUserFinance: before reaccEnt = %+v", reaccEnt)

	if reaccEnt == nil || reaccEnt.RFInfoMap == nil || len(reaccEnt.RFInfoMap) == 0 {
		kdlogger.Debug("payUserFinance: RFInfoMap empty.")
		return nil
	}

	//获取用户投资的本金  最近一期投资的额度为本金，因为投资会自动续期
	var investAmt int64 = 0
	investAmt, errcm = t.getUserInvestAmount(stub, reacc, rackid, reaccEnt.LatestFid)
	if errcm != nil {
		return kdlogger.ErrorECM(errcm.Code, "payUserFinance: getUserInvestAmount failed. error=(%s).", errcm)
	}

	kdlogger.Debug("payUserFinance: acc=%s investAmt=%d (%s,%s)", reacc, investAmt, rackid, reaccEnt.LatestFid)

	var profit int64 = 0
	var delKeyList []string
	var paidFidList []string

	for rfkey, _ := range reaccEnt.RFInfoMap {
		r, f := t.getRackFinanceFromMapKey(rfkey)
		if r != rackid {
			continue
		}

		var rfiKey = t.getRackFinacInfoKey(rackid, f)
		rfiB, err := stateCache.GetState_Ex(stub, rfiKey)
		if err != nil {
			return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "payUserFinance:  GetState(%s,%s) failed. error=(%s).", rackid, f, err)
		}
		//ent中记录了该条记录，肯定是有的，没有则报错
		if rfiB == nil {
			return kdlogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "payUserFinance:  FinancialInfo(%s,%s) not exists.", rackid, f)
		}
		var rfi RackFinancInfo
		err = json.Unmarshal(rfiB, &rfi)
		if err != nil {
			return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "payUserFinance:  Unmarshal(%s,%s) failed. error=(%s).", rackid, f, err)
		}

		//如果已提取过，则不能再提取。这里不报错，不实际执行转账即可
		if strSliceContains(rfi.PayFinanceUserList, reacc) {
			kdlogger.Warn("payUserFinance: %s has paid already, do nothing.", reacc)
			continue
		}

		if rfi.UserProfitMap != nil {
			profit += rfi.UserProfitMap[reacc]
		}

		rfi.PayFinanceUserList = append(rfi.PayFinanceUserList, reacc)
		rfiB, err = json.Marshal(rfi)
		if err != nil {
			return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "payUserFinance:  Marshal(%s,%s) failed. error=(%s).", rackid, f, err)
		}

		err = stateCache.PutState_Ex(stub, rfiKey, rfiB)
		if err != nil {
			return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "payUserFinance:  PutState_Ex(%s,%s) failed. error=(%s).", rackid, f, err)
		}

		kdlogger.Debug("payUserFinance: acc=%s rfi=%+v", reacc, rfi)

		//delete(reaccEnt.RFInfoMap, rfkey)
		delKeyList = append(delKeyList, rfkey)
		paidFidList = append(paidFidList, f)
	}

	var totalAmt = investAmt + profit

	kdlogger.Debug("payUserFinance: %s will pay %d to %s.", accName, totalAmt, reacc)

	_, errcm = t.transferCoin(stub, accName, reacc, transType, desc, totalAmt, invokeTime, sameEntSaveTx)
	if errcm != nil {
		return kdlogger.ErrorECM(errcm.Code, "payUserFinance:  transferCoin(%s) failed. error=(%s).", reacc, errcm)
	}

	//将赎回的理财期号写入已赎回列表
	for _, fid := range paidFidList {
		if !strSliceContains(reaccEnt.PaidFidList, fid) {
			reaccEnt.PaidFidList = append(reaccEnt.PaidFidList, fid)
		}
	}

	//删除已购买的货架理财信息
	for _, rfkey := range delKeyList {
		delete(reaccEnt.RFInfoMap, rfkey)
	}

	errcm = t.setAccountRackInvestInfo(stub, reaccEnt)
	if errcm != nil {
		return kdlogger.ErrorECM(errcm.Code, "payUserFinance:  setAccountRackInvestInfo(%s) failed. error=(%s).", reacc, errcm)
	}

	kdlogger.Debug("payUserFinance: after reaccEnt = %+v", *reaccEnt)
	kdlogger.Info("payUserFinance: %s pay %v,%v for %s,  rf=%+v", accName, investAmt, profit, reacc, reaccEnt)

	return nil
}

const rackFinanceKeyDelim = "_@!&!@_"

func (t *KD) getMapKey4RackFinance(rackid, fid string) string {
	return rackid + rackFinanceKeyDelim + fid
}
func (t *KD) getRackFinanceFromMapKey(key string) (string, string) {
	pair := strings.Split(key, rackFinanceKeyDelim)
	return pair[0], pair[1]
}

func (t *KD) getUserFinanceProfit(stub shim.ChaincodeStubInterface, accName, rackid string) (int64, *ErrorCodeMsg) {
	accEnt, errcm := t.getAccountRackInvestInfo(stub, accName)
	if errcm != nil {
		return 0, kdlogger.ErrorECM(errcm.Code, "getUserFinanceProfit: getAccountEntity(acc=%s) failed. error=(%s).", accName, errcm)
	}
	kdlogger.Debug("getUserFinanceProfit:  accEnt = %+v", accEnt)

	if accEnt == nil || accEnt.RFInfoMap == nil || len(accEnt.RFInfoMap) == 0 {
		kdlogger.Debug("getUserFinanceProfit: RFInfoMap empty.")
		return 0, nil
	}

	var profit int64 = 0

	for rfkey, _ := range accEnt.RFInfoMap {
		r, f := t.getRackFinanceFromMapKey(rfkey)
		if r != rackid {
			continue
		}

		var rfiKey = t.getRackFinacInfoKey(rackid, f)
		rfiB, err := stateCache.GetState_Ex(stub, rfiKey)
		if err != nil {
			return profit, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getUserFinanceProfit:  GetState(%s,%s) failed. error=(%s).", rackid, f, err)
		}
		//ent中记录了该条记录，肯定是有的，没有则报错
		if rfiB == nil {
			return profit, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getUserFinanceProfit:  FinancialInfo(%s,%s) not exists.", rackid, f)
		}
		var rfi RackFinancInfo
		err = json.Unmarshal(rfiB, &rfi)
		if err != nil {
			return profit, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getUserFinanceProfit:  Unmarshal(%s,%s) failed. error=(%s).", rackid, f, err)
		}

		if rfi.UserProfitMap != nil {
			profit += rfi.UserProfitMap[accName]
		}
	}

	return profit, nil
}

func (t *KD) getRestFinanceCapacityForRack(stub shim.ChaincodeStubInterface, rackid, fid string) (int64, *ErrorCodeMsg) {
	var rfc RackFinanceCfg
	_, errcm := t.getRackFinancCfg(stub, rackid, &rfc)
	if errcm != nil {
		return 0, kdlogger.ErrorECM(errcm.Code, "getRestFinanceCapacityForRack:  getRackFinancCfg(%s) failed. error=(%s).", rackid, errcm)
	}

	//获取前一期的将要续期的金额
	var preAmt int64 = 0

	hisFids, errcm := t.getPrevAndCurrFids(stub)
	if errcm != nil {
		return 0, kdlogger.ErrorECM(errcm.Code, "getRestFinanceCapacityForRack: getPrevAndCurrFids failed. error=(%s).", errcm)
	}
	//如果历史fid为空，说明没有前期理财
	if hisFids != nil {
		var preFid string

		//查询剩余投资额时， 最新理财id可能设置了，也可能没设置。所以用入参fid和 hisFids.PreCurrFID[1]（即设置过的最新理财id）相比。
		//如果相同，说明设置过，使用hisFids.PreCurrFID[0]，不相同，说明没设置过，使用hisFids.PreCurrFID[1]
		if fid == hisFids.PreCurrFID[1] {
			preFid = hisFids.PreCurrFID[0]
		} else {
			preFid = hisFids.PreCurrFID[1]
		}
		kdlogger.Debug("getRestFinanceCapacityForRack: preFid=[%s]", preFid)

		//前期理财id为空，说明没有前期，不用处理
		if len(preFid) > 0 {
			preAmt, errcm = t.getRackFinanceAmount(stub, rackid, preFid)
			if errcm != nil {
				return 0, kdlogger.ErrorECM(errcm.Code, "getRestFinanceCapacityForRack: getRackFinanceAmount failed. error=(%s).", errcm)
			}
			kdlogger.Debug("getRestFinanceCapacityForRack: preAmt=%v", preAmt)
		}
	}

	//获取当期理财已投资金额
	var currAmt int64 = 0
	currAmt, errcm = t.getRackFinanceAmount(stub, rackid, fid)
	if errcm != nil {
		return 0, kdlogger.ErrorECM(errcm.Code, "getRestFinanceCapacityForRack: getRackFinanceAmount failed. error=(%s).", errcm)
	}

	kdlogger.Debug("getRestFinanceCapacityForRack: InvestCapacity=%v, preAmt=%v, currAmt=%v", rfc.InvestCapacity, preAmt, currAmt)

	var restAmt = rfc.InvestCapacity - preAmt - currAmt
	if restAmt < 0 {
		kdlogger.Warn("getRestFinanceCapacityForRack: restAmt < 0, something wrong(%d,%d).", rfc.InvestCapacity, preAmt)
		restAmt = 0
	}

	return restAmt, nil
}

/*
//获取某个账户的货架融资信息
func (t *KD) _getAccRackFinanceTx(stub shim.ChaincodeStubInterface, accName, rackid string) ([]byte, *ErrorCodeMsg) {
	accEnt, err := t.getAccountEntity(stub, accName)
	if err != nil {
		return nil, mylog.Errorf("payUserFinance: getAccountEntity(acc=%s) failed. error=(%s).", accName, err)
	}
	mylog.Debug("payUserFinance: before reaccEnt = %+v", accEnt)

	if accEnt.RFInfoMap == nil || len(accEnt.RFInfoMap) == 0 {
		mylog.Debug("payUserFinance: RFInfoMap empty.")
		return nil, nil
	}

	mylog.Debug("payUserFinance: acc=%s investAmt=%d (%s,%s)", accName, investAmt, rackid, accEnt.LatestFid)

	for rfkey, _ := range accEnt.RFInfoMap {
		r, f := t.getRackFinanceFromMapKey(rfkey)
		if r != rackid {
			continue
		}

		var rfiKey = t.getRackFinacInfoKey(rackid, f)
		rfiB, err := stateCache.GetState_Ex(stub, rfiKey)
		if err != nil {
			return mylog.Errorf("payUserFinance:  GetState(%s,%s) failed. error=(%s).", rackid, f, err)
		}
		//ent中记录了该条记录，肯定是有的，没有则报错
		if rfiB == nil {
			return mylog.Errorf("payUserFinance:  FinancialInfo(%s,%s) not exists.", rackid, f)
		}
		var rfi RackFinancInfo
		err = json.Unmarshal(rfiB, &rfi)
		if err != nil {
			return mylog.Errorf("payUserFinance:  Unmarshal(%s,%s) failed. error=(%s).", rackid, f, err)
		}

		//如果已提取过，则不再显示。
		if t.StrSliceContains(rfi.PayFinanceUserList, reacc) {
			mylog.Warn("payUserFinance: %s has paid already, do nothing.", reacc)
			continue
		}

		if rfi.UserProfitMap != nil {
			profit += rfi.UserProfitMap[reacc]
		}

		mylog.Debug("payUserFinance: acc=%s rfi=%+v", reacc, rfi)

	}

	mylog.Debug("payUserFinance: after reaccEnt = %+v", *accEnt)

	return nil
}

func (t *KD) queryAccRackFinanceTx(stub shim.ChaincodeStubInterface, accName string, begIdx, count, begTime, endTime, isAsc bool) ([]byte, *ErrorCodeMsg) {
	var err error

	var retTransInfo []byte
	var queryResult QueryRackFinanceTx
	queryResult.NextSerial = -1
	queryResult.FinanceRecords = []RackFinanceRecd{} //初始化为空，即使下面没查到数据也会返回'[]'

	retTransInfo, err = json.Marshal(queryResult)
	if err != nil {
		return nil, mylog.Errorf("queryAccRackFinanceTx Marshal failed.error=(%s)", err)
	}

	//begIdx从1开始
	if begIdx < 1 {
		begIdx = 1
	}
	//endTime为负数，查询到最新时间
	if endTime < 0 {
		endTime = math.MaxInt64
	}

	if count == 0 {
		mylog.Warn("queryAccRackFinanceTx nothing to do(%d).", count)
		return retTransInfo, nil
	}

	accEnt, err := t.getAccountEntity(stub, accName)
	if err != nil {
		if err == ErrNilEntity {
			mylog.Warn("queryAccRackFinanceTx acc '%s' not exists.")
			return retTransInfo, nil
		}

		return nil, mylog.Errorf("queryAccRackFinanceTx getAccountEntity(%s) failed.error=(%s)", accName, err)
	}

	if accEnt.AccEnt_Ext_RackFinance.RFInfoMap == nil {
		mylog.Warn("queryAccRackFinanceTx acc '%s' have no tx.")
		return retTransInfo, nil
	}

	var loopCnt int64 = 0
	var trans *Transaction
	if isAsc { //升序
		for loop := begIdx; loop <= maxSeq; loop++ {
			//处理了count条时，不再处理
			if loopCnt >= count {
				break
			}

			trans, err = t.getOnceTransInfo(stub, t.getTransInfoKey(stub, loop))
			if err != nil {
				mylog.Error("queryAccRackFinanceTx getQueryTransInfo(idx=%d) failed.error=(%s)", loop, err)
				continue
			}
			//取匹配的transLvl
			var qTrans QueryTransRecd
			if trans.TransLvl&transLvl != 0 && trans.Time >= begTime && trans.Time <= endTime {
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

			trans, err = t.getOnceTransInfo(stub, t.getTransInfoKey(stub, loop))
			if err != nil {
				mylog.Error("queryAccRackFinanceTx getQueryTransInfo(idx=%d) failed.error=(%s)", loop, err)
				continue
			}
			//取匹配的transLvl
			var qTrans QueryTransRecd
			if trans.TransLvl&transLvl != 0 && trans.Time >= begTime && trans.Time <= endTime {
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
		return nil, mylog.Errorf("queryAccRackFinanceTx Marshal failed.error=(%s)", err)
	}

	return retTransInfo, nil
}
*/

func (t *KD) getAccountRackInvestInfoKey(accName string) string {
	return RACK_ACCINVESTINFO_PREFIX + accName
}

func (t *KD) getAccountRackInvestInfo(stub shim.ChaincodeStubInterface, accName string) (*AccRackInvest, *ErrorCodeMsg) {
	arfiB, err := stateCache.GetState_Ex(stub, t.getAccountRackInvestInfoKey(accName))
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getAccountRackInvestInfo: GetState failed.error=(%s), acc=%s", err, accName)
	}

	if arfiB == nil {
		return nil, nil
	} else {
		var arfi AccRackInvest
		err = json.Unmarshal(arfiB, &arfi)
		if err != nil {
			return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getAccountRackInvestInfo: Unmarshal failed.error=(%s), acc=%s", err, accName)
		}
		return &arfi, nil
	}
}

func (t *KD) setAccountRackInvestInfo(stub shim.ChaincodeStubInterface, accRackInvest *AccRackInvest) *ErrorCodeMsg {
	var accName = accRackInvest.EntID
	if len(accName) == 0 {
		return kdlogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "setAccountRackInvestInfo: accName is nil.")
	}

	ariB, err := json.Marshal(accRackInvest)
	if err != nil {
		return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setAccountRackInvestInfo: Marshal failed.error=(%s), acc=%s", err, accName)
	}

	err = stateCache.PutState_Ex(stub, t.getAccountRackInvestInfoKey(accName), ariB)
	if err != nil {
		return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setAccountRackInvestInfo: putState_Ex failed.error=(%s), acc=%s", err, accName)
	}

	return nil
}

/* ----------------------- 货架融资相关 end ----------------------- */

func (t *KD) setAccountPasswd(stub shim.ChaincodeStubInterface, accName, pwd string) *ErrorCodeMsg {
	salt := md5.Sum([]byte(accName))

	hash, err := kdCrypto.GenCipher(pwd, salt[:])
	if err != nil {
		return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setAccountPasswd: GenCipher failed.error=(%s), acc=%s", err, accName)
	}

	err = stateCache.PutState_Ex(stub, t.getUserCipherKey(accName), hash)
	if err != nil {
		return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setAccountPasswd: putState_Ex failed.error=(%s), acc=%s", err, accName)
	}

	return nil
}
func (t *KD) authAccountPasswd(stub shim.ChaincodeStubInterface, accName, pwd string) (bool, *ErrorCodeMsg) {
	cipher, err := stateCache.GetState_Ex(stub, t.getUserCipherKey(accName))
	if err != nil {
		return false, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "AuthAccountPasswd: GetState failed.error=(%s), acc=%s", err, accName)
	}

	if cipher == nil || len(cipher) == 0 {
		return false, kdlogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "AuthAccountPasswd: Cipher is nil, acc=%s", accName)
	}

	ok, err := kdCrypto.AuthPass(cipher, pwd)
	if err != nil {
		return false, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "AuthAccountPasswd: AuthPass failed.error=(%s), acc=%s", err, accName)
	}

	if ok {
		return true, nil
	}

	return false, nil
}

func (t *KD) isSetAccountPasswd(stub shim.ChaincodeStubInterface, accName string) (bool, *ErrorCodeMsg) {

	cipher, err := stateCache.GetState_Ex(stub, t.getUserCipherKey(accName))
	if err != nil {
		return false, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "isSetAccountPasswd: GetState failed.error=(%s), acc=%s", err, accName)
	}

	if cipher == nil || len(cipher) == 0 {
		return false, nil
	}

	return true, nil
}

func (t *KD) changeAccountPasswd(stub shim.ChaincodeStubInterface, accName, oldpwd, newpwd string) *ErrorCodeMsg {
	ok, errcm := t.authAccountPasswd(stub, accName, oldpwd)
	if errcm != nil {
		return kdlogger.ErrorECM(errcm.Code, "changeAccountPasswd: authAccountPasswd failed.error=(%s), acc=%s", errcm, accName)
	}
	if !ok {
		return kdlogger.ErrorECM(ERRCODE_COMMON_CHECK_FAILED, "changeAccountPasswd: authAccountPasswd not pass.")
	}

	errcm = t.setAccountPasswd(stub, accName, newpwd)
	if errcm != nil {
		return kdlogger.ErrorECM(errcm.Code, "changeAccountPasswd: setAccountPasswd failed.error=(%s), acc=%s", errcm, accName)
	}

	return nil
}

func (t *KD) decodeAccountPasswd(pwdBase64 string) (string, *ErrorCodeMsg) {

	pwdEncrypt, err := base64.StdEncoding.DecodeString(pwdBase64)
	if err != nil {
		return "", kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "decodeAccountPasswd: DecodeString failed. error=(%s)", err)
	}

	pwdB, err := kdCrypto.AESDecrypt(256, []byte(PWD_ENCRYPT_KEY), []byte(PWD_ENCRYPT_IV), pwdEncrypt)
	if err != nil {
		return "", kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "decodeAccountPasswd: AESDecrypt failed. error=(%s)", err)
	}

	return string(pwdB), nil
}

func (t *KD) dateConvertWhenLoad(stub shim.ChaincodeStubInterface, srcCcid, key string, valueB []byte) (string, []byte, *ErrorCodeMsg) {
	//var err error
	var newKey = key
	var newValB = valueB

	if srcCcid == "kd1.0" {
		/*
			if strings.HasPrefix(key, ACC_ENTITY_PREFIX) {
				type AccEnt_Ext_RackFinance struct {
					RFInfoMap   map[string]int `json:"rfim"` //用户参与投资的货架融资信息，保存RackFinancInfo的两个key，rackid和financeId。用map是因为容易删除某个元素，因为用户提取积分后，会删除这两个key。map的value无意义。
					LatestFid   string         `json:"lfid"` //用户购买的最新一期的理财
					PaidFidList []string       `json:"pfl"`  //用户已经赎回的理财期号。
				}
				type Old_AccountEntity struct {
					EntID           string            `json:"id"`    //银行/企业/项目/个人ID
					EntType         int               `json:"etp"`   //类型 中央银行:1, 企业:2, 项目:3, 个人:4
					TotalAmount     int64             `json:"tamt"`  //货币总数额(发行或接收)
					RestAmount      int64             `json:"ramt"`  //账户余额
					Time            int64             `json:"time"`  //开户时间
					Owner           string            `json:"own"`   //该实例所属的用户
					OwnerCert       []byte            `json:"ocert"` //证书
					AuthUserCertMap map[string][]byte `json:"aucm"`  //授权用户证书 格式：{user1:cert1, user2:cert2}  因为可能会涉及到某些用户会授权之后操作其他用户的账户，所以map中不仅包含自己的证书，还包含授权用户的证书
					Cipher          []byte            `json:"cip"`   //Cipher
					AccEnt_Ext_RackFinance
				}

				var oldEnt Old_AccountEntity
				err = json.Unmarshal(valueB, &oldEnt)
				if err != nil {
					return "", nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "dateConvertWhenLoad: Unmarshal failed, error=(%s)", err)
				}

				var newEnt AccountEntity
				existEnt, errcm := Base.getAccountEntity(stub, oldEnt.EntID)
				if errcm != nil {
					if errcm != ErrNilEntity {
						return "", nil, kdlogger.ErrorECM(errcm.Code, "dateConvertWhenLoad: getAccountEntity failed, error=(%s)", errcm)
					}
				}

				if existEnt == nil {
					newEnt.EntID = oldEnt.EntID
					newEnt.EntType = oldEnt.EntType
					newEnt.TotalAmount = oldEnt.TotalAmount
					newEnt.RestAmount = oldEnt.RestAmount
					newEnt.Time = oldEnt.Time
					newEnt.Owner = oldEnt.Owner
					newEnt.OwnerPubKeyHash = ""
					newEnt.OwnerIdentityHash = ""
				} else {
					newEnt = *existEnt
					newEnt.EntID = oldEnt.EntID
					newEnt.EntType = oldEnt.EntType
					newEnt.TotalAmount = oldEnt.TotalAmount
					newEnt.RestAmount = oldEnt.RestAmount
					newEnt.Time = oldEnt.Time
					newEnt.Owner = oldEnt.Owner
				}

				err = stateCache.PutState_Ex(stub, t.getUserCipherKey(oldEnt.EntID), oldEnt.Cipher)
				if err != nil {
					return "", nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "dateConvertWhenLoad: putState_Ex(newEnt) failed, error=(%s)", err)
				}

				newValB, err = json.Marshal(newEnt)
				if err != nil {
					return "", nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "dateConvertWhenLoad: Marshal(newEnt) failed, error=(%s)", err)
				}

				var ari AccRackInvest
				ari.EntID = oldEnt.EntID
				ari.LatestFid = oldEnt.AccEnt_Ext_RackFinance.LatestFid
				ari.PaidFidList = oldEnt.AccEnt_Ext_RackFinance.PaidFidList
				ari.RFInfoMap = oldEnt.AccEnt_Ext_RackFinance.RFInfoMap

				errcm = t.setAccountRackInvestInfo(stub, &ari)
				if errcm != nil {
					return "", nil, kdlogger.ErrorECM(errcm.Code, "dateConvertWhenLoad: setAccountRackInvestInfo failed, error=(%s)", errcm)
				}
			}
		*/
	}

	return newKey, newValB, nil
}

func (t *KD) loadAfter(stub shim.ChaincodeStubInterface, srcCcid string) *ErrorCodeMsg {

	if srcCcid == "" {

	}

	return nil
}

func (t *KD) isAdmin(stub shim.ChaincodeStubInterface, accName string) bool {
	return true
}

func (t *KD) transferCoin(stub shim.ChaincodeStubInterface, from, to, transType, description string, amount, transeTime int64, sameEntSaveTrans bool) ([]byte, *ErrorCodeMsg) {

	appidB, err := stateCache.GetState_Ex(stub, APPID_KEY)
	if err != nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "transferCoin: get appid failed, error=(%s).", err)
	}
	if appidB == nil {
		return nil, kdlogger.ErrorECM(ERRCODE_COMMON_INNER_ERROR, "transferCoin: appid not register.")
	}
	var appid = string(appidB)

	var txInfo TransferInfo

	txInfo.AppID = appid
	txInfo.FromID = from
	txInfo.ToID = to
	txInfo.Description = description
	txInfo.TransType = transType
	txInfo.Amount = amount
	txInfo.Time = transeTime

	transInfoCache.Add(stub, &txInfo)

	kdlogger.Debug("transInfos=%+v", transInfoCache.Get(stub))

	return nil, nil
}

func (t *KD) getUserCipherKey(accName string) string {
	return ACCOUT_CIPHER_PREFIX + accName
}

func (t *KD) getTransSeq(stub shim.ChaincodeStubInterface, transSeqKey string) (int64, *ErrorCodeMsg) {
	seqB, err := stateCache.GetState_Ex(stub, transSeqKey)
	if err != nil {
		return -1, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getTransSeq GetState failed.error=(%s)", err)
	}
	//如果不存在则创建
	if seqB == nil {
		err = stateCache.PutState_Ex(stub, transSeqKey, []byte("0"))
		if err != nil {
			return -1, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "initTransSeq PutState failed.error=(%s)", err)
		}
		return 0, nil
	}

	seq, err := strconv.ParseInt(string(seqB), 10, 64)
	if err != nil {
		return -1, kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "getTransSeq ParseInt failed.seq=%+v, error=(%s)", seqB, err)
	}

	return seq, nil
}
func (t *KD) setTransSeq(stub shim.ChaincodeStubInterface, transSeqKey string, seq int64) *ErrorCodeMsg {
	err := stateCache.PutState_Ex(stub, transSeqKey, []byte(strconv.FormatInt(seq, 10)))
	if err != nil {
		return kdlogger.ErrorECM(ERRCODE_COMMON_SYS_ERROR, "setTransSeq PutState failed.error=(%s)", err)
	}

	return nil
}

func main() {
	// for debug
	kdlogger.SetDefaultLvl(shim.LogInfo)

	err := shim.Start(new(KD))
	if err != nil {
		kdlogger.Error("Error starting EventSender chaincode: %s", err)
	}
}
