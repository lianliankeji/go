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

var mylog = InitMylog("kd_alloc")

const (
	ATTR_USRROLE = "usrrole"
	ATTR_USRNAME = "usrname"
	ATTR_USRTYPE = "usrtype"

	CENTERBANK_ACC_KEY      = "__~!@#@!~_kd_alloc_centerBankAccKey__#$%" //央行账户的key。使用的是worldState存储
	ALL_ACC_KEY             = "__~!@#@!~_kd_alloc_allAccInfo__#$%"       //存储所有账户名的key。使用的是worldState存储
	GLOBAL_ALLOCPERCENT_KEY = "__~!@#@!~_kd_alloc_globalallocper__#$%"   //全局的收入分成比例
	ALLOCPERCENT_PREFIX     = "__~!@#@!~_kd_alloc_allocPerPre__"         //每个货架的收入分成比例的key前缀
	ALLOCSEQ_PREFIX         = "__~!@#@!~_kd_alloc_allocSeqPre__"         //
	ALLOC_TX_PREFIX         = "__~!@#@!~_kd_alloc_alloctxPre__"          //每个货架收入分成交易记录
	ACC_ALLOC_TX_PREFIX     = "__~!@#@!~_kd_alloc_acc_alloctxPre__"      //某个账户收入分成交易记录

	MULTI_STRING_DELIM = ':' //所有账户名的分隔符

	RACK_ROLE_SELLER   = "slr"
	RACK_ROLE_FIELDER  = "fld"
	RACK_ROLE_DELIVERY = "dvy"
	RACK_ROLE_PLATFORM = "pfm"
)

//账户信息Entity
// 一系列ID（或账户）都定义为字符串类型。因为putStat函数的第一个参数为字符串类型，这些ID（或账户）都作为putStat的第一个参数；另外从SDK传过来的参数也都是字符串类型。
type AccountEntity struct {
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

type RolesPercent struct {
	Seller   int64 `json:"slr"` //经营者分成比例 因为要和int64参与运算，这里都定义为int64
	Fielder  int64 `json:"fld"` //场地提供者分成比例
	Delivery int64 `json:"dvy"` //送货人分成比例
	Platform int64 `json:"pfm"` //平台分成比例
}

//货架收入分成比例
type EarningAllocPercent struct {
	Rackid string `json:"rid"`
	RolesPercent
	UpdateTime int64 `json:"uptm"`
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
	RolesPercent
}

type EarningAllocTx struct {
	PubEarningAllocTx
}

type AllocAccs struct {
	Seller   string `json:"slr"`
	Fielder  string `json:"fld"`
	Delivery string `json:"dvy"`
	Platform string `json:"pfm"`
}

type QueryAccEarningAllocTx struct {
	Serail        int64            `json:"ser"`
	AccName       string           `json:"acc"`
	Rackid        string           `json:"rid"`
	RoleAmountMap map[string]int64 `json:"ramap"`
	DateTime      int64            `json:"dtm"`
	TotalAmt      int64            `json:"tamt"` //总金额
	GlobalSerial  int64            `json:"gser"`
	RolesPercent
}

type KDALLOC struct {
}

func (t *KDALLOC) Init(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	mylog.Debug("Enter Init")
	mylog.Debug("func =%s, args = %v", function, args)

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

	var eap EarningAllocPercent
	eap.Rackid = "_global__rack___" //全局比例
	eap.Platform = 3                //3%
	eap.Fielder = 3                 //3%
	eap.Delivery = 2                //2%
	eap.Seller = 92                 //92%
	//eap.UpdateTime = times
	eap.UpdateTime = 0

	eapJson, err := json.Marshal(eap)
	if err != nil {
		return nil, mylog.Errorf("Init Marshal error, err=%s.", err)
	}

	err = t.PutState_Ex(stub, t.getGlobalRaciAllocPercentKey(), eapJson)
	if err != nil {
		return nil, mylog.Errorf("Init PutState_Ex error, err=%s.", err)
	}

	return nil, nil
}

// Transaction makes payment of X units from A to B
func (t *KDALLOC) Invoke(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
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
	var userEnt *AccountEntity = nil

	userAttrs, err = t.getUserAttrs(stub)
	if err != nil {
		return nil, mylog.Errorf("Invoke getUserAttrs failed. err=%s", err)
	}

	//开户时不需要校验
	if function != "account" && function != "accountCB" && function != "updateCert" {

		userEnt, err = t.getEntity(stub, accName)
		if err != nil {
			return nil, mylog.Errorf("Invoke getEntity(verifyIdentity) failed. err=%s", err)
		}

		//校验修改Entity的用户身份，只有Entity的所有者才能修改自己的Entity
		if ok, _ := t.verifyIdentity(stub, userEnt, userAttrs); !ok {
			fmt.Println("verify and account(%s) failed. \n", accName)
			return nil, errors.New("user and account check failed.")
		}
	}

	if function == "accountCB" {
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

		upEnt.Cert = cert

		err = t.setEntity(stub, upEnt)
		if err != nil {
			return nil, mylog.Errorf("Invoke(updateCert) setEntity  failed. err=%s", err)
		}

		return nil, nil
	} else if function == "updateEnv" {
		if !t.isAdmin(stub, accName) {
			return nil, mylog.Errorf("Invoke(updateCert) can't exec by %s.", accName)
		}

		var argCount = fixedArgCount + 2
		if len(args) < argCount {
			return nil, mylog.Errorf("Invoke(updateCert) miss arg, got %d, at least need %d.", len(args), argCount)
		}
		key := args[fixedArgCount]
		value := args[fixedArgCount+1]

		if key == "logLevel" {
			lvl, _ := strconv.Atoi(value)
			mylog.SetDefaultLvl(lvl)
			mylog.Info("set logLevel to %d.", lvl)
		}

		return nil, nil
	} else if function == "setAllocCfg" {
		if !t.isAdmin(stub, accName) {
			return nil, mylog.Errorf("Invoke(setAllocCfg) can't exec by %s.", accName)
		}

		var argCount = fixedArgCount + 5
		if len(args) < argCount {
			return nil, mylog.Errorf("Invoke(setAllocCfg) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		rackid := args[fixedArgCount]

		seller, err := strconv.ParseInt(args[fixedArgCount+1], 0, 64)
		if err != nil {
			return nil, mylog.Errorf("Invoke(setAllocCfg) convert seller(%s) failed. err=%s", args[fixedArgCount+1], err)
		}
		fielder, err := strconv.ParseInt(args[fixedArgCount+2], 0, 64)
		if err != nil {
			return nil, mylog.Errorf("Invoke(setAllocCfg) convert fielder(%s) failed. err=%s", args[fixedArgCount+2], err)
		}
		delivery, err := strconv.ParseInt(args[fixedArgCount+3], 0, 64)
		if err != nil {
			return nil, mylog.Errorf("Invoke(setAllocCfg) convert delivery(%s) failed. err=%s", args[fixedArgCount+3], err)
		}
		platform, err := strconv.ParseInt(args[fixedArgCount+4], 0, 64)
		if err != nil {
			return nil, mylog.Errorf("Invoke(setAllocCfg) convert platform(%s) failed. err=%s", args[fixedArgCount+4], err)
		}

		var eap EarningAllocPercent

		eap.Rackid = rackid
		eap.Seller = seller
		eap.Fielder = fielder
		eap.Delivery = delivery
		eap.Platform = platform
		eap.UpdateTime = times

		eapJson, err := json.Marshal(eap)
		if err != nil {
			return nil, mylog.Errorf("Invoke(setAllocCfg) Marshal error, err=%s.", err)
		}

		err = t.PutState_Ex(stub, t.getRackAllocPercentKey(rackid), eapJson)
		if err != nil {
			return nil, mylog.Errorf("Invoke(setAllocCfg) PutState_Ex error, err=%s.", err)
		}

		return nil, nil
	} else if function == "allocEarning" {
		if !t.isAdmin(stub, accName) {
			return nil, mylog.Errorf("Invoke(allocEarning) can't exec by %s.", accName)
		}

		var argCount = fixedArgCount + 7
		if len(args) < argCount {
			return nil, mylog.Errorf("Invoke(allocEarning) miss arg, got %d, at least need %d.", len(args), argCount)
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
			return nil, mylog.Errorf("Invoke(allocEarning) convert totalAmt(%s) failed. err=%s", args[fixedArgCount+6], err)
		}

		var eap EarningAllocPercent

		eapB, err := stub.GetState(t.getRackAllocPercentKey(rackid))
		if err != nil {
			return nil, mylog.Errorf("Invoke(allocEarning) GetState(rackid=%s) failed. err=%s", rackid, err)
		}
		if eapB == nil {
			mylog.Warn("Invoke(allocEarning) GetState(rackid=%s) nil, try to get global.", rackid)
			//没有为该货架单独配置，返回global配置
			eapB, err = stub.GetState(t.getGlobalRaciAllocPercentKey())
			if err != nil {
				return nil, mylog.Errorf("Invoke(allocEarning) GetState(global, rackid=%s) failed. err=%s", rackid, err)
			}
			if eapB == nil {
				return nil, mylog.Errorf("Invoke(allocEarning) GetState(global, rackid=%s) nil.", rackid)
			}
		}

		err = json.Unmarshal(eapB, &eap)
		if err != nil {
			return nil, mylog.Errorf("Invoke(allocEarning) Unmarshal failed. err=%s", err)
		}

		var accs AllocAccs
		accs.Seller = sellerAcc
		accs.Fielder = fielderAcc
		accs.Delivery = deliveryAcc
		accs.Platform = platformAcc

		return t.setAllocEarnTx(stub, rackid, allocKey, totalAmt, &accs, &eap, times)
	}

	//event
	stub.SetEvent("success", []byte("invoke success"))
	return nil, errors.New("unknown Invoke.")
}

// Query callback representing the query of a chaincode
func (t *KDALLOC) Query(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
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
		return nil, mylog.Errorf("Query getEntity failed. err=%s", err)
	}

	//校验用户身份
	if ok, _ := t.verifyIdentity(stub, userEnt, userAttrs); !ok {
		return nil, mylog.Errorf("Query user and account check failed.")
	}

	if function == "query" {

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
	} else if function == "queryState" {
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
	} else if function == "queryRackAlloc" {

		var argCount = fixedArgCount + 7
		if len(args) < argCount {
			return nil, mylog.Errorf("queryRackAlloc miss arg, got %d, need %d.", len(args), argCount)
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
			return nil, mylog.Errorf("queryRackAlloc ParseInt for begSeq(%s) failed. err=%s", args[fixedArgCount+2], err)
		}
		txCount, err = strconv.ParseInt(args[fixedArgCount+3], 0, 64)
		if err != nil {
			return nil, mylog.Errorf("queryRackAlloc ParseInt for txCount(%s) failed. err=%s", args[fixedArgCount+3], err)
		}

		begTime, err = strconv.ParseInt(args[fixedArgCount+4], 0, 64)
		if err != nil {
			return nil, mylog.Errorf("queryRackAlloc ParseInt for begTime(%s) failed. err=%s", args[fixedArgCount+4], err)
		}
		endTime, err = strconv.ParseInt(args[fixedArgCount+5], 0, 64)
		if err != nil {
			return nil, mylog.Errorf("queryRackAlloc ParseInt for endTime(%s) failed. err=%s", args[fixedArgCount+5], err)
		}
		txAcc = args[fixedArgCount+6]

		if len(allocKey) > 0 {
			//是否是管理员帐户，管理员用户才可以查
			if !t.isAdmin(stub, accName) {
				return nil, mylog.Errorf("queryRackAlloc: %s can't query allocKey.", accName)
			}

			//查询某一次的分配情况（由allocKey检索）
			return t.getAllocTxRecdByKey(stub, rackid, allocKey)
		} else if len(txAcc) > 0 {
			//是否是管理员帐户，管理员用户才可以查
			if !t.isAdmin(stub, accName) && accName != txAcc {
				return nil, mylog.Errorf("queryRackAlloc: %s can't query one acc.", accName)
			}

			//查询某一个账户的分配情况（由allocKey检索）
			return t.getOneAccAllocTxRecds(stub, txAcc, begSeq, txCount, begTime, endTime)
		} else {
			//是否是管理员帐户，管理员用户才可以查
			if !t.isAdmin(stub, accName) {
				return nil, mylog.Errorf("queryRackAlloc: %s can't query rack.", accName)
			}

			//查询某一个货架的分配情况（由allocKey检索）
			return t.getAllocTxRecds(stub, rackid, begSeq, txCount, begTime, endTime)
		}

	} else if function == "queryRackAllocCfg" {
		if !t.isAdmin(stub, accName) {
			return nil, mylog.Errorf("queryRackAllocCfg: %s can't query.", accName)
		}

		var argCount = fixedArgCount + 1
		if len(args) < argCount {
			return nil, mylog.Errorf("queryRackAllocCfg miss arg, got %d, need %d.", len(args), argCount)
		}

		var rackid = args[fixedArgCount]

		eapB, err := stub.GetState(t.getRackAllocPercentKey(rackid))
		if err != nil {
			return nil, mylog.Errorf("queryRackAllocCfg GetState(rackid=%s) failed. err=%s", rackid, err)
		}
		if eapB == nil {
			mylog.Warn("queryRackAllocCfg GetState(rackid=%s) nil, try to get global.", rackid)
			//没有为该货架单独配置，返回global配置
			eapB, err = stub.GetState(t.getGlobalRaciAllocPercentKey())
			if err != nil {
				return nil, mylog.Errorf("queryRackAllocCfg GetState(global, rackid=%s) failed. err=%s", rackid, err)
			}
			if eapB == nil {
				return nil, mylog.Errorf("queryRackAllocCfg GetState(global, rackid=%s) nil.", rackid)
			}
		}

		return eapB, nil
	}

	return nil, errors.New("unknown function.")
}

func (t *KDALLOC) verifySign(stub shim.ChaincodeStubInterface, certificate []byte) (bool, error) {
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

func (t *KDALLOC) verifyIdentity(stub shim.ChaincodeStubInterface, ent *AccountEntity, attrs *UserAttrs) (bool, error) {
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

func (t *KDALLOC) getUserAttrs(stub shim.ChaincodeStubInterface) (*UserAttrs, error) {
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

func (t *KDALLOC) getEntity(stub shim.ChaincodeStubInterface, entName string) (*AccountEntity, error) {
	var centerBankByte []byte
	var cb AccountEntity
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

func (t *KDALLOC) isEntityExists(stub shim.ChaincodeStubInterface, entName string) (bool, error) {
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
func (t *KDALLOC) setEntity(stub shim.ChaincodeStubInterface, cb *AccountEntity) error {

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

const (
	TRANS_LVL_CB   = 1
	TRANS_LVL_COMM = 2
)

func (t *KDALLOC) checkAccountName(accName string) error {
	//会用':'作为分隔符分隔多个账户名，所以账户名不能含有':'
	var invalidChars string = string(MULTI_STRING_DELIM)

	if strings.ContainsAny(accName, invalidChars) {
		mylog.Error("isAccountNameValid (acc=%s) failed.", accName)
		return fmt.Errorf("accName '%s' can not contains '%s'.", accName, invalidChars)
	}
	return nil
}

func (t *KDALLOC) saveAccountName(stub shim.ChaincodeStubInterface, accName string) error {
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

func (t *KDALLOC) newAccount(stub shim.ChaincodeStubInterface, accName string, accType int, userName string, cert []byte, times int64, isCBAcc bool) ([]byte, error) {
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

	var ent AccountEntity
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

func (t *KDALLOC) setCenterBankAcc(stub shim.ChaincodeStubInterface, acc string) error {
	err := t.PutState_Ex(stub, CENTERBANK_ACC_KEY, []byte(acc))
	if err != nil {
		mylog.Error("setCenterBankAcc PutState failed.err=%s", err)
		return err
	}

	centerBankAccCache = []byte(acc)

	return nil
}
func (t *KDALLOC) getCenterBankAcc(stub shim.ChaincodeStubInterface) ([]byte, error) {
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

func (t *KDALLOC) getTransSeq(stub shim.ChaincodeStubInterface, transSeqKey string) (int64, error) {
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
func (t *KDALLOC) setTransSeq(stub shim.ChaincodeStubInterface, transSeqKey string, seq int64) error {
	err := t.PutState_Ex(stub, transSeqKey, []byte(strconv.FormatInt(seq, 10)))
	if err != nil {
		mylog.Error("setTransSeq PutState failed.err=%s", err)
		return err
	}

	return nil
}

func (t *KDALLOC) setAllocEarnTx(stub shim.ChaincodeStubInterface, rackid, allocKey string, totalAmt int64,
	accs *AllocAccs, eap *EarningAllocPercent, times int64) ([]byte, error) {

	var eat EarningAllocTx
	eat.Rackid = rackid
	eat.AllocKey = allocKey
	eat.RolesPercent = eap.RolesPercent
	eat.TotalAmt = totalAmt

	eat.AmountMap = make(map[string]map[string]int64)
	eat.AmountMap[RACK_ROLE_SELLER] = make(map[string]int64)
	eat.AmountMap[RACK_ROLE_FIELDER] = make(map[string]int64)
	eat.AmountMap[RACK_ROLE_DELIVERY] = make(map[string]int64)
	eat.AmountMap[RACK_ROLE_PLATFORM] = make(map[string]int64)

	var base = eap.Seller + eap.Fielder + eap.Delivery + eap.Platform

	sellerAmt := totalAmt * eap.Seller / base
	fielderAmt := totalAmt * eap.Fielder / base
	deliveryAmt := totalAmt * eap.Delivery / base
	//上面计算可能有四舍五入的情况，剩余的都放在平台账户
	platformAmt := totalAmt - sellerAmt - fielderAmt - deliveryAmt

	var err error
	err = t.getRolesAllocEarning(sellerAmt, accs.Seller, eat.AmountMap[RACK_ROLE_SELLER])
	if err != nil {
		return nil, mylog.Errorf("setAllocEarnTx getRolesAllocEarning 1 failed.err=%s", err)
	}
	err = t.getRolesAllocEarning(fielderAmt, accs.Fielder, eat.AmountMap[RACK_ROLE_FIELDER])
	if err != nil {
		return nil, mylog.Errorf("setAllocEarnTx getRolesAllocEarning 2 failed.err=%s", err)
	}
	err = t.getRolesAllocEarning(deliveryAmt, accs.Delivery, eat.AmountMap[RACK_ROLE_DELIVERY])
	if err != nil {
		return nil, mylog.Errorf("setAllocEarnTx getRolesAllocEarning 3 failed.err=%s", err)
	}
	err = t.getRolesAllocEarning(platformAmt, accs.Platform, eat.AmountMap[RACK_ROLE_PLATFORM])
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
	err = t.PutState_Ex(stub, txKey, eatJson)
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
	err = t.setOneAccAllocEarnTx(stub, accs.Seller, txKey)
	if err != nil {
		return nil, mylog.Errorf("setAllocEarnTx  setOneAccAllocEarnTx(%s) failed.err=%s", accs.Seller, err)
	}
	checkMap[accs.Seller] = 0

	if _, ok := checkMap[accs.Fielder]; !ok {
		err = t.setOneAccAllocEarnTx(stub, accs.Fielder, txKey)
		if err != nil {
			return nil, mylog.Errorf("setAllocEarnTx  setOneAccAllocEarnTx(%s) failed.err=%s", accs.Fielder, err)
		}
		checkMap[accs.Fielder] = 0
	}

	if _, ok := checkMap[accs.Delivery]; !ok {
		err = t.setOneAccAllocEarnTx(stub, accs.Delivery, txKey)
		if err != nil {
			return nil, mylog.Errorf("setAllocEarnTx  setOneAccAllocEarnTx(%s) failed.err=%s", accs.Delivery, err)
		}
		checkMap[accs.Delivery] = 0
	}

	if _, ok := checkMap[accs.Platform]; !ok {
		err = t.setOneAccAllocEarnTx(stub, accs.Platform, txKey)
		if err != nil {
			return nil, mylog.Errorf("setAllocEarnTx  setOneAccAllocEarnTx(%s) failed.err=%s", accs.Platform, err)
		}
		checkMap[accs.Platform] = 0
	}

	return nil, nil
}

func (t *KDALLOC) setOneAccAllocEarnTx(stub shim.ChaincodeStubInterface, accName, txKey string) error {
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

	err = t.PutState_Ex(stub, accTxKey, newTxsB)
	if err != nil {
		mylog.Error("setOneAccAllocEarnTx PutState failed.err=%s", err)
		return err
	}

	return nil
}

func (t *KDALLOC) getRolesAllocEarning(totalAmt int64, accs string, result map[string]int64) error {

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

func (t *KDALLOC) getAllocTxSeqKey(stub shim.ChaincodeStubInterface, rackid string) string {
	return ALLOCSEQ_PREFIX + rackid + "_"
}

func (t *KDALLOC) getAllocTxKey(stub shim.ChaincodeStubInterface, rackid string, seq int64) string {
	var buf = bytes.NewBufferString(ALLOC_TX_PREFIX)
	buf.WriteString(rackid)
	buf.WriteString("_")
	buf.WriteString(strconv.FormatInt(seq, 10))
	return buf.String()
}

func (t *KDALLOC) getOneAccAllocTxKey(accName string) string {
	return ACC_ALLOC_TX_PREFIX + accName
}

func (t *KDALLOC) getRackAllocPercentKey(rackid string) string {
	return ALLOCPERCENT_PREFIX + rackid
}
func (t *KDALLOC) getGlobalRaciAllocPercentKey() string {
	return GLOBAL_ALLOCPERCENT_KEY
}

func (t *KDALLOC) getAllocTxRecdByKey(stub shim.ChaincodeStubInterface, rackid, allocKey string) ([]byte, error) {

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
func (t *KDALLOC) getAllocTxRecds(stub shim.ChaincodeStubInterface, rackid string, begIdx, count, begTime, endTime int64) ([]byte, error) {
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

func (t *KDALLOC) getOneAccAllocTxRecds(stub shim.ChaincodeStubInterface, accName string, begIdx, count, begTime, endTime int64) ([]byte, error) {
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
		pqaeat, err = t.procOneAccAllocTx(stub, oneString, accName)
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

func (t *KDALLOC) procOneAccAllocTx(stub shim.ChaincodeStubInterface, txKey, accName string) (*QueryAccEarningAllocTx, error) {
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
	qaeat.RolesPercent = eat.RolesPercent
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

func (t *KDALLOC) getAllocTxRecdEntity(stub shim.ChaincodeStubInterface, txKey string) (*EarningAllocTx, error) {
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

func (t *KDALLOC) isAdmin(stub shim.ChaincodeStubInterface, accName string) bool {
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

func (t *KDALLOC) PutState_Ex(stub shim.ChaincodeStubInterface, key string, value []byte) error {
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

	err := shim.Start(new(KDALLOC))
	if err != nil {
		mylog.Error("Error starting EventSender chaincode: %s", err)
	}
}
