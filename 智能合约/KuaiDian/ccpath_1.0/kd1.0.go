package main

import (
	"bufio"
	"bytes"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"time"
	//"sort"
	"crypto/md5"
	"strconv"
	"strings"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	//"github.com/hyperledger/fabric/core/crypto/primitives"
	pb "github.com/hyperledger/fabric/protos/peer"
)

const (
	ENT_CENTERBANK = 1
	ENT_COMPANY    = 2
	ENT_PROJECT    = 3
	ENT_PERSON     = 4

	TRANS_PAY    = 0 //交易支出
	TRANS_INCOME = 1 //交易收入

	ATTR_USRROLE = "usrrole"
	ATTR_USRNAME = "usrname"
	ATTR_USRTYPE = "usrtype"

	//每个key都加上前缀，便于区分，也便于以后在线升级时处理方便
	TRANSSEQ_PREFIX      = "!kd@txSeqPre~"          //序列号生成器的key的前缀。使用的是worldState存储
	TRANSINFO_PREFIX     = "!kd@txInfoPre~"         //全局交易信息的key的前缀。使用的是worldState存储
	ONE_ACC_TRANS_PREFIX = "!kd@oneAccTxPre~"       //存储单个账户的交易的key前缀
	UER_ENTITY_PREFIX    = "!kd@usrEntPre~"         //存储某个用户的用户信息的key前缀。
	ACC_ENTITY_PREFIX    = "!kd@accEntPre~"         //存储某个账户的账户信息的key前缀。
	CENTERBANK_ACC_KEY   = "!kd@centerBankAccKey@!" //央行账户的key。使用的是worldState存储
	ALL_ACC_INFO_KEY     = "!kd@allAccInfoKey@!"    //存储所有账户名的key。使用的是worldState存储
	ACC_STATIC_INFO_KEY  = "!kd@accStatcInfoKey@!"  //存储所有账户统计信息的key。
	ACC_AMTLOCK_PREFIX   = "!kd@accAmtLockPre~"     //账户金额锁定key前缀

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

	RACKFINAC_INVEST = 0 //融资明细中的投资
	RACKFINAC_PROFIT = 1 //融资明细中的收益

	RACK_GLOBAL_CFG_RACK_ID = "_global__rack___" //货架全局配置的id

	MULTI_STRING_DELIM = ':' //多个string的分隔符
	INVALID_MD5_VALUE  = "-"

	ACC_INVALID_CHAR_SET = ",;:/\\"                  //账户中不能包含的字符
	COIN_ISSUE_ACC_ENTID = "issueCoinVirtualAccount" //发行货币的账户id

	RACK_ROLE_SELLER   = "slr"
	RACK_ROLE_FIELDER  = "fld"
	RACK_ROLE_DELIVERY = "dvy"
	RACK_ROLE_PLATFORM = "pfm"
)

//这里定义的错误码会返回给前端，所以不要修改已有的错误码，如果要修改，请和前端一起修改
const (
	ERRCODE_BEGIN                           = iota + 10000
	ERRCODE_TRANS_PAY_ACCOUNT_NOT_EXIST     //付款账号不存在
	ERRCODE_TRANS_PAYEE_ACCOUNT_NOT_EXIST   //收款账号不存在
	ERRCODE_TRANS_BALANCE_NOT_ENOUGH        //账号余额不足d
	ERRCODE_TRANS_PASSWD_INVALID            //密码错误
	ERRCODE_TRANS_AMOUNT_INVALID            //转账金额不合法
	ERRCODE_TRANS_BALANCE_NOT_ENOUGH_BYLOCK //锁定部分余额导致余额不足

)

const (
	FINANC_STAGE_INIT         = iota
	FINANC_STAGE_ISSUE_BEGING //理财发行开始
	FINANC_STAGE_ISSUE_FINISH //理财发行结束，即不能再买该期理财
	FINANC_STAGE_BONUS_FINISH //理财分红结束
)

type UserEntity struct {
	EntID       string   `json:"id"`  //ID
	AuthAccList []string `json:"aal"` //此user授权给了哪些账户
	//AccList     []string `json:"al"`  //此user的账户列表   //目前一个用户就一个账户，暂时不用这个字段
}

//账户信息Entity
// 一系列ID（或账户）都定义为字符串类型。因为putStat函数的第一个参数为字符串类型，这些ID（或账户）都作为putStat的第一个参数；另外从SDK传过来的参数也都是字符串类型。
type AccountEntity struct {
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

//货架融资，给账户信息附加的信息
type AccEnt_Ext_RackFinance struct {
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

var mylog = InitMylog("kd")
var myCrypto = MyCryptoNew()

var ErrNilEntity = errors.New("nil entity.")

type KD struct {
}

func (t *KD) Init(stub shim.ChaincodeStubInterface) pb.Response {
	mylog.Debug("Enter Init")
	function, args := stub.GetFunctionAndParameters()
	mylog.Debug("func =%s, args = %+v", function, args)

	/* 这里不输入当前时间参数，因为fabic0.6版本，如果init输入了变量参数，每次deploy出来的chainCodeId不一致。
	   var argCount = 1
	   if len(args) < argCount {
	       return nil, mylog.Errorf("Init miss arg, got %d, at least need %d.", len(args), argCount)
	   }

	       times, err := strconv.ParseInt(args[0], 0, 64)
	       if err != nil {
	           return nil, mylog.Errorf("Invoke convert times(%s) failed. err=%s", args[0], err)
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
	eap.UpdateTime = 0

	eapJson, err := json.Marshal(eap)
	if err != nil {
		return shim.Error(mylog.SError("Init Marshal error, err=%s.", err))
	}

	err = t.putState_Ex(stub, t.getGlobalRackAllocRateKey(), eapJson)
	if err != nil {
		return shim.Error(mylog.SError("Init PutState_Ex error, err=%s.", err))
	}

	//全局销售额区间奖励积分设置
	var serc ScoreEncouragePercentCfg
	serc.Rackid = RACK_GLOBAL_CFG_RACK_ID //全局比例
	serc.UpdateTime = 0
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
		return shim.Error(mylog.SError("Init Marshal(serc) error, err=%s.", err))
	}

	err = t.putState_Ex(stub, t.getGlobalRackEncourageScoreCfgKey(), sercJson)
	if err != nil {
		return shim.Error(mylog.SError("Init PutState_Ex(serc) error, err=%s.", err))
	}

	var rfc RackFinanceCfg
	rfc.Rackid = RACK_GLOBAL_CFG_RACK_ID //全局
	rfc.UpdateTime = 0
	rfc.ProfitsPercent = 20       //20%的利润率
	rfc.InvestProfitsPercent = 90 //90%的利润分给投资人
	rfc.InvestCapacity = 2000     //目前是积分投资，单位为积分的单位

	rfcJson, err := json.Marshal(rfc)
	if err != nil {
		return shim.Error(mylog.SError("Init Marshal(rfc) error, err=%s.", err))
	}

	err = t.putState_Ex(stub, t.getGlobalRackFinancCfgKey(), rfcJson)
	if err != nil {
		return shim.Error(mylog.SError("Init PutState_Ex(rfc) error, err=%s.", err))
	}

	//虚拟一个超级账户，设置货币发行总额，给央行发行货币。
	var issueEntity AccountEntity
	issueEntity.EntID = COIN_ISSUE_ACC_ENTID
	issueEntity.EntType = -1
	issueEntity.TotalAmount = 10000000000
	issueEntity.RestAmount = 10000000000
	issueEntity.Time = 0
	issueEntity.Owner = "system"

	err = t.setAccountEntity(stub, &issueEntity)
	if err != nil {
		return shim.Error(mylog.SError("Init PutState_Ex(issueEntity) error, err=%s.", err))
	}

	return shim.Success(nil)
}

// Transaction makes payment of X units from A to B
func (t *KD) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	mylog.Debug("Enter Invoke")
	function, args := stub.GetFunctionAndParameters()
	mylog.Debug("func =%s, args = %+v", function, args)
	var err error

	var fixedArgCount = 3
	if len(args) < fixedArgCount {
		return shim.Error(mylog.SError("Invoke miss arg, got %d, at least need %d.", len(args), fixedArgCount))
	}

	var userName = args[0]
	var accName = args[1]
	var invokeTime int64 = 0

	invokeTime, err = strconv.ParseInt(args[2], 0, 64)
	if err != nil {
		return shim.Error(mylog.SError("Invoke convert invokeTime(%s) failed. err=%s", args[2], err))
	}

	var userAttrs *UserAttrs
	var accountEnt *AccountEntity = nil

	userAttrs, err = t.getUserAttrs(stub)
	if err != nil {
		return shim.Error(mylog.SError("Invoke getUserAttrs failed. err=%s", err))
	}

	//开户时和更新证书时不需要校验证书。 开户时证书还没传入无法验证；更新证书如果是admin的证书损坏或更新，也不能验证。
	if function != "account" && function != "accountCB" && function != "updateCert" {

		accountEnt, err = t.getAccountEntity(stub, accName)
		if err != nil {
			return shim.Error(mylog.SError("Invoke getAccountEntity failed. err=%s", err))
		}

		//校验修改Entity的用户身份，只有Entity的所有者才能修改自己的Entity
		if ok, _ := t.verifyIdentity(stub, userName, accountEnt, userAttrs); !ok {
			return shim.Error(mylog.SError("verify and account(%s) failed. \n", accName))
		}
	}

	if function == "issue" {
		mylog.Debug("Enter issue")

		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return shim.Error(mylog.SError("Invoke(issue) miss arg, got %d, at least need %d.", len(args), argCount))
		}

		var issueAmount int64
		issueAmount, err = strconv.ParseInt(args[fixedArgCount], 0, 64)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(issue) convert issueAmount(%s) failed. err=%s", args[fixedArgCount], err))
		}
		mylog.Debug("issueAmount= %+v", issueAmount)

		tmpByte, err := t.getCenterBankAcc(stub)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(issue) getCenterBankAcc failed. err=%s", err))
		}
		//如果没有央行账户，报错。否则校验账户是否一致。
		if tmpByte == nil {
			return shim.Error(mylog.SError("Invoke(issue) getCenterBankAcc nil."))
		} else {
			if accName != string(tmpByte) {
				return shim.Error(mylog.SError("Invoke(issue) centerBank account is %s, can't issue to %s.", string(tmpByte), accName))
			}
		}

		_, err = t.issueCoin(stub, accName, issueAmount, invokeTime)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(issue) issueCoin failed. err=%s", err))
		}
		return shim.Success(nil)

	} else if function == "account" {
		mylog.Debug("Enter account")
		var usrType int

		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return shim.Error(mylog.SError("Invoke(account) miss arg, got %d, at least need %d.", len(args), argCount))
		}

		userCertEncrypt, err := base64.StdEncoding.DecodeString(args[fixedArgCount])
		if err != nil {
			return shim.Error(mylog.SError("Invoke(account) DecodeString failed. err=%s", err))
		}

		userCert, err := myCrypto.AESDecrypt(256, []byte(CERT_ENCRYPT_KEY), []byte(CERT_ENCRYPT_IV), userCertEncrypt)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(account): AESDecrypt failed. err=%s", err))
		}

		usrType = 0

		_, err = t.newAccount(stub, accName, usrType, userName, userCert, invokeTime, false)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(account) newAccount failed. err=%s", err))
		}
		return shim.Success(nil)

	} else if function == "accountCB" {
		mylog.Debug("Enter accountCB")
		var usrType int = 0

		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return shim.Error(mylog.SError("Invoke(accountCB) miss arg, got %d, at least need %d.", len(args), argCount))
		}

		userCertEncrypt, err := base64.StdEncoding.DecodeString(args[fixedArgCount])
		if err != nil {
			return shim.Error(mylog.SError("Invoke(accountCB) DecodeString failed. err=%s", err))
		}
		userCert, err := myCrypto.AESDecrypt(256, []byte(CERT_ENCRYPT_KEY), []byte(CERT_ENCRYPT_IV), userCertEncrypt)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(account): AESDecrypt failed. err=%s", err))
		}

		tmpByte, err := t.getCenterBankAcc(stub)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(accountCB) getCenterBankAcc failed. err=%s", err))
		}

		//如果央行账户已存在，报错
		if tmpByte != nil {
			return shim.Error(mylog.SError("Invoke(accountCB) CBaccount(%s) exists.", string(tmpByte)))
		}

		_, err = t.newAccount(stub, accName, usrType, userName, userCert, invokeTime, true)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(accountCB) openAccount failed. err=%s", err))
		}

		err = t.setCenterBankAcc(stub, accName)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(accountCB) setCenterBankAcc failed. err=%s", err))
		}

		return shim.Success(nil)

	} else if function == "transefer" {
		var argCount = fixedArgCount + 5
		if len(args) < argCount {
			return shim.Error(mylog.SError("Invoke(transefer) miss arg, got %d, at least need %d.", len(args), argCount))
		}

		var toAcc = args[fixedArgCount]
		var transType = args[fixedArgCount+1]
		var description = args[fixedArgCount+2]

		var transAmount int64
		transAmount, err = strconv.ParseInt(args[fixedArgCount+3], 0, 64)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(transefer): convert issueAmount(%s) failed. err=%s", args[fixedArgCount+3], err))
		}
		mylog.Debug("transAmount= %+v", transAmount)

		var sameEntSaveTrans = args[fixedArgCount+4] //如果转出和转入账户相同，是否记录交易 0表示不记录 1表示记录
		var sameEntSaveTransFlag bool = false
		if sameEntSaveTrans == "1" {
			sameEntSaveTransFlag = true
		}

		_, err = t.transferCoin(stub, accName, toAcc, transType, description, transAmount, invokeTime, sameEntSaveTransFlag)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(transefer) transferCoin failed. err=%s", err))
		}
		return shim.Success(nil)

	} else if function == "transefer2" { //带交易密码功能
		var argCount = fixedArgCount + 6
		if len(args) < argCount {
			return shim.Error(mylog.SError("Invoke(transeferUsePwd) miss arg, got %d, at least need %d.", len(args), argCount))
		}

		var toAcc = args[fixedArgCount]
		var transType = args[fixedArgCount+1]
		var description = args[fixedArgCount+2]

		var transAmount int64
		transAmount, err = strconv.ParseInt(args[fixedArgCount+3], 0, 64)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(transeferUsePwd): convert issueAmount(%s) failed. err=%s", args[fixedArgCount+3], err))
		}
		mylog.Debug("transAmount= %+v", transAmount)

		var sameEntSaveTrans = args[fixedArgCount+4] //如果转出和转入账户相同，是否记录交易 0表示不记录 1表示记录
		var sameEntSaveTransFlag bool = false
		if sameEntSaveTrans == "1" {
			sameEntSaveTransFlag = true
		}

		var pwdBase64 = args[fixedArgCount+5]
		pwd, err := t.decodeAccountPasswd(pwdBase64)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(resetAccPwd): decodeAccountPasswd (%s) failed. err=%s", pwdBase64, err))
		}
		mylog.Debug("Invoke(transeferUsePwd): pwd=%s", pwd)

		//验证密码
		setPwd, err := t.isSetAccountPasswd(stub, accName, accountEnt)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(transeferUsePwd): IsSetAccountPasswd failed. err=%s, acc=%s", err, accName))
		}
		if setPwd {
			ok, err := t.authAccountPasswd(stub, accName, pwd, accountEnt)
			if err != nil || !ok {
				return shim.Error(mylog.SError("Invoke(transeferUsePwd): AuthAccountPasswd failed. err=%s, ok=%v", err, ok))
			}
		} else {

			err = t.setAccountPasswd(stub, accName, pwd, accountEnt)
			if err != nil {
				return shim.Error(mylog.SError("Invoke(transeferUsePwd): setAccountPasswd failed. err=%s", err))
			}
		}

		_, err = t.transferCoin(stub, accName, toAcc, transType, description, transAmount, invokeTime, sameEntSaveTransFlag)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(transeferUsePwd) transferCoin2 failed. err=%s", err))
		}
		return shim.Success(nil)

	} else if function == "transefer3" { //带锁定期功能
		var argCount = fixedArgCount + 6
		if len(args) < argCount {
			return shim.Error(mylog.SError("Invoke(transeferLockAmt) miss arg, got %d, at least need %d.", len(args), argCount))
		}

		var toAcc = args[fixedArgCount]
		var transType = args[fixedArgCount+1]
		var description = args[fixedArgCount+2]

		var transAmount int64
		transAmount, err = strconv.ParseInt(args[fixedArgCount+3], 0, 64)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(transeferLockAmt): convert issueAmount(%s) failed. err=%s", args[fixedArgCount+3], err))
		}

		var sameEntSaveTrans = args[fixedArgCount+4] //如果转出和转入账户相同，是否记录交易 0表示不记录 1表示记录
		var sameEntSaveTransFlag bool = false
		if sameEntSaveTrans == "1" {
			sameEntSaveTransFlag = true
		}

		var lockCfgs = args[fixedArgCount+5]

		var lockedThistime int64 = 0
		lockedThistime, _, err = t.setAccountLockAmountCfg(stub, toAcc, lockCfgs, false)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(transeferLockAmt): setAccountLockAmountCfg failed. err=%s", err))
		}

		if lockedThistime > transAmount {
			return shim.Error(mylog.SError("Invoke(transeferLockAmt): lockAmt(%d) > transAmount(%d).", lockedThistime, transAmount))
		}

		_, err = t.transferCoin(stub, accName, toAcc, transType, description, transAmount, invokeTime, sameEntSaveTransFlag)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(transeferLockAmt) transferCoin3 failed. err=%s", err))
		}
		return shim.Success(nil)

	} else if function == "updateCert" {
		if !t.isAdmin(stub, accName) {
			return shim.Error(mylog.SError("Invoke(updateCert) can't exec updateCert by %s.", accName))
		}

		//为了以防万一，加上更新用户cert的功能
		var argCount = fixedArgCount + 3
		if len(args) < argCount {
			return shim.Error(mylog.SError("Invoke(updateCert) miss arg, got %d, at least need %d.", len(args), argCount))
		}

		//先更新该用户自己账户中的cert
		var upUser = args[fixedArgCount]
		var upAcc = args[fixedArgCount+1]
		cert, err := base64.StdEncoding.DecodeString(args[fixedArgCount+2])
		if err != nil {
			return shim.Error(mylog.SError("Invoke(updateCert) DecodeString failed. err=%s", err))
		}

		upEnt, err := t.getAccountEntity(stub, upAcc)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(updateCert) getAccountEntity failed. err=%s", err))
		}

		upEnt.OwnerCert = cert

		err = t.setAccountEntity(stub, upEnt)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(updateCert) setAccountEntity  failed. err=%s", err))
		}

		mylog.Debug("Invoke(updateCert) AccountEntity=%+v", *upEnt)

		//然后更新授权账户中的cert
		var pue *UserEntity
		pue, err = t.getUserEntity(stub, upUser)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(updateCert) getUserEntity failed. err=%s, entname=%s", err, upUser))
		}
		if pue == nil { //为空，没有授权账户
			return shim.Success(nil)
		}

		mylog.Debug("Invoke(updateCert) UserEntity=%+v", *pue)

		for _, acc := range pue.AuthAccList {
			upEnt, err := t.getAccountEntity(stub, acc)
			if err != nil {
				return shim.Error(mylog.SError("Invoke(updateCert) getAccountEntity2 failed. acc=%s err=%s", acc, err))
			}

			if upEnt.AuthUserCertMap != nil {
				if _, ok := upEnt.AuthUserCertMap[upUser]; ok {
					upEnt.AuthUserCertMap[upUser] = cert
				}
			}

			err = t.setAccountEntity(stub, upEnt)
			if err != nil {
				return shim.Error(mylog.SError("Invoke(updateCert) setAccountEntity2  failed. acc=%s err=%s", acc, err))
			}

			mylog.Debug("Invoke(updateCert) AccountEntity2=%+v", *upEnt)
		}

		return shim.Success(nil)

	} else if function == "updateEnv" {
		//更新环境变量
		if !t.isAdmin(stub, accName) {
			return shim.Error(mylog.SError("Invoke(updateEnv) can't exec updateEnv by %s.", accName))
		}

		var argCount = fixedArgCount + 2
		if len(args) < argCount {
			return shim.Error(mylog.SError("Invoke(updateEnv) miss arg, got %d, at least need %d.", len(args), argCount))
		}
		key := args[fixedArgCount]
		value := args[fixedArgCount+1]

		if key == "logLevel" {
			lvl, _ := strconv.Atoi(value)
			// debug=5, info=4, notice=3, warning=2, error=1, critical=0
			mylog.SetDefaultLvl(shim.LoggingLevel(lvl))
			mylog.Info("set logLevel to %d.", lvl)
		}

		return shim.Success(nil)

	} else if function == "AuthCert" { //授权证书
		if !t.isAdmin(stub, accName) {
			return shim.Error(mylog.SError("Invoke(AuthCert) can't exec AuthCert by %s.", accName))
		}

		var argCount = fixedArgCount + 3
		if len(args) < argCount {
			return shim.Error(mylog.SError("Invoke(AuthCert) miss arg, got %d, at least need %d.", len(args), argCount))
		}

		var authAcc = args[fixedArgCount]
		var addedUser = args[fixedArgCount+1]
		var addedCert []byte
		addedCert, err = base64.StdEncoding.DecodeString(args[fixedArgCount+2])
		if err != nil {
			return shim.Error(mylog.SError("Invoke(AuthCert) DecodeString failed. err=%s, arg=%s", err, args[fixedArgCount+2]))
		}

		authEnt, err := t.getAccountEntity(stub, authAcc)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(AuthCert) getAccountEntity failed. err=%s, entname=%s", err, authAcc))
		}

		if authEnt.AuthUserCertMap == nil {
			authEnt.AuthUserCertMap = make(map[string][]byte)
		}
		authEnt.AuthUserCertMap[addedUser] = addedCert

		err = t.setAccountEntity(stub, authEnt)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(AuthCert) setAccountEntity failed. err=%s, entname=%s", err, authAcc))
		}

		mylog.Debug("Invoke(AuthCert) AccountEntity =%+v.", *authEnt)

		var pue *UserEntity
		var ue UserEntity
		pue, err = t.getUserEntity(stub, addedUser)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(AuthCert) getUserEntity failed. err=%s, entname=%s", err, addedUser))
		}
		mylog.Debug("Invoke(AuthCert):  UserEntity before %+v", *pue)

		if pue == nil {
			ue.EntID = addedUser
			ue.AuthAccList = append(ue.AuthAccList, authAcc)
			pue = &ue
		} else {
			if !t.strSliceContains(pue.AuthAccList, authAcc) {
				pue.AuthAccList = append(pue.AuthAccList, authAcc)
			}
		}
		err = t.setUserEntity(stub, pue)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(AuthCert) setUserEntity failed. err=%s, entname=%s", err, authAcc))
		}
		mylog.Debug("Invoke(AuthCert):  UserEntity after %+v", *pue)

		return shim.Success(nil)

	} else if function == "setAllocCfg" {
		if !t.isAdmin(stub, accName) {
			return shim.Error(mylog.SError("Invoke(setAllocCfg) can't exec by %s.", accName))
		}

		var argCount = fixedArgCount + 5
		if len(args) < argCount {
			return shim.Error(mylog.SError("Invoke(setAllocCfg) miss arg, got %d, at least need %d.", len(args), argCount))
		}

		rackid := args[fixedArgCount]

		seller, err := strconv.ParseInt(args[fixedArgCount+1], 0, 64)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(setAllocCfg) convert seller(%s) failed. err=%s", args[fixedArgCount+1], err))
		}
		fielder, err := strconv.ParseInt(args[fixedArgCount+2], 0, 64)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(setAllocCfg) convert fielder(%s) failed. err=%s", args[fixedArgCount+2], err))
		}
		delivery, err := strconv.ParseInt(args[fixedArgCount+3], 0, 64)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(setAllocCfg) convert delivery(%s) failed. err=%s", args[fixedArgCount+3], err))
		}
		platform, err := strconv.ParseInt(args[fixedArgCount+4], 0, 64)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(setAllocCfg) convert platform(%s) failed. err=%s", args[fixedArgCount+4], err))
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
			return shim.Error(mylog.SError("Invoke(setAllocCfg) Marshal error, err=%s.", err))
		}

		var stateKey string
		if rackid == "*" {
			stateKey = t.getGlobalRackAllocRateKey()
		} else {
			stateKey = t.getRackAllocRateKey(rackid)
		}

		err = t.putState_Ex(stub, stateKey, eapJson)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(setAllocCfg) PutState_Ex error, err=%s.", err))
		}

		return shim.Success(nil)

	} else if function == "allocEarning" {
		if !t.isAdmin(stub, accName) {
			return shim.Error(mylog.SError("Invoke(allocEarning) can't exec by %s.", accName))
		}

		var argCount = fixedArgCount + 7
		if len(args) < argCount {
			return shim.Error(mylog.SError("Invoke(allocEarning) miss arg, got %d, at least need %d.", len(args), argCount))
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
			return shim.Error(mylog.SError("Invoke(allocEarning) convert totalAmt(%s) failed. err=%s", args[fixedArgCount+6], err))
		}

		var eap EarningAllocRate

		eapB, err := stub.GetState(t.getRackAllocRateKey(rackid))
		if err != nil {
			return shim.Error(mylog.SError("Invoke(allocEarning) GetState(rackid=%s) failed. err=%s", rackid, err))
		}
		if eapB == nil {
			mylog.Warn("Invoke(allocEarning) GetState(rackid=%s) nil, try to get global.", rackid)
			//没有为该货架单独配置，返回global配置
			eapB, err = stub.GetState(t.getGlobalRackAllocRateKey())
			if err != nil {
				return shim.Error(mylog.SError("Invoke(allocEarning) GetState(global, rackid=%s) failed. err=%s", rackid, err))
			}
			if eapB == nil {
				return shim.Error(mylog.SError("Invoke(allocEarning) GetState(global, rackid=%s) nil.", rackid))
			}
		}

		err = json.Unmarshal(eapB, &eap)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(allocEarning) Unmarshal failed. err=%s", err))
		}

		var accs AllocAccs
		accs.SellerAcc = sellerAcc
		accs.FielderAcc = fielderAcc
		accs.DeliveryAcc = deliveryAcc
		accs.PlatformAcc = platformAcc

		_, err = t.setAllocEarnTx(stub, rackid, allocKey, totalAmt, &accs, &eap, invokeTime)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(allocEarning) setAllocEarnTx failed. err=%s", err))
		}
		return shim.Success(nil)

	} else if function == "setSESCfg" { //设置每个货架的销售额奖励区间比例
		if !t.isAdmin(stub, accName) {
			return shim.Error(mylog.SError("Invoke(setSESCfg) can't exec by %s.", accName))
		}

		var argCount = fixedArgCount + 2
		if len(args) < argCount {
			return shim.Error(mylog.SError("Invoke(setSESCfg) miss arg, got %d, at least need %d.", len(args), argCount))
		}

		var rackid = args[fixedArgCount]
		var cfgStr = args[fixedArgCount+1]

		_, err = t.setRackEncourageScoreCfg(stub, rackid, cfgStr, invokeTime)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(setSESCfg) setRackEncourageScoreCfg failed. err=%s", err))
		}
		return shim.Success(nil)

	} else if function == "encourageScoreForSales" { //根据销售额奖励积分
		var argCount = fixedArgCount + 4
		if len(args) < argCount {
			return shim.Error(mylog.SError("Invoke(encourageScoreForSales) miss arg, got %d, at least need %d.", len(args), argCount))
		}

		var paraStr = args[fixedArgCount]
		var transType = args[fixedArgCount+1]
		var transDesc = args[fixedArgCount+2]
		var sameEntSaveTrans = args[fixedArgCount+3] //如果转出和转入账户相同，是否记录交易 0表示不记录 1表示记录
		var sameEntSaveTransFlag bool = false
		if sameEntSaveTrans == "1" {
			sameEntSaveTransFlag = true
		}

		//使用登录的账户进行转账
		_, err = t.allocEncourageScoreForSales(stub, paraStr, accName, transType, transDesc, invokeTime, sameEntSaveTransFlag)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(encourageScoreForSales) allocEncourageScoreForSales failed. err=%s", err))
		}
		return shim.Success(nil)

	} else if function == "encourageScoreForNewRack" { //新开货架奖励积分
		var argCount = fixedArgCount + 4
		if len(args) < argCount {
			return shim.Error(mylog.SError("Invoke(encourageScoreForNewRack) miss arg, got %d, at least need %d.", len(args), argCount))
		}

		var paraStr = args[fixedArgCount]
		var transType = args[fixedArgCount+1]
		var transDesc = args[fixedArgCount+2]
		var sameEntSaveTrans = args[fixedArgCount+3] //如果转出和转入账户相同，是否记录交易 0表示不记录 1表示记录
		var sameEntSaveTransFlag bool = false
		if sameEntSaveTrans == "1" {
			sameEntSaveTransFlag = true
		}

		//使用登录的账户进行转账
		_, err = t.allocEncourageScoreForNewRack(stub, paraStr, accName, transType, transDesc, invokeTime, sameEntSaveTransFlag)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(encourageScoreForNewRack) allocEncourageScoreForNewRack failed. err=%s", err))
		}
		return shim.Success(nil)

	} else if function == "setFinanceCfg" {
		if !t.isAdmin(stub, accName) {
			return shim.Error(mylog.SError("Invoke(setFinanceCfg) can't exec by %s.", accName))
		}

		var argCount = fixedArgCount + 4
		if len(args) < argCount {
			return shim.Error(mylog.SError("Invoke(setFinanceCfg) miss arg, got %d, at least need %d.", len(args), argCount))
		}

		var rackid = args[fixedArgCount]
		var profitsPercent int
		var investProfitsPercent int
		var investCapacity int64

		profitsPercent, err = strconv.Atoi(args[fixedArgCount+1])
		if err != nil {
			return shim.Error(mylog.SError("Invoke(setFinanceCfg) convert profitsPercent(%s) failed. err=%s", args[fixedArgCount+1], err))
		}
		investProfitsPercent, err = strconv.Atoi(args[fixedArgCount+2])
		if err != nil {
			return shim.Error(mylog.SError("Invoke(setFinanceCfg) convert investProfitsPercent(%s) failed. err=%s", args[fixedArgCount+2], err))
		}
		investCapacity, err = strconv.ParseInt(args[fixedArgCount+3], 0, 64)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(setFinanceCfg) convert investCapacity(%s) failed. err=%s", args[fixedArgCount+3], err))
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
			return shim.Error(mylog.SError("Invoke(setFinanceCfg) Marshal(rfc) error, err=%s.", err))
		}

		var stateKey string
		if rackid == "*" {
			stateKey = t.getGlobalRackFinancCfgKey()
		} else {
			stateKey = t.getRackFinancCfgKey(rackid)
		}

		err = t.putState_Ex(stub, stateKey, rfcJson)
		if err != nil {
			return shim.Error(mylog.SError("IInvoke(setFinanceCfg) PutState_Ex(rfc) error, err=%s.", err))
		}

		return shim.Success(nil)

	} else if function == "buyFinance" {
		var argCount = fixedArgCount + 7
		if len(args) < argCount {
			return shim.Error(mylog.SError("Invoke(buyFinancial) miss arg, got %d, at least need %d.", len(args), argCount))
		}

		var rackid = args[fixedArgCount]
		var financid = args[fixedArgCount+1]
		var payee = args[fixedArgCount+2]
		var amount int64
		amount, err = strconv.ParseInt(args[fixedArgCount+3], 0, 64)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(buyFinancial) convert amount(%s) failed. err=%s", args[fixedArgCount+3], err))
		}

		var transType = args[fixedArgCount+4]
		var transDesc = args[fixedArgCount+5]
		var sameEntSaveTrans = args[fixedArgCount+6] //如果转出和转入账户相同，是否记录交易 0表示不记录 1表示记录
		var sameEntSaveTransFlag bool = false
		if sameEntSaveTrans == "1" {
			sameEntSaveTransFlag = true
		}

		//每次购买时，肯定是购买最新一期的理财，设置为当前的fid
		err = t.setCurrentFid(stub, financid)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(buyFinancial) setCurrentFid failed, err=%s.", err))
		}

		//使用登录的账户进行转账
		_, err = t.userBuyFinance(stub, accName, rackid, financid, payee, transType, transDesc, amount, invokeTime, sameEntSaveTransFlag, false)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(buyFinancial) userBuyFinance failed. err=%s", err))
		}
		return shim.Success(nil)

	} else if function == "financeIssueFinish" {
		if !t.isAdmin(stub, accName) {
			return shim.Error(mylog.SError("Invoke(financeIssueFinish) can't exec by %s.", accName))
		}

		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return shim.Error(mylog.SError("Invoke(financeIssueFinish) miss arg, got %d, at least need %d.", len(args), argCount))
		}

		var financid = args[fixedArgCount]

		//理财结束时，肯定是最新一期的理财，设置为当前的fid
		err = t.setCurrentFid(stub, financid)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(financeIssueFinish) setCurrentFid failed, err=%s.", err))
		}

		err = t.financeIssueFinishAfter(stub, financid, invokeTime)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(financeIssueFinishAfter) financeRenewal failed, err=%s.", err))
		}

		return shim.Success(nil)

	} else if function == "payFinance" {
		var argCount = fixedArgCount + 5
		if len(args) < argCount {
			return shim.Error(mylog.SError("Invoke(payFinance) miss arg, got %d, at least need %d.", len(args), argCount))
		}

		var rackid = args[fixedArgCount]
		var reacc = args[fixedArgCount+1]
		var transType = args[fixedArgCount+2]
		var transDesc = args[fixedArgCount+3]
		var sameEntSaveTrans = args[fixedArgCount+4] //如果转出和转入账户相同，是否记录交易 0表示不记录 1表示记录
		var sameEntSaveTransFlag bool = false
		if sameEntSaveTrans == "1" {
			sameEntSaveTransFlag = true
		}

		err = t.payUserFinance(stub, accName, reacc, rackid, invokeTime, transType, transDesc, sameEntSaveTransFlag)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(payFinance) payUserFinance failed, err=%s.", err))
		}

		return shim.Success(nil)

	} else if function == "financeBouns" {
		if !t.isAdmin(stub, accName) {
			return shim.Error(mylog.SError("Invoke(financeBouns) can't exec by %s.", accName))
		}

		var argCount = fixedArgCount + 2
		if len(args) < argCount {
			return shim.Error(mylog.SError("Invoke(financeBouns) miss arg, got %d, at least need %d.", len(args), argCount))
		}

		var fid = args[fixedArgCount]
		var rackSalesCfg = args[fixedArgCount+1]
		_, err = t.financeBonus(stub, fid, rackSalesCfg, invokeTime)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(financeBouns) financeBonus failed. err=%s", err))
		}
		return shim.Success(nil)

	} else if function == "updateState" {
		if !t.isAdmin(stub, accName) {
			return shim.Error(mylog.SError("Invoke(setWorldState) can't exec by %s.", accName))
		}

		var argCount = fixedArgCount + 4
		if len(args) < argCount {
			return shim.Error(mylog.SError("setWorldState miss arg, got %d, need %d.", len(args), argCount))
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

		_, err = t.loadWorldState(stub, fileName, needHash, sameKeyOverwrite, srcCcid)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(setWorldState) setWorldState failed. err=%s", err))
		}
		return shim.Success(nil)

	} else if function == "lockAccAmt" {
		if !t.isAdmin(stub, accName) {
			return shim.Error(mylog.SError("Invoke(lockAccAmt) can't exec by %s.", accName))
		}

		var argCount = fixedArgCount + 4
		if len(args) < argCount {
			return shim.Error(mylog.SError("Invoke(lockAccAmt) miss arg, got %d, need %d.", len(args), argCount))
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

		lockEnt, err := t.getAccountEntity(stub, lockedAccName)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(lockAccAmt): getAccountEntity failed. err=%s", err))
		}

		_, lockedTotal, err := t.setAccountLockAmountCfg(stub, lockedAccName, lockCfgs, overwriteOld)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(lockAccAmt): setAccountLockAmountCfg failed. err=%s", err))
		}

		if !canLockMoreThanRest && lockedTotal > lockEnt.RestAmount {
			return shim.Error(mylog.SError("Invoke(lockAccAmt): lock amount > account rest(%d,%d).", lockedTotal, lockEnt.RestAmount))
		}

		return shim.Success(nil)

	} else if function == "setAccCfg1" { //设置交易密码
		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return shim.Error(mylog.SError("Invoke(setAccPwd) miss arg, got %d, need %d.", len(args), argCount))
		}

		setPwd, err := t.isSetAccountPasswd(stub, accName, accountEnt)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(setAccPwd): IsSetAccountPasswd failed. err=%s, acc=%s", err, accName))
		}
		//如果已设置，则报错
		if setPwd {
			return shim.Error(mylog.SError("Invoke(setAccPwd): pwd is setted, do nothing, acc=%s", accName))
		}

		var pwdBase64 = args[fixedArgCount]
		pwd, err := t.decodeAccountPasswd(pwdBase64)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(setAccPwd): decodeAccountPasswd (%s) failed. err=%s", pwdBase64, err))
		}
		mylog.Debug("Invoke(setAccPwd): pwd=%s", pwd)

		err = t.setAccountPasswd(stub, accName, pwd, accountEnt)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(setAccPwd) setAccountPasswd failed. err=%s", err))
		}
		return shim.Success(nil)

	} else if function == "setAccCfg2" { //重置交易密码
		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return shim.Error(mylog.SError("Invoke(resetAccPwd) miss arg, got %d, need %d.", len(args), argCount))
		}

		var pwdBase64 = args[fixedArgCount]
		pwd, err := t.decodeAccountPasswd(pwdBase64)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(resetAccPwd): decodeAccountPasswd (%s) failed. err=%s", pwdBase64, err))
		}
		mylog.Debug("Invoke(resetAccPwd): pwd=%s", pwd)

		err = t.setAccountPasswd(stub, accName, pwd, accountEnt)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(resetAccPwd) setAccountPasswd failed. err=%s", err))
		}
		return shim.Success(nil)

	} else if function == "setAccCfg3" { //修改交易密码
		var argCount = fixedArgCount + 2
		if len(args) < argCount {
			return shim.Error(mylog.SError("Invoke(chgAccPwd) miss arg, got %d, need %d.", len(args), argCount))
		}

		var oldpwd, newpwd string
		var oldpwdBase64 = args[fixedArgCount]
		var newpwdBase64 = args[fixedArgCount+1]

		oldpwd, err = t.decodeAccountPasswd(oldpwdBase64)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(chgAccPwd): decodeAccountPasswd o(%s) failed. err=%s", oldpwdBase64, err))
		}
		newpwd, err = t.decodeAccountPasswd(newpwdBase64)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(chgAccPwd): decodeAccountPasswd n(%s) failed. err=%s", newpwdBase64, err))
		}

		mylog.Debug("Invoke(chgAccPwd): opwd=%s", oldpwd)
		mylog.Debug("Invoke(chgAccPwd): npwd=%s", newpwd)

		err = t.changeAccountPasswd(stub, accName, oldpwd, newpwd, accountEnt)
		if err != nil {
			return shim.Error(mylog.SError("Invoke(chgAccPwd) changeAccountPasswd failed. err=%s", err))
		}
		return shim.Success(nil)

	} else {
		//其它函数看是否是query函数
		return t.Query(stub)
	}

	//event
	stub.SetEvent("success", []byte("invoke success"))
	return shim.Error(mylog.SError("unknown Invoke."))
}

// Query callback representing the query of a chaincode
func (t *KD) Query(stub shim.ChaincodeStubInterface) pb.Response {
	mylog.Debug("Enter Query")
	function, args := stub.GetFunctionAndParameters()
	mylog.Debug("func =%s, args = %+v", function, args)

	var err error

	var fixedArgCount = 3
	if len(args) < fixedArgCount {
		return shim.Error(mylog.SError("Query miss arg, got %d, at least need %d.", len(args), fixedArgCount))
	}

	var userName = args[0]
	var accName = args[1]
	var queryTime int64 = 0

	queryTime, err = strconv.ParseInt(args[2], 0, 64)
	if err != nil {
		return shim.Error(mylog.SError("Query convert queryTime(%s) failed. err=%s", args[2], err))
	}

	var userAttrs *UserAttrs
	var accountEnt *AccountEntity = nil

	userAttrs, err = t.getUserAttrs(stub)
	if err != nil {
		return shim.Error(mylog.SError("Query getUserAttrs failed. err=%s", err))
	}

	accountEnt, err = t.getAccountEntity(stub, accName)
	if err != nil {
		if err == ErrNilEntity {
			if function == "isAccExists" { //如果是查询账户是否存在，如果是空，返回不存在
				return shim.Success([]byte("0"))
			} else if function == "getBalance" { //如果是查询余额，如果账户不存，返回0
				return shim.Success([]byte("0"))
			} else if function == "transPreCheck" { //如果是转账预检查，返回付款账户不存在
				return shim.Success([]byte(strconv.FormatInt(ERRCODE_TRANS_PAY_ACCOUNT_NOT_EXIST, 10)))

			}
		}
		return shim.Error(mylog.SError("Query getAccountEntity failed. err=%s", err))
	}

	//校验用户身份
	if ok, _ := t.verifyIdentity(stub, userName, accountEnt, userAttrs); !ok {
		return shim.Error(mylog.SError("Query user and account check failed."))
	}

	if function == "getBalance" { //查询余额

		var queryEntity *AccountEntity = accountEnt

		mylog.Debug("queryEntity=%+v", queryEntity)

		retValue := []byte(strconv.FormatInt(queryEntity.RestAmount, 10))

		return shim.Success(retValue)

	} else if function == "getBalanceAndLocked" { //查询余额及锁定金额
		var qbal QueryBalanceAndLocked

		var queryEntity *AccountEntity = accountEnt
		mylog.Debug("queryEntity=%+v", queryEntity)

		qbal.Balance = queryEntity.RestAmount

		qbal.LockedAmount, qbal.LockCfg, err = t.getAccountLockedAmount(stub, accName, queryTime)
		if err != nil {
			return shim.Error(mylog.SError("getBalanceAndLocked: getAccountLockedAmount(id=%s) failed. err=%s", accName, err))
		}

		if qbal.LockCfg == nil {
			qbal.LockCfg = []CoinLockCfg{} //初始化为空，即使没查到数据也会返回'[]'
		}

		qbalB, err := json.Marshal(qbal)
		if err != nil {
			return shim.Error(mylog.SError("getBalanceAndLocked: Marshal(id=%s) failed. err=%s", accName, err))
		}

		return shim.Success(qbalB)

	} else if function == "getTransInfo" { //查询交易记录
		var argCount = fixedArgCount + 8
		if len(args) < argCount {
			return shim.Error(mylog.SError("queryTx miss arg, got %d, need %d.", len(args), argCount))
		}

		var begSeq int64
		var txCount int64
		var transLvl uint64
		var begTime int64
		var endTime int64
		var txAcc string
		var queryMaxSeq int64
		var queryOrder string

		begSeq, err = strconv.ParseInt(args[fixedArgCount], 0, 64)
		if err != nil {
			return shim.Error(mylog.SError("queryTx ParseInt for begSeq(%s) failed. err=%s", args[fixedArgCount], err))
		}
		txCount, err = strconv.ParseInt(args[fixedArgCount+1], 0, 64)
		if err != nil {
			return shim.Error(mylog.SError("queryTx ParseInt for endSeq(%s) failed. err=%s", args[fixedArgCount+1], err))
		}

		transLvl, err = strconv.ParseUint(args[fixedArgCount+2], 0, 64)
		if err != nil {
			return shim.Error(mylog.SError("queryTx ParseInt for transLvl(%s) failed. err=%s", args[fixedArgCount+2], err))
		}

		begTime, err = strconv.ParseInt(args[fixedArgCount+3], 0, 64)
		if err != nil {
			return shim.Error(mylog.SError("queryTx ParseInt for begTime(%s) failed. err=%s", args[fixedArgCount+3], err))
		}
		endTime, err = strconv.ParseInt(args[fixedArgCount+4], 0, 64)
		if err != nil {
			return shim.Error(mylog.SError("queryTx ParseInt for endTime(%s) failed. err=%s", args[fixedArgCount+4], err))
		}

		//查询指定账户的交易记录
		txAcc = args[fixedArgCount+5]

		queryMaxSeq, err = strconv.ParseInt(args[fixedArgCount+6], 0, 64)
		if err != nil {
			return shim.Error(mylog.SError("queryTx ParseInt for queryMaxSeq(%s) failed. err=%s", args[fixedArgCount+6], err))
		}

		queryOrder = args[fixedArgCount+7]

		var isAsc = false
		if queryOrder == "asc" {
			isAsc = true
		}

		if t.isAdmin(stub, accName) {
			//管理员账户时，如果不传入txAcc，则查询所有交易记录；否则查询指定账户交易记录
			if len(txAcc) == 0 {
				retValue, err := t.queryTransInfos(stub, transLvl, begSeq, txCount, begTime, endTime, queryMaxSeq, isAsc)
				if err != nil {
					return shim.Error(mylog.SError("queryTx queryTransInfos failed. err=%s", err))
				}
				return shim.Success(retValue)
			} else {
				retValue, err := t.queryAccTransInfos(stub, txAcc, begSeq, txCount, begTime, endTime, queryMaxSeq, isAsc)
				if err != nil {
					return shim.Error(mylog.SError("queryTx queryAccTransInfos failed. err=%s", err))
				}
				return shim.Success(retValue)
			}
		} else {
			//非管理员账户，只能查询自己的交易记录，忽略txAcc参数
			retValue, err := t.queryAccTransInfos(stub, accName, begSeq, txCount, begTime, endTime, queryMaxSeq, isAsc)
			if err != nil {
				return shim.Error(mylog.SError("queryTx queryAccTransInfos2 failed. err=%s", err))
			}
			return shim.Success(retValue)
		}

		return shim.Success(nil)

	} else if function == "getAllAccAmt" { //所有账户中钱是否正确
		//是否是管理员帐户，管理员用户才可以查
		if !t.isAdmin(stub, accName) {
			return shim.Error(mylog.SError("%s can't query balance.", accName))
		}

		retValue, err := t.getAllAccAmt(stub)
		if err != nil {
			return shim.Error(mylog.SError("getAllAccAmt failed. err=%s", err))
		}
		return shim.Success(retValue)

	} else if function == "queryState" { //某个state的值
		//是否是管理员帐户，管理员用户才可以查
		if !t.isAdmin(stub, accName) {
			return shim.Error(mylog.SError("%s can't query state.", accName))
		}

		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return shim.Error(mylog.SError("queryState miss arg, got %d, need %d.", len(args), argCount))
		}

		key := args[fixedArgCount]

		retValue, err := stub.GetState(key)
		if err != nil {
			return shim.Error(mylog.SError("queryState GetState failed. err=%s", err))
		}

		return shim.Success(retValue)

	} else if function == "isAccExists" { //账户是否存在
		accExist, err := t.isEntityExists(stub, accName)
		if err != nil {
			return shim.Error(mylog.SError("accExists: isEntityExists (id=%s) failed. err=%s", accName, err))
		}

		var retValue []byte
		if accExist {
			retValue = []byte("1")
		} else {
			retValue = []byte("0")
		}

		return shim.Success(retValue)

	} else if function == "queryRackAlloc" {

		var argCount = fixedArgCount + 7
		if len(args) < argCount {
			return shim.Error(mylog.SError("queryRackAlloc miss arg, got %d, need %d.", len(args), argCount))
		}

		var rackid string
		var allocKey string
		var begSeq int64
		var txCount int64
		var begTime int64
		var endTime int64
		var txAcc string

		rackid = args[fixedArgCount]
		allocKey = args[fixedArgCount+1]

		begSeq, err = strconv.ParseInt(args[fixedArgCount+2], 0, 64)
		if err != nil {
			return shim.Error(mylog.SError("queryRackAlloc ParseInt for begSeq(%s) failed. err=%s", args[fixedArgCount+2], err))
		}
		txCount, err = strconv.ParseInt(args[fixedArgCount+3], 0, 64)
		if err != nil {
			return shim.Error(mylog.SError("queryRackAlloc ParseInt for txCount(%s) failed. err=%s", args[fixedArgCount+3], err))
		}

		begTime, err = strconv.ParseInt(args[fixedArgCount+4], 0, 64)
		if err != nil {
			return shim.Error(mylog.SError("queryRackAlloc ParseInt for begTime(%s) failed. err=%s", args[fixedArgCount+4], err))
		}
		endTime, err = strconv.ParseInt(args[fixedArgCount+5], 0, 64)
		if err != nil {
			return shim.Error(mylog.SError("queryRackAlloc ParseInt for endTime(%s) failed. err=%s", args[fixedArgCount+5], err))
		}
		txAcc = args[fixedArgCount+6]

		if len(allocKey) > 0 {
			//是否是管理员帐户，管理员用户才可以查
			if !t.isAdmin(stub, accName) {
				return shim.Error(mylog.SError("queryRackAlloc: %s can't query allocKey.", accName))
			}

			//查询某一次的分配情况（由allocKey检索）
			retValue, err := t.getAllocTxRecdByKey(stub, rackid, allocKey)
			if err != nil {
				return shim.Error(mylog.SError("queryRackAlloc: getAllocTxRecdByKey failed. err=%s", err))
			}
			return shim.Success(retValue)
		} else {
			if t.isAdmin(stub, accName) {
				if len(txAcc) > 0 {
					//查询某一个账户的分配情况
					retValue, err := t.getOneAccAllocTxRecds(stub, txAcc, begSeq, txCount, begTime, endTime)
					if err != nil {
						return shim.Error(mylog.SError("queryRackAlloc: getOneAccAllocTxRecds failed. err=%s", err))
					}
					return shim.Success(retValue)
				} else {
					//查询某一个货架的分配情况
					retValue, err := t.getAllocTxRecds(stub, rackid, begSeq, txCount, begTime, endTime)
					if err != nil {
						return shim.Error(mylog.SError("queryRackAlloc: getAllocTxRecds failed. err=%s", err))
					}
					return shim.Success(retValue)
				}
			} else {
				//非管理员账户，只能查询自己的交易记录，忽略txAcc参数
				retValue, err := t.getOneAccAllocTxRecds(stub, accName, begSeq, txCount, begTime, endTime)
				if err != nil {
					return shim.Error(mylog.SError("queryRackAlloc: getOneAccAllocTxRecds2 failed. err=%s", err))
				}
				return shim.Success(retValue)
			}
		}

		return shim.Success(nil)

	} else if function == "getRackAllocCfg" {
		if !t.isAdmin(stub, accName) {
			return shim.Error(mylog.SError("getRackAllocCfg: %s can't query.", accName))
		}

		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return shim.Error(mylog.SError("getRackAllocCfg miss arg, got %d, need %d.", len(args), argCount))
		}

		var rackid = args[fixedArgCount]

		eapB, err := t.getRackAllocCfg(stub, rackid, nil)
		if err != nil {
			return shim.Error(mylog.SError("getRackAllocCfg getRackAllocCfg(rackid=%s) failed. err=%s", rackid, err))
		}

		return shim.Success(eapB)

	} else if function == "getSESCfg" {
		if !t.isAdmin(stub, accName) {
			return shim.Error(mylog.SError("getSESCfg: %s can't query.", accName))
		}

		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return shim.Error(mylog.SError("getSESCfg miss arg, got %d, need %d.", len(args), argCount))
		}

		var rackid = args[fixedArgCount]

		sercB, err := t.getRackEncourageScoreCfg(stub, rackid, nil)
		if err != nil {
			return shim.Error(mylog.SError("getSESCfg getRackEncourageScoreCfg(rackid=%s) failed. err=%s", rackid, err))
		}

		return shim.Success(sercB)

	} else if function == "getRackFinanceCfg" {
		if !t.isAdmin(stub, accName) {
			return shim.Error(mylog.SError("getRackFinanceCfg: %s can't query.", accName))
		}

		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return shim.Error(mylog.SError("getRackFinanceCfg miss arg, got %d, need %d.", len(args), argCount))
		}

		var rackid = args[fixedArgCount]

		rfcB, err := t.getRackFinancCfg(stub, rackid, nil)
		if err != nil {
			return shim.Error(mylog.SError("getRackFinanceCfg getRackFinancCfg(rackid=%s) failed. err=%s", rackid, err))
		}

		return shim.Success(rfcB)

	} else if function == "getRackFinanceProfit" {
		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return shim.Error(mylog.SError("getRackFinanceProfit miss arg, got %d, need %d.", len(args), argCount))
		}

		var rackid = args[fixedArgCount]

		var profit int64
		profit, err = t.getUserFinanceProfit(stub, accName, rackid)
		if err != nil {
			return shim.Error(mylog.SError("getRackFinanceProfit getUserFinanceProfit(rackid=%s) failed. err=%s", rackid, err))
		}

		return shim.Success([]byte(strconv.FormatInt(profit, 10)))

	} else if function == "getRackRestFinanceCapacity" {
		if !t.isAdmin(stub, accName) {
			return shim.Error(mylog.SError("getRackFinanceCapacity: %s can't query.", accName))
		}

		var argCount = fixedArgCount + 2
		if len(args) < argCount {
			return shim.Error(mylog.SError("getRackFinanceCapacity miss arg, got %d, need %d.", len(args), argCount))
		}

		var rackid = args[fixedArgCount]
		var fid = args[fixedArgCount+1]

		//新理财发行后，用户购买理财时，前台会查询一下货架剩余的投资额度，传入的fid为最新期的理财id

		restCap, err := t.getRestFinanceCapacityForRack(stub, rackid, fid)
		if err != nil {
			return shim.Error(mylog.SError("getRackFinanceCapacity getFinanceCapacityForRack(rackid=%s) failed. err=%s", rackid, err))
		}

		return shim.Success([]byte(strconv.FormatInt(restCap, 10)))

	} else if function == "getDataState" {
		if !t.isAdmin(stub, accName) {
			return shim.Error(mylog.SError("getWorldState: %s can't query.", accName))
		}

		var argCount = fixedArgCount + 3
		if len(args) < argCount {
			return shim.Error(mylog.SError("getWorldState miss arg, got %d, need %d.", len(args), argCount))
		}

		var needHash = false
		if args[fixedArgCount] == "1" {
			needHash = true
		}

		var flushLimit int
		flushLimit, err = strconv.Atoi(args[fixedArgCount+1])
		if err != nil {
			return shim.Error(mylog.SError("getWorldState: convert flushLimit(%s) failed. err=%s", args[fixedArgCount+1], err))
		}
		if flushLimit < 0 {
			flushLimit = 4096
		}

		var currCcid = args[fixedArgCount+2]

		retValue, err := t.dumpWorldState(stub, queryTime, flushLimit, needHash, currCcid)
		if err != nil {
			return shim.Error(mylog.SError("getWorldState: getWorldState failed. err=%s", err))
		}
		return shim.Success(retValue)

	} else if function == "getInfoForWeb" {
		//是否是管理员帐户，管理员用户才可以查
		if !t.isAdmin(stub, accName) {
			return shim.Error(mylog.SError("%s can't query InfoForWeb.", accName))
		}

		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return shim.Error(mylog.SError("getInfoForWeb miss arg, got %d, need %d.", len(args), argCount))
		}

		retValue, err := t.getInfo4Web(stub, args[fixedArgCount])
		if err != nil {
			return shim.Error(mylog.SError("getInfoForWeb: getInfo4Web failed. err=%s", err))
		}
		return shim.Success(retValue)

	} else if function == "transPreCheck" {
		var argCount = fixedArgCount + 3
		if len(args) < argCount {
			return shim.Error(mylog.SError("transPreCheck miss arg, got %d, need %d.", len(args), argCount))
		}

		toAcc := args[fixedArgCount]
		pwd := args[fixedArgCount+1]
		transAmount, err := strconv.ParseInt(args[fixedArgCount+2], 0, 64)
		if err != nil {
			return shim.Error(mylog.SError("transPreCheck: convert transAmount(%s) failed. err=%s", args[fixedArgCount+2], err))
		}

		mylog.Debug("transPreCheck: accountEnt=%+v", accountEnt)
		//先看密码是否正确
		if len(pwd) > 0 {
			setPwd, err := t.isSetAccountPasswd(stub, accName, accountEnt)
			if err != nil {
				return shim.Error(mylog.SError("transPreCheck: isSetAccountPasswd(%s) failed. err=%s", accName, err))
			}

			if setPwd {
				ok, err := t.authAccountPasswd(stub, accName, pwd, accountEnt)
				if err != nil {
					return shim.Error(mylog.SError("transPreCheck: AuthAccountPasswd(%s) failed. err=%s", accName, err))
				}
				if !ok {
					return shim.Success([]byte(strconv.FormatInt(ERRCODE_TRANS_PASSWD_INVALID, 10)))
				}
			}
		}

		//余额是否足够
		if transAmount < 0 { //如果是内部接口调用，可能会转账金额为0， 这里不检查0
			return shim.Success([]byte(strconv.FormatInt(ERRCODE_TRANS_AMOUNT_INVALID, 10)))
		}
		//看是否有锁定金额
		lockAmt, _, err := t.getAccountLockedAmount(stub, accName, queryTime)
		if err != nil {
			return shim.Error(mylog.SError("transPreCheck: getAccountLockedAmount(id=%s) failed. err=%s", accName, err))
		}

		if transAmount > accountEnt.RestAmount {
			return shim.Success([]byte(strconv.FormatInt(ERRCODE_TRANS_BALANCE_NOT_ENOUGH, 10)))
		}
		//错误码丰富一点，这里再判断是否是因为锁定导致余额不足
		if lockAmt > 0 && transAmount > accountEnt.RestAmount-lockAmt {
			return shim.Success([]byte(strconv.FormatInt(ERRCODE_TRANS_BALANCE_NOT_ENOUGH_BYLOCK, 10)))
		}

		//收款账户是否存在  这个检查放到最后执行
		exists, err := t.isEntityExists(stub, toAcc)
		if err != nil {
			return shim.Error(mylog.SError("transPreCheck: isEntityExists(%s) failed. err=%s", toAcc, err))
		}
		if !exists {
			return shim.Success([]byte(strconv.FormatInt(ERRCODE_TRANS_PAYEE_ACCOUNT_NOT_EXIST, 10)))
		}

		//通过返回0，表示检查通过
		return shim.Success([]byte(strconv.FormatInt(0, 10)))

	} else if function == "isAccSetPwd" { //账户是否设置密码
		setPwd, err := t.isSetAccountPasswd(stub, accName, accountEnt)
		if err != nil {
			return shim.Error(mylog.SError("Query(isAccSetPwd): IsSetAccountPasswd failed. err=%s, acc=%s", err, accName))
		}

		var retValues []byte
		if setPwd {
			retValues = []byte("1")
		} else {
			retValues = []byte("0")
		}

		return shim.Success(retValues)
	}

	return shim.Error(mylog.SError("unknown function."))
}

func (t *KD) queryTransInfos(stub shim.ChaincodeStubInterface, transLvl uint64, begIdx, count, begTime, endTime, queryMaxSeq int64, isAsc bool) ([]byte, error) {
	var maxSeq int64
	var err error

	var retTransInfo []byte
	var queryResult QueryTransResult
	queryResult.NextSerial = -1
	queryResult.MaxSerial = -1
	queryResult.TransRecords = []QueryTransRecd{} //初始化为空，即使下面没查到数据也会返回'[]'

	retTransInfo, err = json.Marshal(queryResult)
	if err != nil {
		return nil, mylog.Errorf("queryTransInfos Marshal failed.err=%s", err)
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
		mylog.Warn("queryTransInfos nothing to do(%d).", count)
		return retTransInfo, nil
	}

	//先判断是否存在交易序列号了，如果不存在，说明还没有交易发生。 这里做这个判断是因为在 getTransSeq 里如果没有设置过序列号的key会自动设置一次，但是在query中无法执行PutStat，会报错
	var seqKey = t.getGlobalTransSeqKey(stub)
	test, err := stub.GetState(seqKey)
	if err != nil {
		return nil, mylog.Errorf("queryTransInfos GetState failed. err=%s", err)
	}
	if test == nil {
		mylog.Info("no trans saved.")
		return retTransInfo, nil
	}

	//先获取当前最大的序列号
	if queryMaxSeq != -1 {
		maxSeq = queryMaxSeq
	} else {
		maxSeq, err = t.getTransSeq(stub, seqKey)
		if err != nil {
			return nil, mylog.Errorf("queryTransInfos getTransSeq failed. err=%s", err)
		}
	}

	if begIdx > maxSeq {
		mylog.Warn("queryTransInfos nothing to do(%d,%d).", begIdx, maxSeq)
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
		       key, _ := t.getTransInfoKey(stub, i)

		       tmpState, err := stub.GetState(key)
		       if err != nil {
		           mylog.Error("getTransInfo GetState(idx=%d) failed.err=%s", i, err)
		           //return nil, err
		           continue
		       }
		       if tmpState == nil {
		           //return nil, mylog.Errorf("getTransInfo GetState nil(idx=%d).", i)
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

			trans, err = t.getOnceTransInfo(stub, t.getTransInfoKey(stub, loop))
			if err != nil {
				mylog.Error("getTransInfo getQueryTransInfo(idx=%d) failed.err=%s", loop, err)
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
				mylog.Error("getTransInfo getQueryTransInfo(idx=%d) failed.err=%s", loop, err)
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
		return nil, mylog.Errorf("getTransInfo Marshal failed.err=%s", err)
	}

	return retTransInfo, nil
}

func (t *KD) queryAccTransInfos(stub shim.ChaincodeStubInterface, accName string, begIdx, count, begTime, endTime, queryMaxSeq int64, isAsc bool) ([]byte, error) {
	var maxSeq int64
	var err error

	var retTransInfo []byte
	var queryResult QueryTransResult
	queryResult.NextSerial = -1
	queryResult.MaxSerial = -1
	queryResult.TransRecords = []QueryTransRecd{} //初始化为空，即使下面没查到数据也会返回'[]'

	retTransInfo, err = json.Marshal(queryResult)
	if err != nil {
		return nil, mylog.Errorf("queryAccTransInfos Marshal failed.err=%s", err)
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
		mylog.Warn("queryAccTransInfos nothing to do(%d).", count)
		return retTransInfo, nil
	}

	//先判断是否存在交易序列号了，如果不存在，说明还没有交易发生。 这里做这个判断是因为在 getTransSeq 里如果没有设置过序列号的key会自动设置一次，但是在query中无法执行PutStat，会报错
	var seqKey = t.getAccTransSeqKey(accName)
	test, err := stub.GetState(seqKey)
	if err != nil {
		return nil, mylog.Errorf("queryAccTransInfos GetState failed. err=%s", err)
	}
	if test == nil {
		mylog.Info("queryAccTransInfos no trans saved.")
		return retTransInfo, nil
	}

	//先获取当前最大的序列号
	if queryMaxSeq != -1 {
		maxSeq = queryMaxSeq
	} else {
		maxSeq, err = t.getTransSeq(stub, seqKey)
		if err != nil {
			return nil, mylog.Errorf("queryAccTransInfos getTransSeq failed. err=%s", err)
		}
	}

	if begIdx > maxSeq {
		mylog.Warn("queryAccTransInfos nothing to do(%d,%d).", begIdx, maxSeq)
		return retTransInfo, nil
	}

	if count < 0 {
		count = maxSeq - begIdx + 1
	}
	/*
	   infoB, err := stub.GetState(t.getOneAccTransKey(accName))
	   if err != nil {
	       return nil, mylog.Errorf("queryAccTransInfos(%s) GetState failed.err=%s", accName, err)
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
	               mylog.Debug("queryAccTransInfos proc %d recds, end.", loop)
	               break
	           }
	           return nil, mylog.Errorf("queryAccTransInfos ReadBytes failed. last=%s, err=%s", string(oneStringB), err)
	       }
	       loop++
	       if begIdx > loop {
	           continue
	       }

	       oneString = string(oneStringB[:len(oneStringB)-1]) //去掉末尾的分隔符

	       trans, err = t.getQueryTransInfo(stub, oneString)
	       if err != nil {
	           mylog.Error("queryAccTransInfos(%s) getQueryTransInfo failed, err=%s.", accName, err)
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

			globTxKeyB, err = stub.GetState(t.getOneAccTransInfoKey(accName, loop))
			if err != nil {
				mylog.Errorf("queryAccTransInfos GetState(globTxKeyB,acc=%s,idx=%d) failed.err=%s", accName, loop, err)
				continue
			}

			trans, err = t.getOnceTransInfo(stub, string(globTxKeyB))
			if err != nil {
				mylog.Error("queryAccTransInfos getQueryTransInfo(idx=%d) failed.err=%s", loop, err)
				continue
			}

			var qTrans QueryTransRecd
			if trans.Time >= begTime && trans.Time <= endTime {
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

			globTxKeyB, err = stub.GetState(t.getOneAccTransInfoKey(accName, loop))
			if err != nil {
				mylog.Errorf("queryAccTransInfos GetState(globTxKeyB,acc=%s,idx=%d) failed.err=%s", accName, loop, err)
				continue
			}

			trans, err := t.getOnceTransInfo(stub, string(globTxKeyB))
			if err != nil {
				mylog.Error("queryAccTransInfos getQueryTransInfo(idx=%d) failed.err=%s", loop, err)
				continue
			}

			var qTrans QueryTransRecd
			if trans.Time >= begTime && trans.Time <= endTime {
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
		return nil, mylog.Errorf("queryAccTransInfos Marshal failed.err=%s", err)
	}

	return retTransInfo, nil
}

func (t *KD) getAllAccAmt(stub shim.ChaincodeStubInterface) ([]byte, error) {
	var qb QueryBalance
	qb.IssueAmount = 0
	qb.AccSumAmount = 0
	qb.AccCount = 0

	accsB, err := stub.GetState(ALL_ACC_INFO_KEY)
	if err != nil {
		return nil, mylog.Errorf("getAllAccAmt GetState failed. err=%s", err)
	}
	if accsB != nil {
		cbAccB, err := t.getCenterBankAcc(stub)
		if err != nil {
			return nil, mylog.Errorf("getAllAccAmt getCenterBankAcc failed. err=%s", err)
		}
		if cbAccB == nil {
			qb.Message += "none centerBank;"
		} else {
			cbEnt, err := t.getAccountEntity(stub, string(cbAccB))
			if err != nil {
				return nil, mylog.Errorf("getAllAccAmt getCenterBankAcc failed. err=%s", err)
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
					mylog.Error("getAllAccAmt ReadBytes failed. err=%s", err)
					continue
				}
			}
			qb.AccCount++
			acc = acc[:len(acc)-1] //去掉末尾的分隔符

			ent, err = t.getAccountEntity(stub, string(acc))
			if err != nil {
				mylog.Error("getAllAccAmt getAccountEntity(%s) failed. err=%s", string(acc), err)
				qb.Message += fmt.Sprintf("get account(%s) info failed;", string(acc))
				continue
			}
			qb.AccSumAmount += ent.RestAmount
		}
	}

	retValue, err := json.Marshal(qb)
	if err != nil {
		return nil, mylog.Errorf("getAllAccAmt Marshal failed. err=%s", err)
	}

	return retValue, nil
}

func (t *KD) getInfo4Web(stub shim.ChaincodeStubInterface, circulateAmtAccName string) ([]byte, error) {

	type QueryWebInfo struct {
		AccountNum       int64 `json:"accountcount"`  //账户数量
		IssueTotalAmount int64 `json:"issuetotalamt"` //预计发行总量
		IssueAmount      int64 `json:"issueamt"`      //已发行数量
		CirculateAmount  int64 `json:"circulateamt"`  //流通数量
	}

	var qwi QueryWebInfo
	qwi.AccountNum = 0
	qwi.IssueTotalAmount = 0
	qwi.IssueAmount = 0
	qwi.CirculateAmount = 0

	issueEntity, err := t.getAccountEntity(stub, COIN_ISSUE_ACC_ENTID)
	if err != nil {
		return nil, mylog.Errorf("getInfo4Web: getIssueEntity failed. err=%s", err)
	}
	qwi.IssueTotalAmount = issueEntity.TotalAmount
	qwi.IssueAmount = issueEntity.TotalAmount - issueEntity.RestAmount

	var asi AccountStatisticInfo
	asiB, err := stub.GetState(ACC_STATIC_INFO_KEY)
	if err != nil {
		return nil, mylog.Errorf("getInfo4Web: GetState failed. err=%s", err)
	}
	if asiB != nil {
		err = json.Unmarshal(asiB, &asi)
		if err != nil {
			return nil, mylog.Errorf("getInfo4Web: Unmarshal failed. err=%s", err)
		}
		qwi.AccountNum = asi.AccountCount
	}

	//如果传入了计算流通货币量的账户，用此账户；否则用央行账户
	if len(circulateAmtAccName) > 0 {
		ent, err := t.getAccountEntity(stub, circulateAmtAccName)
		if err != nil {
			return nil, mylog.Errorf("getInfo4Web: getAccountEntity failed. err=%s", err)
		}
		qwi.CirculateAmount = ent.TotalAmount - ent.RestAmount
	} else {
		cbAccB, err := t.getCenterBankAcc(stub)
		if err != nil {
			return nil, mylog.Errorf("getInfo4Web: getCenterBankAcc failed. err=%s", err)
		}
		if cbAccB != nil {
			cbEnt, err := t.getAccountEntity(stub, string(cbAccB))
			if err != nil {
				return nil, mylog.Errorf("getInfo4Web getAccountEntity failed. err=%s", err)
			}
			qwi.CirculateAmount = cbEnt.TotalAmount - cbEnt.RestAmount
		}
	}

	retValue, err := json.Marshal(qwi)
	if err != nil {
		return nil, mylog.Errorf("getInfo4Web Marshal failed. err=%s", err)
	}

	return retValue, nil
}

func (t *KD) verifySign(stub shim.ChaincodeStubInterface, certificate []byte) (bool, error) {
	/*
			mylog.Debug("verifySign ...")

			sigma, err := stub.GetCallerMetadata()
			if err != nil {
				return false, errors.New("Failed getting metadata")
			}
			payload, err := stub.GetPayload()
			if err != nil {
				return false, errors.New("Failed getting payload")
			}
			binding, err := stub.GetBinding()
			if err != nil {
				return false, errors.New("Failed getting binding")
			}

			//mylog.Debug("passed certificate [% x]", certificate)
			//mylog.Debug("passed sigma [% x]", sigma)
			//mylog.Debug("passed payload [% x]", payload)
			//mylog.Debug("passed binding [% x]", binding)

			ok, err := stub.VerifySignature(
				certificate,
				sigma,
				append(payload, binding...),
			)
			if err != nil {
				mylog.Error("Failed checking signature [%s]", err)
				return ok, err
			}
			if !ok {
				mylog.Error("Invalid signature")
				return ok, err
			}

			mylog.Debug("Check caller...Verified!")
		return ok, err
	*/
	return true, nil
}

func (t *KD) verifyIdentity(stub shim.ChaincodeStubInterface, userName string, ent *AccountEntity, attrs *UserAttrs) (bool, error) {
	/*
	   因为用到了认证用户，不能使用属性中的用户名来验证了。因为属性中存的是ent.Owner
	       //有时获取不到attr，这里做个判断，如果获取到再判断是否和账户中的用户相同
	       if len(attrs.UserName) > 0 && ent.Owner != attrs.UserName {
	           mylog.Errorf("verifyIdentity: user check failed(%s,%s).", ent.Owner, attrs.UserName)
	           return false, mylog.Errorf("verifyIdentity: user check failed(%s,%s).", ent.Owner, attrs.UserName)
	       }
	*/
	/*
		var cert []byte

		if userName == ent.Owner {
			cert = ent.OwnerCert
		} else {
			//登录的user如果不是owner，也不在认证列表中，报错
			if _, ok := ent.AuthUserCertMap[userName]; !ok {
				return false, mylog.Errorf("verifyIdentity: can't find user %s.", userName)
			}
			cert = ent.AuthUserCertMap[userName]
		}

		ok, err := t.verifySign(stub, cert)
		if err != nil {
			return false, mylog.Errorf("verifyIdentity: verifySign error, user=%s err=%s.", userName, err)
		}
		if !ok {
			return false, mylog.Errorf("verifyIdentity: verifySign failed, user=%s.", userName)
		}
	*/

	creatorByte, err := stub.GetCreator()
	if err != nil {
		return false, mylog.Errorf("verifyIdentity: GetCreator error, user=%s err=%s.", userName, err)
	}
	mylog.Debug("verifyIdentity: creatorByte = %s", string(creatorByte))

	certStart := bytes.IndexAny(creatorByte, "-----BEGIN")
	if certStart == -1 {
		return false, mylog.Errorf("verifyIdentity: No certificate found, user=%s.", userName)
	}
	certText := creatorByte[certStart:]

	block, _ := pem.Decode(certText)
	if block == nil {
		return false, mylog.Errorf("verifyIdentity: Decode failed, user=%s.", userName)
	}
	mylog.Debug("verifyIdentity: block = %+v", *block)

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false, mylog.Errorf("verifyIdentity: ParseCertificate failed, user=%s, err=%s.", userName, err)
	}
	mylog.Debug("verifyIdentity: cert = %+v", *cert)

	uname := cert.Subject.CommonName
	mylog.Debug("verifyIdentity: uname = %s", uname)

	return true, nil
}

func (t *KD) getUserAttrs(stub shim.ChaincodeStubInterface) (*UserAttrs, error) {
	/*
		//有时会获取不到attr，不知道怎么回事，先不返回错
		tmpName, err := stub.ReadCertAttribute(ATTR_USRNAME)
		if err != nil {
			mylog.Errorf("ReadCertAttribute(%s) failed. err=%s", ATTR_USRNAME, err)
			//return nil, mylog.Errorf("ReadCertAttribute(%s) failed. err=%s", ATTR_USRNAME, err)
		}
		tmpType, err := stub.ReadCertAttribute(ATTR_USRTYPE)
		if err != nil {
			mylog.Errorf("ReadCertAttribute(%s) failed. err=%s", ATTR_USRTYPE, err)
			//return nil, mylog.Errorf("ReadCertAttribute(%s) failed. err=%s", ATTR_USRTYPE, err)
		}
		tmpRole, err := stub.ReadCertAttribute(ATTR_USRROLE)
		if err != nil {
			mylog.Errorf("ReadCertAttribute(%s) failed. err=%s", ATTR_USRROLE, err)
			//return nil, mylog.Errorf("ReadCertAttribute(%s) failed. err=%s", ATTR_USRROLE, err)
		}
		mylog.Debug("getUserAttrs, userName=%s, userType=%s, userRole=%s", string(tmpName), string(tmpType), string(tmpRole))
	*/
	tmpName := "name"
	tmpRole := "role"
	tmpType := "type"
	var attrs UserAttrs
	attrs.UserName = string(tmpName)
	attrs.UserRole = string(tmpRole)
	attrs.UserType = string(tmpType)

	return &attrs, nil
}

func (t *KD) getAccountEntityKey(accName string) string {
	return ACC_ENTITY_PREFIX + accName
}
func (t *KD) getAccountNameFromKey(entKey string) string {
	return entKey[len(ACC_ENTITY_PREFIX):]
}
func (t *KD) getAccountLockInfoKey(accName string) string {
	return ACC_AMTLOCK_PREFIX + accName
}

func (t *KD) getAccountEntity(stub shim.ChaincodeStubInterface, entName string) (*AccountEntity, error) {
	var entB []byte
	var cb AccountEntity
	var err error

	entB, err = stub.GetState(t.getAccountEntityKey(entName))
	if err != nil {
		return nil, err
	}

	if entB == nil {
		return nil, ErrNilEntity
	}

	if err = json.Unmarshal(entB, &cb); err != nil {
		return nil, mylog.Errorf("getAccountEntity: Unmarshal failed, err=%s.", err)
	}

	return &cb, nil
}

func (t *KD) getAccountLockedAmount(stub shim.ChaincodeStubInterface, accName string, currTime int64) (int64, []CoinLockCfg, error) {
	var acli AccountCoinLockInfo

	var lockinfoKey = t.getAccountLockInfoKey(accName)
	acliB, err := stub.GetState(lockinfoKey)
	if err != nil {
		return math.MaxInt64, nil, mylog.Errorf("getAccountLockedAmount: GetState  failed. err=%s", err)
	}

	var lockAmt int64 = 0
	if acliB == nil {
		lockAmt = 0
	} else {

		err = json.Unmarshal(acliB, &acli)
		if err != nil {
			return math.MaxInt64, nil, mylog.Errorf("getAccountLockedAmount: Unmarshal  failed. err=%s", err)
		}

		for _, lockCfg := range acli.LockList {
			if lockCfg.LockEndTime > currTime {
				lockAmt += lockCfg.LockAmount
			}
		}
	}

	mylog.Debug("getAccountLockedAmount: amount is %d for %s", lockAmt, accName)

	return lockAmt, acli.LockList, nil

}

func (t *KD) isEntityExists(stub shim.ChaincodeStubInterface, entName string) (bool, error) {
	var entB []byte
	var err error

	entB, err = stub.GetState(t.getAccountEntityKey(entName))
	if err != nil {
		return false, err
	}

	if entB == nil {
		return false, nil
	}

	return true, nil
}

//央行数据写入
func (t *KD) setAccountEntity(stub shim.ChaincodeStubInterface, cb *AccountEntity) error {

	jsons, err := json.Marshal(cb)

	if err != nil {
		return mylog.Errorf("marshal cb failed. err=%s", err)
	}

	err = t.putState_Ex(stub, t.getAccountEntityKey(cb.EntID), jsons)

	if err != nil {
		return mylog.Errorf("PutState cb failed. err=%s", err)
	}
	return nil
}

//发行
func (t *KD) issueCoin(stub shim.ChaincodeStubInterface, cbID string, issueAmount, issueTime int64) ([]byte, error) {
	mylog.Debug("Enter issueCoin")

	var err error

	if issueAmount < 0 {
		return nil, mylog.Errorf("issueCoin issueAmount < 0.")
	}
	if issueAmount == 0 {
		return nil, nil
	}

	var cb *AccountEntity
	cb, err = t.getAccountEntity(stub, cbID)
	if err != nil {
		return nil, mylog.Errorf("getCenterBank failed. err=%s", err)
	}

	issueEntity, err := t.getAccountEntity(stub, COIN_ISSUE_ACC_ENTID)
	if err != nil {
		return nil, mylog.Errorf("issue: getIssueEntity failed. err=%s", err)
	}

	mylog.Debug("issue before:cb=%+v, issue=%+v", cb, issueEntity)

	if issueAmount > issueEntity.RestAmount {
		return nil, mylog.Errorf("issue amount not enougth(%v,%v), reject.", issueEntity.RestAmount, issueAmount)
	}

	issueEntity.RestAmount -= issueAmount
	cb.TotalAmount += issueAmount
	cb.RestAmount += issueAmount

	err = t.setAccountEntity(stub, cb)
	if err != nil {
		return nil, mylog.Errorf("issue: setCenterBank failed. err=%s", err)
	}

	err = t.setAccountEntity(stub, issueEntity)
	if err != nil {
		return nil, mylog.Errorf("issue: setIssueEntity failed. err=%s", err)
	}

	mylog.Debug("issue after:cb=%+v, issue=%+v", cb, issueEntity)

	//这里只记录一下央行的收入，不记录支出
	err = t.recordTranse(stub, cb, issueEntity, TRANS_INCOME, "issue", "center bank issue coin.", issueAmount, issueTime)
	if err != nil {
		return nil, mylog.Errorf("issue: recordTranse failed. err=%s", err)
	}

	return nil, nil
}

//转账
func (t *KD) transferCoin(stub shim.ChaincodeStubInterface, from, to, transType, description string, amount, transeTime int64, sameEntSaveTrans bool) ([]byte, error) {
	mylog.Debug("Enter transferCoin")

	var err error

	if amount < 0 {
		return nil, mylog.Errorf("transferCoin failed. invalid amount(%d)", amount)
	}

	//有时前端后台调用这个接口时，可能会传0
	if amount == 0 {
		return nil, nil
	}

	//如果账户相同，并且账户相同时不需要记录交易，直接返回
	if from == to && !sameEntSaveTrans {
		mylog.Warn("transferCoin from equals to.")
		return nil, nil
	}

	var fromEntity, toEntity *AccountEntity
	fromEntity, err = t.getAccountEntity(stub, from)
	if err != nil {
		return nil, mylog.Errorf("transferCoin: getAccountEntity(id=%s) failed. err=%s", from, err)
	}
	toEntity, err = t.getAccountEntity(stub, to)
	if err != nil {
		return nil, mylog.Errorf("transferCoin: getAccountEntity(id=%s) failed. err=%s", to, err)
	}

	//判断是否有锁定金额
	lockAmt, _, err := t.getAccountLockedAmount(stub, from, transeTime)
	if err != nil {
		return nil, mylog.Errorf("transferCoin: getAccountLockedAmount(id=%s) failed. err=%s", from, err)
	}

	if fromEntity.RestAmount-lockAmt < amount {
		return nil, mylog.Errorf("transferCoin: fromEntity(id=%s) restAmount not enough(%d,%d,%d).", from, fromEntity.RestAmount, lockAmt, amount)
	}

	//如果账户相同，并且账户相同时需要记录交易，记录并返回
	if from == to && sameEntSaveTrans {
		err = t.recordTranse(stub, fromEntity, toEntity, TRANS_PAY, transType, description, amount, transeTime)
		if err != nil {
			return nil, mylog.Errorf("transferCoin: setAccountEntity recordTranse fromEntity(id=%s) failed. err=%s", from, err)
		}

		err = t.recordTranse(stub, toEntity, fromEntity, TRANS_INCOME, transType, description, amount, transeTime)
		if err != nil {
			return nil, mylog.Errorf("transferCoin: setAccountEntity recordTranse fromEntity(id=%s) failed. err=%s", from, err)
		}
		return nil, nil
	}

	//账户相同时为什么单独处理？  因为如果走了下面的流程，setAccountEntity两次同一个账户，会导致账户余额变化。 除非在计算并设置完fromEntity之后，再获取一下toEntity，再计算toEntity，这样感觉太呆了

	//mylog.Debug("transferCoin: fromEntity before= %+v", fromEntity)
	//mylog.Debug("transferCoin: toEntity before= %+v", toEntity)

	fromEntity.RestAmount -= amount

	toEntity.RestAmount += amount
	toEntity.TotalAmount += amount

	//mylog.Debug("transferCoin: fromEntity after= %+v", fromEntity)
	//mylog.Debug("transferCoin: toEntity after= %+v", toEntity)

	err = t.setAccountEntity(stub, fromEntity)
	if err != nil {
		return nil, mylog.Errorf("transferCoin: setAccountEntity of fromEntity(id=%s) failed. err=%s", from, err)
	}

	err = t.recordTranse(stub, fromEntity, toEntity, TRANS_PAY, transType, description, amount, transeTime)
	if err != nil {
		return nil, mylog.Errorf("transferCoin: setAccountEntity recordTranse fromEntity(id=%s) failed. err=%s", from, err)
	}

	err = t.setAccountEntity(stub, toEntity)
	if err != nil {
		return nil, mylog.Errorf("transferCoin: setAccountEntity of toEntity(id=%s) failed. err=%s", to, err)
	}

	//两个账户的收入支出都记录交易
	err = t.recordTranse(stub, toEntity, fromEntity, TRANS_INCOME, transType, description, amount, transeTime)
	if err != nil {
		return nil, mylog.Errorf("transferCoin: setAccountEntity recordTranse fromEntity(id=%s) failed. err=%s", from, err)
	}

	return nil, err
}

const (
	TRANS_LVL_CB   = 1
	TRANS_LVL_COMM = 2
)

//记录交易。目前交易分为两种：一种是和央行打交道的，包括央行发行货币、央行给项目或企业转帐，此类交易普通用户不能查询；另一种是项目、企业、个人间互相转账，此类交易普通用户能查询
func (t *KD) recordTranse(stub shim.ChaincodeStubInterface, fromEnt, toEnt *AccountEntity, incomePayFlag int, transType, description string, amount, times int64) error {
	var transInfo Transaction
	//var now = time.Now()

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
	accCB, err := t.getCenterBankAcc(stub)
	if err != nil {
		return mylog.Errorf("recordTranse call getCenterBankAcc failed. err=%s", err)
	}
	if (accCB != nil) && (string(accCB) == transInfo.FromID || string(accCB) == transInfo.ToID) {
		transLevel = TRANS_LVL_CB
	}

	transInfo.TransLvl = transLevel

	err = t.setTransInfo(stub, &transInfo)
	if err != nil {
		return mylog.Errorf("recordTranse call setTransInfo failed. err=%s", err)
	}

	return nil
}

func (t *KD) checkAccountName(accName string) error {
	if strings.ContainsAny(accName, ACC_INVALID_CHAR_SET) {
		return mylog.Errorf("accName '%s' can not contains any of '%s'.", accName, ACC_INVALID_CHAR_SET)
	}
	return nil
}

func (t *KD) saveAccountName(stub shim.ChaincodeStubInterface, accName string) error {
	accB, err := stub.GetState(ALL_ACC_INFO_KEY)
	if err != nil {
		return mylog.Errorf("saveAccountName GetState failed.err=%s", err)
	}

	var accs []byte
	if accB == nil {
		accs = append([]byte(accName), MULTI_STRING_DELIM) //第一次添加accName，最后也要加上分隔符
	} else {
		accs = append(accB, []byte(accName)...)
		accs = append(accs, MULTI_STRING_DELIM)
	}

	err = t.putState_Ex(stub, ALL_ACC_INFO_KEY, accs)
	if err != nil {
		return mylog.Errorf("saveAccountName PutState(accs) failed.err=%s", err)
	}

	var asi AccountStatisticInfo
	asiB, err := stub.GetState(ACC_STATIC_INFO_KEY)
	if asiB == nil {
		asi.AccountCount = 1
	} else {
		err = json.Unmarshal(asiB, &asi)
		if err != nil {
			return mylog.Errorf("saveAccountName Unmarshal failed.err=%s", err)
		}
		asi.AccountCount++
	}

	asiB, err = json.Marshal(asi)
	if err != nil {
		return mylog.Errorf("saveAccountName Marshal failed.err=%s", err)
	}

	err = t.putState_Ex(stub, ACC_STATIC_INFO_KEY, asiB)
	if err != nil {
		return mylog.Errorf("saveAccountName PutState(asiB) failed.err=%s", err)
	}

	return nil
}

/*
func (t *KD) getAllAccountNames(stub shim.ChaincodeStubInterface) ([]byte, error) {
	accB, err := stub.GetState(ALL_ACC_INFO_KEY)
	if err != nil {
		mylog.Error("getAllAccountNames GetState failed.err=%s", err)
		return nil, err
	}
	return accB, nil
}
*/

func (t *KD) newAccount(stub shim.ChaincodeStubInterface, accName string, accType int, userName string, cert []byte, times int64, isCBAcc bool) ([]byte, error) {
	mylog.Debug("Enter openAccount")

	var err error
	var accExist bool

	if err = t.checkAccountName(accName); err != nil {
		return nil, err
	}

	accExist, err = t.isEntityExists(stub, accName)
	if err != nil {
		return nil, mylog.Errorf("isEntityExists (id=%s) failed. err=%s", accName, err)
	}

	if accExist {
		/*  账户已存在时这里不返回错误。目前前端保证账户的唯一性，如果这里返回错误，那么前端需要再调用一次查询账户是否存在的接口，速度会慢一点
		    return nil, mylog.Errorf("account (id=%s) failed, already exists.", accName)
		*/
		return nil, nil
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
	ent.OwnerCert = cert

	err = t.setAccountEntity(stub, &ent)
	if err != nil {
		return nil, mylog.Errorf("openAccount setAccountEntity (id=%s) failed. err=%s", accName, err)
	}

	mylog.Debug("openAccount success: %+v", ent)

	//央行账户此处不保存
	if !isCBAcc {
		err = t.saveAccountName(stub, accName)
		if err != nil {
			return nil, mylog.Errorf("openAccount saveAccountName (id=%s) failed. err=%s", accName, err)
		}
	}

	return nil, nil
}

var centerBankAccCache []byte = nil

func (t *KD) setCenterBankAcc(stub shim.ChaincodeStubInterface, acc string) error {
	err := t.putState_Ex(stub, CENTERBANK_ACC_KEY, []byte(acc))
	if err != nil {
		mylog.Error("setCenterBankAcc PutState failed.err=%s", err)
		return err
	}

	centerBankAccCache = []byte(acc)

	return nil
}
func (t *KD) getCenterBankAcc(stub shim.ChaincodeStubInterface) ([]byte, error) {
	if centerBankAccCache != nil {
		return centerBankAccCache, nil
	}

	bankB, err := stub.GetState(CENTERBANK_ACC_KEY)
	if err != nil {
		mylog.Error("getCenterBankAcc GetState failed.err=%s", err)
		return nil, err
	}

	centerBankAccCache = bankB

	return bankB, nil
}

func (t *KD) getTransSeq(stub shim.ChaincodeStubInterface, transSeqKey string) (int64, error) {
	seqB, err := stub.GetState(transSeqKey)
	if err != nil {
		mylog.Error("getTransSeq GetState failed.err=%s", err)
		return -1, err
	}
	//如果不存在则创建
	if seqB == nil {
		err = t.putState_Ex(stub, transSeqKey, []byte("0"))
		if err != nil {
			mylog.Error("initTransSeq PutState failed.err=%s", err)
			return -1, err
		}
		return 0, nil
	}

	seq, err := strconv.ParseInt(string(seqB), 10, 64)
	if err != nil {
		mylog.Error("getTransSeq ParseInt failed.seq=%+v, err=%s", seqB, err)
		return -1, err
	}

	return seq, nil
}
func (t *KD) setTransSeq(stub shim.ChaincodeStubInterface, transSeqKey string, seq int64) error {
	err := t.putState_Ex(stub, transSeqKey, []byte(strconv.FormatInt(seq, 10)))
	if err != nil {
		mylog.Error("setTransSeq PutState failed.err=%s", err)
		return err
	}

	return nil
}

func (t *KD) getTransInfoKey(stub shim.ChaincodeStubInterface, seq int64) string {
	var buf = bytes.NewBufferString(TRANSINFO_PREFIX)
	buf.WriteString(strconv.FormatInt(seq, 10))
	return buf.String()
}

func (t *KD) getGlobalTransSeqKey(stub shim.ChaincodeStubInterface) string {
	return TRANSSEQ_PREFIX + "global"
}

//获取某个账户的trans seq key
func (t *KD) getAccTransSeqKey(accName string) string {
	return TRANSSEQ_PREFIX + "acc_" + accName
}

func (t *KD) setTransInfo(stub shim.ChaincodeStubInterface, info *Transaction) error {
	//先获取全局seq
	seqGlob, err := t.getTransSeq(stub, t.getGlobalTransSeqKey(stub))
	if err != nil {
		mylog.Error("setTransInfo getTransSeq failed.err=%s", err)
		return err
	}
	seqGlob++

	/*
	   //再获取当前交易级别的seq
	   seqLvl, err := t.getTransSeq(stub, t.getTransSeqKey(stub, info.TransLvl))
	   if err != nil {
	       mylog.Error("setTransInfo getTransSeq failed.err=%s", err)
	       return err
	   }
	   seqLvl++
	*/

	info.GlobalSerial = seqGlob
	//info.Serial = seqLvl
	transJson, err := json.Marshal(info)
	if err != nil {
		return mylog.Errorf("setTransInfo marshal failed. err=%s", err)
	}

	putKey := t.getTransInfoKey(stub, seqGlob)
	err = t.putState_Ex(stub, putKey, transJson)
	if err != nil {
		return mylog.Errorf("setTransInfo PutState failed. err=%s", err)
	}

	/*
	   //from和to账户都记录一次，因为两个账户的交易记录只有一条
	   err = t.setOneAccTransInfo(stub, info.FromID, putKey)
	   if err != nil {
	       return mylog.Errorf("setTransInfo setOneAccTransInfo(%s) failed. err=%s", info.FromID, err)
	   }
	   err = t.setOneAccTransInfo(stub, info.ToID, putKey)
	   if err != nil {
	       return mylog.Errorf("setTransInfo setOneAccTransInfo(%s) failed. err=%s", info.ToID, err)
	   }
	*/
	//目前交易记录收入和支出都记录了，所以这里只用记录一次
	err = t.setOneAccTransInfo(stub, info.FromID, putKey)
	if err != nil {
		return mylog.Errorf("setTransInfo setOneAccTransInfo(%s) failed. err=%s", info.FromID, err)
	}

	//交易信息设置成功后，保存序列号
	err = t.setTransSeq(stub, t.getGlobalTransSeqKey(stub), seqGlob)
	if err != nil {
		return mylog.Errorf("setTransInfo setTransSeq failed. err=%s", err)
	}
	/*
	   err = t.setTransSeq(stub, t.getTransSeqKey(stub, info.TransLvl), seqLvl)
	   if err != nil {
	       mylog.Error("setTransInfo setTransSeq failed. err=%s", err)
	       return errors.New("setTransInfo setTransSeq failed.")
	   }
	*/

	mylog.Debug("setTransInfo OK, info=%+v", info)

	return nil
}

func (t *KD) getOneAccTransInfoKey(accName string, seq int64) string {
	return ONE_ACC_TRANS_PREFIX + accName + "_" + strconv.FormatInt(seq, 10)
}

func (t *KD) setOneAccTransInfo(stub shim.ChaincodeStubInterface, accName, GlobalTransKey string) error {

	seq, err := t.getTransSeq(stub, t.getAccTransSeqKey(accName))
	if err != nil {
		return mylog.Errorf("setOneAccTransInfo getTransSeq failed.err=%s", err)
	}
	seq++

	err = t.putState_Ex(stub, t.getOneAccTransInfoKey(accName, seq), []byte(GlobalTransKey))
	if err != nil {
		return mylog.Errorf("setOneAccTransInfo PutState failed. err=%s", err)
	}

	//交易信息设置成功后，保存序列号
	err = t.setTransSeq(stub, t.getAccTransSeqKey(accName), seq)
	if err != nil {
		return mylog.Errorf("setOneAccTransInfo setTransSeq failed. err=%s", err)
	}

	return nil
}

func (t *KD) getOnceTransInfo(stub shim.ChaincodeStubInterface, key string) (*Transaction, error) {
	var err error
	var trans Transaction

	tmpState, err := stub.GetState(key)
	if err != nil {
		mylog.Error("getTransInfo GetState failed.err=%s", err)
		return nil, err
	}
	if tmpState == nil {
		return nil, mylog.Errorf("getTransInfo GetState nil.")
	}

	err = json.Unmarshal(tmpState, &trans)
	if err != nil {
		return nil, mylog.Errorf("getTransInfo Unmarshal failed. err=%s", err)
	}

	mylog.Debug("getTransInfo OK, info=%+v", trans)

	return &trans, nil
}
func (t *KD) getQueryTransInfo(stub shim.ChaincodeStubInterface, key string) (*QueryTransRecd, error) {
	var err error
	var trans QueryTransRecd

	tmpState, err := stub.GetState(key)
	if err != nil {
		mylog.Error("getQueryTransInfo GetState failed.err=%s", err)
		return nil, err
	}
	if tmpState == nil {
		return nil, mylog.Errorf("getQueryTransInfo GetState nil.")
	}

	err = json.Unmarshal(tmpState, &trans)
	if err != nil {
		return nil, mylog.Errorf("getQueryTransInfo Unmarshal failed. err=%s", err)
	}

	mylog.Debug("getQueryTransInfo OK, info=%+v", trans)

	return &trans, nil
}

func (t *KD) setAllocEarnTx(stub shim.ChaincodeStubInterface, rackid, allocKey string, totalAmt int64,
	accs *AllocAccs, eap *EarningAllocRate, times int64) ([]byte, error) {

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

	var err error
	err = t.getRolesAllocEarning(rolesAllocAmt.SellerAmount, accs.SellerAcc, eat.AmountMap[RACK_ROLE_SELLER])
	if err != nil {
		return nil, mylog.Errorf("setAllocEarnTx getRolesAllocEarning 1 failed.err=%s", err)
	}
	err = t.getRolesAllocEarning(rolesAllocAmt.FielderAmount, accs.FielderAcc, eat.AmountMap[RACK_ROLE_FIELDER])
	if err != nil {
		return nil, mylog.Errorf("setAllocEarnTx getRolesAllocEarning 2 failed.err=%s", err)
	}
	err = t.getRolesAllocEarning(rolesAllocAmt.DeliveryAmount, accs.DeliveryAcc, eat.AmountMap[RACK_ROLE_DELIVERY])
	if err != nil {
		return nil, mylog.Errorf("setAllocEarnTx getRolesAllocEarning 3 failed.err=%s", err)
	}
	err = t.getRolesAllocEarning(rolesAllocAmt.PlatformAmount, accs.PlatformAcc, eat.AmountMap[RACK_ROLE_PLATFORM])
	if err != nil {
		return nil, mylog.Errorf("setAllocEarnTx getRolesAllocEarning 4 failed.err=%s", err)
	}

	seqKey := t.getAllocTxSeqKey(stub, rackid)
	seq, err := t.getTransSeq(stub, seqKey)
	if err != nil {
		return nil, mylog.Errorf("setAllocEarnTx  getTransSeq failed.err=%s", err)
	}
	seq++

	eat.GlobalSerial = seq
	eat.DateTime = times

	eatJson, err := json.Marshal(eat)
	if err != nil {
		return nil, mylog.Errorf("setAllocEarnTx Marshal failed. err=%s", err)
	}
	mylog.Debug("setAllocEarnTx return %s.", string(eatJson))

	var txKey = t.getAllocTxKey(stub, rackid, seq)
	err = t.putState_Ex(stub, txKey, eatJson)
	if err != nil {
		return nil, mylog.Errorf("setAllocEarnTx  PutState_Ex failed.err=%s", err)
	}

	err = t.setTransSeq(stub, seqKey, seq)
	if err != nil {
		return nil, mylog.Errorf("setAllocEarnTx  setTransSeq failed.err=%s", err)
	}

	//记录每个账户的分成情况
	//四种角色有可能是同一个人，所以判断一下，如果已保存过key，则不再保存
	var checkMap = make(map[string]int)
	err = t.setOneAccAllocEarnTx(stub, accs.SellerAcc, txKey)
	if err != nil {
		return nil, mylog.Errorf("setAllocEarnTx  setOneAccAllocEarnTx(%s) failed.err=%s", accs.SellerAcc, err)
	}
	checkMap[accs.SellerAcc] = 0

	if _, ok := checkMap[accs.FielderAcc]; !ok {
		err = t.setOneAccAllocEarnTx(stub, accs.FielderAcc, txKey)
		if err != nil {
			return nil, mylog.Errorf("setAllocEarnTx  setOneAccAllocEarnTx(%s) failed.err=%s", accs.FielderAcc, err)
		}
		checkMap[accs.FielderAcc] = 0
	}

	if _, ok := checkMap[accs.DeliveryAcc]; !ok {
		err = t.setOneAccAllocEarnTx(stub, accs.DeliveryAcc, txKey)
		if err != nil {
			return nil, mylog.Errorf("setAllocEarnTx  setOneAccAllocEarnTx(%s) failed.err=%s", accs.DeliveryAcc, err)
		}
		checkMap[accs.DeliveryAcc] = 0
	}

	if _, ok := checkMap[accs.PlatformAcc]; !ok {
		err = t.setOneAccAllocEarnTx(stub, accs.PlatformAcc, txKey)
		if err != nil {
			return nil, mylog.Errorf("setAllocEarnTx  setOneAccAllocEarnTx(%s) failed.err=%s", accs.PlatformAcc, err)
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

func (t *KD) setOneAccAllocEarnTx(stub shim.ChaincodeStubInterface, accName, txKey string) error {
	var accTxKey = t.getOneAccAllocTxKey(accName)

	txsB, err := stub.GetState(accTxKey)
	if err != nil {
		return mylog.Errorf("setOneAccAllocEarnTx: GetState err = %s", err)
	}

	var newTxsB []byte
	if txsB == nil {
		newTxsB = append([]byte(txKey), MULTI_STRING_DELIM) //第一次添加accName，最后也要加上分隔符
	} else {
		newTxsB = append(txsB, []byte(txKey)...)
		newTxsB = append(newTxsB, MULTI_STRING_DELIM)
	}

	err = t.putState_Ex(stub, accTxKey, newTxsB)
	if err != nil {
		mylog.Error("setOneAccAllocEarnTx PutState failed.err=%s", err)
		return err
	}

	return nil
}

func (t *KD) getRolesAllocEarning(totalAmt int64, accs string, result map[string]int64) error {

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
				return mylog.Errorf("getRolesAllocEarning  accs parse error, '%s' has no ':'.", acc)
			}
			var pair = strings.Split(acc, ":")
			if len(pair) != 2 {
				return mylog.Errorf("getRolesAllocEarning  accs parse error, '%s' format error 1.", acc)
			}
			rat, err = strconv.Atoi(pair[1])
			if err != nil {
				return mylog.Errorf("getRolesAllocEarning  accs parse error, '%s' format error 2.", acc)
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
			return mylog.Errorf("getRolesAllocEarning  accs parse error, '%s' format error 3.", newAccs)
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

func (t *KD) getAllocTxRecdByKey(stub shim.ChaincodeStubInterface, rackid, allocKey string) ([]byte, error) {

	var retTransInfo = []byte("[]") //默认为空数组。 因为和下面的查询所有记录使用同一个restful接口，所以这里也返回数组形式

	//先判断是否存在交易序列号了，如果不存在，说明还没有交易发生。 这里做这个判断是因为在 getTransSeq 里如果没有设置过序列号的key会自动设置一次，但是在query中无法执行PutStat，会报错
	var seqKey = t.getAllocTxSeqKey(stub, rackid)
	test, err := stub.GetState(seqKey)
	if err != nil {
		return nil, mylog.Errorf("getOneAllocRecd GetState(seqKey) failed. err=%s", err)
	}
	if test == nil {
		mylog.Info("getOneAllocRecd no trans saved.")
		return retTransInfo, nil
	}

	//先获取当前最大的序列号
	maxSeq, err := t.getTransSeq(stub, seqKey)
	if err != nil {
		return nil, mylog.Errorf("getOneAllocRecd getTransSeq failed. err=%s", err)
	}

	var txArray []QueryEarningAllocTx = []QueryEarningAllocTx{} //给个默认空值，即使没有数据，marshal之后也会为'[]'

	//从最后往前找，因为查找最新的可能性比较大
	for i := maxSeq; i > 0; i-- { //序列号生成器从1开始
		txkey := t.getAllocTxKey(stub, rackid, i)
		txB, err := stub.GetState(txkey)
		if err != nil {
			mylog.Errorf("getOneAllocRecd GetState(rackid=%s) failed. err=%s", rackid, err)
			continue
		}
		if txB == nil {
			mylog.Errorf("getOneAllocRecd GetState(rackid=%s) nil.", rackid)
			continue
		}

		var eat EarningAllocTx
		err = json.Unmarshal(txB, &eat)
		if err != nil {
			return nil, mylog.Errorf("getOneAllocRecd Unmarshal(rackid=%s) failed. err=%s", rackid, err)
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
		return nil, mylog.Errorf("getOneAllocRecd Marshal(rackid=%s) failed. err=%s", rackid, err)
	}

	return retTransInfo, nil
}
func (t *KD) getAllocTxRecds(stub shim.ChaincodeStubInterface, rackid string, begIdx, count, begTime, endTime int64) ([]byte, error) {
	var maxSeq int64
	var err error
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
		mylog.Warn("getAllocTxRecds nothing to do(%d).", count)
		return retTransInfo, nil
	}

	//先判断是否存在交易序列号了，如果不存在，说明还没有交易发生。 这里做这个判断是因为在 getTransSeq 里如果没有设置过序列号的key会自动设置一次，但是在query中无法执行PutStat，会报错
	var seqKey = t.getAllocTxSeqKey(stub, rackid)
	test, err := stub.GetState(seqKey)
	if err != nil {
		return nil, mylog.Errorf("getAllocTxRecds GetState(seqKey) failed. err=%s", err)
	}
	if test == nil {
		mylog.Info("getAllocTxRecds no trans saved.")
		return retTransInfo, nil
	}

	//先获取当前最大的序列号
	maxSeq, err = t.getTransSeq(stub, seqKey)
	if err != nil {
		return nil, mylog.Errorf("getAllocTxRecds getTransSeq failed. err=%s", err)
	}

	if begIdx > maxSeq {
		mylog.Warn("getAllocTxRecds nothing to do(%d,%d).", begIdx, maxSeq)
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
		txB, err := stub.GetState(txkey)
		if err != nil {
			mylog.Errorf("getAllocTxRecds GetState(rackid=%s) failed. err=%s", rackid, err)
			continue
		}
		if txB == nil {
			mylog.Errorf("getAllocTxRecds GetState(rackid=%s) nil.", rackid)
			continue
		}

		var eat EarningAllocTx
		err = json.Unmarshal(txB, &eat)
		if err != nil {
			return nil, mylog.Errorf("getAllocTxRecds Unmarshal(rackid=%s) failed. err=%s", rackid, err)
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
		return nil, mylog.Errorf("getAllocTxRecds Marshal(rackid=%s) failed. err=%s", rackid, err)
	}

	return retTransInfo, nil
}

func (t *KD) getOneAccAllocTxRecds(stub shim.ChaincodeStubInterface, accName string, begIdx, count, begTime, endTime int64) ([]byte, error) {
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
		mylog.Warn("getOneAccAllocTxRecds nothing to do(%d).", count)
		return resultJson, nil
	}
	//count为负数，查询到最后
	if count < 0 {
		count = math.MaxInt64
	}

	txsB, err := stub.GetState(accTxKey)
	if err != nil {
		return nil, mylog.Errorf("getOneAccAllocTxRecds: GetState(accName=%s) err = %s", accName, err)
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
	for {
		if cnt >= count {
			break
		}
		oneStringB, err = buf.ReadBytes(MULTI_STRING_DELIM)
		if err != nil {
			if err == io.EOF {
				mylog.Debug("getOneAccAllocTxRecds proc %d recds, end.", loop)
				break
			}
			return nil, mylog.Errorf("getOneAccAllocTxRecds ReadBytes failed. last=%s, err=%s", string(oneStringB), err)
		}
		loop++
		if begIdx > loop {
			continue
		}

		oneString = string(oneStringB[:len(oneStringB)-1]) //去掉末尾的分隔符
		var pqaeat *QueryAccEarningAllocTx
		pqaeat, err = t.getOneAccAllocTx(stub, oneString, accName)
		if err != nil {
			return nil, mylog.Errorf("getOneAccAllocTxRecds walker failed. acc=%s, err=%s", accName, err)
		}
		if pqaeat.DateTime >= begTime && pqaeat.DateTime <= endTime {
			pqaeat.Serail = loop
			qaeatArr = append(qaeatArr, *pqaeat)
			cnt++
		}
	}

	resultJson, err = json.Marshal(qaeatArr)
	if err != nil {
		return nil, mylog.Errorf("getOneAccAllocTxRecds Marshal failed. acc=%s, err=%s", accName, err)
	}

	return resultJson, nil
}

func (t *KD) getOneAccAllocTx(stub shim.ChaincodeStubInterface, txKey, accName string) (*QueryAccEarningAllocTx, error) {
	eat, err := t.getAllocTxRecdEntity(stub, txKey)
	if err != nil {
		return nil, mylog.Errorf("procOneAccAllocTx getAllocTxRecdEntity failed. txKey=%s, err=%s", txKey, err)
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

func (t *KD) getAllocTxRecdEntity(stub shim.ChaincodeStubInterface, txKey string) (*EarningAllocTx, error) {
	txB, err := stub.GetState(txKey)
	if err != nil {
		return nil, mylog.Errorf("getAllocTxRecdEntity GetState(txKey=%s) failed. err=%s", txKey, err)
	}
	if txB == nil {
		return nil, mylog.Errorf("getAllocTxRecdEntity GetState(txKey=%s) nil.", txKey)
	}

	var eat EarningAllocTx
	err = json.Unmarshal(txB, &eat)
	if err != nil {
		return nil, mylog.Errorf("getAllocTxRecdEntity Unmarshal(txKey=%s) failed. err=%s", txKey, err)
	}

	return &eat, nil
}

func (t *KD) getRackAllocCfg(stub shim.ChaincodeStubInterface, rackid string, peap *EarningAllocRate) ([]byte, error) {
	var eapB []byte = nil
	var err error

	if rackid != "*" {
		eapB, err = stub.GetState(t.getRackAllocRateKey(rackid))
		if err != nil {
			return nil, mylog.Errorf("getRackAllocCfg GetState(rackid=%s) failed. err=%s", rackid, err)
		}
	}

	if eapB == nil {
		mylog.Warn("getRackAllocCfg GetState(rackid=%s) nil, try to get global.", rackid)
		//没有为该货架单独配置，返回global配置
		eapB, err = stub.GetState(t.getGlobalRackAllocRateKey())
		if err != nil {
			return nil, mylog.Errorf("getRackAllocCfg GetState(global, rackid=%s) failed. err=%s", rackid, err)
		}
		if eapB == nil {
			return nil, mylog.Errorf("getRackAllocCfg GetState(global, rackid=%s) nil.", rackid)
		}
	}

	if peap != nil {
		err = json.Unmarshal(eapB, peap)
		if err != nil {
			return nil, mylog.Errorf("getRackAllocCfg Unmarshal failed. err=%s", rackid, err)
		}
	}

	return eapB, err
}

/* ----------------------- 积分奖励相关 ----------------------- */
func (t *KD) getGlobalRackEncourageScoreCfgKey() string {
	return RACK_SALE_ENC_SCORE_CFG_PREFIX + "global"
}
func (t *KD) getRackEncourageScoreCfgKey(rackid string) string {
	return RACK_SALE_ENC_SCORE_CFG_PREFIX + "rack_" + rackid
}

func (t *KD) setRackEncourageScoreCfg(stub shim.ChaincodeStubInterface, rackid, cfgStr string, invokeTime int64) ([]byte, error) {
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
			return nil, mylog.Errorf("setRackEncourageScoreCfg  rangeRate parse error, '%s' has no ':'.", rangeRate)
		}
		var pair = strings.Split(rangeRate, ":")
		if len(pair) != 2 {
			return nil, mylog.Errorf("setRackEncourageScoreCfg  rangeRate parse error, '%s' format error 1.", rangeRate)
		}
		//"-"表示正无穷
		if pair[0] == "-" {
			rang = math.MaxInt64
		} else {
			rang, err = strconv.ParseInt(pair[0], 0, 64)
			if err != nil {
				return nil, mylog.Errorf("setRackEncourageScoreCfg  rangeRate parse error, '%s' format error 2.", rangeRate)
			}
		}
		percent, err = strconv.Atoi(pair[1])
		if err != nil {
			return nil, mylog.Errorf("setRackEncourageScoreCfg  rangeRate parse error, '%s' format error 3.", rangeRate)
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
		return nil, mylog.Errorf("setRackEncourageScoreCfg Marshal failed. err=%s", err)
	}

	var stateKey string
	if rackid == "*" {
		stateKey = t.getGlobalRackEncourageScoreCfgKey()
	} else {
		stateKey = t.getRackEncourageScoreCfgKey(rackid)
	}

	err = t.putState_Ex(stub, stateKey, sepcJson)
	if err != nil {
		return nil, mylog.Errorf("setRackEncourageScoreCfg PutState_Ex failed. err=%s", err)
	}

	return nil, nil
}

func (t *KD) getRackEncourageScoreCfg(stub shim.ChaincodeStubInterface, rackid string, psepc *ScoreEncouragePercentCfg) ([]byte, error) {

	var sepcB []byte = nil
	var err error

	if rackid != "*" {
		sepcB, err = stub.GetState(t.getRackEncourageScoreCfgKey(rackid))
		if err != nil {
			return nil, mylog.Errorf("getRackEncourageScoreCfg GetState failed.rackid=%s err=%s", rackid, err)
		}
	}

	if sepcB == nil {
		mylog.Warn("getRackEncourageScoreCfg: can not find cfg for %s, will use golobal.", rackid)
		sepcB, err = stub.GetState(t.getGlobalRackEncourageScoreCfgKey())
		if err != nil {
			return nil, mylog.Errorf("getRackEncourageScoreCfg GetState(global cfg) failed.rackid=%s err=%s", rackid, err)
		}
		if sepcB == nil {
			return nil, mylog.Errorf("getRackEncourageScoreCfg GetState(global cfg) nil.rackid=%s", rackid)
		}
	}

	if psepc != nil {
		err = json.Unmarshal(sepcB, psepc)
		if err != nil {
			return nil, mylog.Errorf("getRackEncourageScoreCfg Unmarshal failed.rackid=%s err=%s", rackid, err)
		}
	}

	return sepcB, nil
}

func (t *KD) allocEncourageScoreForSales(stub shim.ChaincodeStubInterface, paraStr string, transFromAcc, transType, transDesc string, invokeTime int64, sameEntSaveTx bool) ([]byte, error) {
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
			mylog.Errorf("encourageScoreBySales  rackRolesSales parse error, '%s' has no '%s'.", rackRolesSales, eleDelim)
			errList = append(errList, rackRolesSales)
			continue
		}
		var eles = strings.Split(rackRolesSales, eleDelim)
		if len(eles) != 6 {
			mylog.Errorf("encourageScoreBySales  rackRolesSales parse error, '%s' format error 1.", rackRolesSales)
			errList = append(errList, rackRolesSales)
			continue
		}

		var rrs RackRolesSales

		rrs.Rackid = eles[0]
		rrs.Sales, err = strconv.ParseInt(eles[1], 0, 64)
		if err != nil {
			mylog.Errorf("encourageScoreBySales  rackRolesSales parse error, '%s' format error 2.", rackRolesSales)
			errList = append(errList, rrs.Rackid)
			continue
		}

		if rrs.Sales <= 0 {
			mylog.Info("encourageScoreBySales sales is 0(rack=%s), do nothing.", rrs.Rackid)
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
		encourageScore, err := t.getRackEncourgeScoreBySales(stub, rrs.Rackid, rrs.Sales)
		if err != nil {
			mylog.Errorf("encourageScoreBySales  getRackEncourgePercentBySales failed, error=%s.", err)
			errList = append(errList, rrs.Rackid)
			continue
		}

		var rres RackRolesEncourageScores
		rres.Rackid = rrs.Rackid
		rres.Scores = encourageScore
		rres.AllocAccs = rrs.AllocAccs

		//销售奖励积分时，货架经营者要补偿销售额同等的积分
		err = t.allocEncourageScore(stub, &rres, transFromAcc, transType, transDesc, invokeTime, sameEntSaveTx, rrs.Sales)
		if err != nil {
			mylog.Errorf("encourageScoreBySales allocEncourageScore failed, error=%s.", err)
			errList = append(errList, rrs.Rackid)
			continue
		}
	}

	if len(errList) > 0 {
		return nil, mylog.Errorf("encourageScoreBySales: has some err,[%s].", strings.Join(errList, ";"))
	}

	return nil, nil
}

func (t *KD) getRackEncourgeScoreBySales(stub shim.ChaincodeStubInterface, rackid string, sales int64) (int64, error) {
	var err error
	var sepc ScoreEncouragePercentCfg
	_, err = t.getRackEncourageScoreCfg(stub, rackid, &sepc)
	if err != nil {
		return 0, mylog.Errorf("getRackEncourgePercent getRackEncourageScoreCfg failed.rackid=%s err=%s", rackid, err)
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

func (t *KD) allocEncourageScore(stub shim.ChaincodeStubInterface, rrs *RackRolesEncourageScores, transFromAcc, transType, transDesc string, invokeTime int64, sameEntSaveTx bool, sellerComps int64) error {
	var ear EarningAllocRate
	_, err := t.getRackAllocCfg(stub, rrs.Rackid, &ear)
	if err != nil {
		return mylog.Errorf("allocEncourageScore getRackAllocCfg failed,Rackid=%s,  error=%s.", rrs.Rackid, err)
	}

	var hasErr = false
	var failedAccList []string

	rolesAllocScore := t.getRackRolesAllocAmt(&ear, rrs.Scores)

	_, err = t.transferCoin(stub, transFromAcc, rrs.SellerAcc, transType, transDesc,
		rolesAllocScore.SellerAmount+sellerComps, invokeTime, sameEntSaveTx)
	if err != nil {
		mylog.Errorf("allocEncourageScore: transferCoin(SellerAcc=%s) failed, error=%s.", rrs.SellerAcc, err)
		hasErr = true
		failedAccList = append(failedAccList, rrs.SellerAcc)
	}

	_, err = t.transferCoin(stub, transFromAcc, rrs.FielderAcc, transType, transDesc,
		rolesAllocScore.FielderAmount, invokeTime, sameEntSaveTx)
	if err != nil {
		mylog.Errorf("allocEncourageScore: transferCoin(FielderAcc=%s) failed, error=%s.", rrs.FielderAcc, err)
		hasErr = true
		failedAccList = append(failedAccList, rrs.FielderAcc)
	}

	_, err = t.transferCoin(stub, transFromAcc, rrs.DeliveryAcc, transType, transDesc,
		rolesAllocScore.DeliveryAmount, invokeTime, sameEntSaveTx)
	if err != nil {
		mylog.Errorf("allocEncourageScore: transferCoin(DeliveryAcc=%s) failed, error=%s.", rrs.DeliveryAcc, err)
		hasErr = true
		failedAccList = append(failedAccList, rrs.DeliveryAcc)
	}

	_, err = t.transferCoin(stub, transFromAcc, rrs.PlatformAcc, transType, transDesc,
		rolesAllocScore.PlatformAmount, invokeTime, sameEntSaveTx)
	if err != nil {
		mylog.Errorf("allocEncourageScore: transferCoin(PlatformAcc=%s) failed, error=%s.", rrs.PlatformAcc, err)
		hasErr = true
		failedAccList = append(failedAccList, rrs.PlatformAcc)
	}

	if hasErr {
		return mylog.Errorf("allocEncourageScore: transferCoin faied, acc=%s", strings.Join(failedAccList, ";"))
	}

	return nil
}

func (t *KD) allocEncourageScoreForNewRack(stub shim.ChaincodeStubInterface, paraStr string, transFromAcc, transType, transDesc string, invokeTime int64, sameEntSaveTx bool) ([]byte, error) {
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
			mylog.Errorf("allocEncourageScoreForNewRack  rackRolesSales parse error, '%s' has no '%s'.", rackRolesScore, eleDelim)
			errList = append(errList, rackRolesScore)
			continue
		}
		var eles = strings.Split(rackRolesScore, eleDelim)
		//至少包含货架id，四个角色
		if len(eles) < 5 {
			mylog.Errorf("allocEncourageScoreForNewRack  rackRolesSales parse error, '%s' format error 1.", rackRolesScore)
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
				mylog.Errorf("allocEncourageScoreForNewRack  rackRolesSales parse error, '%s' format error 2.", rackRolesScore)
				errList = append(errList, rres.Rackid)
				continue
			}
		} else {
			rres.Scores = RACK_NEWRACK_ENC_SCORE_DEFAULT
		}

		rresList = append(rresList, rres)
	}

	for _, rres := range rresList {
		err = t.allocEncourageScore(stub, &rres, transFromAcc, transType, transDesc, invokeTime, sameEntSaveTx, 0)
		if err != nil {
			mylog.Errorf("allocEncourageScoreForNewRack allocEncourageScore failed, error=%s.", err)
			errList = append(errList, rres.Rackid)
			continue
		}
	}

	if len(errList) > 0 {
		return nil, mylog.Errorf("allocEncourageScoreForNewRack: some err,[%s].", strings.Join(errList, ";"))
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

func (t *KD) getRackFinancCfg(stub shim.ChaincodeStubInterface, rackid string, prfc *RackFinanceCfg) ([]byte, error) {

	var rfcB []byte = nil
	var err error

	if rackid != "*" { // "*"表示查询全局配置
		rfcB, err = stub.GetState(t.getRackFinancCfgKey(rackid))
		if err != nil {
			return nil, mylog.Errorf("getRackFinancCfg GetState failed.rackid=%s err=%s", rackid, err)
		}
	}

	if rfcB == nil {
		mylog.Warn("getRackFinancCfg: can not find cfg for %s, will use golobal.", rackid)
		rfcB, err = stub.GetState(t.getGlobalRackFinancCfgKey())
		if err != nil {
			return nil, mylog.Errorf("getRackFinancCfg GetState(global cfg) failed.rackid=%s err=%s", rackid, err)
		}
		if rfcB == nil {
			return nil, mylog.Errorf("getRackFinancCfg GetState(global cfg) nil.rackid=%s", rackid)
		}
	}

	if prfc != nil {
		err = json.Unmarshal(rfcB, prfc)
		if err != nil {
			return nil, mylog.Errorf("getRackFinancCfg Unmarshal failed.rackid=%s err=%s", rackid, err)
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
func (t *KD) userBuyFinance(stub shim.ChaincodeStubInterface, accName, rackid, fid, payee, transType, desc string, amount, invokeTime int64, sameEntSaveTx, isRenewal bool) ([]byte, error) {
	var fiacInfoKey = t.getFinacInfoKey(fid)
	fiB, err := stub.GetState(fiacInfoKey)
	if err != nil {
		return nil, mylog.Errorf("userBuyFinance:  GetState(%s) failed. err=%s.", fiacInfoKey, err)
	}
	var fi FinancialInfo
	if fiB == nil {
		fi.FID = fid
		fi.Time = invokeTime
	} else {
		err = json.Unmarshal(fiB, &fi)
		if err != nil {
			return nil, mylog.Errorf("userBuyFinance:  Unmarshal(fib) failed. err=%s.", err)
		}
		//一般不会出现此情况
		if fi.FID != fid {
			return nil, mylog.Errorf("userBuyFinance:  fid missmatch(%s).", fi.FID)
		}
	}

	var rackInfoKey = t.getRackInfoKey(rackid)
	riB, err := stub.GetState(rackInfoKey)
	if err != nil {
		return nil, mylog.Errorf("userBuyFinance:  GetState(%s) failed. err=%s.", rackInfoKey, err)
	}
	var ri RackInfo
	if riB == nil {
		ri.RackID = rackid
		ri.Time = invokeTime
	} else {
		err = json.Unmarshal(riB, &ri)
		if err != nil {
			return nil, mylog.Errorf("userBuyFinance:  Unmarshal(riB) failed. err=%s.", err)
		}
		//一般不会出现此情况
		if ri.RackID != rackid {
			return nil, mylog.Errorf("userBuyFinance:  rackid missmatch(%s).", ri.RackID)
		}
	}

	//写入货架融资信息
	rackFinacInfoKey := t.getRackFinacInfoKey(rackid, fid)
	rfiB, err := stub.GetState(rackFinacInfoKey)
	if err != nil {
		return nil, mylog.Errorf("userBuyFinance:  GetState(%s) failed. err=%s.", rackFinacInfoKey, err)
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
		_, err = t.getRackFinancCfg(stub, rackid, &rfc)
		if err != nil {
			return nil, mylog.Errorf("userBuyFinance:  getRackFinancCfg failed. err=%s.", err)
		}

		var ear EarningAllocRate
		_, err = t.getRackAllocCfg(stub, rackid, &ear)
		if err != nil {
			return nil, mylog.Errorf("userBuyFinance:  getRackAllocCfg failed. err=%s.", err)
		}

		rfi.RFCfg = rfc.PubRackFinanceCfg
		rfi.RolesAllocRate = ear.RolesRate
	} else {
		err = json.Unmarshal(rfiB, &rfi)
		if err != nil {
			return nil, mylog.Errorf("userBuyFinance:  Unmarshal RackFinancInfo failed. err=%s.", err)
		}

		//如果不是续期，且理财发行完毕，不能购买
		if !isRenewal && rfi.Stage >= FINANC_STAGE_ISSUE_FINISH {
			return nil, mylog.Errorf("userBuyFinance:  finance finish, user can't buy.")
		}

		rfi.AmountFinca += amount
		_, ok := rfi.UserAmountMap[accName]
		if ok {
			//如果用户已提取了，又来买，那么从新记录投资额，不能累计，否则会把前一次的累计进来。
			if t.strSliceContains(rfi.PayFinanceUserList, accName) {
				rfi.AmountFinca -= rfi.UserAmountMap[accName] //实际投资额度要减去上一次的
				rfi.UserAmountMap[accName] = amount
				if isRenewal {
					rfi.UserRenewalMap[accName] = amount
				}
				rfi.PayFinanceUserList = t.strSliceDelete(rfi.PayFinanceUserList, accName)
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
	_, err = t.getRackFinancCfg(stub, rackid, &rfc)
	if err != nil {
		return nil, mylog.Errorf("userBuyFinance:  getRackFinancCfg failed. err=%s.", err)
	}

	//看该货架是否有历史投资，如果有的话，这些投资会自动转到当前融资，就会导致超额。
	var historyFinance int64 = 0
	if !isRenewal { //自动续期时，不需要计算历史投资，因为续期的金额就是历史投资额
		//调用购买理财的接口时，已经将最新的理财期号设置了（调用setCurrentFid），所以这里取前一期的期号
		pfid, err := t.getPreviousFid(stub)
		if err != nil {
			return nil, mylog.Errorf("userBuyFinance: getPreviousFid failed. err=%s.", err)
		}

		mylog.Debug("userBuyFinance: pfid=%s", pfid)

		//有前一期的fid时才计算。如果没有说明没有历史投资
		if len(pfid) > 0 {
			historyFinance, err = t.getRackFinanceAmount(stub, rackid, pfid)
			if err != nil {
				return nil, mylog.Errorf("userBuyFinance: getRackHistoryFinance failed. err=%s.", err)
			}
		}
	}

	//融资额度超出货架支持能力
	if rfi.AmountFinca+historyFinance > rfc.InvestCapacity {
		return nil, mylog.Errorf("userBuyFinance:  AmountFinca > rack's capacity. (%d,%d,%d)", rfi.AmountFinca, historyFinance, rfc.InvestCapacity)
	}

	//用户给融资方转账
	if !isRenewal {
		_, err = t.transferCoin(stub, accName, payee, transType, desc, amount, invokeTime, sameEntSaveTx)
		if err != nil {
			return nil, mylog.Errorf("userBuyFinance: transferCoin failed. err=%s.", err)
		}
	}

	//转账成功后在用户entity中写入相应信息
	accEnt, err := t.getAccountEntity(stub, accName)
	if err != nil {
		return nil, mylog.Errorf("userBuyFinance: getAccountEntity failed. err=%s.", err)
	}
	if accEnt.RFInfoMap == nil {
		accEnt.RFInfoMap = make(map[string]int)
	}
	accEnt.RFInfoMap[t.getMapKey4RackFinance(rackid, fid)] = 0
	accEnt.LatestFid = fid

	err = t.setAccountEntity(stub, accEnt)
	if err != nil {
		return nil, mylog.Errorf("userBuyFinance: setAccountEntity failed. err=%s.", err)
	}

	mylog.Debug("userBuyFinance: ent=%+v", *accEnt)

	if !t.strSliceContains(fi.RackList, ri.RackID) {
		fi.RackList = append(fi.RackList, ri.RackID)
	}
	fiJson, err := json.Marshal(fi)
	if err != nil {
		return nil, mylog.Errorf("userBuyFinance:  Marshal failed. err=%s.", err)
	}

	if !t.strSliceContains(ri.FinacList, fi.FID) {
		ri.FinacList = append(ri.FinacList, fi.FID)
	}

	riJson, err := json.Marshal(ri)
	if err != nil {
		return nil, mylog.Errorf("userBuyFinance:  Marshal failed. err=%s.", err)
	}
	rfiJson, err := json.Marshal(rfi)
	if err != nil {
		return nil, mylog.Errorf("userBuyFinance:  Marshal failed. err=%s.", err)
	}

	err = t.putState_Ex(stub, rackFinacInfoKey, rfiJson)
	if err != nil {
		return nil, mylog.Errorf("userBuyFinance:  PutState failed. err=%s.", err)
	}

	err = t.putState_Ex(stub, rackInfoKey, riJson)
	if err != nil {
		return nil, mylog.Errorf("userBuyFinance:  PutState failed. err=%s.", err)
	}

	err = t.putState_Ex(stub, fiacInfoKey, fiJson)
	if err != nil {
		return nil, mylog.Errorf("userBuyFinance:  PutState failed. err=%s.", err)
	}

	mylog.Debug("userBuyFinance: ri=%+v fi=%+v rfi=%+v", ri, fi, rfi)
	mylog.Debug("userBuyFinance: rfiJson=%s", string(rfiJson))

	return nil, nil
}

func (t *KD) financeBonus(stub shim.ChaincodeStubInterface, fid, rackales string, invokeTime int64) ([]byte, error) {
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
			mylog.Errorf("financeBonus: rackSales parse error, '%s' has no '%s'.", rackSales, eleDelim)
			errRackList = append(errRackList, rackSales)
			continue
		}
		var eles = strings.Split(rackSales, eleDelim)
		if len(eles) < 2 {
			mylog.Errorf("financeBonus: rackSales parse error, '%s' format error 1.", rackSales)
			errRackList = append(errRackList, rackSales)
			continue
		}

		var rackid = eles[0]
		var sales int64
		sales, err = strconv.ParseInt(eles[1], 0, 64)
		if err != nil {
			mylog.Errorf("financeBonus: sales parse error, '%s' format error 2.", rackSales)
			errRackList = append(errRackList, rackid)
			continue
		}

		err = t.financeBonus4OneRack(stub, rackid, fid, sales, invokeTime)
		if err != nil {
			mylog.Errorf("financeBonus: financeBonus4OneRack failed, err=%s", err)
			errRackList = append(errRackList, rackid)
			continue
		}
	}

	if len(errRackList) > 0 {
		return nil, mylog.Errorf("financeBonus: has some err,[%s]", strings.Join(errRackList, ";"))
	}

	return nil, nil
}

func (t *KD) financeBonus4OneRack(stub shim.ChaincodeStubInterface, rackid, fid string, sales, invokeTime int64) error {
	var rackFinacInfoKey = t.getRackFinacInfoKey(rackid, fid)

	rfiB, err := stub.GetState(rackFinacInfoKey)
	if err != nil {
		return mylog.Errorf("financeBonus4OneRack:  GetState(%s) failed. err=%s.", rackFinacInfoKey, err)
	}
	if rfiB == nil {
		return mylog.Errorf("financeBonus4OneRack:  rackFinacInfo not exists(%s,%s).", rackid, fid)
	}
	var rfi RackFinancInfo
	err = json.Unmarshal(rfiB, &rfi)
	if err != nil {
		return mylog.Errorf("financeBonus4OneRack:  Unmarshal failed. err=%s.", err)
	}

	//已分红过不能再分红
	if rfi.Stage >= FINANC_STAGE_BONUS_FINISH {
		return mylog.Errorf("financeBonus4OneRack: rack(rid=%s fid=%s) has bonus, something wrong?", rackid, fid)
	}

	rfi.CEInfo.WareSales = sales

	//货架利润
	var rackProfit = rfi.CEInfo.WareSales * int64(rfi.RFCfg.ProfitsPercent) / 100
	//经营者获取的利润
	var sellerProfit = rackProfit * rfi.RolesAllocRate.SellerRate / (rfi.RolesAllocRate.SellerRate + rfi.RolesAllocRate.FielderRate + rfi.RolesAllocRate.DeliveryRate + rfi.RolesAllocRate.PlatformRate)
	//分给投资者的利润
	var profit = sellerProfit * int64(rfi.RFCfg.InvestProfitsPercent) / 100

	profit = profit / 100 //利润的单位为分，一块钱兑一积分

	mylog.Debug("financeBonus4OneRack: rfi.RFCfg=%+v, rfi.RolesAllocRate=%+v", rfi.RFCfg, rfi.RolesAllocRate)

	var amtCheck int64 = 0
	var profitCheck int64 = 0
	var accProfit int64
	if rfi.UserProfitMap == nil {
		rfi.UserProfitMap = make(map[string]int64)
	}

	var cost = rfi.CEInfo.WareSales * int64(100-rfi.RFCfg.ProfitsPercent) / 100 //成本

	mylog.Debug("financeBonus4OneRack:rackProfit=%d, sellerProfit=%d, profit=%d, cost=%d", rackProfit, sellerProfit, profit, cost)

	for acc, amt := range rfi.UserAmountMap {
		amtCheck += amt
		//accProfit = amt * profit / rfi.AmountFinca
		//accProfit = amt * profit / (cost / 100) //分母不使用投资总额，使用当期成本, cost的单位为分，所以要再除以100
		accProfit = amt * profit / rfi.RFCfg.InvestCapacity
		rfi.UserProfitMap[acc] = accProfit
		profitCheck += accProfit
	}
	if profitCheck > profit || amtCheck != rfi.AmountFinca {
		return mylog.Errorf("financeBonus4OneRack:  bonus check(%d,%d,%d,%d) failed.", profitCheck, profit, amtCheck, rfi.AmountFinca)
	}

	rfi.Stage = FINANC_STAGE_BONUS_FINISH

	rfiJson, err := json.Marshal(rfi)
	if err != nil {
		return mylog.Errorf("financeBonus4OneRack:  Marshal failed. err=%s.", err)
	}

	err = t.putState_Ex(stub, rackFinacInfoKey, rfiJson)
	if err != nil {
		return mylog.Errorf("financeBonus4OneRack:  PutState failed. err=%s.", err)
	}

	mylog.Info("financeBonus4OneRack: statistic(%v,%v,%v,%v), rfi=%+v", rfi.CEInfo.WareSales, rackProfit, sellerProfit, profit, rfi)

	return nil
}

var currentFidCache string

func (t *KD) setCurrentFid(stub shim.ChaincodeStubInterface, currentFid string) error {
	//因为会调用多次，所以用cache加速一下
	if len(currentFidCache) > 0 && currentFidCache == currentFid {
		return nil
	}

	hisB, err := stub.GetState(RACKFINACHISTORY_KEY)
	if err != nil {
		return mylog.Errorf("setCurrentFid: GetState failed. err=%s.", err)
	}
	var his RackFinancHistory
	if hisB == nil {
		his.PreCurrFID[1] = currentFid
		currentFidCache = currentFid
	} else {
		err = json.Unmarshal(hisB, &his)
		if err != nil {
			return mylog.Errorf("setCurrentFid: Unmarshal failed. err=%s.", err)
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
		return mylog.Errorf("setCurrentFid: Marshal failed. err=%s.", err)
	}

	err = t.putState_Ex(stub, RACKFINACHISTORY_KEY, hisB)
	if err != nil {
		return mylog.Errorf("setCurrentFid: PutState_Ex failed. err=%s.", err)
	}

	mylog.Debug("setCurrentFid: his=%+v", his)

	return nil
}

func (t *KD) getPrevAndCurrFids(stub shim.ChaincodeStubInterface) (*RackFinancHistory, error) {
	hisB, err := stub.GetState(RACKFINACHISTORY_KEY)
	if err != nil {
		return nil, mylog.Errorf("getPrevAndCurrFids: GetState failed. err=%s.", err)
	}
	if hisB == nil {
		//return "", mylog.Errorf("getPrevAndCurrFids: nil info.")
		return nil, nil //如果第一次执行，这个可能为空
	}

	var his RackFinancHistory
	err = json.Unmarshal(hisB, &his)
	if err != nil {
		return nil, mylog.Errorf("getPrevAndCurrFids: Unmarshal failed. err=%s.", err)
	}

	return &his, nil
}

func (t *KD) getRecentlyFid(stub shim.ChaincodeStubInterface, getCurrent bool) (string, error) {
	his, err := t.getPrevAndCurrFids(stub)
	if err != nil {
		return "", mylog.Errorf("getRecentlyFid: getPrevAndCurrFids failed. err=%s.", err)
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
func (t *KD) getPreviousFid(stub shim.ChaincodeStubInterface) (string, error) {
	return t.getRecentlyFid(stub, false)
}
func (t *KD) getLatestFid(stub shim.ChaincodeStubInterface) (string, error) {
	return t.getRecentlyFid(stub, true)
}

func (t *KD) getUserInvestAmount(stub shim.ChaincodeStubInterface, accName, rackid, fid string) (int64, error) {
	/*
	   fid, err := t.getLatestFid(stub)
	   if err != nil {
	       return 0, mylog.Errorf("getUserHistoryFinance: getLatestFid failed. err=%s.", err)
	   }
	*/

	ent, err := t.getAccountEntity(stub, accName)
	if err != nil {
		return 0, mylog.Errorf("getUserHistoryFinance: getAccountEntity failed. err=%s.", err)
	}

	var rfkey = t.getMapKey4RackFinance(rackid, fid)

	if ent.RFInfoMap == nil {
		mylog.Debug("getUserHistoryFinance: pair(%+v) not exists in %s's acc.", rfkey, accName)
		return 0, nil
	}

	if _, ok := ent.RFInfoMap[rfkey]; !ok {
		mylog.Debug("getUserHistoryFinance: pair(%+v) not exists in %s's acc.", rfkey, accName)
		return 0, nil
	}

	rfiB, err := stub.GetState(t.getRackFinacInfoKey(rackid, fid))
	if err != nil {
		return 0, mylog.Errorf("getUserHistoryFinance:  GetState failed. err=%s.", err)
	}
	//ent中记录了该条记录，肯定是有的，没有则报错
	if rfiB == nil {
		return 0, mylog.Errorf("getUserHistoryFinance:  FinancialInfo not exists.")
	}
	var rfi RackFinancInfo
	err = json.Unmarshal(rfiB, &rfi)
	if err != nil {
		return 0, mylog.Errorf("getUserHistoryFinance:  Unmarshal failed. err=%s.", err)
	}
	//投资记录没有该账户，报错
	if _, ok := rfi.UserAmountMap[accName]; !ok {
		return 0, mylog.Errorf("getUserHistoryFinance: acc not exists in UserAmountMap.")
	}

	mylog.Debug("getUserHistoryFinance: rfi=%+v", rfi)

	return rfi.UserAmountMap[accName], nil
}

func (t *KD) getRackFinanceAmount(stub shim.ChaincodeStubInterface, rackid, fid string) (int64, error) {
	/*
	   fid, err := t.getLatestFid(stub)
	   if err != nil {
	       return 0, mylog.Errorf("getRackHistoryFinance: getLatestFid failed. err=%s.", err)
	   }
	*/

	rfiB, err := stub.GetState(t.getRackFinacInfoKey(rackid, fid))
	if err != nil {
		return 0, mylog.Errorf("getRackHistoryFinance:  GetState failed. err=%s.", err)
	}
	if rfiB == nil {
		mylog.Debug("getRackHistoryFinance: rfiB is nil.")
		return 0, nil
	}
	var rfi RackFinancInfo
	err = json.Unmarshal(rfiB, &rfi)
	if err != nil {
		return 0, mylog.Errorf("getRackHistoryFinance:  Unmarshal failed. err=%s.", err)
	}
	var totalAmt int64 = 0
	for acc, amt := range rfi.UserAmountMap {
		if !t.strSliceContains(rfi.PayFinanceUserList, acc) {
			totalAmt += amt
		}
	}

	return totalAmt, nil
}

func (t *KD) financeIssueFinishAfter(stub shim.ChaincodeStubInterface, currentFid string, invokeTime int64) error {
	//看是否已经处理过
	finishIdB, err := stub.GetState(RACKFINACISSUEFINISHID_KEY)
	if err != nil {
		return mylog.Errorf("financeIssueFinishAfter: GetState(finishId) failed. err=%s.", err)
	}
	if finishIdB == nil {
		err = t.putState_Ex(stub, RACKFINACISSUEFINISHID_KEY, []byte(currentFid))
		if err != nil {
			return mylog.Errorf("financeIssueFinishAfter: PutState_Ex(finishId) failed. err=%s.", err)
		}
	} else {
		var finishId = string(finishIdB)

		if finishId == currentFid {
			return mylog.Errorf("financeIssueFinishAfter: has finished already.")
		}
	}

	//给本期理财设置为"发行完毕"
	fiB, err := stub.GetState(t.getFinacInfoKey(currentFid))
	if err != nil {
		return mylog.Errorf("financeIssueFinishAfter: GetState(fi=%s) failed. err=%s.", currentFid, err)
	}

	if fiB != nil {
		var fi FinancialInfo
		err = json.Unmarshal(fiB, &fi)
		if err != nil {
			return mylog.Errorf("financeIssueFinishAfter: Unmarshal failed. err=%s.", err)
		}

		for _, rackid := range fi.RackList {
			var rfiKey = t.getRackFinacInfoKey(rackid, currentFid)
			rfiB, err := stub.GetState(rfiKey)
			if err != nil {
				return mylog.Errorf("financeIssueFinishAfter: GetState(rfi=%s,%s) failed. err=%s.", rackid, currentFid, err)
			}
			if rfiB == nil {
				continue
			}

			var rfi RackFinancInfo
			err = json.Unmarshal(rfiB, &rfi)
			if err != nil {
				return mylog.Errorf("financeIssueFinishAfter: Unmarshal(rfi=%s,%s) failed. err=%s.", rackid, currentFid, err)
			}

			mylog.Debug("financeIssueFinishAfter: rfi=%+v", rfi)

			if rfi.Stage >= FINANC_STAGE_ISSUE_FINISH {
				return mylog.Errorf("financeIssueFinishAfter: (%s,%s) has finished already, something wrong?", rackid, currentFid)
			}

			rfi.Stage = FINANC_STAGE_ISSUE_FINISH

			rfiB, err = json.Marshal(rfi)
			if err != nil {
				return mylog.Errorf("financeIssueFinishAfter: Marshal(rfi=%s,%s) failed. err=%s.", rackid, currentFid, err)
			}

			err = t.putState_Ex(stub, rfiKey, rfiB)
			if err != nil {
				return mylog.Errorf("financeIssueFinishAfter: PutState_Ex(rfi=%s,%s) failed. err=%s.", rackid, currentFid, err)
			}
		}
	}

	//为上一期理财续期
	return t.financeRenewalPreviousFinance(stub, currentFid, invokeTime)
}

func (t *KD) financeRenewalPreviousFinance(stub shim.ChaincodeStubInterface, currentFid string, invokeTime int64) error {
	//看上期的理财中，哪些没有提取的自动续期
	//调用理财续期的接口时，已经将最新的理财期号设置了（调用setCurrentFid），所以这里取前一期的期号
	preFid, err := t.getPreviousFid(stub)
	if err != nil {
		return mylog.Errorf("financeRenewal: getPreviousFid failed. err=%s.", err)
	}

	mylog.Debug("financeRenewal: preFid=%s", preFid)

	//没有上期理财，说明是第一次，退出
	if len(preFid) == 0 {
		mylog.Debug("financeRenewal: no preFid.")
		return nil
	}

	if preFid == currentFid {
		return mylog.Errorf("financeRenewal: preFid == currentFid, error.")
	}

	fiB, err := stub.GetState(t.getFinacInfoKey(preFid))
	if err != nil {
		return mylog.Errorf("financeRenewal: GetState(fi=%s) failed. err=%s.", preFid, err)
	}

	//上一期没人买过理财
	if fiB == nil {
		mylog.Debug("financeRenewal: no fiB.")
		return nil
	}

	var fi FinancialInfo
	err = json.Unmarshal(fiB, &fi)
	if err != nil {
		return mylog.Errorf("financeRenewal: Unmarshal failed. err=%s.", err)
	}

	for _, rackid := range fi.RackList {
		var rfiKey = t.getRackFinacInfoKey(rackid, preFid)
		rfiB, err := stub.GetState(rfiKey)
		if err != nil {
			return mylog.Errorf("financeRenewal: GetState(rfi=%s,%s) failed. err=%s.", rackid, preFid, err)
		}
		if rfiB == nil {
			continue
		}

		var rfi RackFinancInfo
		err = json.Unmarshal(rfiB, &rfi)
		if err != nil {
			return mylog.Errorf("financeRenewal: Unmarshal(rfi=%s,%s) failed. err=%s.", rackid, preFid, err)
		}

		mylog.Debug("financeRenewal: rfi=%+v", rfi)

		for acc, amt := range rfi.UserAmountMap {
			//已赎回的用户不在续期
			if t.strSliceContains(rfi.PayFinanceUserList, acc) {
				continue
			}

			//使用info日志，后台可查
			mylog.Info("financeRenewal: renewal for %s,%s", rackid, currentFid)

			//续期，即内部给这些用户买新一期的理财
			_, err = t.userBuyFinance(stub, acc, rackid, currentFid, "", "", "", amt, invokeTime, true, true)
			if err != nil {
				return mylog.Errorf("financeRenewal: userBuyFinance(rfi=%s,%s,%s) failed. err=%s.", rackid, preFid, acc, err)
			}
		}
	}

	return nil
}

func (t *KD) payUserFinance(stub shim.ChaincodeStubInterface, accName, reacc, rackid string, invokeTime int64, transType, desc string, sameEntSaveTx bool) error {
	reaccEnt, err := t.getAccountEntity(stub, reacc)
	if err != nil {
		return mylog.Errorf("payUserFinance: getAccountEntity(acc=%s) failed. err=%s.", reacc, err)
	}
	mylog.Debug("payUserFinance: before reaccEnt = %+v", reaccEnt)

	if reaccEnt.RFInfoMap == nil || len(reaccEnt.RFInfoMap) == 0 {
		mylog.Debug("payUserFinance: RFInfoMap empty.")
		return nil
	}

	//获取用户投资的本金  最近一期投资的额度为本金，因为投资会自动续期
	var investAmt int64 = 0
	investAmt, err = t.getUserInvestAmount(stub, reacc, rackid, reaccEnt.LatestFid)
	if err != nil {
		return mylog.Errorf("payUserFinance: getUserInvestAmount failed. err=%s.", err)
	}

	mylog.Debug("payUserFinance: acc=%s investAmt=%d (%s,%s)", reacc, investAmt, rackid, reaccEnt.LatestFid)

	var profit int64 = 0
	var delKeyList []string
	var paidFidList []string

	for rfkey, _ := range reaccEnt.RFInfoMap {
		r, f := t.getRackFinanceFromMapKey(rfkey)
		if r != rackid {
			continue
		}

		var rfiKey = t.getRackFinacInfoKey(rackid, f)
		rfiB, err := stub.GetState(rfiKey)
		if err != nil {
			return mylog.Errorf("payUserFinance:  GetState(%s,%s) failed. err=%s.", rackid, f, err)
		}
		//ent中记录了该条记录，肯定是有的，没有则报错
		if rfiB == nil {
			return mylog.Errorf("payUserFinance:  FinancialInfo(%s,%s) not exists.", rackid, f)
		}
		var rfi RackFinancInfo
		err = json.Unmarshal(rfiB, &rfi)
		if err != nil {
			return mylog.Errorf("payUserFinance:  Unmarshal(%s,%s) failed. err=%s.", rackid, f, err)
		}

		//如果已提取过，则不能再提取。这里不报错，不实际执行转账即可
		if t.strSliceContains(rfi.PayFinanceUserList, reacc) {
			mylog.Warn("payUserFinance: %s has paid already, do nothing.", reacc)
			continue
		}

		if rfi.UserProfitMap != nil {
			profit += rfi.UserProfitMap[reacc]
		}

		rfi.PayFinanceUserList = append(rfi.PayFinanceUserList, reacc)
		rfiB, err = json.Marshal(rfi)
		if err != nil {
			return mylog.Errorf("payUserFinance:  Marshal(%s,%s) failed. err=%s.", rackid, f, err)
		}

		err = t.putState_Ex(stub, rfiKey, rfiB)
		if err != nil {
			return mylog.Errorf("payUserFinance:  PutState_Ex(%s,%s) failed. err=%s.", rackid, f, err)
		}

		mylog.Debug("payUserFinance: acc=%s rfi=%+v", reacc, rfi)

		//delete(reaccEnt.RFInfoMap, rfkey)
		delKeyList = append(delKeyList, rfkey)
		paidFidList = append(paidFidList, f)
	}

	var totalAmt = investAmt + profit

	mylog.Debug("payUserFinance: %s will pay %d to %s.", accName, totalAmt, reacc)

	_, err = t.transferCoin(stub, accName, reacc, transType, desc, totalAmt, invokeTime, sameEntSaveTx)
	if err != nil {
		return mylog.Errorf("payUserFinance:  transferCoin(%s) failed. err=%s.", reacc, err)
	}

	//上面transferCoin转账之后， reacc会发生变化，所以必须再取一次entity
	reaccEnt, err = t.getAccountEntity(stub, reacc)
	if err != nil {
		return mylog.Errorf("payUserFinance: getAccountEntity2(acc=%s) failed. err=%s.", reacc, err)
	}

	//将赎回的理财期号写入已赎回列表
	for _, fid := range paidFidList {
		if !t.strSliceContains(reaccEnt.PaidFidList, fid) {
			reaccEnt.PaidFidList = append(reaccEnt.PaidFidList, fid)
		}
	}

	//删除已购买的货架理财信息
	for _, rfkey := range delKeyList {
		delete(reaccEnt.RFInfoMap, rfkey)
	}

	err = t.setAccountEntity(stub, reaccEnt)
	if err != nil {
		return mylog.Errorf("payUserFinance:  setAccountEntity(%s) failed. err=%s.", reacc, err)
	}

	mylog.Debug("payUserFinance: after reaccEnt = %+v", *reaccEnt)
	mylog.Info("payUserFinance: %s pay %v,%v for %s,  rf=%+v", accName, investAmt, profit, reacc, reaccEnt.AccEnt_Ext_RackFinance)

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

func (t *KD) getUserFinanceProfit(stub shim.ChaincodeStubInterface, accName, rackid string) (int64, error) {
	accEnt, err := t.getAccountEntity(stub, accName)
	if err != nil {
		return 0, mylog.Errorf("getUserFinanceProfit: getAccountEntity(acc=%s) failed. err=%s.", accName, err)
	}
	mylog.Debug("getUserFinanceProfit:  accEnt = %+v", accEnt)

	if accEnt.RFInfoMap == nil || len(accEnt.RFInfoMap) == 0 {
		mylog.Debug("getUserFinanceProfit: RFInfoMap empty.")
		return 0, nil
	}

	var profit int64 = 0

	for rfkey, _ := range accEnt.RFInfoMap {
		r, f := t.getRackFinanceFromMapKey(rfkey)
		if r != rackid {
			continue
		}

		var rfiKey = t.getRackFinacInfoKey(rackid, f)
		rfiB, err := stub.GetState(rfiKey)
		if err != nil {
			return profit, mylog.Errorf("getUserFinanceProfit:  GetState(%s,%s) failed. err=%s.", rackid, f, err)
		}
		//ent中记录了该条记录，肯定是有的，没有则报错
		if rfiB == nil {
			return profit, mylog.Errorf("getUserFinanceProfit:  FinancialInfo(%s,%s) not exists.", rackid, f)
		}
		var rfi RackFinancInfo
		err = json.Unmarshal(rfiB, &rfi)
		if err != nil {
			return profit, mylog.Errorf("getUserFinanceProfit:  Unmarshal(%s,%s) failed. err=%s.", rackid, f, err)
		}

		if rfi.UserProfitMap != nil {
			profit += rfi.UserProfitMap[accName]
		}
	}

	return profit, nil
}

func (t *KD) getRestFinanceCapacityForRack(stub shim.ChaincodeStubInterface, rackid, fid string) (int64, error) {
	var rfc RackFinanceCfg
	_, err := t.getRackFinancCfg(stub, rackid, &rfc)
	if err != nil {
		return 0, mylog.Errorf("getRestFinanceCapacityForRack:  getRackFinancCfg(%s) failed. err=%s.", rackid, err)
	}

	//获取前一期的将要续期的金额
	var preAmt int64 = 0

	hisFids, err := t.getPrevAndCurrFids(stub)
	if err != nil {
		return 0, mylog.Errorf("getRestFinanceCapacityForRack: getPrevAndCurrFids failed. err=%s.", err)
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
		mylog.Debug("getRestFinanceCapacityForRack: preFid=[%s]", preFid)

		//前期理财id为空，说明没有前期，不用处理
		if len(preFid) > 0 {
			preAmt, err = t.getRackFinanceAmount(stub, rackid, preFid)
			if err != nil {
				return 0, mylog.Errorf("getRestFinanceCapacityForRack: getRackFinanceAmount failed. err=%s.", err)
			}
			mylog.Debug("getRestFinanceCapacityForRack: preAmt=%v", preAmt)
		}
	}

	//获取当期理财已投资金额
	var currAmt int64 = 0
	currAmt, err = t.getRackFinanceAmount(stub, rackid, fid)
	if err != nil {
		return 0, mylog.Errorf("getRestFinanceCapacityForRack: getRackFinanceAmount failed. err=%s.", err)
	}

	mylog.Debug("getRestFinanceCapacityForRack: InvestCapacity=%v, preAmt=%v, currAmt=%v", rfc.InvestCapacity, preAmt, currAmt)

	var restAmt = rfc.InvestCapacity - preAmt - currAmt
	if restAmt < 0 {
		mylog.Warn("getRestFinanceCapacityForRack: restAmt < 0, something wrong(%d,%d).", rfc.InvestCapacity, preAmt)
		restAmt = 0
	}

	return restAmt, nil
}

/*
//获取某个账户的货架融资信息
func (t *KD) _getAccRackFinanceTx(stub shim.ChaincodeStubInterface, accName, rackid string) ([]byte, error) {
	accEnt, err := t.getAccountEntity(stub, accName)
	if err != nil {
		return nil, mylog.Errorf("payUserFinance: getAccountEntity(acc=%s) failed. err=%s.", accName, err)
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
		rfiB, err := stub.GetState(rfiKey)
		if err != nil {
			return mylog.Errorf("payUserFinance:  GetState(%s,%s) failed. err=%s.", rackid, f, err)
		}
		//ent中记录了该条记录，肯定是有的，没有则报错
		if rfiB == nil {
			return mylog.Errorf("payUserFinance:  FinancialInfo(%s,%s) not exists.", rackid, f)
		}
		var rfi RackFinancInfo
		err = json.Unmarshal(rfiB, &rfi)
		if err != nil {
			return mylog.Errorf("payUserFinance:  Unmarshal(%s,%s) failed. err=%s.", rackid, f, err)
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

func (t *KD) queryAccRackFinanceTx(stub shim.ChaincodeStubInterface, accName string, begIdx, count, begTime, endTime, isAsc bool) ([]byte, error) {
	var err error

	var retTransInfo []byte
	var queryResult QueryRackFinanceTx
	queryResult.NextSerial = -1
	queryResult.FinanceRecords = []RackFinanceRecd{} //初始化为空，即使下面没查到数据也会返回'[]'

	retTransInfo, err = json.Marshal(queryResult)
	if err != nil {
		return nil, mylog.Errorf("queryAccRackFinanceTx Marshal failed.err=%s", err)
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

		return nil, mylog.Errorf("queryAccRackFinanceTx getAccountEntity(%s) failed.err=%s", accName, err)
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
				mylog.Error("queryAccRackFinanceTx getQueryTransInfo(idx=%d) failed.err=%s", loop, err)
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
				mylog.Error("queryAccRackFinanceTx getQueryTransInfo(idx=%d) failed.err=%s", loop, err)
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
		return nil, mylog.Errorf("queryAccRackFinanceTx Marshal failed.err=%s", err)
	}

	return retTransInfo, nil
}
*/

/* ----------------------- 货架融资相关 end ----------------------- */

func (t *KD) setAccountPasswd(stub shim.ChaincodeStubInterface, accName, pwd string, accEnt *AccountEntity) error {
	var ent *AccountEntity
	var err error

	if accEnt == nil {
		tmpEnt, err := t.getAccountEntity(stub, accName)
		if err != nil {
			return mylog.Errorf("setAccountPasswd: getAccountEntity failed.err=%s, acc=%s", err, accName)
		}
		ent = tmpEnt
	} else {
		ent = accEnt
	}

	md5Sum := md5.Sum([]byte(accName + strconv.FormatInt(accEnt.Time, 10)))
	var salt []byte
	for _, b := range md5Sum {
		salt = append(salt, b)
	}

	hash, err := myCrypto.GenCipher(pwd, []byte(salt))
	if err != nil {
		return mylog.Errorf("setAccountPasswd: GenCipher failed.err=%s, acc=%s", err, accName)
	}

	ent.Cipher = hash

	err = t.setAccountEntity(stub, ent)
	if err != nil {
		return mylog.Errorf("setAccountPasswd: setAccountEntity failed.err=%s, acc=%s", err, accName)
	}

	return nil
}
func (t *KD) authAccountPasswd(stub shim.ChaincodeStubInterface, accName, pwd string, accEnt *AccountEntity) (bool, error) {
	var ent *AccountEntity
	var err error

	if accEnt == nil {
		tmpEnt, err := t.getAccountEntity(stub, accName)
		if err != nil {
			return false, mylog.Errorf("AuthAccountPasswd: getAccountEntity failed.err=%s, acc=%s", err, accName)
		}
		ent = tmpEnt
	} else {
		ent = accEnt
	}

	if ent.Cipher == nil || len(ent.Cipher) == 0 {
		return false, mylog.Errorf("AuthAccountPasswd: Cipher is nil, acc=%s", accName)
	}

	ok, err := myCrypto.AuthPass(ent.Cipher, pwd)
	if err != nil {
		return false, mylog.Errorf("AuthAccountPasswd: AuthPass failed.err=%s, acc=%s", err, accName)
	}

	if ok {
		return true, nil
	}

	return false, nil
}

func (t *KD) isSetAccountPasswd(stub shim.ChaincodeStubInterface, accName string, accEnt *AccountEntity) (bool, error) {
	var ent *AccountEntity

	if accEnt == nil {
		tmpEnt, err := t.getAccountEntity(stub, accName)
		if err != nil {
			return false, mylog.Errorf("IsSetAccountPasswd: getAccountEntity failed.err=%s, acc=%s", err, accName)
		}
		ent = tmpEnt
	} else {
		ent = accEnt
	}

	if ent.Cipher == nil || len(ent.Cipher) == 0 {
		return false, nil
	}

	return true, nil
}

func (t *KD) changeAccountPasswd(stub shim.ChaincodeStubInterface, accName, oldpwd, newpwd string, accEnt *AccountEntity) error {
	ok, err := t.authAccountPasswd(stub, accName, oldpwd, accEnt)
	if err != nil {
		return mylog.Errorf("changeAccountPasswd: authAccountPasswd failed.err=%s, acc=%s", err, accName)
	}
	if !ok {
		return mylog.Errorf("changeAccountPasswd: authAccountPasswd not pass.")
	}

	err = t.setAccountPasswd(stub, accName, newpwd, accEnt)
	if err != nil {
		return mylog.Errorf("changeAccountPasswd: setAccountPasswd failed.err=%s, acc=%s", err, accName)
	}

	return nil
}

func (t *KD) decodeAccountPasswd(pwdBase64 string) (string, error) {

	pwdEncrypt, err := base64.StdEncoding.DecodeString(pwdBase64)
	if err != nil {
		return "", mylog.Errorf("decodeAccountPasswd: DecodeString failed. err=%s", err)
	}

	pwdB, err := myCrypto.AESDecrypt(256, []byte(PWD_ENCRYPT_KEY), []byte(PWD_ENCRYPT_IV), pwdEncrypt)
	if err != nil {
		return "", mylog.Errorf("decodeAccountPasswd: AESDecrypt failed. err=%s", err)
	}

	return string(pwdB), nil
}

func (t *KD) dumpWorldState(stub shim.ChaincodeStubInterface, queryTime int64, flushLimit int, needHash bool, currCcid string) ([]byte, error) {
	//queryTime单位是毫秒
	var timestamp = time.Unix(queryTime/1000, (queryTime-(queryTime/1000*1000))*1000000)
	var outFile = "/home/kd_worldstate_" + timestamp.Format("20060102_150405.000") + "_" + currCcid[:8]
	fHandle, err := os.OpenFile(outFile, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		return nil, mylog.Errorf("getWorldState: OpenFile failed. err=%s", err)
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
		return nil, mylog.Errorf("getWorldState: keys operation failed. Error accessing state: %s", err)
	}
	defer keysIter.Close()

	for keysIter.HasNext() {
		qws.KeyCount++
		kv, iterErr := keysIter.Next()
		if iterErr != nil {
			mylog.Errorf("getWorldState: getNext failed, %s", iterErr)
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
			mylog.Errorf("getWorldState: Marshal failed. key=%s, err=%s", key, err)
			qws.ErrKeyList = append(qws.ErrKeyList, key)
			continue
		}
		jsonRecd = append(jsonRecd, '\n') //每一个行一个keyValue

		_, err = writer.Write(jsonRecd)
		if err != nil {
			mylog.Errorf("getWorldState: Write failed. key=%s, err=%s", key, err)
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

	mylog.Info("getWorldState: result=%+v.", qws)

	retJson, err := json.Marshal(qws)
	if err != nil {
		return nil, mylog.Errorf("getWorldState: Marshal failed. err=%s", err)
	}

	return retJson, nil
}

func (t *KD) loadWorldState(stub shim.ChaincodeStubInterface, fileName string, needHash, sameKeyOverwrite bool, srcCcid string) ([]byte, error) {
	var inFile = fmt.Sprintf("/home/%s", fileName)
	fHandle, err := os.OpenFile(inFile, os.O_RDONLY, 0755)
	if err != nil {
		return nil, mylog.Errorf("setWorldState: OpenFile failed. err=%s", err)
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
				mylog.Debug("setWorldState: reader end.")
				break
			}

			swsr.ReadErr = true
			return nil, mylog.Errorf("setWorldState: ReadBytes failed. err=%s", err)
		}

		swsr.FileLine++
		swsr.FileSize += int64(len(lineB))

		var oneRecd []string
		err = json.Unmarshal(lineB, &oneRecd)
		if err != nil {
			return nil, mylog.Errorf("setWorldState: Unmarshal failed. line=%s err=%s", string(lineB), err)
		}
		if len(oneRecd) < 2 {
			return nil, mylog.Errorf("setWorldState: oneRecd format error. oneRecd=%v", oneRecd)
		}
		var key = oneRecd[0]
		var value = oneRecd[1]

		if !sameKeyOverwrite {
			testB, err := stub.GetState(key)
			if err != nil {
				return nil, mylog.Errorf("setWorldState: GetState failed. key=%s err=%s", key, err)
			}
			if testB != nil {
				mylog.Debug("setWorldState: has key '%s', not Overwrite.", key)
				continue
			}
		}

		if needHash {
			if len(oneRecd) < 3 {
				mylog.Debug("setWorldState: no hash value, no check.")
			} else {
				var md5val = oneRecd[2]
				if md5val == INVALID_MD5_VALUE {
					mylog.Debug("setWorldState: hash value is invalid, no check.")
				} else {
					var hash = md5.New()
					_, err = io.WriteString(hash, key+value)
					if err != nil {
						return nil, mylog.Errorf("setWorldState: md5 create failed. key=%s.", key)
					} else {
						var newMd5 = hex.EncodeToString(hash.Sum(nil))
						if md5val != newMd5 {
							return nil, mylog.Errorf("setWorldState: md5 check failed. key=%s.", key)
						}
					}
				}
			}
		}

		valueB, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return nil, mylog.Errorf("setWorldState: DecodeString failed. value=%s err=%s", value, err)
		}

		newKey, newValB, err := t.dateConvertWhenUpdate(stub, srcCcid, key, valueB)
		if err != nil {
			return nil, mylog.Errorf("setWorldState: dateConvertWhenUpdate failed.  err=%s", err)
		}

		err = t.putState_Ex(stub, newKey, newValB)
		if err != nil {
			return nil, mylog.Errorf("setWorldState: PutState_Ex failed. key=%s err=%s", key, err)
		}

		swsr.KeyCount++

		mylog.Debug("setWorldState: PutState_Ex Ok, key=%s.", key)
	}

	err = t.updateAfter(stub, srcCcid)
	if err != nil {
		return nil, mylog.Errorf("setWorldState: updateAfter failed.  err=%s", err)
	}

	var endTime = time.Now()
	var runTime = endTime.Sub(begTime)
	swsr.RunTime = runTime.String()

	mylog.Info("setWorldState: result=%+v.", swsr)

	return nil, nil
}
func (t *KD) dateConvertWhenUpdate(stub shim.ChaincodeStubInterface, srcCcid, key string, valueB []byte) (string, []byte, error) {
	var err error
	var newKey = key
	var newValB = valueB

	if srcCcid == "ef38784bad472d640839c1782232aac63985489ee624b67a2d2b23448b03ebfb" {
		//1. 账户的key变化  因为老的账户的key没有前缀，所以根据json里的字段名来区分
		if bytes.Contains(valueB, []byte("\"ocert\"")) && bytes.Contains(valueB, []byte("\"aucm\"")) && bytes.Contains(valueB, []byte("\"id\"")) {
			mylog.Debug("updateDateConvert: got acc key:%s", key)
			if !strings.Contains(key, ACC_ENTITY_PREFIX) {
				newKey = t.getAccountEntityKey(key)
			}
		}

		//4. 修改第一条交易记录
		if key == t.getTransInfoKey(stub, 1) {
			mylog.Debug("updateDateConvert: got trans 1 key:%s", key)

			var trans Transaction
			err = json.Unmarshal(valueB, &trans)
			if err != nil {
				return "", nil, mylog.Errorf("updateDateConvert: Unmarshal(trans) failed. err=%s", err)
			}
			issueEntity, err := t.getAccountEntity(stub, COIN_ISSUE_ACC_ENTID)
			if err != nil {
				return "", nil, mylog.Errorf("updateDateConvert: getIssueEntity failed. err=%s", err)
			}

			trans.ToID = issueEntity.EntID
			newValB, err = json.Marshal(trans)
			if err != nil {
				return "", nil, mylog.Errorf("updateDateConvert: Marshal(trans) failed. err=%s", err)
			}
		}
	} else if srcCcid == "400298896ad7f0695d5e5d2379b67caa7a05930d158ccd0704ddc2696e3d156b" {
	}

	return newKey, newValB, nil
}
func (t *KD) updateAfter(stub shim.ChaincodeStubInterface, srcCcid string) error {

	if srcCcid == "ef38784bad472d640839c1782232aac63985489ee624b67a2d2b23448b03ebfb" {
		//2.  账户统计信息
		accsB, err := stub.GetState(ALL_ACC_INFO_KEY)
		if err != nil {
			return mylog.Errorf("updateDateConvert: GetState(ALL_ACC_INFO_KEY) failed. err=%s", err)
		}
		var accCnt int64 = 0
		if accsB != nil {
			var allAccs = bytes.NewBuffer(accsB)
			for {
				_, err = allAccs.ReadBytes(MULTI_STRING_DELIM)
				if err != nil {
					if err == io.EOF {
						break
					} else {
						return mylog.Errorf("updateDateConvert: ReadBytes failed. err=%s", err)
					}
				}
				accCnt++
			}
		}

		var asi AccountStatisticInfo

		asi.AccountCount = accCnt

		asiB, err := json.Marshal(asi)
		if err != nil {
			return mylog.Errorf("updateDateConvert: Marshal(asi) failed.err=%s", err)
		}
		err = t.putState_Ex(stub, ACC_STATIC_INFO_KEY, asiB)
		if err != nil {
			return mylog.Errorf("updateDateConvert: PutState(asiB) failed.err=%s", err)
		}

		//3.  发行货币的账户修改
		issueEntity, err := t.getAccountEntity(stub, COIN_ISSUE_ACC_ENTID)
		if err != nil {
			return mylog.Errorf("updateDateConvert: getIssueEntity failed. err=%s", err)
		}

		issueEntity.RestAmount = issueEntity.RestAmount - 1000000000 //已给央行发了1000000000
		err = t.setAccountEntity(stub, issueEntity)
		if err != nil {
			return mylog.Errorf("updateDateConvert: setAccountEntity failed. err=%s", err)
		}

	}

	return nil
}

func (t *KD) setAccountLockAmountCfg(stub shim.ChaincodeStubInterface, accName, cfgStr string, overwriteOld bool) (int64, int64, error) {
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
			return 0, 0, mylog.Errorf("setAccountLockAmountCfg parse error, '%s' format error 1.", ele)
		}

		amount, err = strconv.ParseInt(pair[0], 0, 64)
		if err != nil {
			return 0, 0, mylog.Errorf("setAccountLockAmountCfg parse error, '%s' format error 2.", ele)
		}

		endtime, err = strconv.ParseInt(pair[1], 0, 64)
		if err != nil {
			return 0, 0, mylog.Errorf("setAccountLockAmountCfg parse error, '%s' format error 3.", ele)
		}

		lockedThisTime += amount

		//这里要用list来存储，不能用map。map遍历时为随机顺序，会导致下面存储时各个节点的数据不一致
		endtimeAmtList = append(endtimeAmtList, CoinLockCfg{LockEndTime: endtime, LockAmount: amount})
	}

	var acli AccountCoinLockInfo
	var lockinfoKey = t.getAccountLockInfoKey(accName)

	if overwriteOld {
		acli.AccName = accName
	} else {
		acliB, err := stub.GetState(lockinfoKey)
		if err != nil {
			return 0, 0, mylog.Errorf("setAccountLockAmountCfg: GetState  failed. err=%s", err)
		}
		if acliB == nil {
			acli.AccName = accName
		} else {
			err = json.Unmarshal(acliB, &acli)
			if err != nil {
				return 0, 0, mylog.Errorf("setAccountLockAmountCfg: Unmarshal failed. err=%s", err)
			}
		}
	}

	acli.LockList = append(acli.LockList, endtimeAmtList...)

	for _, ele := range acli.LockList {
		lockedTotal += ele.LockAmount
	}

	acliB, err := json.Marshal(acli)
	if err != nil {
		return 0, 0, mylog.Errorf("setAccountLockAmountCfg: Marshal  failed. err=%s", err)
	}
	err = t.putState_Ex(stub, lockinfoKey, acliB)
	if err != nil {
		return 0, 0, mylog.Errorf("setAccountLockAmountCfg: putState_Ex  failed. err=%s", err)
	}

	mylog.Debug("setAccountLockAmountCfg: acliB=%s", string(acliB))

	return lockedThisTime, lockedTotal, nil
}

func (t *KD) getUserEntityKey(userName string) string {
	return UER_ENTITY_PREFIX + userName
}
func (t *KD) getUserEntity(stub shim.ChaincodeStubInterface, userName string) (*UserEntity, error) {
	var entB []byte
	var ue UserEntity
	var err error

	entB, err = stub.GetState(t.getUserEntityKey(userName))
	if err != nil {
		return nil, mylog.Errorf("getUserEntity GetState failed. err=%s", err)
	}

	if entB == nil {
		return nil, nil //UserEntity不一定存在，不存在时不报错
	}

	if err = json.Unmarshal(entB, &ue); err != nil {
		return nil, mylog.Errorf("getUserEntity Unmarshal failed. err=%s", err)
	}

	return &ue, nil
}
func (t *KD) setUserEntity(stub shim.ChaincodeStubInterface, ue *UserEntity) error {
	jsons, err := json.Marshal(ue)

	if err != nil {
		return mylog.Errorf("setUserEntity: Marshal failed. err=%s", err)
	}

	err = t.putState_Ex(stub, t.getUserEntityKey(ue.EntID), jsons)

	if err != nil {
		return mylog.Errorf("setUserEntity: PutState cb failed. err=%s", err)
	}
	return nil
}

func (t *KD) isAdmin(stub shim.ChaincodeStubInterface, accName string) bool {
	//获取管理员帐号(央行账户作为管理员帐户)
	tmpByte, err := t.getCenterBankAcc(stub)
	if err != nil {
		mylog.Error("Query getCenterBankAcc failed. err=%s", err)
		return false
	}
	//如果没有央行账户
	if tmpByte == nil {
		mylog.Errorf("Query getCenterBankAcc nil.")
		return false
	}

	return string(tmpByte) == accName
}

func (t *KD) putState_Ex(stub shim.ChaincodeStubInterface, key string, value []byte) error {
	//当key为空字符串时，0.6的PutState接口不会报错，但是会导致chainCode所在的contianer异常退出。
	if key == "" {
		return mylog.Errorf("PutState_Ex key err.")
	}
	mylog.Debug("putState_Ex: k=%s,v=%s", key, string(value))
	return stub.PutState(key, value)
}
func (t *KD) strSliceContains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}

	return false
}

func (t *KD) strSliceDelete(list []string, value string) []string {
	var newList []string

	for _, v := range list {
		if v != value {
			newList = append(newList, v)
		}
	}

	return newList
}

func main() {
	// for debug
	mylog.SetDefaultLvl(shim.LogInfo)

	//primitives.SetSecurityLevel("SHA3", 256)

	err := shim.Start(new(KD))
	if err != nil {
		mylog.Error("Error starting EventSender chaincode: %s", err)
	}
}
