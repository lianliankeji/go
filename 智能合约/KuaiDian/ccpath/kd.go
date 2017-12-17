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

var mylog = InitMylog("kd")

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

	//因为目前账户entity的key是账户名，为了防止自己定义的如下key和账户名冲突，所以下面的key里都含有特殊字符
	TRANSSEQ_PREFIX      = "!kd@txSeqPre~"          //序列号生成器的key的前缀。使用的是worldState存储
	TRANSINFO_PREFIX     = "!kd@txInfoPre~"         //交易信息的key的前缀。使用的是worldState存储
	ONE_ACC_TRANS_PREFIX = "!kd@oneAccTxPre~"       //存储单个账户的交易的key前缀
	UER_ENTITY_PREFIX    = "!kd@usrEntPre~"         //存储某个用户的用户信息的key前缀。目前用户名和账户名相同，而账户entity的key是账户名，所以用户entity加个前缀区分
	CENTERBANK_ACC_KEY   = "!kd@centerBankAccKey@!" //央行账户的key。使用的是worldState存储
	ALL_ACC_KEY          = "!kd@allAccInfoKey@!"    //存储所有账户名的key。使用的是worldState存储

	MULTI_STRING_DELIM = ':' //多个string的分隔符
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

var ErrNilEntity = errors.New("nil entity.")

/*
   注意点：
   1、 存储数量较少的字符串数组时，可以使用[]string类型；如果字符串数组的长度可能会非常庞大，则不要使用[]string类型,
       可以使用[]byte直接存放在worldState中，每个字符串之间使用某种字符例如':'来分割，读取时使用bytes.Buffer.ReadBytes(':')来获取每一个字符串。
       否则如果用[]string类型虽然处理上比较方便（marshal和unmarshal转换格式），但是unmarshal操作在记录量级较大时非常耗费时间，影响响应速度。
       本合约中，每个用户的交易记录就使用[]byte直接存放的方式（没有单独记录用户的交易，而是把全局交易的key记录下来），因为交易记录可能会非常多。
*/
type KD struct {
}

func (t *KD) Init(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	mylog.Debug("Enter Init")
	mylog.Debug("func =%s, args = %v", function, args)

	return nil, nil
}

// Transaction makes payment of X units from A to B
func (t *KD) Invoke(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
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
	var invokeTime int64 = 0

	invokeTime, err = strconv.ParseInt(args[2], 0, 64)
	if err != nil {
		return nil, mylog.Errorf("Invoke convert invokeTime(%s) failed. err=%s", args[2], err)
	}

	var userAttrs *UserAttrs
	var userEnt *AccountEntity = nil

	userAttrs, err = t.getUserAttrs(stub)
	if err != nil {
		return nil, mylog.Errorf("Invoke getUserAttrs failed. err=%s", err)
	}

	//开户时和更新证书时不需要校验证书。 开户时证书还没传入无法验证；更新证书如果是admin的证书损坏或更新，也不能验证。
	if function != "account" && function != "accountCB" && function != "updateCert" {

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

		return t.issueCoin(stub, accName, issueAmount, invokeTime)

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

		return t.newAccount(stub, accName, usrType, userName, userCert, invokeTime, false)

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

		_, err = t.newAccount(stub, accName, usrType, userName, userCert, invokeTime, true)
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
		var argCount = fixedArgCount + 5
		if len(args) < argCount {
			mylog.Error("Invoke(transefer) miss arg, got %d, at least need %d.", len(args), argCount)
			return nil, errors.New("Invoke(transefer) miss arg.")
		}

		var toAcc = args[fixedArgCount]
		var transType = args[fixedArgCount+1]
		var description = args[fixedArgCount+2]

		var transAmount int64
		transAmount, err = strconv.ParseInt(args[fixedArgCount+3], 0, 64)
		if err != nil {
			return nil, mylog.Errorf("convert issueAmount(%s) failed. err=%s", args[fixedArgCount+3], err)
		}
		mylog.Debug("transAmount= %v", transAmount)

		var sameEntSaveTrans = args[fixedArgCount+4] //如果转出和转入账户相同，是否记录交易 0表示不记录 1表示记录
		var sameEntSaveTransFlag bool = false
		if sameEntSaveTrans == "1" {
			sameEntSaveTransFlag = true
		}

		return t.transferCoin(stub, accName, toAcc, transType, description, transAmount, invokeTime, sameEntSaveTransFlag)

	} else if function == "updateCert" {
		if !t.isAdmin(stub, accName) {
			return nil, mylog.Errorf("Invoke(updateCert) can't exec updateCert by %s.", accName)
		}

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

		upEnt.OwnerCert = cert

		err = t.setEntity(stub, upEnt)
		if err != nil {
			return nil, mylog.Errorf("Invoke(updateCert) setEntity  failed. err=%s", err)
		}

		return nil, nil
	} else if function == "updateEnv" {
		//更新环境变量
		if !t.isAdmin(stub, accName) {
			return nil, mylog.Errorf("Invoke(updateEnv) can't exec updateEnv by %s.", accName)
		}

		var argCount = fixedArgCount + 2
		if len(args) < argCount {
			return nil, mylog.Errorf("Invoke(updateEnv) miss arg, got %d, at least need %d.", len(args), argCount)
		}
		key := args[fixedArgCount]
		value := args[fixedArgCount+1]

		if key == "logLevel" {
			lvl, _ := strconv.Atoi(value)
			mylog.SetDefaultLvl(lvl)
			mylog.Info("set logLevel to %d.", lvl)
		}

		return nil, nil
	} else if function == "AuthCert" { //授权证书
		if !t.isAdmin(stub, accName) {
			return nil, mylog.Errorf("Invoke(AuthCert) can't exec AuthCert by %s.", accName)
		}

		var argCount = fixedArgCount + 2
		if len(args) < argCount {
			return nil, mylog.Errorf("Invoke(AuthCert) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		/*
			var authAcc = args[fixedArgCount]
			var addedUser = args[fixedArgCount+1]
			var addedCert []byte
			addedCert, err = base64.StdEncoding.DecodeString(args[fixedArgCount+2])
			if err != nil {
				return nil, mylog.Errorf("Invoke(AuthCert) DecodeString failed. err=%s, arg=%s", err, args[fixedArgCount+2])
			}

			authEnt, err := t.getEntity(stub, authAcc)
			if err != nil {
				return nil, mylog.Errorf("Invoke(AuthCert) getEntity failed. err=%s, entname=%s", err, authAcc)
			}
		*/

	}

	//event
	stub.SetEvent("success", []byte("invoke success"))
	return nil, errors.New("unknown Invoke.")
}

// Query callback representing the query of a chaincode
func (t *KD) Query(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
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
	var userEnt *AccountEntity = nil

	userAttrs, err = t.getUserAttrs(stub)
	if err != nil {
		return nil, mylog.Errorf("Query getUserAttrs failed. err=%s", err)
	}

	userEnt, err = t.getEntity(stub, accName)
	if err != nil {
		if err == ErrNilEntity {
			if function == "isAccExists" { //如果是查询账户是否存在，如果是空，返回不存在
				return []byte("0"), nil
			} else if function == "getBalance" { //如果是查询余额，如果账户不存，返回0
				return []byte("0"), nil
			}
		}
		return nil, mylog.Errorf("Query getEntity failed. err=%s", err)
	}

	//校验用户身份
	if ok, _ := t.verifyIdentity(stub, userEnt, userAttrs); !ok {
		return nil, mylog.Errorf("Query user and account check failed.")
	}

	if function == "getBalance" { //查询余额

		var queryEntity *AccountEntity
		queryEntity, err = t.getEntity(stub, accName)
		if err != nil {
			mylog.Error("getEntity queryEntity(id=%s) failed. err=%s", accName, err)
			return nil, err
		}
		mylog.Debug("queryEntity=%v", queryEntity)

		retValue := []byte(strconv.FormatInt(queryEntity.RestAmount, 10))
		mylog.Debug("retValue=%v, %s", retValue, string(retValue))

		return retValue, nil
	} else if function == "getTransInfo" { //查询交易记录
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
			if !t.isAdmin(stub, accName) {
				return nil, mylog.Errorf("%s can't query tx info.", accName)
			}
			return t.queryTransInfos(stub, transLvl, begSeq, txCount, begTime, endTime)
		} else {
			//管理员用户 或者 用户自己才能查询某用户的交易记录
			if !t.isAdmin(stub, accName) && accName != txAcc {
				return nil, mylog.Errorf("%s can't query %s's tx info.", accName, txAcc)
			}
			return t.queryAccTransInfos(stub, txAcc, begSeq, txCount, begTime, endTime)
		}

	} else if function == "getAllAccAmt" { //所有账户中钱是否正确
		//是否是管理员帐户，管理员用户才可以查
		if !t.isAdmin(stub, accName) {
			return nil, mylog.Errorf("%s can't query balance.", accName)
		}

		return t.getAllAccAmt(stub)
	} else if function == "queryState" { //某个state的值
		//是否是管理员帐户，管理员用户才可以查
		if !t.isAdmin(stub, accName) {
			return nil, mylog.Errorf("%s can't query state.", accName)
		}

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
	} else if function == "isAccExists" { //账户是否存在
		accExist, err := t.isEntityExists(stub, accName)
		if err != nil {
			mylog.Error("accExists: isEntityExists (id=%s) failed. err=%s", accName, err)
			return nil, errors.New("accExists: isEntityExists failed.")
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

func (t *KD) queryTransInfos(stub shim.ChaincodeStubInterface, transLvl uint64, begIdx, count, begTime, endTime int64) ([]byte, error) {
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
		var qTrans QueryTrans
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

func (t *KD) queryAccTransInfos(stub shim.ChaincodeStubInterface, accName string, begIdx, count, begTime, endTime int64) ([]byte, error) {
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
	//count为负数，查询到最后
	if count < 0 {
		count = math.MaxInt64
	}

	infoB, err := stub.GetState(t.getOneAccTransKey(accName))
	if err != nil {
		return nil, mylog.Errorf("queryAccTransInfos(%s) GetState failed.err=%s", accName, err)
	}
	if infoB == nil {
		return retTransInfo, nil
	}

	var transArr []QueryTrans = []QueryTrans{} //初始化为空数组，即使下面没查到数据也会返回'[]'
	var loopCnt int64 = 0
	var trans *QueryTrans
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
		var qTrans QueryTrans
		if trans.Time >= begTime && trans.Time <= endTime {
			qTrans.Serial = loop
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

func (t *KD) getAllAccAmt(stub shim.ChaincodeStubInterface) ([]byte, error) {
	var qb QueryBalance
	qb.IssueAmount = 0
	qb.AccSumAmount = 0
	qb.AccCount = 0

	accsB, err := stub.GetState(ALL_ACC_KEY)
	if err != nil {
		mylog.Error("getAllAccAmt GetState failed. err=%s", err)
		return nil, errors.New("getAllAccAmt GetState failed.")
	}
	if accsB != nil {

		cbAccB, err := t.getCenterBankAcc(stub)
		if err != nil {
			mylog.Error("getAllAccAmt getCenterBankAcc failed. err=%s", err)
			return nil, errors.New("getAllAccAmt getCenterBankAcc failed.")
		}
		if cbAccB == nil {
			qb.Message += "none centerBank;"
		} else {
			cbEnt, err := t.getEntity(stub, string(cbAccB))
			if err != nil {
				mylog.Error("getAllAccAmt getCenterBankAcc failed. err=%s", err)
				return nil, errors.New("getAllAccAmt getCenterBankAcc failed.")
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

			ent, err = t.getEntity(stub, string(acc))
			if err != nil {
				mylog.Error("getAllAccAmt getEntity(%s) failed. err=%s", string(acc), err)
				qb.Message += fmt.Sprintf("get account(%s) info failed;", string(acc))
				continue
			}
			qb.AccSumAmount += ent.RestAmount
		}
	}

	retValue, err := json.Marshal(qb)
	if err != nil {
		mylog.Error("getAllAccAmt Marshal failed. err=%s", err)
		return nil, errors.New("getAllAccAmt Marshal failed.")
	}

	return retValue, nil
}

func (t *KD) verifySign(stub shim.ChaincodeStubInterface, certificate []byte) (bool, error) {
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

func (t *KD) verifyIdentity(stub shim.ChaincodeStubInterface, ent *AccountEntity, attrs *UserAttrs) (bool, error) {
	//有时获取不到attr，这里做个判断
	if len(attrs.UserName) > 0 && ent.Owner != attrs.UserName {
		mylog.Errorf("verifyIdentity: user check failed(%s,%s).", ent.Owner, attrs.UserName)
		return false, mylog.Errorf("verifyIdentity: user check failed(%s,%s).", ent.Owner, attrs.UserName)
	}

	ok, err := t.verifySign(stub, ent.OwnerCert)
	if err != nil {
		return false, mylog.Errorf("verifyIdentity: verifySign error, err=%s.", err)
	}
	if !ok {
		return false, mylog.Errorf("verifyIdentity: verifySign failed.")
	}

	return true, nil
}

func (t *KD) getUserAttrs(stub shim.ChaincodeStubInterface) (*UserAttrs, error) {
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

func (t *KD) getEntity(stub shim.ChaincodeStubInterface, entName string) (*AccountEntity, error) {
	var centerBankByte []byte
	var cb AccountEntity
	var err error

	centerBankByte, err = stub.GetState(entName)
	if err != nil {
		return nil, err
	}

	if centerBankByte == nil {
		return nil, ErrNilEntity
	}

	if err = json.Unmarshal(centerBankByte, &cb); err != nil {
		return nil, errors.New("get data of centerBank failed.")
	}

	return &cb, nil
}

func (t *KD) isEntityExists(stub shim.ChaincodeStubInterface, entName string) (bool, error) {
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
func (t *KD) setEntity(stub shim.ChaincodeStubInterface, cb *AccountEntity) error {

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
func (t *KD) issueCoin(stub shim.ChaincodeStubInterface, cbID string, issueAmount, issueTime int64) ([]byte, error) {
	mylog.Debug("Enter issueCoin")

	var err error

	var cb *AccountEntity
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
		return nil, mylog.Errorf("issue: setCenterBank failed. err=%s", err)
	}

	//虚拟一个超级账户，给央行发行货币。因为给央行发行货币，是不需要转账的源头的
	var fromEntity AccountEntity
	fromEntity.EntID = "198405051114"
	fromEntity.EntType = -1
	fromEntity.RestAmount = math.MaxInt64
	fromEntity.TotalAmount = math.MaxInt64
	fromEntity.Owner = "fanxiaotian"

	//这里只记录一下央行的收入，不记录支出
	err = t.recordTranse(stub, cb, &fromEntity, TRANS_INCOME, "issue", "center bank issue coin.", issueAmount, issueTime)
	if err != nil {
		return nil, mylog.Errorf("issue: recordTranse failed. err=%s", err)
	}

	return nil, nil
}

//转账
func (t *KD) transferCoin(stub shim.ChaincodeStubInterface, from, to, transType, description string, amount, transeTime int64, sameEntSaveTrans bool) ([]byte, error) {
	mylog.Debug("Enter transferCoin")

	var err error

	if amount <= 0 {
		return nil, mylog.Errorf("transferCoin failed. invalid amount(%d)", amount)
	}

	//如果账户相同，并且账户相同时不需要记录交易，直接返回
	if from == to && !sameEntSaveTrans {
		mylog.Warn("transferCoin from equals to.")
		return nil, nil
	}

	var fromEntity, toEntity *AccountEntity
	fromEntity, err = t.getEntity(stub, from)
	if err != nil {
		return nil, mylog.Errorf("getEntity(id=%s) failed. err=%s", from, err)
	}
	toEntity, err = t.getEntity(stub, to)
	if err != nil {
		return nil, mylog.Errorf("getEntity(id=%s) failed. err=%s", to, err)
	}

	if fromEntity.RestAmount < amount {
		return nil, mylog.Errorf("fromEntity(id=%s) restAmount not enough.", from)
	}

	//如果账户相同，并且账户相同时需要记录交易，交易并返回
	if from == to && sameEntSaveTrans {
		err = t.recordTranse(stub, fromEntity, toEntity, TRANS_PAY, transType, description, amount, transeTime)
		if err != nil {
			return nil, mylog.Errorf("setEntity recordTranse fromEntity(id=%s) failed. err=%s", from, err)
		}

		err = t.recordTranse(stub, toEntity, fromEntity, TRANS_INCOME, transType, description, amount, transeTime)
		if err != nil {
			return nil, mylog.Errorf("setEntity recordTranse fromEntity(id=%s) failed. err=%s", from, err)
		}
		return nil, nil
	}

	//账户相同时为什么单独处理？  因为如果走了下面的流程，setEntity两次同一个账户，会导致账户余额变化。 除非在计算并设置完fromEntity之后，再获取一下toEntity，再计算toEntity，这样感觉太呆了

	mylog.Debug("fromEntity before= %v", fromEntity)
	mylog.Debug("toEntity before= %v", toEntity)

	fromEntity.RestAmount -= amount
	toEntity.RestAmount += amount
	toEntity.TotalAmount += amount

	mylog.Debug("fromEntity after= %v", fromEntity)
	mylog.Debug("toEntity after= %v", toEntity)

	err = t.setEntity(stub, fromEntity)
	if err != nil {
		return nil, mylog.Errorf("setEntity of fromEntity(id=%s) failed. err=%s", from, err)
	}

	err = t.recordTranse(stub, fromEntity, toEntity, TRANS_PAY, transType, description, amount, transeTime)
	if err != nil {
		return nil, mylog.Errorf("setEntity recordTranse fromEntity(id=%s) failed. err=%s", from, err)
	}

	err = t.setEntity(stub, toEntity)
	if err != nil {
		return nil, mylog.Errorf("setEntity of toEntity(id=%s) failed. err=%s", to, err)
	}

	//两个账户的收入支出都记录交易
	err = t.recordTranse(stub, toEntity, fromEntity, TRANS_INCOME, transType, description, amount, transeTime)
	if err != nil {
		return nil, mylog.Errorf("setEntity recordTranse fromEntity(id=%s) failed. err=%s", from, err)
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

func (t *KD) checkAccountName(accName string) error {
	//会用':'作为分隔符分隔多个账户名，所以账户名不能含有':'
	var invalidChars string = string(MULTI_STRING_DELIM)

	if strings.ContainsAny(accName, invalidChars) {
		mylog.Error("isAccountNameValid (acc=%s) failed.", accName)
		return fmt.Errorf("accName '%s' can not contains '%s'.", accName, invalidChars)
	}
	return nil
}

func (t *KD) saveAccountName(stub shim.ChaincodeStubInterface, accName string) error {
	accB, err := stub.GetState(ALL_ACC_KEY)
	if err != nil {
		mylog.Error("saveAccountName GetState failed.err=%s", err)
		return err
	}

	var accs []byte
	if accB == nil {
		accs = append([]byte(accName), MULTI_STRING_DELIM) //第一次添加accName，最后也要加上分隔符
	} else {
		accs = append(accB, []byte(accName)...)
		accs = append(accs, MULTI_STRING_DELIM)
	}

	err = t.PutState_Ex(stub, ALL_ACC_KEY, accs)
	if err != nil {
		mylog.Error("setCenterBankAcc PutState failed.err=%s", err)
		return err
	}
	return nil
}

func (t *KD) getAllAccountNames(stub shim.ChaincodeStubInterface) ([]byte, error) {
	accB, err := stub.GetState(ALL_ACC_KEY)
	if err != nil {
		mylog.Error("getAllAccountNames GetState failed.err=%s", err)
		return nil, err
	}
	return accB, nil
}

func (t *KD) newAccount(stub shim.ChaincodeStubInterface, accName string, accType int, userName string, cert []byte, times int64, isCBAcc bool) ([]byte, error) {
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

	err = t.setEntity(stub, &ent)
	if err != nil {
		return nil, mylog.Errorf("openAccount setEntity (id=%s) failed. err=%s", accName, err)
	}

	mylog.Debug("openAccount success: %v", ent)

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
	err := t.PutState_Ex(stub, CENTERBANK_ACC_KEY, []byte(acc))
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
func (t *KD) setTransSeq(stub shim.ChaincodeStubInterface, transSeqKey string, seq int64) error {
	err := t.PutState_Ex(stub, transSeqKey, []byte(strconv.FormatInt(seq, 10)))
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

func (t *KD) getGlobTransSeqKey(stub shim.ChaincodeStubInterface) string {
	return TRANSSEQ_PREFIX + "global_"
}

func (t *KD) setTransInfo(stub shim.ChaincodeStubInterface, info *Transaction) error {
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

func (t *KD) getOneAccTransKey(accName string) string {
	return ONE_ACC_TRANS_PREFIX + accName
}

func (t *KD) setOneAccTransInfo(stub shim.ChaincodeStubInterface, accName, transKey string) error {
	var accTransKey = t.getOneAccTransKey(accName)

	tmpState, err := stub.GetState(accTransKey)
	if err != nil {
		return mylog.Errorf("setOneAccTransInfo GetState(%s) failed.err=%s", accName, err)
	}

	var newTxsB []byte
	if tmpState == nil {
		newTxsB = append([]byte(transKey), MULTI_STRING_DELIM) //每一次添加accName，最后都要加上分隔符，bytes.Buffer.ReadBytes(MULTI_STRING_DELIM)需要
	} else {
		newTxsB = append(tmpState, []byte(transKey)...)
		newTxsB = append(newTxsB, MULTI_STRING_DELIM)
	}

	err = t.PutState_Ex(stub, accTransKey, newTxsB)
	if err != nil {
		return mylog.Errorf("setOneAccTransInfo PutState_Ex(%s) failed.err=%s", accName, err)
	}

	return nil
}

func (t *KD) getTransInfo(stub shim.ChaincodeStubInterface, key string) (*Transaction, error) {
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
func (t *KD) getQueryTransInfo(stub shim.ChaincodeStubInterface, key string) (*QueryTrans, error) {
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

func (t *KD) PutState_Ex(stub shim.ChaincodeStubInterface, key string, value []byte) error {
	//当key为空字符串时，0.6的PutState接口不会报错，但是会导致chainCode所在的contianer异常退出。
	if key == "" {
		return mylog.Errorf("PutState_Ex key err.")
	}
	return stub.PutState(key, value)
}

func main() {
	// for debug
	mylog.SetDefaultLvl(MYLOG_LVL_INFO)

	primitives.SetSecurityLevel("SHA3", 256)

	err := shim.Start(new(KD))
	if err != nil {
		mylog.Error("Error starting EventSender chaincode: %s", err)
	}
}
