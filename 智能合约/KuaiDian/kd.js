// app index
const express = require('express');  
const app = express();  

//socketio功能需要使用如下的http和sockio
const http = require('http').Server(app);
//这里的path参数，client端connnect时也必须用这个参数，会拼在url请求中，io.connect("https://xxx/yyy", { path:'/kdsocketio' })。nginx中可以用它来匹配url。
const sockio = require('socket.io')(http, {path: '/kdsocketio'});  

const bodyParser = require('body-parser');  

const hfc = require('hfc');
//const fs = require('fs');
const util = require('util');

const request = require('request');
const asyncc = require('async');

//const readline = require('readline');
const user = require('./lib/user');
const common = require('./lib/common');

var logger = common.createLog("kd")
logger.setLogLevel(logger.logLevel.INFO)
//logger.setLogLevel(logger.logLevel.DEBUG)

// block
logger.info(" **** starting HFC sample ****");

var MEMBERSRVC_ADDRESS = "grpc://127.0.0.1:7054";
var PEER_ADDRESS = "grpc://127.0.0.1:7051";
var EVENTHUB_ADDRESS = "grpc://127.0.0.1:7053";

// var pem = fs.readFileSync('./cert/us.blockchain.ibm.com.cert'); 
var chain = hfc.newChain("kdChain");
var keyValStorePath = "/usr/local/llwork/hfc_keyValStore";

chain.setDevMode(false);
chain.setECDSAModeForGRPC(true);

chain.eventHubConnect(EVENTHUB_ADDRESS);

var eh = chain.getEventHub();


var socketClientCnt = 0
var sockEnvtChainUpdt = "chainDataUpdt"

process.on('exit', function (){
  logger.info(" ****  kd exit ****");
  chain.eventHubDisconnect();
  //fs.closeSync(wFd);
  user.onExit();
});

chain.setKeyValStore(hfc.newFileKeyValStore(keyValStorePath));
chain.setMemberServicesUrl(MEMBERSRVC_ADDRESS);
chain.addPeer(PEER_ADDRESS);


chain.setDeployWaitTime(55); //http请求默认超时是60s（nginx），所以这里的超时时间要少于60s，否则http请求会超时失败
chain.setInvokeWaitTime(10);

// parse application/x-www-form-urlencoded  
app.use(bodyParser.urlencoded({ extended: false }))  
// parse application/json  
app.use(bodyParser.json())  

var retCode = {
    OK:                     0,
    ACCOUNT_NOT_EXISTS:     1001,
    ENROLL_ERR:             1002,
    GETUSER_ERR:            1003,
    GETUSERCERT_ERR:        1004,
    USER_EXISTS:            1005,
    GETACCBYMVID_ERR:       1006,
    GET_CERT_ERR:           1007,
    PUT_CERT_ERR:           1008,
    
    ERROR:                  0xffffffff
}

//此处的用户类型要和chainCode中的一致
var userType = {
    CENTERBANK: 1,
    COMPANY:    2,
    PROJECT:    3,
    PERSON:     4
}

var attrRoles = {
    CENTERBANK: "centerbank",
    COMPANY:    "company",
    PROJECT:    "project",
    PERSON:     "person"
}

var attrKeys = {
    USRROLE: "usrrole",
    USRNAME: "usrname",
    USRTYPE: "usrtype"
}

var admin = "WebAppAdmin"
var adminPasswd = "DJY27pEnl16d"

var getCertAttrKeys = [attrKeys.USRROLE, attrKeys.USRNAME, attrKeys.USRTYPE]

var isConfidential = false;

//注册处理函数
var routeTable = {
    '/kd/deploy'      : {'GET' : handle_deploy,    'POST' : handle_deploy},
    '/kd/register'    : {'GET' : handle_register,  'POST' : handle_register},
    '/kd/invoke'      : {'GET' : handle_invoke,    'POST' : handle_invoke},
    '/kd/query'       : {'GET' : handle_query,     'POST' : handle_query},
    '/kd/chain'       : {'GET' : handle_chain,     'POST' : handle_chain},
    '/kd/setenv'      : {'GET' : handle_setenv,    'POST' : handle_setenv},
  //'/kd/test'        : {'GET' : handle_test,      'POST' : handle_test},
}

//for test
function handle_test(params, res, req){  
    var body = {
        code : retCode.OK,
        msg: "OK",
        result: ""
    };
    
    body.result=params
    res.send(body)
    return
}
function handle_setenv(params, res, req){  
    var body = {
        code : retCode.OK,
        msg: "OK",
        result: ""
    };
    
    var key=params.k
    var value=params.v
    
    if (key == "logLevel") {
        logger.setLogLevel(parseInt(value))
        body.result="set log level to " + value
        logger.info(body.result)
    }
    
    
    res.send(body)
    return
}


const globalCcid = "400298896ad7f0695d5e5d2379b67caa7a05930d158ccd0704ddc2696e3d156b"

// restfull
function handle_deploy(params, res, req){  
    var body = {
        code : retCode.OK,
        msg: "OK",
        result: ""
    };

    chain.enroll(admin, adminPasswd, function (err, user) {
        if (err) {
            logger.error("Failed to enroll: error=%s",err);
            body.code=retCode.ENROLL_ERR;
            body.msg="enroll error"
            res.send(body)
            return

        } else {
            var deployNo = params.dno
            
            var deployRequest = {
                fcn: "init",
                args: [],  //deploy时不要加参数，否则每次部署的chainCodeId不一致
                chaincodePath: "/usr/local/llwork/kuaidian/ccpath",
                confidential: isConfidential,
            };
            
            /*
            if (deployNo == "1") {
                deployRequest.chaincodePath = "/usr/local/llwork/KuaiDian/ccpath"
            } else if (deployNo == "2") {
                deployRequest.chaincodePath = "/usr/local/llwork/KuaiDian/alloc_ccpath"
            } else {
                logger.error("Failed to deploy: please enter deployNo.");
                body.code=retCode.ENROLL_ERR;
                body.msg="deploy error"
                res.send(body)
                return
            }
            */
            
            logger.info("===deploy begin===")

            // Trigger the deploy transaction
            var deployTx = user.deploy(deployRequest);
            
            var isSend = false;  //判断是否已发过回应。 有时操作比较慢时，可能超时等原因先走了'error'的流程，但是当操作完成之后，又会走‘complete’流程再次发回应，此时会发生内部错误，导致脚本异常退出
            // Print the deploy results
            deployTx.on('complete', function(results) {
                logger.info("===deploy end===")
                logger.info("results.chaincodeID=========="+results.chaincodeID);
                if (!isSend) {
                    isSend = true
                    body.result = results.chaincodeID
                    res.send(body)
                }
            });

            deployTx.on('error', function(err) {
                logger.error("deploy error: %j", err);
                body.code=retCode.ERROR;
                body.msg="deploy error"
                if (!isSend) {
                    isSend = true
                    res.send(body)
                }
            });
            
        }
    });
}


function handle_invoke(params, res, req) { 
    logger.info("Enter Invoke")

    var body = {
        code : retCode.OK,
        msg: "OK",
        result: ""
    };
    
    var enrollUser = params.usr;
    var func = params.func;
    
    chain.getUser(enrollUser, function (err, user) {
        if (err || !user.isEnrolled()) {
            logger.error("invoke(%s): failed to get user: %s ", func, enrollUser, err);
            body.code=retCode.GETUSER_ERR;
            body.msg="tx error"
            res.send(body) 
            return
        }

        common.getCert(keyValStorePath, enrollUser, function (err, TCert) {
            if (err) {
                logger.error("invoke(%s): failed to getUserCert: %s. err=%s", func, enrollUser, err);
                body.code=retCode.GETUSERCERT_ERR;
                body.msg="tx error"
                res.send(body) 
                return
            }

            var ccId = params.ccId;
            if (ccId ==undefined || ccId.length == 0) {
                ccId = globalCcid
            }

            var acc = params.acc;
            var invokeRequest = {
                chaincodeID: ccId,
                fcn: func,
                confidential: isConfidential,
                attrs: getCertAttrKeys,  //代码里会获取用户的attr，这里要开启
                args: [enrollUser, acc,  Date.now() + ""],  //getTime()要转为字符串
                userCert: TCert
            }

            if (func == "account" || func == "accountCB") {
                invokeRequest.args.push(TCert.encode().toString('base64'))
            } else if (func == "issue") {
                var amt = params.amt;
                invokeRequest.args.push(amt)

            } else if (func == "transefer") {
                var reacc = params.reacc;
                var amt = params.amt;
                var transType = params.tstp;
                if (transType == undefined)
                    transType = ""
                var description = params.desc;
                if (description == undefined)
                    description = ""
                var sameEntSaveTrans = params.sest; //如果转出和转入账户相同，是否记录交易 0表示不记录 1表示记录
                if (sameEntSaveTrans == undefined)
                    sameEntSaveTrans = "1" //默认记录

                invokeRequest.args.push(reacc, transType, description, amt, sameEntSaveTrans)
            } else if (func == "transeferUsePwd") {
                invokeRequest.fcn = "transefer2" //内部用transefer2
                
                var reacc = params.reacc;
                var amt = params.amt;
                var transType = params.tstp;
                if (transType == undefined)
                    transType = ""
                var description = params.desc;
                if (description == undefined)
                    description = ""
                var sameEntSaveTrans = params.sest; //如果转出和转入账户相同，是否记录交易 0表示不记录 1表示记录
                if (sameEntSaveTrans == undefined)
                    sameEntSaveTrans = "1" //默认记录
                
                var pwd = params.pwd
                if (pwd == undefined || pwd.length == 0) {
                    logger.error("invoke(%s): failed, err=pwd is empty.", func);
                    body.code=retCode.ERROR;
                    body.msg="tx error: pwd is empty."
                    res.send(body) 
                    return
                }

                invokeRequest.args.push(reacc, transType, description, amt, sameEntSaveTrans, pwd)
            } else if (func == "transeferAndLock") {
                invokeRequest.fcn = "transefer3" //内部用transefer3
                
                var reacc = params.reacc;
                var amt = params.amt;
                var transType = params.tstp;
                if (transType == undefined)
                    transType = ""
                var description = params.desc;
                if (description == undefined)
                    description = ""
                var sameEntSaveTrans = params.sest; //如果转出和转入账户相同，是否记录交易 0表示不记录 1表示记录
                if (sameEntSaveTrans == undefined)
                    sameEntSaveTrans = "1" //默认记录
                
                var lockEndTime = params.letm
                var lockAmt = params.lamt
                
                if (lockAmt > amt) {
                    logger.error("invoke(%s): failed, err=lock amount big than transefer-amount.", func);
                    body.code=retCode.ERROR;
                    body.msg="tx error: lock amount big than transefer-amount."
                    res.send(body) 
                    return
                }

                invokeRequest.args.push(reacc, transType, description, amt, sameEntSaveTrans, lockEndTime, lockAmt)
            } else if (func == "updateEnv") {
                var key = params.key;
                var value = params.val;
                invokeRequest.args.push(key, value)
            } else if (func == "setAllocCfg") {
                var rackid = params.rid;
                var seller = params.slr;
                var platform = params.pfm;
                var fielder = params.fld;
                var delivery = params.dvy;
                invokeRequest.args.push(rackid, seller, fielder, delivery, platform)
            } else if (func == "allocEarning") {
                var rackid = params.rid;
                var seller = params.slr;
                var platform = params.pfm;
                var fielder = params.fld;
                var delivery = params.dvy;
                var totalAmt = params.tamt;
                var allocKey = params.ak;  
                invokeRequest.args.push(rackid, seller, fielder, delivery, platform, allocKey, totalAmt)
            } else if (func == "setSESCfg") {
                var rackid = params.rid;
                var cfg = params.cfg;
                invokeRequest.args.push(rackid, cfg)
                
            } else if (func == "encourageScoreForSales" || func == "encourageScoreForNewRack") {
                var cfg = params.cfg;
                var transType = params.tstp;
                if (transType == undefined)
                    transType = ""
                var description = params.desc;
                if (description == undefined)
                    description = ""
                var sameEntSaveTrans = params.sest; //如果转出和转入账户相同，是否记录交易 0表示不记录 1表示记录
                if (sameEntSaveTrans == undefined)
                    sameEntSaveTrans = "1" //默认记录
                
                invokeRequest.args.push(cfg, transType, description, sameEntSaveTrans)
                
            } else if (func == "buyFinance") {
                var rackid = params.rid;
                var financid = params.fid;
                var payee = params.pee;
                var amout = params.amt;
                var transType = params.tstp;
                if (transType == undefined)
                    transType = ""
                var description = params.desc;
                if (description == undefined)
                    description = ""
                var sameEntSaveTrans = params.sest; //如果转出和转入账户相同，是否记录交易 0表示不记录 1表示记录
                if (sameEntSaveTrans == undefined)
                    sameEntSaveTrans = "1" //默认记录
                
                invokeRequest.args.push(rackid, financid, payee, amout, transType, description, sameEntSaveTrans)
                
            } else if (func == "financeIssueFinish") {
                var financid = params.fid;
                invokeRequest.args.push(financid)
                
            } else if (func == "payFinance") {
                var rackid = params.rid;
                var payee = params.pee;
                var transType = params.tstp;
                if (transType == undefined)
                    transType = ""
                var description = params.desc;
                if (description == undefined)
                    description = ""
                var sameEntSaveTrans = params.sest; //如果转出和转入账户相同，是否记录交易 0表示不记录 1表示记录
                if (sameEntSaveTrans == undefined)
                    sameEntSaveTrans = "1" //默认记录
                
                invokeRequest.args.push(rackid, payee, transType, description, sameEntSaveTrans)
                
            } else if (func == "financeBouns") {
                var financid = params.fid;
                var rackSalesCfg = params.rscfg;
                invokeRequest.args.push(financid, rackSalesCfg)
                
            } else if (func == "setFinanceCfg") {
                var rackid = params.rid;
                var profitsPercent = params.prop;
                var investProfitsPercent = params.ivpp;
                var investCapacity = params.ivc;
                invokeRequest.args.push(rackid, profitsPercent, investProfitsPercent, investCapacity)
                
            } else if (func == "updateCert") {
                var upUser = params.uusr;
                var upAcc = params.uacc;
                var upCert = params.ucert;
                invokeRequest.args.push(upUser, upAcc, upCert)
                
            } else if (func == "AuthCert") {
                var authAcc = params.aacc;
                var authUser = params.ausr;
                var authCert = params.acert;
                invokeRequest.args.push(authAcc, authUser, authCert)
                
            } else if (func == "setWorldState") {
                var fileName = params.fnm;
                var needHash = params.hash;
                if (needHash == undefined)
                    needHash = "0"
                var sameKeyOverwrite = params.skow;
                if (sameKeyOverwrite == undefined)
                    sameKeyOverwrite = "1"  //默认相同的key覆盖
                
                var srcCcid = params.sccid;
                
                invokeRequest.args.push(fileName, needHash, sameKeyOverwrite, srcCcid)
            }

            __invokePreCheck(func, params, user, TCert, function(err, checkOk){
                // invoke
                var tx = user.invoke(invokeRequest);

                var isSend = false;  //判断是否已发过回应。 有时操作比较慢时，可能超时等原因先走了'error'的流程，但是当操作完成之后，又会走‘complete’流程再次发回应，此时会发生内部错误，导致脚本异常退出
                tx.on('complete', function (results) {
                    var retInfo = results.result.toString()  // like: "Tx 2eecbc7b-eb1b-40c0-818d-4340863862fe complete"
                    logger.debug("invoke completed successfully: results=%j", retInfo);
                    
                    var txId = retInfo.replace("Tx ", '').replace(" complete", '')
                    if (!isSend) {
                        isSend = true
                        body.result = {'txid': txId}
                        res.send(body)
                    }
                    
                    //去掉无用的信息,不打印
                    invokeRequest.chaincodeID = invokeRequest.chaincodeID.substr(0,3) + "*" //打印前四个字符，看id是否正确
                    invokeRequest.userCert = "*"
                    if (func == "account" || func == "accountCB")
                        invokeRequest.args[invokeRequest.args.length - 1] = "*"
                    logger.info("Invoke success: request=%j, results=%s",invokeRequest, results.result.toString());
                    
                    //通知前端数据更新
                    if (socketClientCnt > 0) {
                        __getPushDataForWeb(function(data) {
                            sockio.sockets.emit(sockEnvtChainUpdt,data);//给所有客户端广播消息
                        })
                    }
                    
                });
                tx.on('error', function (error) {
                    body.code=retCode.ERROR;
                    body.msg="tx error"
                    if (!isSend) {
                        isSend = true
                        res.send(body)
                    }

                    //去掉无用的信息,不打印
                    invokeRequest.chaincodeID = invokeRequest.chaincodeID.substr(0,3) + "*" //打印前四个字符，看id是否正确
                    invokeRequest.userCert = "*"
                    if (func == "account" || func == "accountCB")
                        invokeRequest.args[invokeRequest.args.length - 1] = "*"
                    logger.error("Invoke failed : request=%j, error=%j",invokeRequest,error);
                });
            })
        });
    });
}

function __invokePreCheck(invokeFunc, invokeParams, user, TCert, cb) {
    
    /*
    if (invokeFunc == "transefer" || invokeFunc == transeferUsePwd) {
        var queryParams = {}
        queryParams.func = "transPreCheck"
        queryParams.usr = invokeParams.usr
        queryParams.ccId = invokeParams.ccId
        queryParams.acc = invokeParams.acc
        queryParams.reacc = invokeParams.reacc
        queryParams.amt = invokeParams.amt
        queryParams.pwd = invokeParams.pwd

        __execLiteQuery(queryParams, null, user, TCert, function(err, qBody, queryRequest, resultStr){
            if (err) {
                return cb(err)
            }
        })
        
    } else {
        cb(null, true)
    }
    */
    cb(null, true)
}

var queryCache = {}

function handle_query(params, res, req) {
    var func = params.func;

    //getInfoForWeb 做个缓存
    if (func == "getInfoForWeb") {
        var funcCache = queryCache[func]
        if (funcCache != undefined) {
            var nowTime = Date.now() / 1000
            if (Math.abs(nowTime - funcCache.lastQTm) < funcCache.qIntv) {
                return res.send(funcCache.body)
            }
        }
    }

    logger.info("Enter Query")
    
    __execQuery(params, req, true, function(err, qBody){
        if (err) {
            return res.send(qBody)
        }

        if (func == "getInfoForWeb") {
            var funcCache = queryCache[func]
            if (funcCache == undefined) {
                var tmpCache = {}
                tmpCache.body = qBody
                tmpCache.lastQTm = Date.now() / 1000
                tmpCache.qIntv = 10  //刷新间隔
                queryCache[func] = tmpCache
            } else {
                funcCache.body = qBody
                funcCache.lastQTm = Date.now() / 1000
            }
        }

        return res.send(qBody)
    })
}

//轻量级查询，直接查询，不获取usr和cert等信息
function __execLiteQuery(params, req, user, TCert, outputQReslt, cb) { 

    logger.debug("**** enter __execLiteQuery ****");

    var body = {
        code : retCode.OK,
        msg: "OK",
        result: ""
    };

    var enrollUser = params.usr;  
    var func = params.func;

    var ccId = params.ccId;
    if (ccId ==undefined || ccId.length == 0) {
        ccId = globalCcid
    }

    var acc = params.acc;
    /* 现在都需要账户acc
    if (acc == undefined)  //acc可能不需要
        acc = ""
    */

    var queryRequest = {
        chaincodeID: ccId,
        fcn: func,
        attrs: getCertAttrKeys, //代码里会获取用户的attr，这里要开启
        userCert: TCert,
        args: [enrollUser, acc, Date.now() + ""],
        confidential: isConfidential
    };
    
    if (func == "getTransInfo"){
        var begSeq = params.bsq;
        if (begSeq == undefined) 
            begSeq = "0"
        
        var count = params.cnt;
        if (count == undefined) 
            count = "-1"  //-1表示查询所有
        
        var translvl = params.trsLvl;
        if (translvl == undefined) 
            translvl = "2"
        
        var begTime = params.btm;
        if (begTime == undefined) 
            begTime = "0"

        var endTime = params.etm;
        if (endTime == undefined) 
            endTime = "-1"  //-1表示查询到最新的时间

        var qAcc = params.qacc;
        if (qAcc == undefined) 
            qAcc = ""

        var maxSeq = params.msq;
        if (maxSeq == undefined) 
            maxSeq = "-1" //不输入默认为-1

        var order = params.ord;
        if (order == undefined) 
            order = "desc" //不输入默认为降序，即从最新的数据查起

        queryRequest.args.push(begSeq, count, translvl, begTime, endTime, qAcc, maxSeq, order)
        
    } else if (func == "queryRackAlloc") {
        var rackid = params.rid
        var allocKey = params.ak
        if (allocKey == undefined) 
            allocKey = ""  //有值说明查询某次的分陪情况

        var begSeq = params.bsq;
        if (begSeq == undefined) 
            begSeq = "0"
        
        var count = params.cnt;
        if (count == undefined) 
            count = "-1"  //-1表示查询所有

        var begTime = params.btm;
        if (begTime == undefined)
            begTime = "0"

        var endTime = params.etm;
        if (endTime == undefined) 
            endTime = "-1"  //-1表示查询到最新的时间

        var qAcc = params.qacc;
        if (qAcc == undefined) 
            qAcc = ""    //有值说明查询某个账户的分配情况
        
        queryRequest.args.push(rackid, allocKey, begSeq, count, begTime, endTime, qAcc)
        
    } else if (func == "getRackAllocCfg" || func == "getSESCfg" || func == "getRackFinanceCfg") {
        var rackid = params.rid
        queryRequest.args.push(rackid)
        
    } else if (func == "queryState"){
        var key = params.key
        queryRequest.args.push(key)
    } else if (func == "getRackFinanceProfit") {
        var rackid = params.rid
        queryRequest.args.push(rackid)
    } else if (func == "getRackRestFinanceCapacity") {
        var rackid = params.rid
        var fid = params.fid
        queryRequest.args.push(rackid, fid)
    } else if (func == "getWorldState") {
        var needHash = params.hash
        if (needHash == undefined) 
            needHash = "0"    //默认不用hash
        var flushLimit = params.flmt
        if (flushLimit == undefined) 
            flushLimit = "-1"    //默认不用hash
        queryRequest.args.push(needHash, flushLimit, queryRequest.chaincodeID)
    } else if (func == "transPreCheck") {
        var reacc = params.reacc
        var amt = params.amt
        var pwd = params.pwd
        if (pwd == undefined)
            pwd = ""

        queryRequest.args.push(reacc, pwd, amt)

    } else if (func == "getInfoForWeb") {
        queryRequest.args.push("kdcoinpool") //目前计算流通货币的账户
    }
    
    // query
    var tx = user.query(queryRequest);

    var isSend = false;  //判断是否已发过回应。 有时操作比较慢时，可能超时等原因先走了'error'的流程，但是当操作完成之后，又会走‘complete’流程再次发回应，此时会发生内部错误，导致脚本异常退出
    tx.on('complete', function (results) {
        body.code=retCode.OK;
        //var obj = JSON.parse(results.result.toString()); 
        //logger.debug("obj=", obj)
        var resultStr = results.result.toString()
        if (!isSend) {
            isSend = true
            
            if (func == "getInfoForWeb") {
                //获取区块链信息
                __getChainInfoForWeb(function (err, chain){
                    if (err) {
                        logger.error("__getChainInfoForWeb err:", err)
                        body.code=retCode.GETUSER_ERR;
                        body.msg="getChainInfo error"
                        return cb(err, body)
                    }

                    logger.debug("chain %j", chain);
                    var queryObj = JSON.parse(resultStr)
                   
                    chain.accountCnt = queryObj.accountcount
                    chain.issuedAmt = queryObj.issueamt
                    chain.totalAmt = queryObj.issuetotalamt    
                    chain.circulateAmt = queryObj.circulateamt
                    chain.issueBeg = "201801"       //当前发行周期的起始日期
                    chain.issueEnd = "201812"       //当前发行周期的结束日期
                    
                    body.result = chain
                    
                    cb(null, body)
                })
            } else {
                if (func == "getTransInfo" || func == "getBalanceAndLocked") { //如下几种函数的result返回json格式
                    body.result = JSON.parse(resultStr)
                } else {
                    body.result = resultStr
                }
                cb(null, body)
            }
            
            if (outputQReslt == true) {
                //去掉无用的信息,不打印
                queryRequest.userCert = "*" 
                queryRequest.chaincodeID = queryRequest.chaincodeID.substr(0,3) + "*" //打印前四个字符，看id是否正确
                var maxPrtLen = 256
                if (resultStr.length > maxPrtLen)
                    resultStr = resultStr.substr(0, maxPrtLen) + "......"
                logger.info("Query success: request=%j, results=%s",queryRequest, resultStr);
            }
        }
    });

    tx.on('error', function (error) {
        if (!isSend) {
            isSend = true
            body.code=retCode.GETUSER_ERR;
            body.msg="query error"
            cb(error, body)
            
            if (outputQReslt == true) {
                //去掉无用的信息,不打印
                queryRequest.userCert = "*"
                queryRequest.chaincodeID = queryRequest.chaincodeID.substr(0,3) + "*" //打印前四个字符，看id是否正确
                logger.error("Query failed : request=%j, error=%j", queryRequest, error.msg);
            }
        }
    });
}

function __execQuery(params, req, outputQReslt, cb) { 
    var body = {
        code : retCode.OK,
        msg: "OK",
        result: ""
    };

    logger.debug("Enter __execQuery")
    
    var enrollUser = params.usr;  
    var func = params.func;

    chain.getUser(enrollUser, function (err, user) {
        if (err || !user.isEnrolled()) {
            //获取用户失败，或者用户没有登陆。一般情况是没有注册该用户。

            if (func == "isAccExists") { //如果是查询账户是否存在，这里返回不存在"0"
                body.result = "0"
                cb(null, body)
                logger.warn("Query(%s): getUser failed, user %s not exsit, return 0.", func, enrollUser)
                return 
            } else if (func == "getBalance") { //如果是查询账户余额，这里返回"0"
                body.result = "0"
                cb(null, body)
                logger.warn("Query(%s): getUser failed, user %s not exsit, return 0.", func, enrollUser)
                return
            }

            body.code=retCode.GETUSER_ERR;
            body.msg="query error"
            cb(logger.errorf("Query(%s): failed to get user %s, err=%s", func, enrollUser, err),  body)
            return
        }

        common.getCert(keyValStorePath, enrollUser, function (err, TCert) {
            if (err) {
                //目前发生错误的情况为证书文件不存在，是因为证书认证这个功能没加以前，就存在了一些用户 ，所以上面的getUser成功，而这里失败。 
                //失败时，暂时特殊处理isAccExists和getBalance两个函数
                if (func == "isAccExists") { //如果是查询账户是否存在，这里返回不存在"0"
                    body.result = "0"
                    cb(null, body)
                    logger.warn("Query(%s): getCert failed, user %s not exsit, return 0.", func, enrollUser)
                    return
                } else if (func == "getBalance") { //如果是查询账户余额，这里返回"0"
                    body.result = "0"
                    cb(null, body)
                    logger.warn("Query(%s): getCert failed, user %s not exsit, return 0.", func, enrollUser)
                    return
                }
            
                body.code=retCode.GETUSERCERT_ERR;
                body.msg="query error"
                cb(logger.errorf("Query(%s): failed to getUserCert %s, err=%s", func, enrollUser, err), body)
                return
            }
            
            logger.debug("**** run __execLiteQuery ****");
  
            __execLiteQuery(params, req, user, TCert, outputQReslt, function(err, qBody){
                if (err) {
                    return cb(err, qBody)
                }

                cb(null, qBody)
            })
        })
    });
}


function handle_register(params, res, req) { 
    var userName = params.usr;

    logger.info("Enter Register")

    var body = {
        code : retCode.OK,
        msg: "OK",
        result: ""
    };
    
    chain.enroll(admin, adminPasswd, function (err, adminUser) {
        
        if (err) {
            logger.error("ERROR: register enroll failed. user: %s", userName);
            body.code = retCode.ERROR
            body.msg = "register error"
            res.send(body) 
            return;
        }

        logger.debug("admin affiliation: %s", adminUser.getAffiliation());
        
        chain.setRegistrar(adminUser);
        
        var usrType = params.usrTp;
        if (usrType == undefined) {
            usrType = userType.PERSON + ""      //转为字符串格式
        }
        
        var registrationRequest = {
            roles: [ 'client' ],
            enrollmentID: userName,
            registrar: adminUser,
            affiliation: __getUserAffiliation(usrType),
            //此处的三个属性名需要和chainCode中的一致
            attributes: [{name: attrKeys.USRROLE, value: __getUserAttrRole(usrType)}, 
                         {name: attrKeys.USRNAME, value: userName}, 
                         {name: attrKeys.USRTYPE, value: usrType}]
        };
        
        logger.debug("register: registrationRequest =", registrationRequest)
        
        chain.registerAndEnroll(registrationRequest, function(err) {
            if (err) {
                logger.error("register: couldn't register name ", userName, err)
                body.code = retCode.ERROR
                body.msg = "register error"
                res.send(body) 
                return
            }

            //如果需要同时开户，则执行开户
            var funcName = params.func
            if (funcName == "account" || funcName == "accountCB") {
                chain.getUser(userName, function (err, user) {
                    if (err || !user.isEnrolled()) {
                        logger.error("register: failed to get user: %s ",userName, err);
                        body.code=retCode.GETUSER_ERR;
                        body.msg="tx error"
                        res.send(body) 
                        return
                    }
            
                    user.getUserCert(null, function (err, TCert) {
                        if (err) {
                            logger.error("register: failed to getUserCert: %s",userName);
                            body.code=retCode.GETUSERCERT_ERR;
                            body.msg="tx error"
                            res.send(body) 
                            return
                        }
                        logger.debug("%s putCert's pk=[%s] cert=[%s]", userName, TCert.privateKey.getPrivate('hex'), TCert.encode().toString('base64'))

                        common.putCert(keyValStorePath, userName, TCert, function(err){
                            if (err) {
                                logger.error("register: failed to putCert: %s",userName);
                                body.code=retCode.PUT_CERT_ERR;
                                body.msg="tx error"
                                res.send(body) 
                                return
                            }

                            handle_invoke(params, res, req)
                        })
                    })
                })
            }
        });
    });   
}

//ip
const CHAIN_IP = "127.0.0.1"
//节点信息
const URL_CHAIN_PEER    = util.format("http://%s:7050/network/peers", CHAIN_IP)
//交易信息 http://127.0.0.1:7050/transactions/{UUID}
const URL_CHAIN_TX       = util.format("http://%s:7050/transactions/", CHAIN_IP)
//当前链信息
const URL_CHAIN_INFO     = util.format("http://%s:7050/chain", CHAIN_IP)
//链上交易信息 http://127.0.0.1:7050/chain/blocks/{NUM}
const URL_CHAIN_BLOCK    = util.format("http://%s:7050/chain/blocks/", CHAIN_IP)

function handle_chain(params, res, req) {
    logger.info("Enter Chain")

    var body = {
        code : retCode.OK,
        msg: "OK",
        result: ""
    };
    
    var funcName = params.func
    /*
    if (funcName == "") {
        //并行查询
        async.parallel(
        [
          function(callback) {
            
          },
          function(callback) {
            
          }
        ],
         
         function(err, results) {
            // the results array will equal ['one','two'] even thoug
            // the second function had a shorter             
            timeout
         }
        ); 
    }
    */
    
}

/*
    chainInfo = {
        latestBlock : 120,
        nodesCnt : 4,
        txRecords: [
            {
                node: "",
                txid: "xxxx",
                txInfo: "xxxx",
                block: 10,
                seconds: 1517191532
            }
        ]
    }
*/
function __getChainInfoForWeb(cb) {
    
    var chainInfo = {}
    
    request(URL_CHAIN_INFO, function (err, resp, body) {
        if (err || resp.statusCode != 200) {
            logger.error("request(%s) error: %j, resp=%j", URL_CHAIN_INFO, err, resp);
            return cb(err)
        }

        var chainObj = JSON.parse(body)
        var latestBlockNum = chainObj.height - 1 //从0开始编号，这里减1
        chainInfo.latestBlock = latestBlockNum
        
        
        request(URL_CHAIN_PEER, function (err, resp, body) {
            if (err || resp.statusCode != 200) {
                logger.error("request(%s) error: %j, resp=%j", URL_CHAIN_PEER, err, resp);
                return cb(err)
            }

            var peersObj = JSON.parse(body)
            chainInfo.nodesCnt = peersObj.peers.length
            
            
            //查询交易信息
            var txRecords = []
            //目前只取最新的5条记录
            __getBlockInfo(latestBlockNum, 5, txRecords, function(err){
                chainInfo.txRecords = txRecords
                cb (null, chainInfo)
            })
        })
    })
}

//txRecords作为入参传入，因为里面有递归调用，如果在本函数里用局部变量定义无法在递归中传递
function __getBlockInfo(latestBlockNum, queryTxCnt, txRecords, cb) {
    //从1开始，0块没有交易信息
    if (latestBlockNum > 0) {
        var txRecdPerBlock = 1 //目前一个区块记录一条交易
        var queryBlockCnt = Math.ceil(queryTxCnt / txRecdPerBlock)  //用需要的记录数除以每个区块的交易数，并向上取整，得到要查询的区块数
        
        var begIdx = latestBlockNum - queryBlockCnt + 1
        if (begIdx < 1 )
            begIdx = 1

        var blockList = []
        for (var i=begIdx; i<=latestBlockNum; i++) {
            blockList[i-begIdx] = i
        } 
       
        var tmpRecds = {}
        var keyList = []
        asyncc.map(blockList, function(blockIdx, callback) {
            request(URL_CHAIN_BLOCK + blockIdx, function (err, resp, body) {
                if (err || resp.statusCode != 200) {
                    logger.error("request(%s) error: %j, resp=%j", URL_CHAIN_BLOCK, err, resp);
                    return cb(err)
                }
                
                var blockObj = JSON.parse(body)
                //有的块没有交易
                if (blockObj.transactions != undefined) {
                    //var txIdx = latestBlockNum - blockIdx      //倒序
                    tmpRecds[blockIdx] = {}
                    tmpRecds[blockIdx].block = blockIdx
                    tmpRecds[blockIdx].txid = blockObj.transactions[0].txid //目前一个块记录一条交易，所以这里只取第一个位置即可
                    tmpRecds[blockIdx].seconds = blockObj.transactions[0].timestamp.seconds
                    tmpRecds[blockIdx].txInfo =  blockObj.transactions[0].payload
                    
                    keyList[keyList.length] = blockIdx
                } 
                
                callback(null, null) //必须调用一下callback，否则不会认为执行完毕
            })}, function(err, results) {
                
                keyList.sort(__sort_down)  //降序
                //最新的数据放在最上面
                for (var i=0; i<keyList.length; i++) {
                    var recd = tmpRecds[keyList[i]]
                    var arr = (new Buffer(recd.txInfo,'base64')).toString().split('\n')
                    /*
                    for (var i=0; i<arr.length; i++){
                        arr[i] = arr[i].trim()
                        logger.debug("arr[%d]=[%s]", i, arr[i]);
                    }
                    */
                    // arr的第1个元素为空，第二个元素包含ccid， 第三个为invode函数名，后面为参数列表
                    //var invokeFunc = arr[2]
                    if (arr.length < 5) //小于5没有参数列表(目前参数至少是2个参数)，可能是init调用，不显示
                        continue

                    var accountName = arr[4].trim()  //第5个元素为账户信息
                    //先过滤centerBank和kdcoinpool的交易
                    if (accountName.indexOf("centerBank") >= 0 || accountName.indexOf("kdcoinpool") >= 0) {
                        accountName = (new Buffer(common.md5sum(accountName), 'hex')).toString('base64')
                    }
 
                    var txIdx = txRecords.length
                    txRecords[txIdx] = recd
                    txRecords[txIdx].node = accountName

                    //最多记录 queryTxCnt 条
                    if (txIdx >= queryTxCnt - 1)
                        break
                }
                
                //记录不够，再查一次
                if (txRecords.length < queryTxCnt) {
                    //从上次查到的最小序列号开始
                    __getBlockInfo(keyList[keyList.length-1] - 1, queryTxCnt, txRecords, cb)
                } else {
                    cb (null)
                }
            }
        )
    } else {
        // latestBlockNum 小于等于0，说明已处理完毕
        cb (null)
    }
}


function __getUserAttrRole(usrType) {
    if (usrType == userType.CENTERBANK) {
        return attrRoles.CENTERBANK
    } else if (usrType == userType.COMPANY) {
        return attrRoles.COMPANY
    } else if (usrType == userType.PROJECT) {
        return attrRoles.PROJECT
    } else if (usrType == userType.PERSON) {
        return attrRoles.PERSON
    } else {
        logger.error("unknown user type:", usrType)
        return "unknown"
    }
}

function __getUserAffiliation(usrType) {
    return "bank_a"
}


/* socket.io 处理 begin  */
function __getPushDataForWeb(cb) { 

    var params = {}
    var req = null
    
    params.func = "getInfoForWeb"
    params.usr = "centerBank"
    params.acc = "centerBank"

    logger.debug('enter getPushData.')
    
    __execQuery(params, req, false, function(err, qBody){
        logger.debug('enter getPushData callback. err=%s', err)
        
        if (err) {
            return cb(qBody)
        }

        logger.debug('enter getPushData callback, data=%j', qBody)
        
        return cb(qBody)
    })
}

//of的内容getchaininfo，是跟在客户端请求的url中，ip后面。 sockio中叫namespace。 客户端请求时，url使用，例如io.connect("https://XXX/getchaininfo") 用这个参数可以区分多种请求
sockio.of('/getchaininfo') 
  .on('connection', function(socket){
    socketClientCnt++
    logger.info('a user connected, client count=%d', socketClientCnt);

    __getPushDataForWeb(function(data) {
        socket.emit(sockEnvtChainUpdt, data);
    })
    
    socket.on('disconnect', function(){
        socketClientCnt--
        logger.info('user disconnected, client count=%d', socketClientCnt);
    });
});
/* socket.io 处理   end  */



function __sort_down(x, y) {
    return (x < y) ? 1 : -1      
}

//公共处理
function __handle_comm__(req, res) {
    res.set({'Content-Type':'text/json','Encodeing':'utf8', 'Access-Control-Allow-Origin':'*'});

    var params
    var method = req.method
    
    if (method == "GET")
        params = req.query
    else if (method == "POST")
        params = req.body
    else {
        var body = {
            code : retCode.ERROR,
            msg: "only support 'GET' and 'POST' method.",
            result: ""
        };
        res.send(body)
        return
    }
    var path = req.route.path
    
    var handle = routeTable[path][method]
    if (handle == undefined) {
        var body = {
            code : retCode.ERROR,
            msg: util.format("path '%s' do not support '%s' method.", path, method),
            result: ""
        };
        res.send(body)
        return
    }
    
    //调用处理函数
    return handle(params, res, req)
}

for (var path in routeTable) {
    app.get(path, __handle_comm__)
    app.post(path, __handle_comm__)
}

/*
user.init(function(err) {
    if (err) {
        logger.error("init error, exit. err=%s", err)
        process.exit(1) //调用这个接口触发exit事件
    }
    logger.info("init ok.")
 
    var port = 8188
    app.listen(port, "127.0.0.1");
    logger.info("listen on %d...", port);
})
*/



var kdport = 8588

http.listen(kdport, "127.0.0.1");
logger.info("default ccid : %s", globalCcid);
logger.info("listen on %d...", kdport);


