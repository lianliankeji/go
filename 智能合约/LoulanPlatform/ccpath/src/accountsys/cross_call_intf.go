package main

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
