package main

import (
	"sync"

	"github.com/hyperledger/fabric/core/chaincode/shim"
)

type TransferInfo struct {
	FromID      string `json:"fid"`  //发送方ID
	ToID        string `json:"tid"`  //接收方ID
	Amount      int64  `json:"amt"`  //交易数额
	Description string `json:"desc"` //交易描述
	TransType   string `json:"tstp"` //交易类型，前端传入，透传
	Time        int64  `json:"time"` //交易时间
	AppID       string `json:"app"`  //应用ID  目前一条链一个账户体系，但是可能会有多种应用，所以交易信息记录一下应用id，可以按应用来过滤交易信息
}
type InvokeResult struct {
	TransInfos []TransferInfo `json:"transinfos"`
	Payload    []byte         `json:"payload"`
}

/*
 */
type TransferInfoCache struct {
	transCache map[string][]TransferInfo
	lock       sync.RWMutex //这里的map类似于全局变量，访问需要加锁
}

func NewTransferInfoCache() *TransferInfoCache {
	var t TransferInfoCache
	t.transCache = make(map[string][]TransferInfo)
	return &t
}

func (t *TransferInfoCache) Create(stub shim.ChaincodeStubInterface) {
}

func (t *TransferInfoCache) Destroy(stub shim.ChaincodeStubInterface) {
	t.lock.Lock()
	delete(t.transCache, stub.GetTxID())
	t.lock.Unlock()
}

func (t *TransferInfoCache) Get(stub shim.ChaincodeStubInterface) []TransferInfo {
	t.lock.RLock() //读锁
	defer t.lock.RUnlock()
	return t.transCache[stub.GetTxID()]

}

func (t *TransferInfoCache) Add(stub shim.ChaincodeStubInterface, transInfo *TransferInfo) {
	t.lock.Lock()
	t.transCache[stub.GetTxID()] = append(t.transCache[stub.GetTxID()], *transInfo)
	t.lock.Unlock()
}
