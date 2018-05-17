package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"runtime"
	"sync"

	"github.com/hyperledger/fabric/core/chaincode/shim"
)

/**************************************************************************/
/******************************** 错误码 ***********************************/
/**************************************************************************/
//这里定义的错误码会返回给前端，所以不要修改已有的错误码，如果要修改，请和前端一起修改
//错误码开始  本合约的错误码从 10000--99999，其他合约不要冲突
const ERRCODE_BEGIN = 10000

//转账的错误码
const (
	ERRCODE_TRANS_BEGIN                     = iota + 10000
	ERRCODE_TRANS_PAY_ACCOUNT_NOT_EXIST     //付款账号不存在
	ERRCODE_TRANS_PAYEE_ACCOUNT_NOT_EXIST   //收款账号不存在
	ERRCODE_TRANS_BALANCE_NOT_ENOUGH        //账号余额不足
	ERRCODE_TRANS_PASSWD_INVALID            //密码错误（老版本中使用密码验证，新版本不再使用）
	ERRCODE_TRANS_AMOUNT_INVALID            //转账金额不合法
	ERRCODE_TRANS_BALANCE_NOT_ENOUGH_BYLOCK //锁定部分余额导致余额不足
)

//公共错误码
const (
	ERRCODE_COMMON_BEGIN                  = iota + 90000
	ERRCODE_COMMON_PARAM_INVALID          //参数不合法
	ERRCODE_COMMON_SYS_ERROR              //系统错误 即调用任何的系统函数（非合约内实现的函数统称为系统函数）出错
	ERRCODE_COMMON_INNER_ERROR            //内部错误 合约内部逻辑等出现错误
	ERRCODE_COMMON_IDENTITY_VERIFY_FAILED //身份校验失败
	ERRCODE_COMMON_CHECK_FAILED           //检查失败，比如检查用户是否存在、用户是否有权限等等
)

//错误码结束  本合约的错误码从 10000--99999，其他合约不要冲突
const ERRCODE_END = 99999

/**************************************************************************/
/******************************** 错误码 ***********************************/
/**************************************************************************/

var commlogger = NewMylogger("comm")

type MyCrypto struct {
}

func MyCryptoNew() *MyCrypto {
	return &MyCrypto{}
}

func (mh *MyCrypto) genHash(salt []byte, pwd string) ([]byte, error) {
	var err error

	var saltLen = len(salt)
	if saltLen < 1 {
		return nil, commlogger.Errorf("genHash: saltLen < 1")
	}
	commlogger.Debug("genHash: salt=%s", hex.EncodeToString(salt))

	var offset = saltLen / 2

	var md5Hash = md5.New()
	var md5Bytes []byte
	md5Bytes = append(md5Bytes, salt[:offset]...)
	md5Bytes = append(md5Bytes, []byte(pwd)...)
	md5Bytes = append(md5Bytes, salt[offset:]...)

	_, err = md5Hash.Write(md5Bytes)
	if err != nil {
		return nil, commlogger.Errorf("genHash: md5Hash.Write failed, err=%s", err)
	}
	var md5Sum = md5Hash.Sum(nil)
	commlogger.Debug("genHash: md5Sum=%s", hex.EncodeToString(md5Sum))

	var sha512Hash = sha512.New()
	var sha512Bytes []byte
	offset = saltLen - offset
	sha512Bytes = append(sha512Bytes, salt[:offset]...)
	sha512Bytes = append(sha512Bytes, md5Sum...)
	sha512Bytes = append(sha512Bytes, salt[offset:]...)

	_, err = sha512Hash.Write(sha512Bytes)
	if err != nil {
		return nil, commlogger.Errorf("genHash: sha512Hash.Write failed, err=%s", err)
	}

	var sha512Sum = sha512Hash.Sum(nil)
	commlogger.Debug("genHash: sha512Sum=%s", hex.EncodeToString(sha512Sum))

	return sha512Sum, nil
}

func (mh *MyCrypto) GenCipher(pwd string, salt []byte) ([]byte, error) {
	var err error

	/* 不能在智能合约中生成随机的salt，因为每个peer上生成的不一样，而salt又要保存在链上，会导致各peer数据不一致
	var saltLen = 16
	salt := make([]byte, saltLen)
	readLen, err := rand.Read(salt)
	if err != nil || readLen != saltLen {
		return nil, base_logger.Errorf("genCipher: rand.Read failed, err=%s, len=%d", err, readLen)
	}
	*/

	var saltDefault = []byte{150, 150, 35, 49, 60, 234, 156, 23, 182, 13, 65, 32, 77, 83, 66, 98}
	var addSalt []byte
	if salt == nil || len(salt) == 0 {
		addSalt = saltDefault
	} else {
		addSalt = salt
	}

	hash, err := mh.genHash(addSalt, pwd)
	if err != nil {
		return nil, commlogger.Errorf("genCipher: genHash failed, err=%s", err)
	}

	var str = fmt.Sprintf("33$%s$%s", base64.StdEncoding.EncodeToString(addSalt), base64.StdEncoding.EncodeToString(hash))
	commlogger.Debug("str=%s", str)

	return []byte(str), nil
}

func (mh *MyCrypto) AuthPass(cipher []byte, pwd string) (bool, error) {

	var sliceArr = bytes.Split(cipher, []byte("$"))
	if len(sliceArr) < 3 {
		return false, commlogger.Errorf("AuthPass: cipher format error.")
	}

	salt, err := base64.StdEncoding.DecodeString(string(sliceArr[1]))
	if err != nil {
		return false, commlogger.Errorf("AuthPass: DecodeString salt failed, err=%s.", err)
	}
	hashSum, err := base64.StdEncoding.DecodeString(string(sliceArr[2]))
	if err != nil {
		return false, commlogger.Errorf("AuthPass: DecodeString hashSum failed, err=%s.", err)
	}
	commlogger.Debug("salt=%s, hashSum=%s", hex.EncodeToString(salt), hex.EncodeToString(hashSum))

	hash, err := mh.genHash(salt, pwd)
	if err != nil {
		return false, commlogger.Errorf("AuthPass: genHash failed, err=%s", err)
	}

	commlogger.Debug("hash=%s, hashSum=%s", hex.EncodeToString(hash), hex.EncodeToString(hashSum))
	if bytes.Equal(hash, hashSum) {
		return true, nil
	}

	return false, nil
}

func (me *MyCrypto) AESEncrypt(bits int, key, iv, data []byte) ([]byte, error) {
	var err error

	if bits != 128 && bits != 192 && bits != 256 {
		return nil, commlogger.Errorf("AESEncrypt: bits must be 128 or 192 or 256.")
	}
	if len(key)*8 < bits {
		return nil, commlogger.Errorf("AESEncrypt: key must longer than %d bytes.", bits/8)
	}
	var newKey = key[:bits/8]

	if len(iv) < aes.BlockSize {
		return nil, commlogger.Errorf("AESEncrypt: iv must longer than %d bytes.", aes.BlockSize)
	}
	var newIv = iv[:aes.BlockSize]

	block, err := aes.NewCipher(newKey)
	if err != nil {
		return nil, commlogger.Errorf("AESEncrypt: NewCipher failed, err=%s", err)
	}

	//CFB模式
	stream := cipher.NewCFBEncrypter(block, newIv)

	encrypted := make([]byte, len(data))
	stream.XORKeyStream(encrypted, data)

	return encrypted, nil
}
func (me *MyCrypto) AESDecrypt(bits int, key, iv, data []byte) ([]byte, error) {
	var err error

	if bits != 128 && bits != 192 && bits != 256 {
		return nil, commlogger.Errorf("AESDecrypt: bits must be 128 or 192 or 256.")
	}
	if len(key)*8 < bits {
		return nil, commlogger.Errorf("AESDecrypt: key must longer than %d.", bits)
	}
	var newKey = key[:bits/8]

	if len(iv) < aes.BlockSize {
		return nil, commlogger.Errorf("AESDecrypt: iv must longer than %d bytes.", aes.BlockSize)
	}
	var newIv = iv[:aes.BlockSize]

	block, err := aes.NewCipher(newKey)
	if err != nil {
		return nil, commlogger.Errorf("AESDecrypt: NewCipher failed, err=%s", err)
	}

	//CFB模式
	stream := cipher.NewCFBDecrypter(block, newIv)

	decrypted := make([]byte, len(data))
	stream.XORKeyStream(decrypted, data)

	return decrypted, nil
}

func (me *MyCrypto) Sha256(data []byte) ([]byte, error) {

	sha := sha256.New()
	_, err := sha.Write(data)
	if err != nil {
		return nil, commlogger.Errorf("Sha256: sha.Write failed, err=%s", err)
	}

	return sha.Sum(nil), nil

}

type MYLOG struct {
	logger *shim.ChaincodeLogger
}

var __myLoggerSet []*MYLOG

func NewMylogger(module string) *MYLOG {
	var logger = shim.NewLogger(module)
	logger.SetLevel(shim.LogInfo)
	var mylogger = MYLOG{logger}
	__myLoggerSet = append(__myLoggerSet, &mylogger)
	return &mylogger
}

// debug=5, info=4, notice=3, warning=2, error=1, critical=0
func (m *MYLOG) SetDefaultLvl(lvl shim.LoggingLevel) {
	m.logger.SetLevel(lvl)
}

func (m *MYLOG) _addStackInfo(format string) string {
	pc, file, line, ok := runtime.Caller(2)
	if !ok {
		return format
	}

	return fmt.Sprintf("%s:%d[%s] %s", path.Base(file), line, runtime.FuncForPC(pc).Name(), format)
}

func (m *MYLOG) Debug(format string, args ...interface{}) {
	m.logger.Debugf(m._addStackInfo(format), args...)
}
func (m *MYLOG) Info(format string, args ...interface{}) {
	m.logger.Infof(m._addStackInfo(format), args...)
}
func (m *MYLOG) Notice(format string, args ...interface{}) {
	m.logger.Noticef(m._addStackInfo(format), args...)
}

func (m *MYLOG) Warn(format string, args ...interface{}) {
	m.logger.Warningf(m._addStackInfo(format), args...)
}
func (m *MYLOG) Error(format string, args ...interface{}) {
	m.logger.Errorf(m._addStackInfo(format), args...)
}

//输出错误信息，并返回错误对象
func (m *MYLOG) Errorf(format string, args ...interface{}) error {
	//日志追加stack信息
	m.logger.Errorf(m._addStackInfo(format), args...)
	//返回的信息不追加stack信息
	return fmt.Errorf(format, args...)
}

//输出错误信息，并返回错误信息
func (m *MYLOG) SError(format string, args ...interface{}) string {
	//日志追加stack信息
	m.logger.Errorf(m._addStackInfo(format), args...)
	//返回的信息不追加stack信息
	return fmt.Sprintf(format, args...)
}

//输出错误信息，并返回 ErrorCodeMsg 对象
func (m *MYLOG) ErrorECM(code int32, format string, args ...interface{}) *ErrorCodeMsg {
	//日志追加stack信息
	m.logger.Errorf(m._addStackInfo(format), args...)
	//返回的信息不追加stack信息
	return NewErrorCodeMsg(code, fmt.Sprintf(format, args...))
}

func (m *MYLOG) Critical(format string, args ...interface{}) {
	m.logger.Criticalf(m._addStackInfo(format), args...)
}

func (m *MYLOG) GetLoggers() []*MYLOG {
	return __myLoggerSet
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

type StateWorldCache struct {
	stateCache map[string]map[string][]byte
	lock       sync.RWMutex //这里的map类似于全局变量，访问需要加锁
}

func (t *StateWorldCache) Create(stub shim.ChaincodeStubInterface) {
	t.lock.Lock()

	if t.stateCache == nil {
		t.stateCache = make(map[string]map[string][]byte)
	}
	t.stateCache[stub.GetTxID()] = make(map[string][]byte)

	t.lock.Unlock()
}

func (t *StateWorldCache) Destroy(stub shim.ChaincodeStubInterface) {
	t.lock.Lock()
	delete(t.stateCache, stub.GetTxID())
	t.lock.Unlock()
}

func (t *StateWorldCache) GetState_Ex(stub shim.ChaincodeStubInterface, key string) ([]byte, error) {
	t.lock.RLock() //读锁
	if len(t.stateCache[stub.GetTxID()]) > 0 {
		if value, ok := t.stateCache[stub.GetTxID()][key]; ok {
			t.lock.RUnlock()
			return value, nil
		}
	}
	t.lock.RUnlock()

	value, err := stub.GetState(key)
	if err == nil {
		t.lock.Lock() //写锁
		t.stateCache[stub.GetTxID()][key] = value
		t.lock.Unlock()
	}

	return value, err
}

func (t *StateWorldCache) PutState_Ex(stub shim.ChaincodeStubInterface, key string, value []byte) error {
	err := stub.PutState(key, value)
	if err == nil {
		t.lock.Lock()
		t.stateCache[stub.GetTxID()][key] = value
		t.lock.Unlock()
	}
	return err
}

type ErrorCodeMsg struct {
	Code    int32
	Message string
}

func (r *ErrorCodeMsg) toJson() string {
	return fmt.Sprintf("{\"code\":%d,\"msg\":\"%s\"}", r.Code, r.Message)
}

//String方法只返回错误信息，要返回code和msg使用toJson方法
func (r *ErrorCodeMsg) String() string {
	return r.Message
}

func NewErrorCodeMsg(code int32, msg string) *ErrorCodeMsg {
	return &ErrorCodeMsg{Code: code, Message: msg}
}

func NewErrorCodeMsgFromString(jsonStr string) (*ErrorCodeMsg, error) {
	var ecm ErrorCodeMsg
	err := json.Unmarshal([]byte(jsonStr), &ecm)
	if err != nil {
		return nil, err
	}

	return &ecm, nil
}
