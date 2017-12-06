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

var mylog = InitMylog("frt")

const (
	ENT_CENTERBANK = 1
	ENT_COMPANY    = 2
	ENT_PROJECT    = 3
	ENT_PERSON     = 4

	ATTR_USRROLE = "usrrole"
	ATTR_USRNAME = "usrname"
	ATTR_USRTYPE = "usertype"

	TRANSSEQ_PREFIX      = "__~!@#@!~_frt_transSeqPre__"         //序列号生成器的key的前缀。使用的是worldState存储
	TRANSINFO_PREFIX     = "__~!@#@!~_frt_transInfoPre__"        //交易信息的key的前缀。使用的是worldState存储
	CENTERBANK_ACC_KEY   = "__~!@#@!~_frt_centerBankAccKey__#$%" //央行账户的key。使用的是worldState存储
	ALL_ACC_KEY          = "__~!@#@!~_frt_allAccInfo__#$%"       //存储所有账户名的key。使用的是worldState存储
	ONE_ACC_TRANS_PREFIX = "__~!@#@!~_frt_oneAccTransPre__"      //存储单个用户的交易的key前缀

	ALL_ACC_DELIM = ':' //所有账户名的分隔符
)

//账户信息Entity
// 一系列ID（或账户）都定义为字符串类型。因为putStat函数的第一个参数为字符串类型，这些ID（或账户）都作为putStat的第一个参数；另外从SDK传过来的参数也都是字符串类型。
type Entity struct {
	EntID       string `json:"id"`          //银行/企业/项目/个人ID
	EntType     int    `json:"entType"`     //类型 中央银行:1, 企业:2, 项目:3, 个人:4
	TotalAmount int64  `json:"totalAmount"` //货币总数额(发行或接收)
	RestAmount  int64  `json:"restAmount"`  //账户余额
	User        string `json:"user"`        //该实例所属的用户
	Time        int64  `json:"time"`        //开户时间
	Cert        []byte `json:"cert"`        //证书
}

type UserAttrs struct {
	UserRole string `json:"role"`
	UserName string `json:"name"`
	UserType string `json:"type"`
}

//供查询的交易内容
type QueryTrans struct {
	Serial int64 `json:"ser"` //交易序列号，返回给查询结果用，储存时
	PubTrans
}

type PubTrans struct {
	FromID       string `json:"fid"`   //发送方ID
	ToID         string `json:"tid"`   //接收方ID
	Amount       int64  `json:"amt"`   //交易数额
	TransType    string `json:"trstp"` //交易类型，前端传入，透传
	TxID         string `json:"txid"`  //交易ID
	Time         int64  `json:"time"`  //交易时间
	GlobalSerial int64  `json:"gser"`  //全局交易序列号
}

//交易内容  注意，QueryTrans中的字段名（包括json字段名）不能和Transaction中的字段名重复，否则解析会出问题
type Transaction struct {
	PubTrans
	FromType int    `json:"fromtype"` //发送方角色 centerBank:1, 企业:2, 项目:3
	ToType   int    `json:"totype"`   //接收方角色 企业:2, 项目:3
	TransLvl uint64 `json:"transLvl"` //交易级别
}

//查询的对账信息
type QueryBalance struct {
	IssueAmount  int64  `json:"issueAmount"`  //市面上发行货币的总量
	AccCount     int64  `json:"accCount"`     //所有账户的总量
	AccSumAmount int64  `json:"accSumAmount"` //所有账户的货币的总量
	Message      string `json:"message"`      //对账附件信息
}

type FRT struct {
}

func (t *FRT) Init(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	mylog.Debug("Enter Init")
	mylog.Debug("func =%s, args = %v", function, args)

	return nil, nil
}

// Transaction makes payment of X units from A to B
func (t *FRT) Invoke(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	mylog.Debug("Enter Invoke")
	mylog.Debug("func =%s, args = %v", function, args)
	var err error

	var fixedArgCount = 3
	if len(args) < fixedArgCount {
		mylog.Error("Invoke miss arg, got %d, at least need %d.", len(args), fixedArgCount)
		return nil, errors.New("Invoke miss arg.")
	}

	var userName = args[0]
	var accName = args[1]
	var times int64 = 0

	times, err = strconv.ParseInt(args[2], 0, 64)
	if err != nil {
		mylog.Error("Invoke convert times(%s) failed. err=%s", args[2], err)
		return nil, errors.New("Invoke convert times failed.")
	}

	var userAttrs *UserAttrs
	var userEnt *Entity = nil

	userAttrs, err = t.getUserAttrs(stub)
	if err != nil {
		return nil, mylog.Errorf("Invoke getUserAttrs failed. err=%s", err)
	}

	//开户时不需要校验
	if function != "account" && function != "accountCB" {

		userEnt, err = t.getEntity(stub, accName)
		if err != nil {
			return nil, mylog.Errorf("Invoke getEntity failed. err=%s", err)
		}

		//校验修改Entity的用户身份，只有Entity的所有者才能修改自己的Entity
		if ok, _ := t.verifyIdentity(stub, userEnt, userAttrs); !ok {
			fmt.Println("verify and account(%s) failed. \n", accName)
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

		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			mylog.Error("Invoke(account) miss arg, got %d, at least need %d.", len(args), argCount)
			return nil, errors.New("Invoke(account) miss arg.")
		}

		userCert, err := base64.StdEncoding.DecodeString(args[fixedArgCount])
		if err != nil {
			mylog.Error("Invoke(account) DecodeString failed. err=%s", err)
			return nil, errors.New("Invoke(account) DecodeString failed.")
		}

		usrType = 0

		return t.newAccount(stub, accName, usrType, userName, userCert, times, false)

	} else if function == "accountCB" {
		mylog.Debug("Enter accountCB")
		var usrType int = 0

		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			mylog.Error("Invoke(accountCB) miss arg, got %d, at least need %d.", len(args), argCount)
			return nil, errors.New("Invoke(accountCB) miss arg.")
		}

		userCert, err := base64.StdEncoding.DecodeString(args[fixedArgCount])
		if err != nil {
			mylog.Error("Invoke(accountCB) DecodeString failed. err=%s", err)
			return nil, errors.New("Invoke(accountCB) DecodeString failed.")
		}

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

		_, err = t.newAccount(stub, accName, usrType, userName, userCert, times, true)
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
			mylog.Error("convert issueAmount(%s) failed. err=%s", args[fixedArgCount+2], err)
			return nil, errors.New("convert issueAmount failed.")
		}
		mylog.Debug("transAmount= %v", transAmount)

		return t.transferCoin(stub, accName, toAcc, transType, transAmount, times)

	} else if function == "updateCert" {
		//为了以防万一，加上更新用户cert的功能
		var argCount = fixedArgCount + 2
		if len(args) < argCount {
			return nil, mylog.Errorf("Invoke(updateCert) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		var upAcc = args[fixedArgCount]
		cert, err := base64.StdEncoding.DecodeString(args[fixedArgCount+1])
		if err != nil {
			return nil, mylog.Errorf("Invoke(updateCert) DecodeString failed. err=%s", err)
		}

		upEnt, err := t.getEntity(stub, upAcc)
		if err != nil {
			return nil, mylog.Errorf("Invoke(updateCert) getEntity failed. err=%s", err)
		}

		upEnt.Cert = cert

		err = t.setEntity(stub, upEnt)
		if err != nil {
			return nil, mylog.Errorf("Invoke(updateCert) setEntity  failed. err=%s", err)
		}

		return nil, nil
	}

	//event
	stub.SetEvent("success", []byte("invoke success"))
	return nil, errors.New("unknown Invoke.")
}

// Query callback representing the query of a chaincode
func (t *FRT) Query(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	mylog.Debug("Enter Query")
	mylog.Debug("func =%s, args = %v", function, args)

	var err error

	var fixedArgCount = 2
	if len(args) < fixedArgCount {
		return nil, mylog.Errorf("Query miss arg, got %d, at least need %d.", len(args), fixedArgCount)
	}

	//var userName = args[0]
	var accName = args[1]

	var userAttrs *UserAttrs
	var userEnt *Entity = nil

	userAttrs, err = t.getUserAttrs(stub)
	if err != nil {
		return nil, mylog.Errorf("Query getUserAttrs failed. err=%s", err)
	}

	userEnt, err = t.getEntity(stub, accName)
	if err != nil {
		return nil, mylog.Errorf("Query getEntity failed. err=%s", err)
	}

	//校验用户身份
	if ok, _ := t.verifyIdentity(stub, userEnt, userAttrs); !ok {
		return nil, mylog.Errorf("Query user and account check failed.")
	}

	//获取管理员帐号(央行账户作为管理员帐户)
	tmpByte, err := t.getCenterBankAcc(stub)
	if err != nil {
		return nil, mylog.Errorf("Query getCenterBankAcc failed. err=%s", err)
	}
	//如果没有央行账户，报错。
	if tmpByte == nil {
		return nil, mylog.Errorf("Query getCenterBankAcc nil.")
	}

	var adminAcc string = string(tmpByte)

	if function == "query" {

		var queryEntity *Entity
		queryEntity, err = t.getEntity(stub, accName)
		if err != nil {
			mylog.Error("getEntity queryEntity(id=%s) failed. err=%s", accName, err)
			return nil, err
		}
		mylog.Debug("queryEntity=%v", queryEntity)

		retValue := []byte(strconv.FormatInt(queryEntity.RestAmount, 10))
		mylog.Debug("retValue=%v, %s", retValue, string(retValue))

		return retValue, nil
	} else if function == "queryTx" {
		var argCount = fixedArgCount + 6
		if len(args) < argCount {
			mylog.Error("queryTx miss arg, got %d, need %d.", len(args), argCount)
			return nil, errors.New("queryTx miss arg.")
		}

		var begSeq int64
		var txCount int64
		var transLvl uint64
		var begTime int64
		var endTime int64
		var txAcc string

		begSeq, err = strconv.ParseInt(args[fixedArgCount], 0, 64)
		if err != nil {
			mylog.Error("queryTx ParseInt for begSeq(%s) failed. err=%s", args[fixedArgCount], err)
			return nil, errors.New("queryTx parse begSeq failed.")
		}
		txCount, err = strconv.ParseInt(args[fixedArgCount+1], 0, 64)
		if err != nil {
			mylog.Error("queryTx ParseInt for endSeq(%s) failed. err=%s", args[fixedArgCount+1], err)
			return nil, errors.New("queryTx parse endSeq failed.")
		}

		transLvl, err = strconv.ParseUint(args[fixedArgCount+2], 0, 64)
		if err != nil {
			mylog.Error("queryTx ParseInt for transLvl(%s) failed. err=%s", args[fixedArgCount+2], err)
			return nil, errors.New("queryTx parse transLvl failed.")
		}

		begTime, err = strconv.ParseInt(args[fixedArgCount+3], 0, 64)
		if err != nil {
			return nil, mylog.Errorf("queryTx ParseInt for begTime(%s) failed. err=%s", args[fixedArgCount+3], err)
		}
		endTime, err = strconv.ParseInt(args[fixedArgCount+4], 0, 64)
		if err != nil {
			return nil, mylog.Errorf("queryTx ParseInt for endTime(%s) failed. err=%s", args[fixedArgCount+4], err)
		}

		//查询指定账户的交易记录
		txAcc = args[fixedArgCount+5]

		//如果没指定查询某个账户的交易，则返回所有的交易记录
		if len(txAcc) == 0 {
			//是否是管理员帐户，管理员用户才可以查所有交易记录
			if adminAcc != accName {
				return nil, mylog.Errorf("%s can't query tx info.", accName)
			}
			return t.queryTransInfos(stub, transLvl, begSeq, txCount, begTime, endTime)
		} else {
			//管理员用户 或者 用户自己才能查询某用户的交易记录
			if adminAcc != accName && accName != txAcc {
				return nil, mylog.Errorf("%s can't query %s's tx info.", accName, txAcc)
			}
			return t.queryAccTransInfos(stub, txAcc, begSeq, txCount, begTime, endTime)
		}

	} else if function == "queryBalance" {
		//是否是管理员帐户，管理员用户才可以查
		if adminAcc != accName {
			return nil, mylog.Errorf("%s can't query balance.", accName)
		}

		return t.queryBalance(stub)
	} else if function == "queryState" {
		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			mylog.Error("queryState miss arg, got %d, need %d.", len(args), argCount)
			return nil, errors.New("queryState miss arg.")
		}

		//是否是管理员帐户，管理员用户才可以查
		if adminAcc != accName {
			return nil, mylog.Errorf("%s can't query state.", accName)
		}

		key := args[fixedArgCount]

		retValues, err := stub.GetState(key)
		if err != nil {
			mylog.Error("queryState GetState failed. err=%s", err)
			return nil, errors.New("queryState GetState failed.")
		}

		return retValues, nil
	} else if function == "queryAcc" {
		accExist, err := t.isEntityExists(stub, accName)
		if err != nil {
			mylog.Error("queryAcc: isEntityExists (id=%s) failed. err=%s", accName, err)
			return nil, errors.New("queryAcc: isEntityExists failed.")
		}

		var retValues []byte
		if accExist {
			retValues = []byte("1")
		} else {
			retValues = []byte("0")
		}

		return retValues, nil
	}

	return nil, errors.New("unknown function.")
}

func (t *FRT) queryTransInfos(stub shim.ChaincodeStubInterface, transLvl uint64, begIdx, count, begTime, endTime int64) ([]byte, error) {
	var maxSeq int64
	var err error
	var retTransInfo []byte = []byte("[]") //返回的结果是一个json数组，所以如果结果为空，返回空数组

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
	var seqKey = t.getGlobTransSeqKey(stub)
	test, err := stub.GetState(seqKey)
	if err != nil {
		mylog.Error("queryTransInfos GetState failed. err=%s", err)
		return nil, errors.New("queryTransInfos GetState failed.")
	}
	if test == nil {
		mylog.Info("no trans saved.")
		return retTransInfo, nil
	}

	//先获取当前最大的序列号
	maxSeq, err = t.getTransSeq(stub, seqKey)
	if err != nil {
		mylog.Error("queryTransInfos getTransSeq failed. err=%s", err)
		return nil, errors.New("queryTransInfos getTransSeq failed.")
	}

	if begIdx > maxSeq {
		mylog.Warn("queryTransInfos nothing to do(%d,%d).", begIdx, maxSeq)
		return retTransInfo, nil
	}

	if count < 0 {
		count = maxSeq - begIdx + 1
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
	var transArr []QueryTrans = []QueryTrans{} //初始化为空，即使下面没查到数据也会返回'[]'
	var loopCnt int64 = 0
	var qTrans QueryTrans
	for loop := begIdx; loop <= maxSeq; loop++ {
		//处理了count条时，不再处理
		if loopCnt >= count {
			break
		}

		trans, err := t.getTransInfo(stub, t.getTransInfoKey(stub, loop))
		if err != nil {
			mylog.Error("getTransInfo getQueryTransInfo(idx=%d) failed.err=%s", loop, err)
			continue
		}
		//取匹配的transLvl
		if trans.TransLvl&transLvl != 0 && trans.Time >= begTime && trans.Time <= endTime {
			qTrans.Serial = trans.GlobalSerial
			qTrans.PubTrans = trans.PubTrans
			transArr = append(transArr, qTrans)
			loopCnt++
		}
	}

	retTransInfo, err = json.Marshal(transArr)
	if err != nil {
		return nil, mylog.Errorf("getTransInfo Marshal failed.err=%s", err)
	}

	return retTransInfo, nil
}

func (t *FRT) queryAccTransInfos(stub shim.ChaincodeStubInterface, accName string, begIdx, count, begTime, endTime int64) ([]byte, error) {
	var retTransInfo []byte = []byte("[]") //返回的结果是一个json数组，所以如果结果为空，返回空数组

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

	infoB, err := stub.GetState(t.getOneAccTransKey(accName))
	if err != nil {
		return nil, mylog.Errorf("queryAccTransInfos(%s) GetState failed.err=%s", accName, err)
	}
	if infoB == nil {
		return retTransInfo, nil
	}
	var allTransList []string
	err = json.Unmarshal(infoB, &allTransList)
	if err != nil {
		return nil, mylog.Errorf("queryAccTransInfos(%s) Unmarshal failed.err=%s", accName, err)
	}

	//begIdx是从1开始，数组从0开始
	begIdx = begIdx - 1
	var allLen = int64(len(allTransList))
	if begIdx >= allLen {
		mylog.Warn("queryAccTransInfos(%s) nothing to do(%d,%d).", accName, begIdx, allLen)
		return retTransInfo, nil
	}
	if count < 0 {
		count = allLen - begIdx
	}

	var transArr []QueryTrans = []QueryTrans{} //初始化为空，即使下面没查到数据也会返回'[]'
	var qTrans QueryTrans
	var loopCnt int64 = 0
	for i := begIdx; i < allLen; i++ {
		if loopCnt >= count {
			break
		}
		trans, err := t.getQueryTransInfo(stub, allTransList[i])
		if err != nil {
			mylog.Error("queryAccTransInfos(%s) getQueryTransInfo failed, err=%s.", accName, err)
			continue
		}
		if trans.Time >= begTime && trans.Time <= endTime {
			qTrans.Serial = int64(i + 1)
			qTrans.PubTrans = trans.PubTrans
			transArr = append(transArr, qTrans)
			loopCnt++
		}
	}

	retTransInfo, err = json.Marshal(transArr)
	if err != nil {
		return nil, mylog.Errorf("getTransInfo Marshal failed.err=%s", err)
	}

	return retTransInfo, nil
}

func (t *FRT) queryBalance(stub shim.ChaincodeStubInterface) ([]byte, error) {
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

func (t *FRT) verifySign(stub shim.ChaincodeStubInterface, certificate []byte) (bool, error) {
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

	mylog.Debug("passed certificate [% x]", certificate)
	mylog.Debug("passed sigma [% x]", sigma)
	mylog.Debug("passed payload [% x]", payload)
	mylog.Debug("passed binding [% x]", binding)

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

func (t *FRT) verifyIdentity(stub shim.ChaincodeStubInterface, ent *Entity, attrs *UserAttrs) (bool, error) {
	//有时获取不到attr，这里做个判断
	if len(attrs.UserName) > 0 && ent.User != attrs.UserName {
		mylog.Errorf("verifyIdentity: user check failed(%s,%s).", ent.User, attrs.UserName)
		return false, mylog.Errorf("verifyIdentity: user check failed(%s,%s).", ent.User, attrs.UserName)
	}

	ok, err := t.verifySign(stub, ent.Cert)
	if err != nil {
		return false, mylog.Errorf("verifyIdentity: verifySign error, err=%s.", err)
	}
	if !ok {
		return false, mylog.Errorf("verifyIdentity: verifySign failed.")
	}

	return true, nil
}

func (t *FRT) getUserAttrs(stub shim.ChaincodeStubInterface) (*UserAttrs, error) {
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

	var attrs UserAttrs
	attrs.UserName = string(tmpName)
	attrs.UserRole = string(tmpRole)
	attrs.UserType = string(tmpType)

	return &attrs, nil
}

func (t *FRT) getEntity(stub shim.ChaincodeStubInterface, entName string) (*Entity, error) {
	var centerBankByte []byte
	var cb Entity
	var err error

	centerBankByte, err = stub.GetState(entName)
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

func (t *FRT) isEntityExists(stub shim.ChaincodeStubInterface, entName string) (bool, error) {
	var centerBankByte []byte
	var err error

	centerBankByte, err = stub.GetState(entName)
	if err != nil {
		return false, err
	}

	if centerBankByte == nil {
		return false, nil
	}

	return true, nil
}

//央行数据写入
func (t *FRT) setEntity(stub shim.ChaincodeStubInterface, cb *Entity) error {

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

//发行frt
func (t *FRT) issueCoin(stub shim.ChaincodeStubInterface, cbID string, issueAmount, times int64) ([]byte, error) {
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

	t.recordTranse(stub, &fromEntity, cb, "issue", issueAmount, times)

	return nil, nil
}

//frt转账
func (t *FRT) transferCoin(stub shim.ChaincodeStubInterface, from, to, transType string, amount, times int64) ([]byte, error) {
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
		mylog.Error("getEntity(id=%s) failed. err=%s", from, err)
		return nil, errors.New("getEntity from failed.")
	}
	toEntity, err = t.getEntity(stub, to)
	if err != nil {
		mylog.Error("getEntity(id=%s) failed. err=%s", to, err)
		return nil, errors.New("getEntity to failed.")
	}

	if fromEntity.RestAmount < amount {
		mylog.Error("fromEntity(id=%s) restAmount not enough.", from)
		return nil, errors.New("fromEntity restAmount not enough.")
	}

	mylog.Debug("fromEntity before= %v", fromEntity)
	mylog.Debug("toEntity before= %v", toEntity)

	fromEntity.RestAmount -= amount
	toEntity.RestAmount += amount
	toEntity.TotalAmount += amount

	mylog.Debug("fromEntity after= %v", fromEntity)
	mylog.Debug("toEntity after= %v", toEntity)

	err = t.setEntity(stub, fromEntity)
	if err != nil {
		mylog.Error("setEntity of fromEntity(id=%s) failed. err=%s", from, err)
		return nil, errors.New("setEntity of from failed.")
	}
	err = t.setEntity(stub, toEntity)
	if err != nil {
		mylog.Error("setEntity of toEntity(id=%s) failed. err=%s", to, err)
		return nil, errors.New("setEntity of to failed.")
	}

	err = t.recordTranse(stub, fromEntity, toEntity, transType, amount, times)

	return nil, err
}

const (
	TRANS_LVL_CB   = 1
	TRANS_LVL_COMM = 2
)

//记录交易。目前交易分为两种：一种是和央行打交道的，包括央行发行货币、央行给项目或企业转帐，此类交易普通用户不能查询；另一种是项目、企业、个人间互相转账，此类交易普通用户能查询
func (t *FRT) recordTranse(stub shim.ChaincodeStubInterface, fromEnt, toEnt *Entity, transType string, amount, times int64) error {
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

	var transLevel uint64 = TRANS_LVL_COMM
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

func (t *FRT) checkAccountName(accName string) error {
	//会用':'作为分隔符分隔多个账户名，所以账户名不能含有':'
	var invalidChars string = string(ALL_ACC_DELIM)

	if strings.ContainsAny(accName, invalidChars) {
		mylog.Error("isAccountNameValid (acc=%s) failed.", accName)
		return fmt.Errorf("accName '%s' can not contains '%s'.", accName, invalidChars)
	}
	return nil
}

func (t *FRT) saveAccountName(stub shim.ChaincodeStubInterface, accName string) error {
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

func (t *FRT) getAllAccountNames(stub shim.ChaincodeStubInterface) ([]byte, error) {
	accB, err := stub.GetState(ALL_ACC_KEY)
	if err != nil {
		mylog.Error("getAllAccountNames GetState failed.err=%s", err)
		return nil, err
	}
	return accB, nil
}

func (t *FRT) newAccount(stub shim.ChaincodeStubInterface, accName string, accType int, userName string, cert []byte, times int64, isCBAcc bool) ([]byte, error) {
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
	ent.User = userName
	ent.Cert = cert

	err = t.setEntity(stub, &ent)
	if err != nil {
		mylog.Error("openAccount setEntity (id=%s) failed. err=%s", accName, err)
		return nil, errors.New("openAccount setEntity failed.")
	}

	mylog.Debug("openAccount success: %v", ent)

	//央行账户此处不保存
	if !isCBAcc {
		err = t.saveAccountName(stub, accName)
	}

	return nil, err
}

var centerBankAccCache []byte = nil

func (t *FRT) setCenterBankAcc(stub shim.ChaincodeStubInterface, acc string) error {
	err := t.PutState_Ex(stub, CENTERBANK_ACC_KEY, []byte(acc))
	if err != nil {
		mylog.Error("setCenterBankAcc PutState failed.err=%s", err)
		return err
	}

	centerBankAccCache = []byte(acc)

	return nil
}
func (t *FRT) getCenterBankAcc(stub shim.ChaincodeStubInterface) ([]byte, error) {
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

func (t *FRT) getTransSeq(stub shim.ChaincodeStubInterface, transSeqKey string) (int64, error) {
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
func (t *FRT) setTransSeq(stub shim.ChaincodeStubInterface, transSeqKey string, seq int64) error {
	err := t.PutState_Ex(stub, transSeqKey, []byte(strconv.FormatInt(seq, 10)))
	if err != nil {
		mylog.Error("setTransSeq PutState failed.err=%s", err)
		return err
	}

	return nil
}

func (t *FRT) getTransInfoKey(stub shim.ChaincodeStubInterface, seq int64) string {
	var buf = bytes.NewBufferString(TRANSINFO_PREFIX)
	buf.WriteString("_")
	buf.WriteString(strconv.FormatInt(seq, 10))
	return buf.String()
}

/*
func (t *FRT) getTransSeqKey(stub shim.ChaincodeStubInterface, transLvl int) string {
	var buf = bytes.NewBufferString(TRANSSEQ_PREFIX)
	buf.WriteString("lvl")
	buf.WriteString(strconv.Itoa(transLvl))
	buf.WriteString("_")
	return buf.String()
}
*/
func (t *FRT) getGlobTransSeqKey(stub shim.ChaincodeStubInterface) string {
	return TRANSSEQ_PREFIX + "global_"
}

func (t *FRT) setTransInfo(stub shim.ChaincodeStubInterface, info *Transaction) error {
	//先获取全局seq
	seqGlob, err := t.getTransSeq(stub, t.getGlobTransSeqKey(stub))
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
		mylog.Error("setTransInfo marshal failed. err=%s", err)
		return errors.New("setTransInfo marshal failed.")
	}

	putKey := t.getTransInfoKey(stub, seqGlob)
	err = t.PutState_Ex(stub, putKey, transJson)
	if err != nil {
		mylog.Error("setTransInfo PutState failed. err=%s", err)
		return errors.New("setTransInfo PutState failed.")
	}

	//from和to账户都记录一次
	err = t.setOneAccTransInfo(stub, info.FromID, putKey)
	if err != nil {
		return mylog.Errorf("setTransInfo setOneAccTransInfo(%s) failed. err=%s", info.FromID, err)
	}
	err = t.setOneAccTransInfo(stub, info.ToID, putKey)
	if err != nil {
		return mylog.Errorf("setTransInfo setOneAccTransInfo(%s) failed. err=%s", info.ToID, err)
	}

	//交易信息设置成功后，保存序列号
	err = t.setTransSeq(stub, t.getGlobTransSeqKey(stub), seqGlob)
	if err != nil {
		mylog.Error("setTransInfo setTransSeq failed. err=%s", err)
		return errors.New("setTransInfo setTransSeq failed.")
	}
	/*
		err = t.setTransSeq(stub, t.getTransSeqKey(stub, info.TransLvl), seqLvl)
		if err != nil {
			mylog.Error("setTransInfo setTransSeq failed. err=%s", err)
			return errors.New("setTransInfo setTransSeq failed.")
		}
	*/

	mylog.Debug("setTransInfo OK, info=%v", info)

	return nil
}

func (t *FRT) getOneAccTransKey(accName string) string {
	return ONE_ACC_TRANS_PREFIX + accName
}

func (t *FRT) setOneAccTransInfo(stub shim.ChaincodeStubInterface, accName, transKey string) error {
	var accTransKey = t.getOneAccTransKey(accName)

	tmpState, err := stub.GetState(accTransKey)
	if err != nil {
		return mylog.Errorf("setOneAccTransInfo GetState(%s) failed.err=%s", accName, err)
	}

	var transList []string
	if tmpState != nil {
		err = json.Unmarshal(tmpState, &transList)
		if err != nil {
			return mylog.Errorf("setOneAccTransInfo Unmarshal(%s) failed.err=%s", accName, err)
		}
	}

	transList = append(transList, transKey)

	jsonList, err := json.Marshal(transList)
	if err != nil {
		return mylog.Errorf("setOneAccTransInfo Marshal(%s) failed.err=%s", accName, err)
	}

	err = t.PutState_Ex(stub, accTransKey, jsonList)
	if err != nil {
		return mylog.Errorf("setOneAccTransInfo PutState_Ex(%s) failed.err=%s", accName, err)
	}

	return nil
}

func (t *FRT) getTransInfo(stub shim.ChaincodeStubInterface, key string) (*Transaction, error) {
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
func (t *FRT) getQueryTransInfo(stub shim.ChaincodeStubInterface, key string) (*QueryTrans, error) {
	var err error
	var trans QueryTrans

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

func (t *FRT) PutState_Ex(stub shim.ChaincodeStubInterface, key string, value []byte) error {
	//当key为空字符串时，0.6的PutState接口不会报错，但是会导致chainCode所在的contianer异常退出。
	if key == "" {
		mylog.Error("PutState_Ex key err.")
		return errors.New("PutState_Ex key err.")
	}
	return stub.PutState(key, value)
}

func main() {
	// for debug
	mylog.SetDefaultLvl(0)

	primitives.SetSecurityLevel("SHA3", 256)

	err := shim.Start(new(FRT))
	if err != nil {
		fmt.Printf("Error starting EventSender chaincode: %s", err)
	}
}
