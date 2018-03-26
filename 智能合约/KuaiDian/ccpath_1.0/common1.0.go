package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

type MyCrypto struct {
}

func MyCryptoNew() *MyCrypto {
	return &MyCrypto{}
}

func (mh *MyCrypto) genHash(salt []byte, pwd string) ([]byte, error) {
	var err error

	var saltLen = len(salt)
	if saltLen < 1 {
		return nil, mylog.Errorf("genHash: saltLen < 1")
	}
	mylog.Debug("genHash: salt=%s", hex.EncodeToString(salt))

	var offset = saltLen / 2

	var md5Hash = md5.New()
	var md5Bytes []byte
	md5Bytes = append(md5Bytes, salt[:offset]...)
	md5Bytes = append(md5Bytes, []byte(pwd)...)
	md5Bytes = append(md5Bytes, salt[offset:]...)

	_, err = md5Hash.Write(md5Bytes)
	if err != nil {
		return nil, mylog.Errorf("genHash: md5Hash.Write failed, err=%s", err)
	}
	var md5Sum = md5Hash.Sum(nil)
	mylog.Debug("genHash: md5Sum=%s", hex.EncodeToString(md5Sum))

	var sha512Hash = sha512.New()
	var sha512Bytes []byte
	offset = saltLen - offset
	sha512Bytes = append(sha512Bytes, salt[:offset]...)
	sha512Bytes = append(sha512Bytes, md5Sum...)
	sha512Bytes = append(sha512Bytes, salt[offset:]...)

	_, err = sha512Hash.Write(sha512Bytes)
	if err != nil {
		return nil, mylog.Errorf("genHash: sha512Hash.Write failed, err=%s", err)
	}

	var sha512Sum = sha512Hash.Sum(nil)
	mylog.Debug("genHash: sha512Sum=%s", hex.EncodeToString(sha512Sum))

	return sha512Sum, nil
}

func (mh *MyCrypto) GenCipher(pwd string, salt []byte) ([]byte, error) {
	var err error

	/* 不能在智能合约中生成随机的salt，因为每个peer上生成的不一样，而salt又要保存在链上，会导致各peer数据不一致
	var saltLen = 16
	salt := make([]byte, saltLen)
	readLen, err := rand.Read(salt)
	if err != nil || readLen != saltLen {
		return nil, mylog.Errorf("genCipher: rand.Read failed, err=%s, len=%d", err, readLen)
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
		return nil, mylog.Errorf("genCipher: genHash failed, err=%s", err)
	}

	var str = fmt.Sprintf("33$%s$%s", base64.StdEncoding.EncodeToString(addSalt), base64.StdEncoding.EncodeToString(hash))
	mylog.Debug("str=%s", str)

	return []byte(str), nil
}

func (mh *MyCrypto) AuthPass(cipher []byte, pwd string) (bool, error) {

	var sliceArr = bytes.Split(cipher, []byte("$"))
	if len(sliceArr) < 3 {
		return false, mylog.Errorf("AuthPass: cipher format error.")
	}

	salt, err := base64.StdEncoding.DecodeString(string(sliceArr[1]))
	if err != nil {
		return false, mylog.Errorf("AuthPass: DecodeString salt failed, err=%s.", err)
	}
	hashSum, err := base64.StdEncoding.DecodeString(string(sliceArr[2]))
	if err != nil {
		return false, mylog.Errorf("AuthPass: DecodeString hashSum failed, err=%s.", err)
	}
	mylog.Debug("salt=%s, hashSum=%s", hex.EncodeToString(salt), hex.EncodeToString(hashSum))

	hash, err := mh.genHash(salt, pwd)
	if err != nil {
		return false, mylog.Errorf("AuthPass: genHash failed, err=%s", err)
	}

	if bytes.Equal(hash, hashSum) {
		return true, nil
	}

	return false, nil
}

func (me *MyCrypto) AESEncrypt(bits int, key, iv, data []byte) ([]byte, error) {
	var err error

	if bits != 128 && bits != 192 && bits != 256 {
		return nil, mylog.Errorf("AESEncrypt: bits must be 128 or 192 or 256.")
	}
	if len(key)*8 < bits {
		return nil, mylog.Errorf("AESEncrypt: key must longer than %d bytes.", bits/8)
	}
	var newKey = key[:bits/8]

	if len(iv) < aes.BlockSize {
		return nil, mylog.Errorf("AESEncrypt: iv must longer than %d bytes.", aes.BlockSize)
	}
	var newIv = iv[:aes.BlockSize]

	block, err := aes.NewCipher(newKey)
	if err != nil {
		return nil, mylog.Errorf("AESEncrypt: NewCipher failed, err=%s", err)
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
		return nil, mylog.Errorf("AESDecrypt: bits must be 128 or 192 or 256.")
	}
	if len(key)*8 < bits {
		return nil, mylog.Errorf("AESDecrypt: key must longer than %d.", bits)
	}
	var newKey = key[:bits/8]

	if len(iv) < aes.BlockSize {
		return nil, mylog.Errorf("AESDecrypt: iv must longer than %d bytes.", aes.BlockSize)
	}
	var newIv = iv[:aes.BlockSize]

	block, err := aes.NewCipher(newKey)
	if err != nil {
		return nil, mylog.Errorf("AESDecrypt: NewCipher failed, err=%s", err)
	}

	//CFB模式
	stream := cipher.NewCFBDecrypter(block, newIv)

	decrypted := make([]byte, len(data))
	stream.XORKeyStream(decrypted, data)

	return decrypted, nil
}
