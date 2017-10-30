package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	//"time"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/crypto/primitives"
)

var mylog = InitMylog("rack")

const (
	ENT_CENTERBANK = 1
	ENT_COMPANY    = 2
	ENT_PROJECT    = 3
	ENT_PERSON     = 4

	ATTR_ROLE    = "role"
	ATTR_USRNAME = "usrname"
	ATTR_USRTYPE = "usertype"

	TRANSSEQ_PREFIX    = "__~!@#@!~_rack_transSeqPre__"         //序列号生成器的key的前缀。使用的是worldState存储
	TRANSINFO_PREFIX   = "__~!@#@!~_rack_transInfoPre__"        //交易信息的key的前缀。使用的是worldState存储
	FIID_PREFIX        = "__~!@#@!~_rack_FinacInfoPre__"        //理财发行信息的key的前缀。使用的是worldState存储
	RACKID_PREFIX      = "__~!@#@!~_rack_RackInfoPre__"         //货架信息的key的前缀。使用的是worldState存储
	RACKFIID_PREFIX    = "__~!@#@!~_rack_RackFinacInfoPre__"    //货架融资信息的key的前缀。使用的是worldState存储
	DFID_TX_PREFIX     = "__~!@#@!~_rack_dfIdTxPre__"           //欠款融资id交易信息的key的前缀。使用的是worldState存储
	CENTERBANK_ACC_KEY = "__~!@#@!~_rack_centerBankAccKey__#$%" //央行账户的key。使用的是worldState存储
	ALL_ACC_KEY        = "__~!@#@!~_rack_allAccInfo__#$%"       //存储所有账户名的key。使用的是worldState存储

	ALL_ACC_DELIM = ':' //所有账户名的分隔符

	TRANS_LVL_CB   = 1 //交易级别，银行
	TRANS_LVL_COMM = 2 //交易级别，普通
)

//账户信息Entity  保存在链上
// 一系列ID（或账户）都定义为字符串类型。因为putStat函数的第一个参数为字符串类型，这些ID（或账户）都作为putStat的第一个参数；另外从SDK传过来的参数也都是字符串类型。
type Entity struct {
	EntID       string           `json:"id"`    //银行/企业/项目/个人ID
	EntDesc     string           `json:"desc"`  //描述信息
	EntType     int              `json:"entTp"` //类型 中央银行:1, 企业:2, 项目:3, 个人:4
	TotalAmount int64            `json:"ttAmt"` //货币总数额(发行或接收)
	RestAmount  int64            `json:"rtAmt"` //账户余额
	User        string           `json:"user"`  //该实例所属的用户
	Time        int64            `json:"time"`  //创建时间
	DFIdMap     map[string]int64 `json:"dfMap"` //理财id的map，理财id为key，购买金额为value。只保存已购买未赎回的理财产品
}

//查询账户信息  不用保存在区块链
type QueryEntity struct {
	TotalAmount int64            `json:"totalAmount"` //注意，这里指的是账户所有余额，即Entity中RestAmount
	DFIdMap     map[string]int64 `json:"dfMap"`       //理财id的map，理财id为key，购买金额为value。只保存已购买未赎回的理财产品
}

//供查询的交易内容 保存在链上
type PublicTrans struct {
	Serial    int64  `json:"ser"`   //全局交易序列号
	FromID    string `json:"fmid"`  //发送方ID
	ToID      string `json:"toid"`  //接收方ID
	Amount    int64  `json:"amt"`   //交易数额
	TransType string `json:"trstp"` //交易类型，前端传入，透传
	Time      int64  `json:"time"`  //交易时间
}

//交易内容  保存在链上 注意，QueryTrans中的字段名（包括json字段名）不能和Transaction中的字段名重复，否则解析会出问题
type Transaction struct {
	PublicTrans         //公开信息
	FromType     int    `json:"fmtp"`  //发送方角色 centerBank:1, 企业:2, 项目:3
	ToType       int    `json:"totp"`  //接收方角色 企业:2, 项目:3
	TxID         string `json:"txid"`  //交易ID
	TransLvl     int    `json:"trsLv"` //交易级别
	GlobalSerial int64  `json:"gSer"`  //全局交易序列号
	FId          string `json:"fid"`   //
	RackId       string `json:"rid"`   //
}

//查询的对账信息 不用保存在区块链
type QueryBalance struct {
	IssueAmount  int64  `json:"issueAmount"`  //市面上发行货币的总量
	AccCount     int64  `json:"accCount"`     //所有账户的总量
	AccSumAmount int64  `json:"accSumAmount"` //所有账户的货币的总量
	Message      string `json:"message"`      //对账附件信息
}

//理财发行信息 保存在链上
type FinancialInfo struct {
	FID           string   `json:"fid"`    //发行理财id，每期一个id。可以以年月日为id
	AmountPerUnit int64    `json:"amtPUt"` //每份理财金额
	TotalUnit     int64    `json:"tUt"`    //理财总份数
	SelledUnit    int64    `json:"sUt"`    //理财卖出份数
	Deadline      int64    `json:"ddLi"`   //到期时间
	FinacAccount  string   `json:"facc"`   //金融公司账户
	RackList      []string `json:"rlst"`   //本期有多少货架参与融资
	Time          int64    `json:"time"`   //创建时间
	IsClosed      int8     `json:"cls"`    //是否已结束 0:false  1:true
	SerialNum     int64    `json:"serNo"`  //序列号
}

//位置信息
type Position struct {
	Address string `json:"addr"`
}

//货架信息 保存在链上
type RackInfo struct {
	RackID    string   `json:"rid"`   //货架id
	Desc      string   `json:"desc"`  //货架描述
	Pos       Position `json:"pos"`   //位置
	AmountCap int64    `json:"amtC"`  //货架支撑的金额数。即货架能支持多少投资
	FinacList []string `json:"flst"`  //货架参与过哪些融资
	Time      int64    `json:"time"`  //创建时间
	SerialNum int64    `json:"serNo"` //序列号
	Actived   int8     `json:"act"`   //是否在使用 0:false 1:true
}

type CostEarnInfo struct {
	WareCost        int64 `json:"wc"`  //商品成本
	TransportCost   int64 `json:"tpc"` //运输成本
	MaintenanceCost int64 `json:"mtc"` //维护成本
	TraderCost      int64 `json:"tc"`  //零售商成本
	WareEarning     int64 `json:"we"`  //卖出商品收益
	BrandEarning    int64 `json:"be"`  //品牌收益
}

//货架融资信息 保存在链上
type RackFinancInfo struct {
	RackID        string           `json:"rid"`   //货架id
	FID           string           `json:"fid"`   //发行理财id
	Time          int64            `json:"time"`  //创建时间
	SerialNum     int64            `json:"serNo"` //序列号
	AmountFinca   int64            `json:"amtf"`  //实际投资额度
	CEInfo        CostEarnInfo     `json:"cei"`   //成本及收益
	UserAmountMap map[string]int64 `json:"uamp"`  //每个用户投资的金额
	UserProfitMap map[string]int64 `json:"upmp"`  //每个用户收益的金额
	Closed        bool             `json:"cls"`   //是否已结束
}

type QueryFinac struct {
	FinancialInfo
	RFInfoList []RackFinancInfo `json:"rfList"`
}
type QueryRack struct {
	RackInfo
	RFInfoList []RackFinancInfo `json:"rfList"`
}

type RACK struct {
}

//var centerBankId = 0xDE0B6B3A7640000
//var centerBankId = "10000000000000000000"

func (t *RACK) Init(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	mylog.Debug("Enter Init")
	mylog.Debug("func =%s, args = %v", function, args)

	return nil, nil
}

// Transaction makes payment of X units from A to B
func (t *RACK) Invoke(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	mylog.Debug("Enter Invoke")
	mylog.Debug("func =%s, args = %v", function, args)
	var err error

	var fixedArgCount = 4
	if len(args) < fixedArgCount {
		mylog.Error("Invoke miss arg, got %d, at least need %d.", len(args), fixedArgCount)
		return nil, errors.New("Invoke miss arg.")
	}

	//verify user and account
	var userName = args[0]
	var accName = args[1]
	var userCert []byte
	var times int64 = 0

	userCert, _ = base64.StdEncoding.DecodeString(args[2])

	times, err = strconv.ParseInt(args[3], 0, 64)
	if err != nil {
		mylog.Error("Invoke convert times(%s) failed. err=%s", args[3], err)
		return nil, errors.New("Invoke convert times failed.")
	}

	//开户时不需要校验
	if function != "account" && function != "accountCB" {
		if ok, _ := t.checkAccountOfUser(stub, userName, accName, userCert); !ok {
			fmt.Println("verify user(%s) and account(%s) failed. \n", userName, accName)
			return nil, errors.New("user and account check failed.")
		}
	}

	if function == "issue" {
		mylog.Debug("Enter issue")

		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			mylog.Error("Invoke(issue) miss arg, got %d, at least need %d.", len(args), argCount)
			return nil, errors.New("Invoke(issue) miss arg.")
		}

		var issueAmount int64
		issueAmount, err = strconv.ParseInt(args[fixedArgCount], 0, 64)
		if err != nil {
			mylog.Error("Invoke(issue) convert issueAmount(%s) failed. err=%s", args[fixedArgCount], err)
			return nil, errors.New("Invoke(issue) convert issueAmount failed.")
		}
		mylog.Debug("issueAmount= %v", issueAmount)

		tmpByte, err := t.getCenterBankAcc(stub)
		if err != nil {
			mylog.Error("Invoke(issue) getCenterBankAcc failed. err=%s", err)
			return nil, errors.New("Invoke(issue) getCenterBankAcc failed.")
		}
		//如果没有央行账户，报错。否则校验账户是否一致。
		if tmpByte == nil {
			mylog.Error("Invoke(issue) getCenterBankAcc nil.")
			return nil, errors.New("Invoke(issue) getCenterBankAcc nil.")
		} else {
			if accName != string(tmpByte) {
				mylog.Error("Invoke(issue) centerBank account is %s, can't issue to %s.", string(tmpByte), accName)
				return nil, errors.New("Invoke(issue) centerBank account conflict.")
			}
		}

		return t.issueCoin(stub, accName, issueAmount, times)

	} else if function == "account" {
		mylog.Debug("Enter account")
		var usrType int

		tmpType, err := stub.ReadCertAttribute(ATTR_USRTYPE)
		if err != nil {
			mylog.Error("ReadCertAttribute(%s) failed. err=%s", ATTR_USRTYPE, err)
			//return nil, errors.New("ReadCertAttribute failed.")
		}
		tmpName, err := stub.ReadCertAttribute(ATTR_USRNAME)
		if err != nil {
			mylog.Error("ReadCertAttribute(%s) failed. err=%s", ATTR_USRTYPE, err)
			//return nil, errors.New("ReadCertAttribute failed.")
		}
		tmpRole, err := stub.ReadCertAttribute(ATTR_ROLE)
		if err != nil {
			mylog.Error("ReadCertAttribute(%s) failed. err=%s", ATTR_USRTYPE, err)
			//return nil, errors.New("ReadCertAttribute failed.")
		}
		mylog.Debug("userName=%s, userType=%s, userRole=%s", string(tmpName), string(tmpType), string(tmpRole))
		/*
			usrType, err = strconv.Atoi(string(tmpType))
			if err != nil {
				mylog.Error("convert usrType(%s) failed. err=%s", tmpType, err)
				//return nil, errors.New("convert usrType failed.")
			}
		*/
		usrType = 0

		return t.newAccount(stub, accName, usrType, userName, times, false)

	} else if function == "accountCB" {
		mylog.Debug("Enter accountCB")
		var usrType int = 0

		tmpByte, err := t.getCenterBankAcc(stub)
		if err != nil {
			mylog.Error("Invoke(accountCB) getCenterBankAcc failed. err=%s", err)
			return nil, errors.New("Invoke(accountCB) getCenterBankAcc failed.")
		}

		//如果央行账户已存在，报错
		if tmpByte != nil {
			mylog.Error("Invoke(accountCB) CBaccount(%s) exists.", string(tmpByte))
			return nil, errors.New("Invoke(accountCB) account exists.")
		}

		_, err = t.newAccount(stub, accName, usrType, userName, times, true)
		if err != nil {
			mylog.Error("Invoke(accountCB) openAccount failed. err=%s", err)
			return nil, errors.New("Invoke(accountCB) openAccount failed.")
		}

		err = t.setCenterBankAcc(stub, accName)
		if err != nil {
			mylog.Error("Invoke(accountCB) setCenterBankAcc failed. err=%s", err)
			return nil, errors.New("Invoke(accountCB) setCenterBankAcc failed.")
		}

		return nil, nil

	} else if function == "transefer" {
		var argCount = fixedArgCount + 3
		if len(args) < argCount {
			mylog.Error("Invoke(transefer) miss arg, got %d, at least need %d.", len(args), argCount)
			return nil, errors.New("Invoke(transefer) miss arg.")
		}

		var toAcc = args[fixedArgCount]
		var transType = args[fixedArgCount+1]

		var transAmount int64
		transAmount, err = strconv.ParseInt(args[fixedArgCount+2], 0, 64)
		if err != nil {
			mylog.Error("Invoke(transefer) convert issueAmount(%s) failed. err=%s", args[fixedArgCount+2], err)
			return nil, errors.New("Invoke(transefer) convert issueAmount failed.")
		}
		mylog.Debug("transAmount= %v", transAmount)

		return t.transferCoin(stub, accName, toAcc, "", "", transType, transAmount, times)

	} else if function == "addRack" {
		var argCount = fixedArgCount + 4
		if len(args) < argCount {
			mylog.Error("Invoke(addRack) miss arg, got %d, at least need %d.", len(args), argCount)
			return nil, errors.New("Invoke(addRack) miss arg.")
		}

		var rackid = args[fixedArgCount] //理财id
		var desc = args[fixedArgCount+1] //描述
		var addr = args[fixedArgCount+2] //位置
		var capacity int64
		capacity, err = strconv.ParseInt(args[fixedArgCount+3], 0, 64) //
		if err != nil {
			mylog.Error("Invoke(addRack) convert capacity(%s) failed. err=%s", args[fixedArgCount+3], err)
			return nil, errors.New("Invoke(addRack) convert capacity failed.")
		}

		var ri RackInfo
		ri.RackID = rackid
		ri.Desc = desc
		ri.Pos.Address = addr
		ri.AmountCap = capacity
		ri.Time = times
		ri.SerialNum = 0 /////
		ri.Actived = 1

		var rackInfoKey = t.getRackInfoKey(rackid)

		riB, err := stub.GetState(rackInfoKey)
		if err != nil {
			mylog.Error("Invoke(addRack) GetState(%s) failed. err=%s.", rackInfoKey, err)
			return nil, errors.New("Invoke(addRack) GetState failed.")
		}
		if riB != nil {
			mylog.Error("Invoke(addRack) RackInfo exists already.")
			return nil, errors.New("Invoke(addRack) RackInfo exists already.")
		}

		riJson, err := json.Marshal(ri)
		if err != nil {
			mylog.Error("Invoke(addRack) Marshal failed. err=%s.", err)
			return nil, errors.New("Invoke(addRack) Marshal failed.")
		}

		err = t.PutState_Ex(stub, rackInfoKey, riJson)
		if err != nil {
			mylog.Error("Invoke(addRack) PutState failed. err=%s.", err)
			return nil, errors.New("Invoke(addRack) PutState failed.")
		}

		return nil, nil
	} else if function == "delRack" {
		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			mylog.Error("Invoke(delRack) miss arg, got %d, at least need %d.", len(args), argCount)
			return nil, errors.New("Invoke(delRack) miss arg.")
		}

		var rackid = args[fixedArgCount] //理财id

		var rackInfoKey = t.getRackInfoKey(rackid)

		riB, err := stub.GetState(rackInfoKey)
		if err != nil {
			mylog.Error("Invoke(delRack) GetState(%s) failed. err=%s.", rackInfoKey, err)
			return nil, errors.New("Invoke(delRack) GetState failed.")
		}
		if riB == nil {
			mylog.Error("Invoke(delRack) RackInfo(%d) not exists.", rackid)
			return nil, errors.New("Invoke(delRack) RackInfo exists already.")
		}

		var ri RackInfo
		err = json.Unmarshal(riB, &ri)
		if err != nil {
			mylog.Error("Invoke(delRack) Unmarshal failed. err=%s.", err)
			return nil, errors.New("Invoke(delRack) Unmarshal failed.")
		}
		//一般不会出现此情况
		if ri.RackID != rackid {
			mylog.Error("Invoke(delRack) rackid missmatch(%s).", ri.RackID)
			return nil, errors.New("Invoke(delRack) rackid missmatch.")
		}

		ri.Actived = 0

		riJson, err := json.Marshal(ri)
		if err != nil {
			mylog.Error("Invoke(delRack) Marshal failed. err=%s.", err)
			return nil, errors.New("Invoke(delRack) Marshal failed.")
		}

		err = t.PutState_Ex(stub, rackInfoKey, riJson)
		if err != nil {
			mylog.Error("Invoke(delRack) PutState failed. err=%s.", err)
			return nil, errors.New("Invoke(delRack) PutState failed.")
		}

		return nil, nil
	} else if function == "finacIssue" {
		var argCount = fixedArgCount + 5
		if len(args) < argCount {
			mylog.Error("Invoke(finacIssue) miss arg, got %d, at least need %d.", len(args), argCount)
			return nil, errors.New("Invoke(finacIssue) miss arg.")
		}

		var fid = args[fixedArgCount]        //理财id
		var finacAcc = args[fixedArgCount+1] //金融公司账户
		var amtPerUnit int64                 //每份理财金额
		var totalUnit int64                  //理财总份数
		var deadline int64                   //到期时间
		amtPerUnit, err = strconv.ParseInt(args[fixedArgCount+2], 0, 64)
		if err != nil {
			mylog.Error("Invoke(finacIssue) convert amtPerUnit(%s) failed. err=%s", args[fixedArgCount+2], err)
			return nil, errors.New("Invoke(finacIssue) convert amtPerUnit failed.")
		}
		totalUnit, err = strconv.ParseInt(args[fixedArgCount+3], 0, 64)
		if err != nil {
			mylog.Error("Invoke(finacIssue) convert totalUnit(%s) failed. err=%s", args[fixedArgCount+3], err)
			return nil, errors.New("Invoke(finacIssue) convert totalUnit failed.")
		}
		deadline, err = strconv.ParseInt(args[fixedArgCount+4], 0, 64)
		if err != nil {
			mylog.Error("Invoke(finacIssue) convert deadline(%s) failed. err=%s", args[fixedArgCount+4], err)
			return nil, errors.New("Invoke(finacIssue) convert deadline failed.")
		}

		var fi FinancialInfo
		fi.FID = fid
		fi.AmountPerUnit = amtPerUnit
		fi.TotalUnit = totalUnit
		fi.SelledUnit = 0
		fi.Deadline = deadline
		fi.FinacAccount = finacAcc
		fi.Time = times
		fi.IsClosed = 0
		fi.SerialNum = 0 /////

		//看账户是否存在
		ok, err := t.isEntityExists(stub, finacAcc)
		if err != nil {
			return nil, mylog.Errorf("Invoke(finacIssue) GetState(%s) failed. err=%s.", finacAcc, err)
		}
		if !ok {
			return nil, mylog.Errorf("Invoke(finacIssue) acc(%s) not exists.", finacAcc)
		}

		//看该信息是否已存在
		var fiacInfoKey = t.getFinacInfoKey(fid)

		fiB, err := stub.GetState(fiacInfoKey)
		if err != nil {
			mylog.Error("Invoke(finacIssue) GetState(%s) failed. err=%s.", fiacInfoKey, err)
			return nil, errors.New("Invoke(finacIssue) GetState failed.")
		}
		if fiB != nil {
			mylog.Error("Invoke(finacIssue) FinancialInfo exists already.")
			return nil, errors.New("Invoke(finacIssue) FinancialInfo exists already.")
		}

		fiJson, err := json.Marshal(fi)
		if err != nil {
			mylog.Error("Invoke(finacIssue) Marshal failed. err=%s.", err)
			return nil, errors.New("Invoke(finacIssue) Marshal failed.")
		}

		err = t.PutState_Ex(stub, fiacInfoKey, fiJson)
		if err != nil {
			mylog.Error("Invoke(finacIssue) PutState failed. err=%s.", err)
			return nil, errors.New("Invoke(finacIssue) PutState failed.")
		}

		return nil, nil
	} else if function == "userBuyFinac" {
		var argCount = fixedArgCount + 3
		if len(args) < argCount {
			return nil, mylog.Errorf("Invoke(buyFinac) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		var fid = args[fixedArgCount]      //理财id
		var rackid = args[fixedArgCount+1] //货架id
		var amout int64                    //购买的钱数
		amout, err = strconv.ParseInt(args[fixedArgCount+2], 0, 64)
		if err != nil {
			return nil, mylog.Errorf("Invoke(buyFinac) convert amout(%s) failed. err=%s", args[fixedArgCount+2], err)
		}

		//判断理财id是否存在及合法
		var fiacInfoKey = t.getFinacInfoKey(fid)
		fiB, err := stub.GetState(fiacInfoKey)
		if err != nil {
			return nil, mylog.Errorf("Invoke(buyFinac) GetState(%s) failed. err=%s.", fiacInfoKey, err)
		}
		if fiB == nil {
			return nil, mylog.Errorf("Invoke(buyFinac) FinancialInfo not exists.")
		}
		var fi FinancialInfo
		err = json.Unmarshal(fiB, &fi)
		if err != nil {
			return nil, mylog.Errorf("Invoke(buyFinac) Unmarshal failed. err=%s.", err)
		}
		//一般不会出现此情况
		if fi.FID != fid {
			return nil, mylog.Errorf("Invoke(buyFinac) fid missmatch(%s).", fi.FID)
		}
		if fi.Deadline < times || fi.IsClosed != 0 {
			return nil, mylog.Errorf("Invoke(buyFinac) financ(%s) is over deadline or closed(%d).", fi.FID, fi.IsClosed)
		}

		var rackInfoKey = t.getRackInfoKey(rackid)
		riB, err := stub.GetState(rackInfoKey)
		if err != nil {
			return nil, mylog.Errorf("Invoke(buyFinac) GetState(%s) failed. err=%s.", rackInfoKey, err)
		}
		if riB == nil {
			return nil, mylog.Errorf("Invoke(buyFinac) RackInfo(%d) not exists.", rackid)
		}
		var ri RackInfo
		err = json.Unmarshal(riB, &ri)
		if err != nil {
			return nil, mylog.Errorf("Invoke(buyFinac) Unmarshal failed. err=%s.", err)
		}
		//一般不会出现此情况
		if ri.RackID != rackid {
			return nil, mylog.Errorf("Invoke(buyFinac) rackid missmatch(%s).", ri.RackID)
		}
		if ri.Actived == 0 {
			return nil, mylog.Errorf("Invoke(buyFinac) rack(%s) is invactived.", rackid)
		}

		//写入货架融资信息
		rackFinacInfoKey := t.getRackFinacInfoKey(rackid, fid)
		rfiB, err := stub.GetState(rackFinacInfoKey)
		if err != nil {
			return nil, mylog.Errorf("Invoke(buyFinac) GetState(%s) failed. err=%s.", rackFinacInfoKey, err)
		}
		var rfi RackFinancInfo
		if rfiB == nil {
			rfi.RackID = rackid
			rfi.FID = fid
			rfi.Time = times
			rfi.SerialNum = 0 /////
			rfi.AmountFinca = amout
			rfi.UserAmountMap = make(map[string]int64)
			rfi.UserAmountMap[accName] = amout
		} else {
			err = json.Unmarshal(rfiB, &rfi)
			if err != nil {
				return nil, mylog.Errorf("Invoke(buyFinac) Unmarshal RackFinancInfo failed. err=%s.", err)
			}
			rfi.AmountFinca += amout
			_, ok := rfi.UserAmountMap[accName]
			if ok {
				rfi.UserAmountMap[accName] += amout
			} else {
				rfi.UserAmountMap[accName] = amout
			}
		}
		//融资额度超出货架支持能力
		if rfi.AmountFinca > ri.AmountCap {
			return nil, mylog.Errorf("Invoke(buyFinac) AmountFinca > rack's capacity. (%d,%d)", rfi.AmountFinca, ri.AmountCap)
		}

		_, err = t.transferCoin(stub, accName, fi.FinacAccount, fi.FID, ri.RackID, "", amout, times)
		if err != nil {
			return nil, mylog.Errorf("Invoke(transferCoin) failed. err=%s.", err)
		}

		if !t.Contains(fi.RackList, ri.RackID) {
			fi.RackList = append(fi.RackList, ri.RackID)
		}
		fiJson, err := json.Marshal(fi)
		if err != nil {
			return nil, mylog.Errorf("Invoke(buyFinac) Marshal failed. err=%s.", err)
		}

		if !t.Contains(ri.FinacList, fi.FID) {
			ri.FinacList = append(ri.FinacList, fi.FID)
		}

		riJson, err := json.Marshal(ri)
		if err != nil {
			return nil, mylog.Errorf("Invoke(buyFinac) Marshal failed. err=%s.", err)
		}
		rfiJson, err := json.Marshal(rfi)
		if err != nil {
			return nil, mylog.Errorf("Invoke(buyFinac) Marshal failed. err=%s.", err)
		}

		err = t.PutState_Ex(stub, rackFinacInfoKey, rfiJson)
		if err != nil {
			return nil, mylog.Errorf("Invoke(buyFinac) PutState failed. err=%s.", err)
		}

		err = t.PutState_Ex(stub, rackInfoKey, riJson)
		if err != nil {
			return nil, mylog.Errorf("Invoke(buyFinac) PutState failed. err=%s.", err)
		}

		err = t.PutState_Ex(stub, fiacInfoKey, fiJson)
		if err != nil {
			return nil, mylog.Errorf("Invoke(buyFinac) PutState failed. err=%s.", err)
		}
		return nil, nil
	} else if function == "finacDead" {
		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			mylog.Error("Invoke(finacDead) miss arg, got %d, at least need %d.", len(args), argCount)
			return nil, errors.New("Invoke(finacDead) miss arg.")
		}

		var fid = args[fixedArgCount] //理财id

		var fiacInfoKey = t.getFinacInfoKey(fid)

		fiB, err := stub.GetState(fiacInfoKey)
		if err != nil {
			mylog.Error("Invoke(finacDead) GetState(%s) failed. err=%s.", fiacInfoKey, err)
			return nil, errors.New("Invoke(finacDead) GetState failed.")
		}
		if fiB == nil {
			mylog.Error("Invoke(finacDead) FinancialInfo not exists.")
			return nil, errors.New("Invoke(finacDead) FinancialInfo not exists.")
		}
		var fi FinancialInfo
		err = json.Unmarshal(fiB, &fi)
		if err != nil {
			mylog.Error("Invoke(finacDead) Unmarshal failed. err=%s.", err)
			return nil, errors.New("Invoke(finacDead) Unmarshal failed.")
		}
		//一般不会出现此情况
		if fi.FID != fid {
			mylog.Error("Invoke(finacDead) fid missmatch(%s).", fi.FID)
			return nil, errors.New("Invoke(finacDead) fid missmatch.")
		}

		if times < fi.Deadline {
			mylog.Error("Invoke(finacDead) can't to dead(%v).", fi.Deadline)
			return nil, errors.New("Invoke(finacDead) can't to dead.")
		}

		//已经关闭，则不用再关闭
		if fi.IsClosed == 1 {
			return nil, nil
		}

		fi.IsClosed = 1

		fiJson, err := json.Marshal(fi)
		if err != nil {
			mylog.Error("Invoke(finacDead) Marshal failed. err=%s.", err)
			return nil, errors.New("Invoke(finacDead) Marshal failed.")
		}

		err = t.PutState_Ex(stub, fiacInfoKey, fiJson)
		if err != nil {
			mylog.Error("Invoke(finacDead) PutState failed. err=%s.", err)
			return nil, errors.New("Invoke(finacDead) PutState failed.")
		}

		return nil, nil
	} else if function == "finacBonus" {
		var argCount = fixedArgCount + 4
		if len(args) < argCount {
			return nil, mylog.Errorf("Invoke(finacBonus) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		var fid = args[fixedArgCount]      //理财id
		var rackid = args[fixedArgCount+1] //
		var wareCost int64                 //
		var wareEarning int64              //
		wareCost, err = strconv.ParseInt(args[fixedArgCount+2], 0, 64)
		if err != nil {
			return nil, mylog.Errorf("Invoke(buyFinac) convert wareCost(%s) failed. err=%s", args[fixedArgCount+2], err)
		}
		wareEarning, err = strconv.ParseInt(args[fixedArgCount+3], 0, 64)
		if err != nil {
			return nil, mylog.Errorf("Invoke(buyFinac) convert wareEarning(%s) failed. err=%s", args[fixedArgCount+3], err)
		}

		var rackFinacInfoKey = t.getRackFinacInfoKey(rackid, fid)

		rfiB, err := stub.GetState(rackFinacInfoKey)
		if err != nil {
			return nil, mylog.Errorf("Invoke(finacBonus) GetState(%s) failed. err=%s.", rackFinacInfoKey, err)
		}
		if rfiB == nil {
			return nil, mylog.Errorf("Invoke(finacBonus) FinancialInfo not exists.")
		}
		var rfi RackFinancInfo
		err = json.Unmarshal(rfiB, &rfi)
		if err != nil {
			return nil, mylog.Errorf("Invoke(finacBonus) Unmarshal failed. err=%s.", err)
		}

		rfi.CEInfo.WareCost = wareCost
		rfi.CEInfo.WareEarning = wareEarning

		var totalEarning = rfi.CEInfo.WareEarning
		var totalCost = rfi.CEInfo.WareCost

		if totalCost >= totalEarning {
			return nil, mylog.Errorf("Invoke(finacBonus) Unmarshal failed. err=%s.", err)
		}

		var profit = totalEarning - totalCost
		var amtCheck int64 = 0
		var profitCheck int64 = 0
		var accProfit int64
		if rfi.UserProfitMap == nil {
			rfi.UserProfitMap = make(map[string]int64)
		}
		for acc, amt := range rfi.UserAmountMap {
			amtCheck += amt
			accProfit = amt * profit / rfi.AmountFinca
			rfi.UserProfitMap[acc] = accProfit
			profitCheck += accProfit
		}
		if profitCheck > profit || amtCheck > rfi.AmountFinca {
			return nil, mylog.Errorf("Invoke(finacBonus) bonus check(%d,%d,%d,%d) failed.", profitCheck, profit, amtCheck, rfi.AmountFinca)
		}
		mylog.Debug("Invoke(finacBonus) rfi=%v", rfi)

		rfiJson, err := json.Marshal(rfi)
		if err != nil {
			return nil, mylog.Errorf("Invoke(finacBonus) Marshal failed. err=%s.", err)
		}

		err = t.PutState_Ex(stub, rackFinacInfoKey, rfiJson)
		if err != nil {
			return nil, mylog.Errorf("Invoke(finacBonus) PutState failed. err=%s.", err)
		}

		return nil, nil
	}

	//event
	stub.SetEvent("success", []byte("invoke success"))
	return nil, mylog.Errorf("unknown invoke(%s).", function)
}

// Query callback representing the query of a chaincode
func (t *RACK) Query(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	mylog.Debug("Enter Query")
	mylog.Debug("func =%s, args = %v", function, args)

	var err error

	var fixedArgCount = 2
	if len(args) < fixedArgCount {
		mylog.Error("Invoke miss arg, got %d, at least need %d.", len(args), fixedArgCount)
		return nil, errors.New("Invoke miss arg.")
	}
	var userName = args[0]
	var accName = args[1]

	if function == "query" {
		//verify user and account
		var userCert []byte
		//userCert, err = base64.StdEncoding.DecodeString(args[2])

		if ok, _ := t.checkAccountOfUser(stub, userName, accName, userCert); !ok {
			fmt.Println("verify user(%s) and account(%s) failed. \n", userName, accName)
			return nil, errors.New("user and account check failed.")
		}

		var queryEntity *Entity
		queryEntity, err = t.getEntity(stub, accName)
		if err != nil {
			mylog.Error("getEntity queryEntity(id=%s) failed. err=%s", accName, err)
			return nil, err
		}
		mylog.Debug("queryEntity=%v", queryEntity)

		var qEnt QueryEntity
		qEnt.TotalAmount = queryEntity.RestAmount
		qEnt.DFIdMap = queryEntity.DFIdMap
		if qEnt.DFIdMap == nil {
			qEnt.DFIdMap = make(map[string]int64)
		}
		//retValue := []byte(strconv.FormatInt(queryEntity.RestAmount, 10))
		retValue, err := json.Marshal(qEnt)
		if err != nil {
			mylog.Error("getEntity Marshal failed. err=%s", err)
			return nil, err
		}

		mylog.Debug("retValue=%v, %s", retValue, string(retValue))

		return retValue, nil
	} else if function == "queryTx" {
		var argCount = fixedArgCount + 3
		if len(args) < argCount {
			mylog.Error("queryTx miss arg, got %d, need %d.", len(args), argCount)
			return nil, errors.New("queryTx miss arg.")
		}

		var begSeq int64
		var endSeq int64
		var transLvl int

		begSeq, err = strconv.ParseInt(args[fixedArgCount], 0, 64)
		if err != nil {
			mylog.Error("queryTx ParseInt for begSeq(%s) failed. err=%s", args[fixedArgCount], err)
			return nil, errors.New("queryTx parse begSeq failed.")
		}
		endSeq, err = strconv.ParseInt(args[fixedArgCount+1], 0, 64)
		if err != nil {
			mylog.Error("queryTx ParseInt for endSeq(%s) failed. err=%s", args[fixedArgCount+1], err)
			return nil, errors.New("queryTx parse endSeq failed.")
		}

		//如果begSeq<=0，则从第一条开始；endSeq为-1，表示到最后一条记录
		if begSeq <= 0 {
			begSeq = 1 //本合约的交易记录从1开始编号的
		}
		if endSeq < 0 {
			endSeq = math.MaxInt64 //设置为Int64最大值
		}

		transLvl, err = strconv.Atoi(args[fixedArgCount+2])
		if err != nil {
			mylog.Error("queryTx ParseInt for transLvl(%s) failed. err=%s", args[fixedArgCount+2], err)
			return nil, errors.New("queryTx parse transLvl failed.")
		}

		return t.queryTransInfos(stub, transLvl, begSeq, endSeq)

	} else if function == "queryBalance" {
		return t.queryBalance(stub)
	} else if function == "queryState" {
		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			mylog.Error("queryState miss arg, got %d, need %d.", len(args), argCount)
			return nil, errors.New("queryState miss arg.")
		}
		key := args[fixedArgCount]

		retValues, err := stub.GetState(key)
		if err != nil {
			mylog.Error("queryState GetState failed. err=%s", err)
			return nil, errors.New("queryState GetState failed.")
		}

		return retValues, nil
	} else if function == "queryFinac" {
		var argCount = fixedArgCount + 2
		if len(args) < argCount {
			return nil, mylog.Errorf("queryState miss arg, got %d, need %d.", len(args), argCount)
		}
		finacid := args[fixedArgCount]
		rackid := args[fixedArgCount+1]

		var retValue = []byte("{}") //返回类型为 QueryFinac 类型，为空时返回空JSON结构体
		var qf QueryFinac

		//判断理财id是否存在及合法
		var fiacInfoKey = t.getFinacInfoKey(finacid)
		fiB, err := stub.GetState(fiacInfoKey)
		if err != nil {
			return retValue, mylog.Errorf("Query(queryFinac) GetState(fid=%s) failed. err=%s.", fiacInfoKey, err)
		}
		if fiB == nil {
			mylog.Warn("Query(queryFinac) FinancialInfo(%s) not exists.", finacid)
			return retValue, nil //不存在时返回空结构体
		}
		var fi FinancialInfo
		err = json.Unmarshal(fiB, &fi)
		if err != nil {
			return nil, mylog.Errorf("Query(queryFinac) Unmarshal  FinancialInfo failed. err=%s.", err)
		}
		//一般不会出现此情况
		if fi.FID != finacid {
			return nil, mylog.Errorf("Query(queryFinac) fid missmatch(%s).", fi.FID)
		}

		qf.FinancialInfo = fi
		qf.RFInfoList = []RackFinancInfo{}

		var rackList []string
		if len(rackid) == 0 {
			//不查货架的融资信息
		} else if rackid == "*" {
			//查询所有的rackid
			rackList = fi.RackList
		} else {
			rackList = []string{rackid}
		}

		for _, rid := range rackList {
			rackFinacInfoKey := t.getRackFinacInfoKey(rid, finacid)
			rfiB, err := stub.GetState(rackFinacInfoKey)
			if err != nil {
				return nil, mylog.Errorf("Query(queryFinac) GetState(%s,%s) failed. err=%s.", finacid, rid, err)
			}
			if rfiB == nil {
				mylog.Warn("Query(queryFinac) RackFinancInfo(%s,%s) not exists.", finacid, rid)
				continue //如果该记录不存在，则继续处理
			}
			var rfi RackFinancInfo
			err = json.Unmarshal(rfiB, &rfi)
			if err != nil {
				return nil, mylog.Errorf("Query(queryFinac) Unmarshal RackFinancInfo failed. err=%s.", err)
			}
			mylog.Debug("k=%s, rfiB=%s rfi=%v", rackFinacInfoKey, rfiB, rfi)
			qf.RFInfoList = append(qf.RFInfoList, rfi)
			mylog.Debug("RFInfoList=%v", qf.RFInfoList)
		}

		retValue, err = json.Marshal(qf)
		if err != nil {
			return nil, mylog.Errorf("Query(queryFinac) Unmarshal RackFinancInfo failed. err=%s.", err)
		}
		return retValue, nil
	} else if function == "queryRack" {
		var argCount = fixedArgCount + 2
		if len(args) < argCount {
			return nil, mylog.Errorf("queryState miss arg, got %d, need %d.", len(args), argCount)
		}
		rackid := args[fixedArgCount]
		finacid := args[fixedArgCount+1]

		var retValue = []byte("{}") //返回类型为 QueryRack 类型，为空时返回空JSON结构体
		var qr QueryRack

		var rackInfoKey = t.getRackInfoKey(rackid)
		riB, err := stub.GetState(rackInfoKey)
		if err != nil {
			return nil, mylog.Errorf("Query(queryRack) GetState(%s) failed. err=%s.", rackInfoKey, err)
		}
		if riB == nil {
			mylog.Warn("Query(queryRack) RackInfo(%s) not exists.", rackid)
			return retValue, nil
		}
		var ri RackInfo
		err = json.Unmarshal(riB, &ri)
		if err != nil {
			return nil, mylog.Errorf("Query(queryRack) Unmarshal failed. err=%s.", err)
		}
		//一般不会出现此情况
		if ri.RackID != rackid {
			return nil, mylog.Errorf("Query(queryRack) rackid missmatch(%s).", ri.RackID)
		}

		qr.RackInfo = ri
		qr.RFInfoList = []RackFinancInfo{}

		var finacList []string
		if len(finacid) == 0 {
			//不查货架的融资信息
		} else if finacid == "*" {
			//查询所有的rackid
			finacList = ri.FinacList
		} else {
			finacList = []string{finacid}
		}

		for _, fid := range finacList {
			rackFinacInfoKey := t.getRackFinacInfoKey(rackid, fid)
			rfiB, err := stub.GetState(rackFinacInfoKey)
			if err != nil {
				return nil, mylog.Errorf("Query(queryRack) GetState(%s,%s) failed. err=%s.", fid, rackid, err)
			}
			if rfiB == nil {
				mylog.Warn("Query(queryRack) RackFinancInfo(%s,%s) not exists.", fid, rackid)
				continue //如果该记录不存在，则继续处理
			}
			var rfi RackFinancInfo
			err = json.Unmarshal(rfiB, &rfi)
			if err != nil {
				return nil, mylog.Errorf("Query(queryRack) Unmarshal RackFinancInfo failed. err=%s.", err)
			}
			qr.RFInfoList = append(qr.RFInfoList, rfi)
		}

		retValue, err = json.Marshal(qr)
		if err != nil {
			return nil, mylog.Errorf("Query(queryRack) Unmarshal RackFinancInfo failed. err=%s.", err)
		}
		return retValue, nil
	} else if function == "queryAcc" {
		accExist, err := t.isEntityExists(stub, accName)
		if err != nil {
			return nil, mylog.Errorf("Query(queryAcc): isEntityExists (id=%s) failed. err=%s", accName, err)
		}

		var retValues []byte
		if accExist {
			retValues = []byte("1")
		} else {
			retValues = []byte("0")
		}

		return retValues, nil
	}

	return nil, mylog.Errorf("unknown query(%s).", function)
}

func (t *RACK) queryTransInfos(stub shim.ChaincodeStubInterface, transLvl int, begIdx, endIdx int64) ([]byte, error) {
	var maxSeq int64
	var err error
	var retTransInfo []byte = []byte("[]") //返回的结果是一个json数组，所以如果结果为空，返回空数组

	if endIdx < begIdx {
		mylog.Warn("queryTransInfos nothing to do(%d,%d).", begIdx, endIdx)
		return retTransInfo, nil
	}

	//先判断是否存在交易序列号了，如果不存在，说明还没有交易发生。 这里做这个判断是因为在getTransSeq里如果没有设置过序列号的key会自动设置一次，但是在query中无法执行PutStat，会报错
	var seqKey = t.getTransSeqKey(stub, transLvl)
	test, err := stub.GetState(seqKey)
	if err != nil {
		mylog.Error("queryTransInfos GetState failed. err=%s", err)
		return nil, errors.New("queryTransInfos GetState failed.")
	}
	if test == nil {
		mylog.Warn("no trans saved.")
		return retTransInfo, nil
	}

	//先获取当前最大的序列号
	maxSeq, err = t.getTransSeq(stub, seqKey)
	if err != nil {
		mylog.Error("queryTransInfos getTransSeq failed. err=%s", err)
		return nil, errors.New("queryTransInfos getTransSeq failed.")
	}

	if endIdx > maxSeq {
		endIdx = maxSeq

		if endIdx < begIdx {
			mylog.Warn("queryTransInfos nothing to do(%d,%d).", begIdx, endIdx)
			return retTransInfo, nil
		}
	}
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
				mylog.Error("getTransInfo GetState nil(idx=%d).", i)
				//return nil, errors.New("getTransInfo GetState nil.")
				continue
			}
			//获取的TransInfo已经是JSON格式的了，这里直接给拼接为JSON数组，以提高效率。
			retTransInfo.Write(tmpState)
			retTransInfo.WriteByte(',')
		}
		retTransInfo.Truncate(retTransInfo.Len() - 1) //去掉最后的那个','
		retTransInfo.WriteByte(']')
	*/
	var transArr []PublicTrans

	for i := begIdx; i <= endIdx; i++ {
		trans, err := t.getQueryTransInfo(stub, t.getTransInfoKey(stub, transLvl, i))
		if err != nil {
			mylog.Error("getTransInfo getQueryTransInfo(idx=%d) failed.err=%s", i, err)
			continue
		}
		transArr = append(transArr, *trans)
	}

	retTransInfo, err = json.Marshal(transArr)
	if err != nil {
		mylog.Error("getTransInfo Marshal failed.err=%s", err)
		return nil, errors.New("getTransInfo Marshal failed")
	}

	return retTransInfo, nil
}

func (t *RACK) queryBalance(stub shim.ChaincodeStubInterface) ([]byte, error) {
	var qb QueryBalance
	qb.IssueAmount = 0
	qb.AccSumAmount = 0
	qb.AccCount = 0

	accsB, err := stub.GetState(ALL_ACC_KEY)
	if err != nil {
		mylog.Error("queryBalance GetState failed. err=%s", err)
		return nil, errors.New("queryBalance GetState failed.")
	}
	if accsB != nil {

		cbAccB, err := t.getCenterBankAcc(stub)
		if err != nil {
			mylog.Error("queryBalance getCenterBankAcc failed. err=%s", err)
			return nil, errors.New("queryBalance getCenterBankAcc failed.")
		}
		if cbAccB == nil {
			qb.Message += "none centerBank;"
		} else {
			cbEnt, err := t.getEntity(stub, string(cbAccB))
			if err != nil {
				mylog.Error("queryBalance getCenterBankAcc failed. err=%s", err)
				return nil, errors.New("queryBalance getCenterBankAcc failed.")
			}
			qb.IssueAmount = cbEnt.TotalAmount - cbEnt.RestAmount
		}

		var allAccs = bytes.NewBuffer(accsB)
		var acc []byte
		var ent *Entity
		for {
			acc, err = allAccs.ReadBytes(ALL_ACC_DELIM)
			if err != nil {
				if err == io.EOF {
					break
				} else {
					mylog.Error("queryBalance ReadBytes failed. err=%s", err)
					continue
				}
			}
			qb.AccCount++
			acc = acc[:len(acc)-1] //去掉末尾的分隔符

			ent, err = t.getEntity(stub, string(acc))
			if err != nil {
				mylog.Error("queryBalance getEntity(%s) failed. err=%s", string(acc), err)
				qb.Message += fmt.Sprintf("get account(%s) info failed;", string(acc))
				continue
			}
			qb.AccSumAmount += ent.RestAmount
		}
	}

	retValue, err := json.Marshal(qb)
	if err != nil {
		mylog.Error("queryBalance Marshal failed. err=%s", err)
		return nil, errors.New("queryBalance Marshal failed.")
	}

	return retValue, nil
}

func (t *RACK) isCaller(stub shim.ChaincodeStubInterface, certificate []byte) (bool, error) {
	mylog.Debug("Check caller...")

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
}

func (t *RACK) checkAccountOfUser(stub shim.ChaincodeStubInterface, userName string, account string, userCert []byte) (bool, error) {

	//*
	callerCert, err := stub.GetCallerCertificate()
	mylog.Debug("caller certificate: %x", callerCert)

	ok, err := t.isCaller(stub, callerCert)
	if err != nil {
		mylog.Error("call isCaller failed(callerCert).")
	}
	if !ok {
		mylog.Error("is not Caller(callerCert).")
	}

	ok, err = t.isCaller(stub, userCert)
	if err != nil {
		mylog.Error("call isCaller failed(userCert).")
	}
	if !ok {
		mylog.Error("is not userCert(userCert).")
	}
	//*/

	return true, nil
}

func (t *RACK) getEntity(stub shim.ChaincodeStubInterface, cbId string) (*Entity, error) {
	var centerBankByte []byte
	var cb Entity
	var err error

	centerBankByte, err = stub.GetState(cbId)
	if err != nil {
		return nil, err
	}

	if centerBankByte == nil {
		return nil, errors.New("nil entity.")
	}

	if err = json.Unmarshal(centerBankByte, &cb); err != nil {
		return nil, errors.New("get data of centerBank failed.")
	}

	return &cb, nil
}

func (t *RACK) isEntityExists(stub shim.ChaincodeStubInterface, cbId string) (bool, error) {
	var centerBankByte []byte
	var err error

	centerBankByte, err = stub.GetState(cbId)
	if err != nil {
		return false, err
	}

	if centerBankByte == nil {
		return false, nil
	}

	return true, nil
}

//央行数据写入
func (t *RACK) setEntity(stub shim.ChaincodeStubInterface, cb *Entity) error {

	jsons, err := json.Marshal(cb)

	if err != nil {
		mylog.Error("marshal cb failed. err=%s", err)
		return errors.New("marshal cb failed.")
	}

	err = t.PutState_Ex(stub, cb.EntID, jsons)

	if err != nil {
		mylog.Error("PutState cb failed. err=%s", err)
		return errors.New("PutState cb failed.")
	}
	return nil
}

//发行
func (t *RACK) issueCoin(stub shim.ChaincodeStubInterface, cbID string, issueAmount, times int64) ([]byte, error) {
	mylog.Debug("Enter issueCoin")

	var err error

	var cb *Entity
	cb, err = t.getEntity(stub, cbID)
	if err != nil {
		mylog.Error("getCenterBank failed. err=%s", err)
		return nil, errors.New("getCenterBank failed.")
	}
	mylog.Debug("issue before:%v", cb)

	if issueAmount >= math.MaxInt64-cb.TotalAmount {
		mylog.Error("issue amount will be overflow(%v,%v), reject.", math.MaxInt64, cb.TotalAmount)
		return nil, errors.New("issue amount will be overflow.")
	}

	cb.TotalAmount += issueAmount
	cb.RestAmount += issueAmount

	mylog.Debug("issue after:%v", cb)

	err = t.setEntity(stub, cb)
	if err != nil {
		mylog.Error("setCenterBank failed. err=%s", err)
		return nil, errors.New("setCenterBank failed.")
	}

	//虚拟一个超级账户，给央行发行货币。因为给央行发行货币，是不需要转账的源头的
	var fromEntity Entity
	fromEntity.EntID = "198405051114"
	fromEntity.EntType = -1
	fromEntity.RestAmount = math.MaxInt64
	fromEntity.TotalAmount = math.MaxInt64
	fromEntity.User = "fanxiaotian"

	t.recordTranse(stub, &fromEntity, cb, "", "", "issue", issueAmount, times)

	return nil, nil
}

//转账
func (t *RACK) transferCoin(stub shim.ChaincodeStubInterface, from, to, fId, rackId, transType string, amount, times int64) ([]byte, error) {
	mylog.Debug("Enter transferCoin")

	var err error

	if amount <= 0 {
		mylog.Error("transferCoin failed. invalid amount(%d)", amount)
		return nil, errors.New("transferCoin failed. invalid amount.")
	}
	if from == to {
		mylog.Error("transferCoin from equals to.")
		return nil, errors.New("transferCoin from equals to.")
	}

	var fromEntity, toEntity *Entity
	fromEntity, err = t.getEntity(stub, from)
	if err != nil {
		mylog.Error("transferCoin: getEntity(id=%s) failed. err=%s", from, err)
		return nil, errors.New("getEntity from failed.")
	}
	toEntity, err = t.getEntity(stub, to)
	if err != nil {
		mylog.Error("transferCoin: getEntity(id=%s) failed. err=%s", to, err)
		return nil, errors.New("getEntity to failed.")
	}

	if fromEntity.RestAmount < amount {
		mylog.Error("transferCoin: fromEntity(id=%s) restAmount not enough.", from)
		return nil, errors.New("fromEntity restAmount not enough.")
	}
	mylog.Debug("fromEntity before= %v", fromEntity)
	mylog.Debug("toEntity before= %v", toEntity)

	fromEntity.RestAmount -= amount
	toEntity.RestAmount += amount
	toEntity.TotalAmount += amount

	/*
		var recordFid string = ""
		//********** 添加子账户处理
		//如果转出账户是央行账户，那么直接转出，无需处理子账户。因为央行账户没有以融资id的子账户，所以DFIdMap为nil
		if fromEntity.DFIdMap != nil {
			v, ok := fromEntity.DFIdMap[fId]
			if !ok {
				mylog.Error("transferCoin: fromEntity(id=%s) has no such dfid '%s'.", from, fId)
				return nil, errors.New("fromEntity has no such dfid.")
			}
			if v < amount {
				mylog.Error("transferCoin: fromEntity(id=%s) sub acc(id=%s) restAmount not enough.", from, fId)
				return nil, errors.New("fromEntity sub acc restAmount not enough.")
			}
			fromEntity.DFIdMap[fId] -= amount

			//只有转出账户有dfid子账户时，才需要记录该交易到以dfid为key的交易记录里
			recordFid = fId
		}
		if toEntity.DFIdMap != nil {
			_, ok := toEntity.DFIdMap[fId]
			if !ok {
				toEntity.DFIdMap[fId] = amount
			} else {
				toEntity.DFIdMap[fId] += amount
			}
		}
		//********** 添加子账户处理
	*/

	mylog.Debug("fromEntity after= %v", fromEntity)
	mylog.Debug("toEntity after= %v", toEntity)

	err = t.setEntity(stub, fromEntity)
	if err != nil {
		mylog.Error("transferCoin: setEntity of fromEntity(id=%s) failed. err=%s", from, err)
		return nil, errors.New("setEntity of from failed.")
	}
	err = t.setEntity(stub, toEntity)
	if err != nil {
		mylog.Error("transferCoin: setEntity of toEntity(id=%s) failed. err=%s", to, err)
		return nil, errors.New("setEntity of to failed.")
	}

	err = t.recordTranse(stub, fromEntity, toEntity, fId, rackId, transType, amount, times)

	return nil, err
}

//记录交易。目前交易分为两种：一种是和央行打交道的，包括央行发行货币、央行给项目或企业转帐，此类交易普通用户不能查询；另一种是项目、企业、个人间互相转账，此类交易普通用户能查询
func (t *RACK) recordTranse(stub shim.ChaincodeStubInterface, fromEnt, toEnt *Entity, fId, rackId, transType string, amount, times int64) error {
	var transInfo Transaction
	//var now = time.Now()

	transInfo.FromID = fromEnt.EntID
	transInfo.FromType = fromEnt.EntType
	transInfo.ToID = toEnt.EntID
	transInfo.ToType = toEnt.EntType
	//transInfo.Time = now.Unix()*1000 + int64(now.Nanosecond()/1000000) //单位毫秒
	transInfo.Time = times
	transInfo.Amount = amount
	transInfo.TxID = stub.GetTxID()
	transInfo.TransType = transType
	transInfo.FId = fId
	transInfo.RackId = rackId

	var transLevel int = TRANS_LVL_COMM
	accCB, err := t.getCenterBankAcc(stub)
	if err != nil {
		mylog.Error("recordTranse call getCenterBankAcc failed. err=%s", err)
		return errors.New("recordTranse getCenterBankAcc failed.")
	}
	if (accCB != nil) && (string(accCB) == transInfo.FromID || string(accCB) == transInfo.ToID) {
		transLevel = TRANS_LVL_CB
	}

	transInfo.TransLvl = transLevel

	err = t.setTransInfo(stub, &transInfo)
	if err != nil {
		mylog.Error("recordTranse call setTransInfo failed. err=%s", err)
		return errors.New("recordTranse setTransInfo failed.")
	}

	return nil
}

func (t *RACK) checkAccountName(accName string) error {
	//会用':'作为分隔符分隔多个账户名，所以账户名不能含有':'
	var invalidChars string = string(ALL_ACC_DELIM)

	if strings.ContainsAny(accName, invalidChars) {
		mylog.Error("isAccountNameValid (acc=%s) failed.", accName)
		return fmt.Errorf("accName '%s' can not contains '%s'.", accName, invalidChars)
	}
	return nil
}

func (t *RACK) saveAccountName(stub shim.ChaincodeStubInterface, accName string) error {
	accB, err := stub.GetState(ALL_ACC_KEY)
	if err != nil {
		mylog.Error("saveAccountName GetState failed.err=%s", err)
		return err
	}

	var accs []byte
	if accB == nil {
		accs = append([]byte(accName), ALL_ACC_DELIM) //第一次添加accName，最后也要加上分隔符
	} else {
		accs = append(accB, []byte(accName)...)
		accs = append(accs, ALL_ACC_DELIM)
	}

	err = t.PutState_Ex(stub, ALL_ACC_KEY, accs)
	if err != nil {
		mylog.Error("setCenterBankAcc PutState failed.err=%s", err)
		return err
	}
	return nil
}

func (t *RACK) getAllAccountNames(stub shim.ChaincodeStubInterface) ([]byte, error) {
	accB, err := stub.GetState(ALL_ACC_KEY)
	if err != nil {
		mylog.Error("getAllAccountNames GetState failed.err=%s", err)
		return nil, err
	}
	return accB, nil
}

func (t *RACK) newAccount(stub shim.ChaincodeStubInterface, accName string, accType int, userName string, times int64, isCBAcc bool) ([]byte, error) {
	mylog.Debug("Enter openAccount")

	var err error
	var accExist bool

	if err = t.checkAccountName(accName); err != nil {
		return nil, err
	}

	accExist, err = t.isEntityExists(stub, accName)
	if err != nil {
		mylog.Error("isEntityExists (id=%s) failed. err=%s", accName, err)
		return nil, errors.New("isEntityExists failed.")
	}

	if accExist {
		mylog.Warn("account (id=%s) failed, already exists.", accName)
		return nil, fmt.Errorf("account(%s) is eixsts.", accName)
	}

	var ent Entity
	//var now = time.Now()

	ent.EntID = accName
	ent.EntType = accType
	ent.RestAmount = 0
	ent.TotalAmount = 0
	//ent.Time = now.Unix()*1000 + int64(now.Nanosecond()/1000000) //单位毫秒
	ent.Time = times
	ent.DFIdMap = nil

	//非央行账户需要建立以dfid为key的子账户
	if !isCBAcc {
		ent.DFIdMap = make(map[string]int64)
	}

	err = t.setEntity(stub, &ent)
	if err != nil {
		mylog.Error("openAccount setEntity (id=%s) failed. err=%s", accName, err)
		return nil, errors.New("openAccount setEntity failed.")
	}

	mylog.Debug("openAccount success: %v", ent)

	//央行账户此处不保存
	if !isCBAcc {
		ent.DFIdMap = make(map[string]int64)
		err = t.saveAccountName(stub, accName)
	}

	return nil, err
}

var centerBankAccCache []byte = nil

func (t *RACK) setCenterBankAcc(stub shim.ChaincodeStubInterface, acc string) error {
	err := t.PutState_Ex(stub, CENTERBANK_ACC_KEY, []byte(acc))
	if err != nil {
		mylog.Error("setCenterBankAcc PutState failed.err=%s", err)
		return err
	}

	centerBankAccCache = []byte(acc)

	return nil
}
func (t *RACK) getCenterBankAcc(stub shim.ChaincodeStubInterface) ([]byte, error) {
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

func (t *RACK) getTransSeq(stub shim.ChaincodeStubInterface, transSeqKey string) (int64, error) {
	seqB, err := stub.GetState(transSeqKey)
	if err != nil {
		mylog.Error("getTransSeq GetState failed.err=%s", err)
		return -1, err
	}
	//如果不存在则创建
	if seqB == nil {
		err = t.PutState_Ex(stub, transSeqKey, []byte("0"))
		if err != nil {
			mylog.Error("initTransSeq PutState failed.err=%s", err)
			return -1, err
		}
		return 0, nil
	}

	seq, err := strconv.ParseInt(string(seqB), 10, 64)
	if err != nil {
		mylog.Error("getTransSeq ParseInt failed.seq=%v, err=%s", seqB, err)
		return -1, err
	}

	return seq, nil
}
func (t *RACK) setTransSeq(stub shim.ChaincodeStubInterface, transSeqKey string, seq int64) error {
	err := t.PutState_Ex(stub, transSeqKey, []byte(strconv.FormatInt(seq, 10)))
	if err != nil {
		mylog.Error("setTransSeq PutState failed.err=%s", err)
		return err
	}

	return nil
}

func (t *RACK) getTransInfoKey(stub shim.ChaincodeStubInterface, transLvl int, seq int64) string {
	var buf = bytes.NewBufferString(TRANSINFO_PREFIX)
	buf.WriteString("lvl")
	buf.WriteString(strconv.Itoa(transLvl))
	buf.WriteString("_")
	buf.WriteString(strconv.FormatInt(seq, 10))
	return buf.String()
}
func (t *RACK) getTransSeqKey(stub shim.ChaincodeStubInterface, transLvl int) string {
	var buf = bytes.NewBufferString(TRANSSEQ_PREFIX)
	buf.WriteString("lvl")
	buf.WriteString(strconv.Itoa(transLvl))
	buf.WriteString("_")
	return buf.String()
}
func (t *RACK) getGlobTransSeqKey(stub shim.ChaincodeStubInterface) string {
	return TRANSSEQ_PREFIX + "global_"
}

func (t *RACK) setTransInfo(stub shim.ChaincodeStubInterface, info *Transaction) error {
	//先获取全局seq
	seqGlob, err := t.getTransSeq(stub, t.getGlobTransSeqKey(stub))
	if err != nil {
		mylog.Error("setTransInfo getTransSeq failed.err=%s", err)
		return err
	}
	seqGlob++

	//再获取当前交易级别的seq
	seqLvl, err := t.getTransSeq(stub, t.getTransSeqKey(stub, info.TransLvl))
	if err != nil {
		mylog.Error("setTransInfo getTransSeq failed.err=%s", err)
		return err
	}
	seqLvl++

	info.GlobalSerial = seqGlob
	info.Serial = seqLvl
	transJson, err := json.Marshal(info)
	if err != nil {
		mylog.Error("setTransInfo marshal failed. err=%s", err)
		return errors.New("setTransInfo marshal failed.")
	}

	var transKey = t.getTransInfoKey(stub, info.TransLvl, seqLvl)
	err = t.PutState_Ex(stub, transKey, transJson)
	if err != nil {
		mylog.Error("setTransInfo PutState failed. err=%s", err)
		return errors.New("setTransInfo PutState failed.")
	}

	//交易信息设置成功后，保存序列号
	err = t.setTransSeq(stub, t.getGlobTransSeqKey(stub), seqGlob)
	if err != nil {
		mylog.Error("setTransInfo setTransSeq failed. err=%s", err)
		return errors.New("setTransInfo setTransSeq failed.")
	}
	err = t.setTransSeq(stub, t.getTransSeqKey(stub, info.TransLvl), seqLvl)
	if err != nil {
		mylog.Error("setTransInfo setTransSeq failed. err=%s", err)
		return errors.New("setTransInfo setTransSeq failed.")
	}

	/*
		if len(info.fId) > 0 {
			err = t.recordFIdTrans(stub, transKey, info.DfId)
			if err != nil {
				mylog.Error("setTransInfo recordDfIdTrans failed. err=%s", err)
				return errors.New("setTransInfo recordDfIdTrans failed.")
			}
		}
	*/

	mylog.Debug("setTransInfo OK, info=%v", info)

	return nil
}

//记录某个dfid下的交易
func (t *RACK) getDfIdTransKey(dfId string) string {
	return DFID_TX_PREFIX + dfId
}
func (t *RACK) recordFIdTrans(stub shim.ChaincodeStubInterface, transKey, dfId string) error {

	recdB, err := stub.GetState(t.getDfIdTransKey(dfId))
	if err != nil {
		mylog.Error("recordDfIdTrans GetState failed. err=%s", err)
		return errors.New("recordDfIdTrans GetState failed.")
	}

	var transList []string
	if recdB == nil {
		transList = append(transList, transKey)
	} else {
		err = json.Unmarshal(recdB, &transList)
		if err != nil {
			mylog.Error("recordDfIdTrans Unmarshal failed. err=%s", err)
			return errors.New("recordDfIdTrans Unmarshal failed.")
		}
		transList = append(transList, transKey)
	}

	recdJson, err := json.Marshal(transList)
	if err != nil {
		mylog.Error("recordDfIdTrans Marshal failed. err=%s", err)
		return errors.New("recordDfIdTrans Marshal failed.")
	}

	err = t.PutState_Ex(stub, t.getDfIdTransKey(dfId), recdJson)
	if err != nil {
		mylog.Error("recordDfIdTrans PutState failed. err=%s", err)
		return errors.New("recordDfIdTrans PutState failed.")
	}

	return nil
}

func (t *RACK) getTransInfo(stub shim.ChaincodeStubInterface, key string) (*Transaction, error) {
	var err error
	var trans Transaction

	tmpState, err := stub.GetState(key)
	if err != nil {
		mylog.Error("getTransInfo GetState failed.err=%s", err)
		return nil, err
	}
	if tmpState == nil {
		mylog.Error("getTransInfo GetState nil.")
		return nil, errors.New("getTransInfo GetState nil.")
	}

	err = json.Unmarshal(tmpState, &trans)
	if err != nil {
		mylog.Error("getTransInfo Unmarshal failed. err=%s", err)
		return nil, errors.New("getTransInfo Unmarshal failed.")
	}

	mylog.Debug("getTransInfo OK, info=%v", trans)

	return &trans, nil
}
func (t *RACK) getQueryTransInfo(stub shim.ChaincodeStubInterface, key string) (*PublicTrans, error) {
	var err error
	var trans PublicTrans

	tmpState, err := stub.GetState(key)
	if err != nil {
		mylog.Error("getQueryTransInfo GetState failed.err=%s", err)
		return nil, err
	}
	if tmpState == nil {
		mylog.Error("getQueryTransInfo GetState nil.")
		return nil, errors.New("getQueryTransInfo GetState nil.")
	}

	err = json.Unmarshal(tmpState, &trans)
	if err != nil {
		mylog.Error("getQueryTransInfo Unmarshal failed. err=%s", err)
		return nil, errors.New("getQueryTransInfo Unmarshal failed.")
	}

	mylog.Debug("getQueryTransInfo OK, info=%v", trans)

	return &trans, nil
}

func (t *RACK) getFinacInfoKey(fiId string) string {
	return FIID_PREFIX + fiId
}
func (t *RACK) getRackInfoKey(rId string) string {
	return RACKID_PREFIX + rId
}
func (t *RACK) getRackFinacInfoKey(rackId, finacId string) string {
	return RACKFIID_PREFIX + rackId + "_" + finacId
}

func (t *RACK) PutState_Ex(stub shim.ChaincodeStubInterface, key string, value []byte) error {
	//当key为空字符串时，0.6的PutState接口不会报错，但是会导致chainCode所在的contianer异常退出。
	if key == "" {
		mylog.Error("myPutState key err.")
		return errors.New("myPutState key err.")
	}
	return stub.PutState(key, value)
}

func (t *RACK) Contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}

	return false
}

func main() {
	// for debug
	mylog.SetDefaultLvl(0)

	primitives.SetSecurityLevel("SHA3", 256)

	err := shim.Start(new(RACK))
	if err != nil {
		fmt.Printf("Error starting EventSender chaincode: %s", err)
	}
}
