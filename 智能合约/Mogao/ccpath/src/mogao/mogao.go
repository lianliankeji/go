package main

import (
	"bytes"
	"encoding/json"
	//"bufio"
	//"bytes"
	//"encoding/base64"
	//"encoding/hex"
	//"encoding/json"
	//"fmt"
	//"io"
	//"math"
	//"os"
	//"time"
	//"sort"
	"strconv"
	//"strings"

	"github.com/hyperledger/fabric/core/chaincode/shim"
)

const (
	MODULE_NAME                    = "mg"
	MOVIE_INFO_PREFIX              = "!mg@mvIfPre~"       //影片信息key前缀
	MOVIE_COMMENT_INFO_PREFIX      = "!mg@mvCmtIfPre~"    //影片评论信息key前缀
	MOVIE_INCOME_ALLOC_RATE_PREFIX = "!mg@mvInAlocRtPre~" //影片收入分配比例
	MOVIE_GLOBAL_CFG_KEY           = "!mg@mvGlbCfg@!"     //
	MOVIE_ALLOCTX_SEQ_PREFIX       = "!mg@alocTxSeqPre~"  //每部影片的分成记录的序列号的key前缀
	MOVIE_ALLOCTX_PREFIX           = "!mg@alocTxPre~"     //每部影片收入分成记录

	MOVIE_GLOBAL_ID = "__global@movie_id__" //影片一些全局配置的影片id

)

/***********************************************************/
//影片信息
type MovieInfo struct {
	MovieId    string `json:"mid"`  //
	MovieName  string `json:"mnm"`  //
	Uploader   string `json:"upr"`  //
	UpdateTime int64  `json:"uptm"` // 记录此条记录的时间
}

type CommentInfo struct {
	Comment      string `json:"cmt"`  //评论
	CmtTime      int64  `json:"ctm"`  //评论时间
	FavourateCnt int64  `json:"fvc"`  //点攒数
	UpdateTime   int64  `json:"uptm"` // 记录此条记录的时间
}

type CommentatorCommentInfo struct {
	Commentator string        `json:"cmtr"` //专家评论人
	Comments    []CommentInfo `json:"cmts"` //
}

//影片评论信息
type MovieCommentInfo struct {
	MovieId string                   `json:"mid"` //
	CCI     []CommentatorCommentInfo `json:"cci"` //
}

//影片观看信息
type MovieWatchInfo struct {
	MovieId  string `json:"mid"` //
	WatchCnt int64  `json:"wc"`  //
}

/*
计算公式:
上传人：影片总收入 * UploaderRate
评论人：影片总收入 * CommentatorRate * FixedRate / 评论人总数 + 影片总收入 * CommentatorRate * FavourateRate * 当前评论人评论得分 / 所有评论人的所有评论的总得分
评论人的收入分两部分，一部分是FixedRate给出，只要发表评论就有；一部分是FavourateRate给出，由评论的点赞数给出
*/
//每部影片评论人收入分成比例
type cmtorIncomeAllocRate struct {
	//这两个rate计算方式  FixedRate/(FixedRate+FavourateRate)
	FixedRate     int   `json:"fxr"`  //分给评论人的的分成部分， 按此固定部分的比例平分给各评论人
	FavourateRate int   `json:"frr"`  //分给评论人的的分成部分， 按此好评比例，再按照每个评论的点赞数分成
	UpdateTime    int64 `json:"uptm"` // 记录此条记录的时间
}
type movieIncomePlatformRate struct {
	//这个rate计算方式 PlatformRate/RateBase
	PlatformRate int   `json:"pfr"`  //分给平台的分成部分
	RateBase     int   `json:"rb"`   //平台比例基数 100， 1000， 10000表示百分比、千分比、万分比
	UpdateTime   int64 `json:"uptm"` // 记录此条记录的时间
}

type MovieGlobalCfg struct {
	CIAR cmtorIncomeAllocRate    `json:"ciar"`
	MIPR movieIncomePlatformRate `json:"mipr"`
}

//每部影片收入分成比例
type MovieIncomeAllocRateCfg struct {
	MovieId          string `json:"mid"`   //
	UploaderRate     int    `json:"uplr"`  //
	CommentatorsRate int    `json:"cmtrr"` //
	PlatformRate     int    `json:"pltr"`  // 这个从全局中计算得出？ 前两个rate应该由上传者指定，这个rate应该由平台指定
	UpdateTime       int64  `json:"uptm"`  // 记录此条记录的时间
}

//影片收入情况记录
type MovieIncomeTx struct {
	MovieId                  string           `json:"mid"`   //
	DateTime                 int64            `json:"dt"`    //
	TotalIncome              int64            `json:"tinc"`  //
	UploaderIncome           int64            `json:"uinc"`  //
	CommentatorsIncome       map[string]int64 `json:"cinc"`  //
	PlatformIncome           int64            `json:"pinc"`  //
	UploaderRate             int              `json:"uplr"`  //
	CommentatorsRate         int              `json:"cmtrr"` //
	PlatformRate             int              `json:"pltr"`  //
	CommentatorFavourate     map[string]int64 `json:"cfav"`  //
	CommentatorFixedRate     int              `json:"fxr"`   //
	CommentatorFavourateRate int              `json:"frr"`   //
	GlobalSerial             int64            `json:"gser"`
}

type MOGAO struct {
}

var mogao MOGAO

var mglogger = NewMylogger("mg")

//包初始化函数
func init() {
	//注册base中的hook函数
	InitHook = mogao.Init
	InvokeHook = mogao.Invoke
}

func (m *MOGAO) Init(stub shim.ChaincodeStubInterface, ifas *BaseInitArgs) ([]byte, error) {
	mglogger.Debug("Enter Init")
	function, args := stub.GetFunctionAndParameters()
	mglogger.Debug("func =%s, args = %+v", function, args)

	//目前需要一个参数
	var fixedArgCount = ifas.FixedArgCount
	if len(args) < fixedArgCount {
		return nil, mglogger.Errorf("Init miss arg, got %d, at least need %d.", len(args), fixedArgCount)
	}
	var initTime = ifas.InitTime

	if function == "init" { //合约实例化时，默认会执行init函数，除非在调用合约实例化接口时指定了其它的函数

		err := Base.setIssueAmountTotal(stub, 100000000, initTime)
		if err != nil {
			return nil, mglogger.Errorf("Init setIssueAmountTotal error, err=%s.", err)
		}

		var miar MovieIncomeAllocRateCfg
		miar.MovieId = MOVIE_GLOBAL_ID
		miar.UpdateTime = initTime
		miar.UploaderRate = 85
		miar.CommentatorsRate = 10
		miar.PlatformRate = 5

		miarJson, err := json.Marshal(miar)
		if err != nil {
			return nil, mglogger.Errorf("Init(init) Marshal(miar) failed, err=%s", err)
		}
		err = Base.putState_Ex(stub, m.getMovieIncomeAllocRateKey(MOVIE_GLOBAL_ID), miarJson)
		if err != nil {
			return nil, mglogger.Errorf("Init(init) putState_Ex(miar) failed, err=%s", err)
		}

		var mgc MovieGlobalCfg
		mgc.CIAR.UpdateTime = initTime
		mgc.CIAR.FixedRate = 7
		mgc.CIAR.FavourateRate = 3

		mgc.MIPR.UpdateTime = initTime
		mgc.MIPR.PlatformRate = 50
		mgc.MIPR.RateBase = 100

		mgcJson, err := json.Marshal(mgc)
		if err != nil {
			return nil, mglogger.Errorf("Init(init) Marshal(mgc) failed, err=%s", err)
		}
		err = Base.putState_Ex(stub, MOVIE_GLOBAL_CFG_KEY, mgcJson)
		if err != nil {
			return nil, mglogger.Errorf("Init(init) putState_Ex(mgc) failed, err=%s", err)
		}

		return nil, nil

	} else if function == "upgrade" { //升级时默认会执行upgrade函数，除非在调用合约升级接口时指定了其它的函数

		return nil, nil
	} else {

		return nil, mglogger.Errorf("unkonwn function '%s'", function)
	}

}

// Transaction makes payment of X units from A to B
func (m *MOGAO) Invoke(stub shim.ChaincodeStubInterface, ifas *BaseInvokeArgs) ([]byte, error) {
	mglogger.Debug("Enter Invoke")
	function, args := stub.GetFunctionAndParameters()
	mglogger.Debug("func =%s, args = %+v", function, args)
	//var err error

	var fixedArgCount = ifas.FixedArgCount
	if len(args) < fixedArgCount {
		return nil, mglogger.Errorf("Invoke miss arg, got %d, at least need %d.", len(args), fixedArgCount)
	}

	var accName = ifas.AccountName

	//var userName = args[0]
	/*
		//	var accName = args[1]
		//	var invokeTime int64 = 0

		invokeTime, err = strconv.ParseInt(args[2], 0, 64)
		if err != nil {
			return nil, mglogger.Errorf("Invoke convert invokeTime(%s) failed. err=%s", args[2], err)
		}
	*/
	//var userAttrs *UserAttrs
	//var accountEnt *AccountEntity = nil

	if function == "setMovieInfo" {
		var argCount = fixedArgCount + 3
		if len(args) < argCount {
			return nil, mglogger.Errorf("Invoke(setMovieInfo) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		if !Base.isAdmin(stub, accName) {
			return nil, mglogger.Errorf("Invoke can't exec setMovieInfo by %s.", accName)
		}

		var mi MovieInfo
		mi.MovieId = args[fixedArgCount]
		mi.MovieName = args[fixedArgCount+1]
		mi.Uploader = args[fixedArgCount+2]
		mi.UpdateTime = ifas.InvokeTime

		miJson, err := json.Marshal(mi)
		if err != nil {
			return nil, mglogger.Errorf("Invoke(setMovieInfo) Marshal failed, err=%s.", err)
		}

		//TODO: 要不要检测数据是否已存在?
		err = Base.putState_Ex(stub, m.getMovieInfoKey(mi.MovieId), miJson)
		if err != nil {
			return nil, mglogger.Errorf("Invoke(setMovieInfo) putState_Ex failed, err=%s.", err)
		}

		return nil, nil

	} else if function == "setMovieComment" {
		var argCount = fixedArgCount + 4
		if len(args) < argCount {
			return nil, mglogger.Errorf("Invoke(setMovieComment) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		if !Base.isAdmin(stub, accName) {
			return nil, mglogger.Errorf("Invoke can't exec setMovieComment by %s.", accName)
		}
		var movieId = args[fixedArgCount]
		var commentator = args[fixedArgCount+1]
		var comment = args[fixedArgCount+2]
		favourateCnt, err := strconv.ParseInt(args[fixedArgCount+3], 0, 64)
		if err != nil {
			return nil, mglogger.Errorf("Invoke(setMovieComment) convert favourateCnt(%s) failed. err=%s", args[fixedArgCount+3], err)
		}
		//看这个电影是否存在
		pmi, err := m.getMovieInfo(stub, movieId)
		if err != nil {
			return nil, mglogger.Errorf("Invoke(setMovieComment) getMovieInfo failed. id=%s, err=%s", movieId, err)
		}
		if pmi == nil {
			return nil, mglogger.Errorf("Invoke(setMovieComment)  movie not registe. id=%s", movieId)
		}

		pmci, err := m.getMovieCommentInfo(stub, movieId)
		if err != nil {
			return nil, mglogger.Errorf("Invoke(setMovieComment) getMovieCommentInfo failed. id=%s, err=%s", movieId, err)
		}
		var mci MovieCommentInfo
		if pmci == nil {
			mci.MovieId = movieId
			mci.CCI = append(mci.CCI, CommentatorCommentInfo{
				Commentator: commentator,
				Comments:    append([]CommentInfo{}, CommentInfo{Comment: comment, FavourateCnt: favourateCnt, UpdateTime: ifas.InvokeTime})})
		} else {
			mci = *pmci
			var matched = false
			for _, cci := range mci.CCI {
				if cci.Commentator == commentator {
					matched = true
					cci.Comments = append(cci.Comments, CommentInfo{Comment: comment, FavourateCnt: favourateCnt, UpdateTime: ifas.InvokeTime})
					break
				}
			}
			if !matched {
				mci.CCI = append(mci.CCI, CommentatorCommentInfo{
					Commentator: commentator,
					Comments:    append([]CommentInfo{}, CommentInfo{Comment: comment, FavourateCnt: favourateCnt, UpdateTime: ifas.InvokeTime})})
			}
		}

		mciJson, err := json.Marshal(mci)
		if err != nil {
			return nil, mglogger.Errorf("Invoke(setMovieComment) Marshal failed. id=%s, err=%s", movieId, err)
		}

		err = Base.putState_Ex(stub, m.getMovieCommentInfoKey(movieId), mciJson)
		if err != nil {
			return nil, mglogger.Errorf("Invoke(setMovieComment) putState_Ex failed, err=%s.", err)
		}

		return nil, nil

	} else if function == "setMovieIncomeAllocRate" {
		var argCount = fixedArgCount + 5
		if len(args) < argCount {
			return nil, mglogger.Errorf("Invoke(setMovieIncomeAllocRate) miss arg, got %d, at least need %d.", len(args), argCount)
		}

		var movieId = args[fixedArgCount]
		uploaderRate, err := strconv.Atoi(args[fixedArgCount+1])
		if err != nil {
			return nil, mglogger.Errorf("Invoke(setMovieIncomeAllocRate) convert uploaderRate(%s) failed, err=%s.", args[fixedArgCount+1], err)
		}
		commentatorRate, err := strconv.Atoi(args[fixedArgCount+2])
		if err != nil {
			return nil, mglogger.Errorf("Invoke(setMovieIncomeAllocRate) convert commentatorRate(%s) failed, err=%s.", args[fixedArgCount+2], err)
		}
		platformRate, err := strconv.Atoi(args[fixedArgCount+3])
		if err != nil {
			return nil, mglogger.Errorf("Invoke(setMovieIncomeAllocRate) convert platformRate(%s) failed, err=%s.", args[fixedArgCount+3], err)
		}

		var globalFlag = args[fixedArgCount+4]
		if len(globalFlag) > 0 {
			movieId = MOVIE_GLOBAL_ID
		}

		var miar MovieIncomeAllocRateCfg
		miar.MovieId = movieId
		miar.UpdateTime = ifas.InvokeTime
		miar.UploaderRate = uploaderRate
		miar.CommentatorsRate = commentatorRate
		miar.PlatformRate = platformRate

		miarJson, err := json.Marshal(miar)
		if err != nil {
			return nil, mglogger.Errorf("Invoke(setMovieIncomeAllocRate) Marshal failed. id=%s, err=%s", movieId, err)
		}

		err = Base.putState_Ex(stub, m.getMovieIncomeAllocRateKey(movieId), miarJson)
		if err != nil {
			return nil, mglogger.Errorf("Invoke(setMovieIncomeAllocRate) putState_Ex failed, err=%s.", err)
		}

		return nil, nil

	} else if function == "computeMovieIncome" {
		var movieId = args[fixedArgCount]
		income, err := strconv.ParseInt(args[fixedArgCount+1], 0, 64)
		if err != nil {
			return nil, mglogger.Errorf("Invoke(computeMovieIncome) convert income(%s) failed, err=%s.", args[fixedArgCount+1], err)
		}

		pmci, err := m.getMovieCommentInfo(stub, movieId)
		if err != nil {
			return nil, mglogger.Errorf("Invoke(computeMovieIncome) getMovieCommentInfo failed. id=%s, err=%s", movieId, err)
		}
		if pmci == nil {
			return nil, mglogger.Errorf("Invoke(computeMovieIncome) getMovieCommentInfo nil. id=%s", movieId)
		}

		mgc, err := m.getMovieGlobalCfg(stub)
		if err != nil {
			return nil, mglogger.Errorf("Invoke(computeMovieIncome) getMovieGlobalCfg failed. id=%s, err=%s", movieId, err)
		}
		if mgc == nil {
			return nil, mglogger.Errorf("Invoke(computeMovieIncome) getMovieGlobalCfg nil. id=%s", movieId)
		}

		pmiar, err := m.getMovieIncomeAllocRate(stub, movieId)
		if err != nil {
			return nil, mglogger.Errorf("Invoke(computeMovieIncome) getMovieIncomeAllocRate failed, id=%s, err=%s.", movieId, err)
		}
		if pmiar == nil {
			mglogger.Info("Invoke(computeMovieIncome) getMovieIncomeAllocRate nil, try to get global.")
			//do not use ":=" here
			pmiar, err = m.getMovieIncomeAllocRate(stub, MOVIE_GLOBAL_ID)
			if err != nil {
				return nil, mglogger.Errorf("Invoke(computeMovieIncome) getMovieIncomeAllocRate global failed, id=%s, err=%s.", movieId, err)
			}
			if pmiar == nil {
				return nil, mglogger.Errorf("Invoke(computeMovieIncome) getMovieIncomeAllocRate global nil, id=%s, err=%s.", movieId, err)
			}
		}

		var base = int64(pmiar.PlatformRate + pmiar.UploaderRate + pmiar.CommentatorsRate)
		var uploaderIncome = income * int64(pmiar.UploaderRate) / base
		var commentatorsIncome = income * int64(pmiar.CommentatorsRate) / base
		var platformIncome = income - uploaderIncome - commentatorsIncome

		var CommentatorCmtFavourateMap = make(map[string]int64)
		var CommentatorIncomeMap = make(map[string]int64)
		var totalFavourate int64 = 0
		var totalCommentator int = 0
		for _, cci := range pmci.CCI {
			CommentatorCmtFavourateMap[cci.Commentator] = 0
			totalCommentator++
			for _, cmt := range cci.Comments {
				CommentatorCmtFavourateMap[cci.Commentator] += cmt.FavourateCnt
				totalFavourate += cmt.FavourateCnt
			}
		}

		var cmtrTotalIncome int64 = 0
		var fixedIncomeTotal = commentatorsIncome * int64(mgc.CIAR.FixedRate) / int64(mgc.CIAR.FixedRate+mgc.CIAR.FavourateRate)
		var favourateIncomeTotal = commentatorsIncome - fixedIncomeTotal
		var fixedIncome = fixedIncomeTotal / int64(totalCommentator)

		for cmtr, fav := range CommentatorCmtFavourateMap {
			var cmtrIncome = fixedIncome + fav*favourateIncomeTotal/totalFavourate
			CommentatorIncomeMap[cmtr] = cmtrIncome
			cmtrTotalIncome += cmtrIncome
		}

		if cmtrTotalIncome > commentatorsIncome {
			return nil, mglogger.Errorf("Invoke(computeMovieIncome) something wrong?(%d, %d). id=%s", cmtrTotalIncome, commentatorsIncome, movieId)
		} else {
			platformIncome += commentatorsIncome - cmtrTotalIncome
		}

		if uploaderIncome+cmtrTotalIncome+platformIncome != income {
			return nil, mglogger.Errorf("Invoke(computeMovieIncome) something wrong2?(%d, %d, %d). id=%s", uploaderIncome, cmtrTotalIncome, platformIncome, movieId)
		}

		var mit MovieIncomeTx
		mit.MovieId = movieId
		mit.DateTime = ifas.InvokeTime
		mit.TotalIncome = income
		mit.UploaderIncome = uploaderIncome
		mit.CommentatorsIncome = CommentatorIncomeMap
		mit.PlatformIncome = platformIncome
		mit.UploaderRate = pmiar.UploaderRate
		mit.CommentatorsRate = pmiar.CommentatorsRate
		mit.PlatformRate = pmiar.PlatformRate
		mit.CommentatorFavourate = CommentatorCmtFavourateMap
		mit.CommentatorFixedRate = mgc.CIAR.FixedRate
		mit.CommentatorFavourateRate = mgc.CIAR.FavourateRate

		seqKey := m.getAllocTxSeqKey(movieId)
		seq, err := Base.getTransSeq(stub, seqKey)
		if err != nil {
			return nil, mglogger.Errorf("Invoke(computeMovieIncome) getTransSeq failed. id=%s, err=%s", movieId, err)
		}
		seq++
		err = Base.setTransSeq(stub, seqKey, seq)
		if err != nil {
			return nil, mglogger.Errorf("Invoke(computeMovieIncome) setTransSeq failed. id=%s, err=%s", movieId, err)
		}

		mit.GlobalSerial = seq

		mitJson, err := json.Marshal(mit)
		if err != nil {
			return nil, mglogger.Errorf("Invoke(computeMovieIncome) Marshal failed. id=%s, err=%s", movieId, err)
		}

		var txKey = m.getAllocTxKey(movieId, seq)
		err = Base.putState_Ex(stub, txKey, mitJson)
		if err != nil {
			return nil, mglogger.Errorf("Invoke(computeMovieIncome) putState_Ex failed. id=%s, err=%s", movieId, err)
		}

		return mitJson, nil
	} else {

		//其它函数看是否是query函数
		return m.Query(stub, ifas)
	}
}

// Query callback representing the query of a chaincode
func (m *MOGAO) Query(stub shim.ChaincodeStubInterface, ifas *BaseInvokeArgs) ([]byte, error) {
	mglogger.Debug("Enter Query")
	function, args := stub.GetFunctionAndParameters()
	mglogger.Debug("func =%s, args = %+v", function, args)

	//var err error

	//var fixedArgCount = ifas.argCount

	//var userName = args[0]
	//var accName = args[1]
	/*
			//	var queryTime int64 = 0

			queryTime, err = strconv.ParseInt(args[2], 0, 64)
			if err != nil {
				return nil, mglogger.Errorf("Query convert queryTime(%s) failed. err=%s", args[2], err)
			}
		var userAttrs *UserAttrs
		var accountEnt *AccountEntity = nil
	*/

	if function == "XXXX" {
		return nil, nil

	} else {
		return nil, mglogger.Errorf("unknown function:%s.", function)
	}
}

func (m *MOGAO) getMovieInfoKey(movieId string) string {
	return MOVIE_INFO_PREFIX + movieId
}

func (m *MOGAO) getMovieCommentInfoKey(movieId string) string {
	return MOVIE_COMMENT_INFO_PREFIX + movieId
}

func (m *MOGAO) getMovieIncomeAllocRateKey(movieId string) string {
	return MOVIE_INCOME_ALLOC_RATE_PREFIX + movieId
}

func (m *MOGAO) getMovieInfo(stub shim.ChaincodeStubInterface, movieId string) (*MovieInfo, error) {
	movieBytes, err := stub.GetState(m.getMovieInfoKey(movieId))
	if err != nil {
		return nil, mglogger.Errorf("getMovieInfo: GetState failed, id=%s err=%s.", movieId, err)
	}
	if movieBytes == nil {
		return nil, nil
	}

	var mi MovieInfo
	err = json.Unmarshal(movieBytes, &mi)
	if err != nil {
		return nil, mglogger.Errorf("getMovieInfo: Unmarshal failed, id=%s err=%s.", movieId, err)
	}

	return &mi, nil
}
func (m *MOGAO) getMovieCommentInfo(stub shim.ChaincodeStubInterface, movieId string) (*MovieCommentInfo, error) {
	movieCmtBytes, err := stub.GetState(m.getMovieCommentInfoKey(movieId))
	if err != nil {
		return nil, mglogger.Errorf("getMovieCommentInfo: GetState failed, id=%s err=%s.", movieId, err)
	}
	if movieCmtBytes == nil {
		return nil, nil
	}

	var mci MovieCommentInfo
	err = json.Unmarshal(movieCmtBytes, &mci)
	if err != nil {
		return nil, mglogger.Errorf("getMovieCommentInfo: Unmarshal failed, id=%s err=%s.", movieId, err)
	}

	return &mci, nil
}
func (m *MOGAO) getMovieIncomeAllocRate(stub shim.ChaincodeStubInterface, movieId string) (*MovieIncomeAllocRateCfg, error) {
	movieIARBytes, err := stub.GetState(m.getMovieIncomeAllocRateKey(movieId))
	if err != nil {
		return nil, mglogger.Errorf("getMovieIncomeAllocRate: GetState failed, id=%s err=%s.", movieId, err)
	}
	if movieIARBytes == nil {
		return nil, nil
	}

	var miar MovieIncomeAllocRateCfg
	err = json.Unmarshal(movieIARBytes, &miar)
	if err != nil {
		return nil, mglogger.Errorf("getMovieIncomeAllocRate: Unmarshal failed, id=%s err=%s.", movieId, err)
	}

	return &miar, nil
}

func (m *MOGAO) getMovieGlobalCfg(stub shim.ChaincodeStubInterface) (*MovieGlobalCfg, error) {
	movieGlbBytes, err := stub.GetState(MOVIE_GLOBAL_CFG_KEY)
	if err != nil {
		return nil, mglogger.Errorf("getMovieGlobalCfg: GetState failed, err=%s.", err)
	}
	if movieGlbBytes == nil {
		return nil, nil
	}

	var mgc MovieGlobalCfg
	err = json.Unmarshal(movieGlbBytes, &mgc)
	if err != nil {
		return nil, mglogger.Errorf("getMovieGlobalCfg: Unmarshal failed, err=%s.", err)
	}

	return &mgc, nil
}

//获取序列号生成器的key
func (m *MOGAO) getAllocTxSeqKey(movieid string) string {
	return MOVIE_ALLOCTX_SEQ_PREFIX + movieid
}

func (m *MOGAO) getAllocTxKey(movieid string, seq int64) string {
	var buf = bytes.NewBufferString(MOVIE_ALLOCTX_PREFIX)
	buf.WriteString(movieid)
	buf.WriteString("_")
	buf.WriteString(strconv.FormatInt(seq, 10))
	return buf.String()
}
