package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/shim"
)

func init() {
	verifyIdentityHook = commVerifyIdentity
}

var commLogger = NewMylogger("common")

type MyCrypto struct {
}

func MyCryptoNew() *MyCrypto {
	return &MyCrypto{}
}

func (me *MyCrypto) Hash160(data []byte) ([]byte, error) {

	sha := sha256.New()
	_, err := sha.Write(data)
	if err != nil {
		return nil, commLogger.Errorf("Hash160: sha.Write failed, err=%s", err)
	}

	var ripemd160 = NewRipemd160()
	rip := ripemd160.New()
	_, err = rip.Write(sha.Sum(nil))
	if err != nil {
		return nil, commLogger.Errorf("Hash160: rip.Write failed, err=%s", err)
	}

	return rip.Sum(nil), nil

}

func (me *MyCrypto) Sha256(data []byte) ([]byte, error) {

	sha := sha256.New()
	_, err := sha.Write(data)
	if err != nil {
		return nil, commLogger.Errorf("Sha256: sha.Write failed, err=%s", err)
	}

	return sha.Sum(nil), nil

}

type MYLOG struct {
	logger *shim.ChaincodeLogger
}

func NewMylogger(module string) *MYLOG {
	var logger = shim.NewLogger(module)
	logger.SetLevel(shim.LogInfo)
	return &MYLOG{logger}
}

// debug=5, info=4, notice=3, warning=2, error=1, critical=0
func (m *MYLOG) SetDefaultLvl(lvl shim.LoggingLevel) {
	m.logger.SetLevel(lvl)
}

func (m *MYLOG) Debug(format string, args ...interface{}) {
	m.logger.Debugf(format, args...)
}
func (m *MYLOG) Info(format string, args ...interface{}) {
	m.logger.Infof(format, args...)
}
func (m *MYLOG) Notice(format string, args ...interface{}) {
	m.logger.Noticef(format, args...)
}

func (m *MYLOG) Warn(format string, args ...interface{}) {
	m.logger.Warningf(format, args...)
}
func (m *MYLOG) Error(format string, args ...interface{}) {
	m.logger.Errorf(format, args...)
}

//输出错误信息，并返回错误对象
func (m *MYLOG) Errorf(format string, args ...interface{}) error {
	var info = fmt.Sprintf(format, args...)
	m.logger.Errorf(info)
	return fmt.Errorf(info)
}

//输出错误信息，并返回错误信息
func (m *MYLOG) SError(format string, args ...interface{}) string {
	var info = fmt.Sprintf(format, args...)
	m.logger.Errorf(info)
	return info
}

func (m *MYLOG) Critical(format string, args ...interface{}) {
	m.logger.Criticalf(format, args...)
}

func strSliceContains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}

	return false
}

func strSliceDelete(list []string, value string) []string {
	var newList []string

	for _, v := range list {
		if v != value {
			newList = append(newList, v)
		}
	}

	return newList
}

const (
	CONTROL_CC_NAME              = "sysctrlcc"
	CONTROL_CC_GETPARA_FUNC_NAME = "getParameter"
)

func needCheckSign(stub shim.ChaincodeStubInterface) bool {
	//默认返回true，除非读取到指定参数

	var args = util.ToChaincodeArgs(CONTROL_CC_GETPARA_FUNC_NAME, "checkSiagnature")

	response := stub.InvokeChaincode(CONTROL_CC_NAME, args, "")
	if response.Status != shim.OK {
		commLogger.Errorf("needCheckSign: InvokeChaincode failed, response=%+v.", response)
		return true
	}

	paraValue := string(response.Payload)
	if paraValue == "0" {
		return false
	}

	return true
}

var tmpltCrypto = MyCryptoNew()

func verifySign(stub shim.ChaincodeStubInterface, ownerPubKeyHash string, sign, signMsg []byte) error {

	if chk := needCheckSign(stub); !chk {
		commLogger.Debug("verifySign: do not neec check signature.")
		return nil
	}

	secp256k1 := NewSecp256k1()

	commLogger.Debug("verifySign: sign = %v", sign)
	commLogger.Debug("verifySign: signMsg = %v", signMsg)

	if code := secp256k1.VerifySignatureValidity(sign); code != 1 {
		return commLogger.Errorf("verifySign: sign invalid, code=%v.", code)
	}

	pubKey, err := secp256k1.RecoverPubkey(signMsg, sign)
	if err != nil {
		return commLogger.Errorf("verifySign: RecoverPubkey failed,err=%s", err)
	}
	commLogger.Debug("verifySign: pubKey = %v", pubKey)

	hash, err := tmpltCrypto.Hash160(pubKey)
	if err != nil {
		return commLogger.Errorf("verifySign: Hash160 error, err=%s.", err)
	}
	var userPubKeyHash = base64.StdEncoding.EncodeToString(hash)
	commLogger.Debug("verifySign: userPubKeyHash = %s", userPubKeyHash)
	commLogger.Debug("verifySign: OwnerPubKeyHash = %s", ownerPubKeyHash)

	if userPubKeyHash != ownerPubKeyHash {
		return commLogger.Errorf("verifySign: sign invalid.")
	}

	return nil
}

func commVerifyIdentity(stub shim.ChaincodeStubInterface, userName string, sign, signMsg []byte, ownerPubKeyHash, ownerIdentityHash string) error {

	if chk := needCheckSign(stub); !chk {
		commLogger.Debug("verifySign: do not neec check signature.")
		return nil
	}

	creatorByte, err := stub.GetCreator()
	if err != nil {
		return commLogger.Errorf("verifyIdentity: GetCreator error, user=%s err=%s.", userName, err)
	}
	commLogger.Debug("verifyIdentity: creatorByte = %s", string(creatorByte))

	certStart := bytes.IndexAny(creatorByte, "-----BEGIN")
	if certStart == -1 {
		return commLogger.Errorf("verifyIdentity: No certificate found, user=%s.", userName)
	}
	certText := creatorByte[certStart:]

	hash, err := tmpltCrypto.Hash160(certText)
	if err != nil {
		return commLogger.Errorf("verifyIdentity: Hash160 error, user=%s err=%s.", userName, err)
	}

	var userIdHash = base64.StdEncoding.EncodeToString(hash)

	commLogger.Debug("verifyIdentity: userIdHash = %s", userIdHash)

	commLogger.Debug("verifyIdentity: entIdHash = %s", ownerIdentityHash)

	if userIdHash != ownerIdentityHash {
		return commLogger.Errorf("verifyIdentity: indentity invalid.")
	}

	return verifySign(stub, ownerPubKeyHash, sign, signMsg)
}
