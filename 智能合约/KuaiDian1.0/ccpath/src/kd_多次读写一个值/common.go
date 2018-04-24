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
	"fmt"
	"path"
	"runtime"

	"github.com/hyperledger/fabric/core/chaincode/shim"
)

var logger = NewMylogger("comm")

type MyCrypto struct {
}

func MyCryptoNew() *MyCrypto {
	return &MyCrypto{}
}

func (mh *MyCrypto) genHash(salt []byte, pwd string) ([]byte, error) {
	var err error

	var saltLen = len(salt)
	if saltLen < 1 {
		return nil, logger.Errorf("genHash: saltLen < 1")
	}
	logger.Debug("genHash: salt=%s", hex.EncodeToString(salt))

	var offset = saltLen / 2

	var md5Hash = md5.New()
	var md5Bytes []byte
	md5Bytes = append(md5Bytes, salt[:offset]...)
	md5Bytes = append(md5Bytes, []byte(pwd)...)
	md5Bytes = append(md5Bytes, salt[offset:]...)

	_, err = md5Hash.Write(md5Bytes)
	if err != nil {
		return nil, logger.Errorf("genHash: md5Hash.Write failed, err=%s", err)
	}
	var md5Sum = md5Hash.Sum(nil)
	logger.Debug("genHash: md5Sum=%s", hex.EncodeToString(md5Sum))

	var sha512Hash = sha512.New()
	var sha512Bytes []byte
	offset = saltLen - offset
	sha512Bytes = append(sha512Bytes, salt[:offset]...)
	sha512Bytes = append(sha512Bytes, md5Sum...)
	sha512Bytes = append(sha512Bytes, salt[offset:]...)

	_, err = sha512Hash.Write(sha512Bytes)
	if err != nil {
		return nil, logger.Errorf("genHash: sha512Hash.Write failed, err=%s", err)
	}

	var sha512Sum = sha512Hash.Sum(nil)
	logger.Debug("genHash: sha512Sum=%s", hex.EncodeToString(sha512Sum))

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
		return nil, logger.Errorf("genCipher: genHash failed, err=%s", err)
	}

	var str = fmt.Sprintf("33$%s$%s", base64.StdEncoding.EncodeToString(addSalt), base64.StdEncoding.EncodeToString(hash))
	logger.Debug("str=%s", str)

	return []byte(str), nil
}

func (mh *MyCrypto) AuthPass(cipher []byte, pwd string) (bool, error) {

	var sliceArr = bytes.Split(cipher, []byte("$"))
	if len(sliceArr) < 3 {
		return false, logger.Errorf("AuthPass: cipher format error.")
	}

	salt, err := base64.StdEncoding.DecodeString(string(sliceArr[1]))
	if err != nil {
		return false, logger.Errorf("AuthPass: DecodeString salt failed, err=%s.", err)
	}
	hashSum, err := base64.StdEncoding.DecodeString(string(sliceArr[2]))
	if err != nil {
		return false, logger.Errorf("AuthPass: DecodeString hashSum failed, err=%s.", err)
	}
	logger.Debug("salt=%s, hashSum=%s", hex.EncodeToString(salt), hex.EncodeToString(hashSum))

	hash, err := mh.genHash(salt, pwd)
	if err != nil {
		return false, logger.Errorf("AuthPass: genHash failed, err=%s", err)
	}

	logger.Debug("hash=%s, hashSum=%s", hex.EncodeToString(hash), hex.EncodeToString(hashSum))
	if bytes.Equal(hash, hashSum) {
		return true, nil
	}

	return false, nil
}

func (me *MyCrypto) AESEncrypt(bits int, key, iv, data []byte) ([]byte, error) {
	var err error

	if bits != 128 && bits != 192 && bits != 256 {
		return nil, logger.Errorf("AESEncrypt: bits must be 128 or 192 or 256.")
	}
	if len(key)*8 < bits {
		return nil, logger.Errorf("AESEncrypt: key must longer than %d bytes.", bits/8)
	}
	var newKey = key[:bits/8]

	if len(iv) < aes.BlockSize {
		return nil, logger.Errorf("AESEncrypt: iv must longer than %d bytes.", aes.BlockSize)
	}
	var newIv = iv[:aes.BlockSize]

	block, err := aes.NewCipher(newKey)
	if err != nil {
		return nil, logger.Errorf("AESEncrypt: NewCipher failed, err=%s", err)
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
		return nil, logger.Errorf("AESDecrypt: bits must be 128 or 192 or 256.")
	}
	if len(key)*8 < bits {
		return nil, logger.Errorf("AESDecrypt: key must longer than %d.", bits)
	}
	var newKey = key[:bits/8]

	if len(iv) < aes.BlockSize {
		return nil, logger.Errorf("AESDecrypt: iv must longer than %d bytes.", aes.BlockSize)
	}
	var newIv = iv[:aes.BlockSize]

	block, err := aes.NewCipher(newKey)
	if err != nil {
		return nil, logger.Errorf("AESDecrypt: NewCipher failed, err=%s", err)
	}

	//CFB模式
	stream := cipher.NewCFBDecrypter(block, newIv)

	decrypted := make([]byte, len(data))
	stream.XORKeyStream(decrypted, data)

	return decrypted, nil
}

func (me *MyCrypto) Hash160(data []byte) ([]byte, error) {

	sha := sha256.New()
	_, err := sha.Write(data)
	if err != nil {
		return nil, logger.Errorf("Hash160: sha.Write failed, err=%s", err)
	}

	var ripemd160 = NewRipemd160()
	rip := ripemd160.New()
	_, err = rip.Write(sha.Sum(nil))
	if err != nil {
		return nil, logger.Errorf("Hash160: rip.Write failed, err=%s", err)
	}

	return rip.Sum(nil), nil

}

func (me *MyCrypto) Sha256(data []byte) ([]byte, error) {

	sha := sha256.New()
	_, err := sha.Write(data)
	if err != nil {
		return nil, logger.Errorf("Sha256: sha.Write failed, err=%s", err)
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
