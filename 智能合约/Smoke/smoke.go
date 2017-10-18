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

var mylog = InitMylog("smk")

const (
	ENT_CENTERBANK = 1
	ENT_COMPANY    = 2
	ENT_PROJECT    = 3
	ENT_PERSON     = 4

	ATTR_ROLE    = "role"
	ATTR_USRNAME = "usrname"
	ATTR_USRTYPE = "usertype"

	TRANSSEQ_PREFIX    = "__~!@#@!~_smk_transSeqPre__"         //序列号生成器的key的前缀。使用的是worldState存储
	TRANSINFO_PREFIX   = "__~!@#@!~_smk_transInfoPre__"        //交易信息的key的前缀。使用的是worldState存储
	DFID_PREFIX        = "__~!@#@!~_smk_dfIdPre__"             //欠款融资id的key的前缀。使用的是worldState存储
	DFID_TX_PREFIX     = "__~!@#@!~_smk_dfIdTxPre__"           //欠款融资id交易信息的key的前缀。使用的是worldState存储
	CENTERBANK_ACC_KEY = "__~!@#@!~_smk_centerBankAccKey__#$%" //央行账户的key。使用的是worldState存储
	ALL_ACC_KEY        = "__~!@#@!~_smk_allAccInfo__#$%"       //存储所有账户名的key。使用的是worldState存储

	ALL_ACC_DELIM = ':' //所有账户名的分隔符

	TRANS_LVL_CB   = 1 //交易级别，银行
	TRANS_LVL_COMM = 2 //交易级别，普通
)

//账户信息Entity
// 一系列ID（或账户）都定义为字符串类型。因为putStat函数的第一个参数为字符串类型，这些ID（或账户）都作为putStat的第一个参数；另外从SDK传过来的参数也都是字符串类型。
type Entity struct {
	EntID       string `json:"id"`          //银行/企业/项目/个人ID
	EntDesc     string `json:"desc"`        //描述信息
	EntType     int    `json:"entType"`     //类型 中央银行:1, 企业:2, 项目:3, 个人:4
	TotalAmount int64  `json:"totalAmount"` //货币总数额(发行或接收)
	RestAmount  int64  `json:"restAmount"`  //账户余额
	User        string `json:"user"`        //该实例所属的用户
	Time        int64  `json:"time"`        //创建时间

	DFIdMap map[string]int64 `json:"dfIdMap"` //贷款融资id的map，DFId作为子账户
}

//查询账户信息
type QueryEntity struct {
	TotalAmount int64            `json:"totalAmount"` //注意，这里指的是账户所有余额，即Entity中RestAmount
	DFIdMap     map[string]int64 `json:"dfIdMap"`     //贷款融资id的map，DFId作为子账户
}

//供查询的交易内容
type QueryTrans struct {
	Serial    int64  `json:"serial"`    //全局交易序列号
	FromID    string `json:"fromid"`    //发送方ID
	ToID      string `json:"toid"`      //接收方ID
	Amount    int64  `json:"amount"`    //交易数额
	TransType string `json:"transtype"` //交易类型，前端传入，透传
	Time      int64  `json:"time"`      //交易时间
}

//交易内容  注意，QueryTrans中的字段名（包括json字段名）不能和Transaction中的字段名重复，否则解析会出问题
type Transaction struct {
	QueryTrans
	FromType     int    `json:"fromtype"`     //发送方角色 centerBank:1, 企业:2, 项目:3
	ToType       int    `json:"totype"`       //接收方角色 企业:2, 项目:3
	TxID         string `json:"txid"`         //交易ID
	TransLvl     int    `json:"transLvl"`     //交易级别
	GlobalSerial int64  `json:"globalSerial"` //全局交易序列号
	DfId         string `json:"dfid"`         //欠款融资id
}

type TraceInfo struct {
	Time     int64  `json:"time"`     //物流时间
	TraceMsg string `json:"tracemsg"` //物流信息
}

//查询的对账信息
type QueryBalance struct {
	IssueAmount  int64  `json:"issueAmount"`  //市面上发行货币的总量
	AccCount     int64  `json:"accCount"`     //所有账户的总量
	AccSumAmount int64  `json:"accSumAmount"` //所有账户的货币的总量
	Message      string `json:"message"`      //对账附件信息
}

type DebtFinance struct {
	DFID              string `json:"DFId"`        //欠款融资id
	DebtInfo          string `json:"debtInfo"`    //欠款信息
	DebtAmount        string `json:"debtAmt"`     //欠款金额。 不用运算，只是保存，所以用字符串类型
	DebtTime          int64  `json:"debtTime"`    //欠款时间
	DebtContractNo    string `json:"debtContNo"`  //欠款合同编号
	FinanceInfo       string `json:"finacInfo"`   //融资信息
	FinanceAmount     string `json:"finacAmt"`    //融资总额
	FinanceTime       int64  `json:"finacTime"`   //融资时间
	FinanceContractNo string `json:"finacContNo"` //融资合同编号
}

type DfidTransInfo struct {
	AccName string `json:"accName"`
	Amount  int64  `json:"amount"`
	Time    int64  `json:"time"`
}

type QueryDfidTransInfo struct {
	DebtFinance
	TransInfoList []DfidTransInfo `json:"transInfoList"`
}

type SMK struct {
}

//var centerBankId = 0xDE0B6B3A7640000
//var centerBankId = "10000000000000000000"

func (t *SMK) Init(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	mylog.Debug("Enter Init")
	mylog.Debug("func =%s, args = %v", function, args)

	/*
		var totalAmount int64
		var inited bool
		var err error

		inited, err = t.isEntityExists(stub, centerBankId)
		if err != nil {
			mylog.Error("check centerBank init failed. err=%s", err)
			return nil, errors.New("check centerBank init failed.")
		}
		//如果中央银行已经初始化过，则不必再初始化，否则可能会覆盖已有的值
		if inited {
			mylog.Warn("check centerBank init failed.")
			return nil, nil
		}

		totalAmount, err = strconv.ParseInt(args[0], 0, 64)
		if err != nil {
			mylog.Error("convert totalAmount(%s) failed. err=%s", args[0], err)
			return nil, errors.New("convert totalAmount failed.")
		}

		//var cb CenterBank
		var cb Entity
		cb.TotalAmount = totalAmount
		cb.RestAmount = totalAmount
		cb.EntID = centerBankId
		cb.EntType = ENT_CENTERBANK

		err = t.setEntity(stub, &cb)
		if err != nil {
			mylog.Error("setCenterBank failed. err=%s", err)
			return nil, errors.New("setCenterBank failed.")
		}
	*/

	return nil, nil
}

// Transaction makes payment of X units from A to B
func (t *SMK) Invoke(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
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
	var times int64 = 0
	var userCert []byte

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

		return t.openAccount(stub, accName, usrType, userName, times, false)

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

		_, err = t.openAccount(stub, accName, usrType, userName, times, true)
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
		var argCount = fixedArgCount + 4
		if len(args) < argCount {
			mylog.Error("Invoke(transefer) miss arg, got %d, at least need %d.", len(args), argCount)
			return nil, errors.New("Invoke(transefer) miss arg.")
		}

		var toAcc = args[fixedArgCount]
		var transType = args[fixedArgCount+1]
		var dfId = args[fixedArgCount+2]

		var transAmount int64
		transAmount, err = strconv.ParseInt(args[fixedArgCount+3], 0, 64)
		if err != nil {
			mylog.Error("convert issueAmount(%s) failed. err=%s", args[fixedArgCount+3], err)
			return nil, errors.New("convert issueAmount failed.")
		}
		mylog.Debug("transAmount= %v", transAmount)

		return t.transferCoin(stub, accName, toAcc, dfId, transType, transAmount, times)

	} else if function == "trace" {
		var argCount = fixedArgCount + 2
		if len(args) < argCount {
			mylog.Error("Invoke(transefer) miss arg, got %d, at least need %d.", len(args), argCount)
			return nil, errors.New("Invoke(transefer) miss arg.")
		}

		var wareId = args[fixedArgCount]
		var msg = args[fixedArgCount+1]

		return nil, t.recordTrace(stub, wareId, msg, times)
	} else if function == "debt" {
		var argCount = fixedArgCount + 4
		if len(args) < argCount {
			mylog.Error("Invoke(debt) miss arg, got %d, at least need %d.", len(args), argCount)
			return nil, errors.New("Invoke(debt) miss arg.")
		}

		var dfId = args[fixedArgCount]
		var debtInfo = args[fixedArgCount+1]
		var contractNo = args[fixedArgCount+2]
		var debtAmout = args[fixedArgCount+3]

		var dfInfoKey = t.getDfInfoKey(dfId)
		var dfB, dfJson []byte
		var df DebtFinance

		dfB, err = stub.GetState(dfInfoKey)
		if err != nil {
			mylog.Error("Invoke(debt) GetState(%s) failed. err=%s.", dfInfoKey, err)
			return nil, errors.New("Invoke(debt) GetState failed.")
		}
		if dfB != nil {
			mylog.Error("Invoke(debt) DebtFinance exists already.")
			return nil, errors.New("Invoke(debt) DebtFinance exists already.")
		}

		df.DFID = dfId
		df.DebtInfo = debtInfo
		df.DebtContractNo = contractNo
		df.DebtTime = times
		df.DebtAmount = debtAmout

		dfJson, err = json.Marshal(df)
		if err != nil {
			mylog.Error("Invoke(debt) Marshal failed. err=%s.", err)
			return nil, errors.New("Invoke(debt) Marshal failed.")
		}

		err = t.PutState_Ex(stub, dfInfoKey, dfJson)
		if err != nil {
			mylog.Error("Invoke(debt) PutState failed. err=%s.", err)
			return nil, errors.New("Invoke(debt) PutState failed.")
		}

		return nil, nil
	} else if function == "finance" {
		var argCount = fixedArgCount + 4
		if len(args) < argCount {
			mylog.Error("Invoke(finance) miss arg, got %d, at least need %d.", len(args), argCount)
			return nil, errors.New("Invoke(finance) miss arg.")
		}

		var dfId = args[fixedArgCount]
		var finacInfo = args[fixedArgCount+1]
		var contractNo = args[fixedArgCount+2]
		var finacAmout = args[fixedArgCount+3]

		var dfInfoKey = t.getDfInfoKey(dfId)
		var dfB, dfJson []byte
		var df DebtFinance

		dfB, err = stub.GetState(dfInfoKey)
		if err != nil {
			mylog.Error("Invoke(finance) GetState(%s) failed. err=%s.", dfInfoKey, err)
			return nil, errors.New("Invoke(finance) GetState failed.")
		}
		//如果没有供应商记入的欠款信息，则不能融资
		if dfB == nil {
			mylog.Error("Invoke(finance) DebtInfo not exists.")
			return nil, errors.New("Invoke(finance) DebtInfo not exists.")
		}

		err = json.Unmarshal(dfB, &df)
		if err != nil {
			mylog.Error("Invoke(finance) Unmarshal failed. err=%s.", err)
			return nil, errors.New("Invoke(finance) Unmarshal failed.")
		}

		//如果已发起过一次融资，则不能再次发起
		if df.FinanceTime > 0 {
			mylog.Error("Invoke(finance) finance already.")
			return nil, errors.New("Invoke(finance) finance already.")
		}

		df.FinanceInfo = finacInfo
		df.FinanceTime = times
		df.DebtContractNo = contractNo
		df.FinanceAmount = finacAmout

		dfJson, err = json.Marshal(df)
		if err != nil {
			mylog.Error("Invoke(finance) Marshal failed. err=%s.", err)
			return nil, errors.New("Invoke(finance) Marshal failed.")
		}

		err = t.PutState_Ex(stub, dfInfoKey, dfJson)
		if err != nil {
			mylog.Error("Invoke(finance) PutState failed. err=%s.", err)
			return nil, errors.New("Invoke(finance) PutState failed.")
		}

		return nil, nil
	}

	//event
	stub.SetEvent("success", []byte("invoke success"))
	return nil, errors.New("unknown Invoke.")
}

// Query callback representing the query of a chaincode
func (t *SMK) Query(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	mylog.Debug("Enter Query")
	mylog.Debug("func =%s, args = %v", function, args)

	if function == "query" {
		var argCount = 2
		if len(args) < argCount {
			mylog.Error("query miss arg, got %d, need %d.", len(args), argCount)
			return nil, errors.New("query miss arg.")
		}

		var err error
		var accName = args[0]
		var userName = args[1]

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
		var argCount = 4
		if len(args) < argCount {
			mylog.Error("queryTx miss arg, got %d, need %d.", len(args), argCount)
			return nil, errors.New("queryTx miss arg.")
		}

		var err error
		//var userName = args[0]
		var begSeq int64
		var endSeq int64
		var transLvl int

		begSeq, err = strconv.ParseInt(args[1], 0, 64)
		if err != nil {
			mylog.Error("queryTx ParseInt for begSeq(%s) failed. err=%s", args[1], err)
			return nil, errors.New("queryTx parse begSeq failed.")
		}
		endSeq, err = strconv.ParseInt(args[2], 0, 64)
		if err != nil {
			mylog.Error("queryTx ParseInt for endSeq(%s) failed. err=%s", args[2], err)
			return nil, errors.New("queryTx parse endSeq failed.")
		}

		//如果begSeq<=0，则从第一条开始；endSeq为-1，表示到最后一条记录
		if begSeq <= 0 {
			begSeq = 1 //本合约的交易记录从1开始编号的
		}
		if endSeq < 0 {
			endSeq = math.MaxInt64 //设置为Int64最大值
		}

		transLvl, err = strconv.Atoi(args[3])
		if err != nil {
			mylog.Error("queryTx ParseInt for transLvl(%s) failed. err=%s", args[3], err)
			return nil, errors.New("queryTx parse transLvl failed.")
		}

		return t.queryTransInfos(stub, transLvl, begSeq, endSeq)

	} else if function == "queryBalance" {
		return t.queryBalance(stub)
	} else if function == "queryState" {
		var argCount = 1
		if len(args) < argCount {
			mylog.Error("queryState miss arg, got %d, need %d.", len(args), argCount)
			return nil, errors.New("queryState miss arg.")
		}
		key := args[0]

		retValues, err := stub.GetState(key)
		if err != nil {
			mylog.Error("queryState GetState failed. err=%s", err)
			return nil, errors.New("queryState GetState failed.")
		}

		return retValues, nil
	} else if function == "queryTrace" {
		var argCount = 1
		if len(args) < argCount {
			mylog.Error("queryTrace miss arg, got %d, need %d.", len(args), argCount)
			return nil, errors.New("queryTrace miss arg.")
		}
		wareId := args[0]

		retValues, err := stub.GetState(wareId)
		if err != nil {
			mylog.Error("queryTrace GetState failed. err=%s", err)
			return nil, errors.New("queryTrace GetState failed.")
		}
		if retValues == nil {
			retValues = []byte("[]")
		} else {
			retValues = append([]byte("["), retValues...)
			retValues = append(retValues, ']')
		}

		return retValues, nil
	} else if function == "queryDfid" {
		var argCount = 1
		if len(args) < argCount {
			mylog.Error("queryDfid miss arg, got %d, need %d.", len(args), argCount)
			return nil, errors.New("queryDfid miss arg.")
		}
		dfId := args[0]

		//先获取DebtFinance的信息
		var qdf QueryDfidTransInfo
		dfB, err := stub.GetState(t.getDfInfoKey(dfId))
		if err != nil {
			mylog.Error("queryDfid GetState(DfInfo) failed. err=%s", err)
			return nil, errors.New("queryDfid GetState failed.")
		}
		if dfB == nil {
			mylog.Error("queryDfid no dfInfo for such id '%s'.", dfId)
			return nil, errors.New("queryDfid no dfInfo for such id.")
		}
		err = json.Unmarshal(dfB, &qdf.DebtFinance)
		if err != nil {
			mylog.Error("queryDfid Unmarshal(DebtFinance) failed. err=%s", err)
			return nil, errors.New("queryDfid Unmarshal failed.")
		}

		dftB, err := stub.GetState(t.getDfIdTransKey(dfId))
		if err != nil {
			mylog.Error("queryDfid GetState(DfTranseInfo) failed. err=%s", err)
			return nil, errors.New("queryDfid GetState failed.")
		}
		if dftB == nil {
			qdf.TransInfoList = []DfidTransInfo{}
		} else {
			var transkList []string
			err = json.Unmarshal(dftB, &transkList)
			if err != nil {
				mylog.Error("queryDfid Unmarshal(DfidTransInfo) failed. err=%s", err)
				return nil, errors.New("queryDfid Unmarshal failed.")
			}
			var transB []byte
			var transInfo Transaction
			for _, k := range transkList {
				transB, err = stub.GetState(k)
				if err != nil {
					mylog.Error("queryDfid GetState(TranseInfo) failed. err=%s", err)
					//return nil, errors.New("queryDfid GetState failed.")
					continue
				}
				if transB == nil {
					mylog.Error("queryDfid GetState(TranseInfo) for '%s' is nil.", k)
					continue
				}
				err = json.Unmarshal(transB, &transInfo)
				if err != nil {
					mylog.Error("queryDfid Unmarshal(TranseInfo) failed. err=%s", err)
					continue
				}
				qdf.TransInfoList = append(qdf.TransInfoList, DfidTransInfo{AccName: transInfo.ToID, Amount: transInfo.Amount, Time: transInfo.Time})
			}
		}

		retValues, err := json.Marshal(qdf)
		if err != nil {
			mylog.Error("queryDfid Marshal(qdf) failed. err=%s", err)
			return nil, errors.New("queryDfid Marshal(qdf) failed.")
		}

		return retValues, nil
	}

	return nil, errors.New("unknown function.")
}

func (t *SMK) queryTransInfos(stub shim.ChaincodeStubInterface, transLvl int, begIdx, endIdx int64) ([]byte, error) {
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
	var transArr []QueryTrans

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

func (t *SMK) queryBalance(stub shim.ChaincodeStubInterface) ([]byte, error) {
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

func (t *SMK) isCaller(stub shim.ChaincodeStubInterface, certificate []byte) (bool, error) {
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

func (t *SMK) checkAccountOfUser(stub shim.ChaincodeStubInterface, userName string, account string, userCert []byte) (bool, error) {

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

func (t *SMK) getEntity(stub shim.ChaincodeStubInterface, cbId string) (*Entity, error) {
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

func (t *SMK) isEntityExists(stub shim.ChaincodeStubInterface, cbId string) (bool, error) {
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
func (t *SMK) setEntity(stub shim.ChaincodeStubInterface, cb *Entity) error {

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
func (t *SMK) issueCoin(stub shim.ChaincodeStubInterface, cbID string, issueAmount, times int64) ([]byte, error) {
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

	t.recordTranse(stub, &fromEntity, cb, "", "issue", issueAmount, times)

	return nil, nil
}

//转账
func (t *SMK) transferCoin(stub shim.ChaincodeStubInterface, from, to, dfId, transType string, amount, times int64) ([]byte, error) {
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

	var recordDfid string = ""

	//********** 添加子账户处理
	//如果转出账户是央行账户，那么直接转出，无需处理子账户。因为央行账户没有以融资id的子账户，所以DFIdMap为nil
	if fromEntity.DFIdMap != nil {
		v, ok := fromEntity.DFIdMap[dfId]
		if !ok {
			mylog.Error("transferCoin: fromEntity(id=%s) has no such dfid '%s'.", from, dfId)
			return nil, errors.New("fromEntity has no such dfid.")
		}
		if v < amount {
			mylog.Error("transferCoin: fromEntity(id=%s) sub acc(id=%s) restAmount not enough.", from, dfId)
			return nil, errors.New("fromEntity sub acc restAmount not enough.")
		}
		fromEntity.DFIdMap[dfId] -= amount

		//只有转出账户有dfid子账户时，才需要记录该交易到以dfid为key的交易记录里
		recordDfid = dfId
	}
	if toEntity.DFIdMap != nil {
		_, ok := toEntity.DFIdMap[dfId]
		if !ok {
			toEntity.DFIdMap[dfId] = amount
		} else {
			toEntity.DFIdMap[dfId] += amount
		}
	}
	//********** 添加子账户处理

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

	err = t.recordTranse(stub, fromEntity, toEntity, recordDfid, transType, amount, times)

	return nil, err
}

//记录交易。目前交易分为两种：一种是和央行打交道的，包括央行发行货币、央行给项目或企业转帐，此类交易普通用户不能查询；另一种是项目、企业、个人间互相转账，此类交易普通用户能查询
func (t *SMK) recordTranse(stub shim.ChaincodeStubInterface, fromEnt, toEnt *Entity, dfId, transType string, amount, times int64) error {
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
	transInfo.DfId = dfId

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

func (t *SMK) checkAccountName(accName string) error {
	//会用':'作为分隔符分隔多个账户名，所以账户名不能含有':'
	var invalidChars string = string(ALL_ACC_DELIM)

	if strings.ContainsAny(accName, invalidChars) {
		mylog.Error("isAccountNameValid (acc=%s) failed.", accName)
		return fmt.Errorf("accName '%s' can not contains '%s'.", accName, invalidChars)
	}
	return nil
}

func (t *SMK) saveAccountName(stub shim.ChaincodeStubInterface, accName string) error {
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

func (t *SMK) getAllAccountNames(stub shim.ChaincodeStubInterface) ([]byte, error) {
	accB, err := stub.GetState(ALL_ACC_KEY)
	if err != nil {
		mylog.Error("getAllAccountNames GetState failed.err=%s", err)
		return nil, err
	}
	return accB, nil
}

func (t *SMK) openAccount(stub shim.ChaincodeStubInterface, accName string, accType int, userName string, times int64, isCBAcc bool) ([]byte, error) {
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

func (t *SMK) setCenterBankAcc(stub shim.ChaincodeStubInterface, acc string) error {
	err := t.PutState_Ex(stub, CENTERBANK_ACC_KEY, []byte(acc))
	if err != nil {
		mylog.Error("setCenterBankAcc PutState failed.err=%s", err)
		return err
	}

	centerBankAccCache = []byte(acc)

	return nil
}
func (t *SMK) getCenterBankAcc(stub shim.ChaincodeStubInterface) ([]byte, error) {
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

func (t *SMK) getTransSeq(stub shim.ChaincodeStubInterface, transSeqKey string) (int64, error) {
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
func (t *SMK) setTransSeq(stub shim.ChaincodeStubInterface, transSeqKey string, seq int64) error {
	err := t.PutState_Ex(stub, transSeqKey, []byte(strconv.FormatInt(seq, 10)))
	if err != nil {
		mylog.Error("setTransSeq PutState failed.err=%s", err)
		return err
	}

	return nil
}

func (t *SMK) getTransInfoKey(stub shim.ChaincodeStubInterface, transLvl int, seq int64) string {
	var buf = bytes.NewBufferString(TRANSINFO_PREFIX)
	buf.WriteString("lvl")
	buf.WriteString(strconv.Itoa(transLvl))
	buf.WriteString("_")
	buf.WriteString(strconv.FormatInt(seq, 10))
	return buf.String()
}
func (t *SMK) getTransSeqKey(stub shim.ChaincodeStubInterface, transLvl int) string {
	var buf = bytes.NewBufferString(TRANSSEQ_PREFIX)
	buf.WriteString("lvl")
	buf.WriteString(strconv.Itoa(transLvl))
	buf.WriteString("_")
	return buf.String()
}
func (t *SMK) getGlobTransSeqKey(stub shim.ChaincodeStubInterface) string {
	return TRANSSEQ_PREFIX + "global_"
}

func (t *SMK) setTransInfo(stub shim.ChaincodeStubInterface, info *Transaction) error {
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

	if len(info.DfId) > 0 {
		err = t.recordDfIdTrans(stub, transKey, info.DfId)
		if err != nil {
			mylog.Error("setTransInfo recordDfIdTrans failed. err=%s", err)
			return errors.New("setTransInfo recordDfIdTrans failed.")
		}
	}

	mylog.Debug("setTransInfo OK, info=%v", info)

	return nil
}

//记录某个dfid下的交易
func (t *SMK) getDfIdTransKey(dfId string) string {
	return DFID_TX_PREFIX + dfId
}
func (t *SMK) recordDfIdTrans(stub shim.ChaincodeStubInterface, transKey, dfId string) error {

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

func (t *SMK) getTransInfo(stub shim.ChaincodeStubInterface, key string) (*Transaction, error) {
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
func (t *SMK) getQueryTransInfo(stub shim.ChaincodeStubInterface, key string) (*QueryTrans, error) {
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

func (t *SMK) recordTrace(stub shim.ChaincodeStubInterface, wareId, msg string, times int64) error {
	var err error

	//根据商品id获取已有的记录
	infoB, err := stub.GetState(wareId)
	if err != nil {
		mylog.Error("recordTrace GetState failed.err=%s", err)
		return err
	}

	var info TraceInfo
	info.Time = times
	info.TraceMsg = msg

	infoJson, err := json.Marshal(info)
	if err != nil {
		mylog.Error("recordTrace Marshal failed.err=%s", err)
		return err
	}

	if infoB == nil {
		infoB = infoJson
	} else {
		infoB = append(infoB, ',')
		infoB = append(infoB, infoJson...)
	}

	err = t.PutState_Ex(stub, wareId, infoB)
	if err != nil {
		mylog.Error("recordTrace PutState failed. err=%s", err)
		return err
	}

	return nil
}

func (t *SMK) getDfInfoKey(dfId string) string {
	return DFID_PREFIX + dfId
}

func (t *SMK) PutState_Ex(stub shim.ChaincodeStubInterface, key string, value []byte) error {
	//当key为空字符串时，0.6的PutState接口不会报错，但是会导致chainCode所在的contianer异常退出。
	if key == "" {
		mylog.Error("myPutState key err.")
		return errors.New("myPutState key err.")
	}
	return stub.PutState(key, value)
}

func main() {
	// for debug
	mylog.SetDefaultLvl(0)

	primitives.SetSecurityLevel("SHA3", 256)

	err := shim.Start(new(SMK))
	if err != nil {
		fmt.Printf("Error starting EventSender chaincode: %s", err)
	}
}
