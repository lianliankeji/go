// app index
var express = require('express');  
var app = express();  

var bodyParser = require('body-parser');  

var hfc = require('hfc');
var fs = require('fs');
var util = require('util');
const readline = require('readline');
const user = require('./lib/user');
const common = require('./lib/common');

var logger = common.createLog("alloc")
logger.setLogLevel(logger.logLevel.INFO)
//logger.setLogLevel(logger.logLevel.DEBUG)

// block
logger.info(" **** starting HFC sample ****");

var MEMBERSRVC_ADDRESS = "grpc://127.0.0.1:7054";
var PEER_ADDRESS = "grpc://127.0.0.1:7051";
var EVENTHUB_ADDRESS = "grpc://127.0.0.1:7053";

// var pem = fs.readFileSync('./cert/us.blockchain.ibm.com.cert'); 
var chain = hfc.newChain("allocChain");
var keyValStorePath = "/usr/local/llwork/hfc_keyValStore";

chain.setDevMode(false);
chain.setECDSAModeForGRPC(true);

chain.eventHubConnect(EVENTHUB_ADDRESS);

var eh = chain.getEventHub();

process.on('exit', function (){
  logger.info(" ****  alloc exit ****");
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
    '/alloc/deploy'      : {'GET' : handle_deploy,    'POST' : handle_deploy},
    '/alloc/register'    : {'GET' : handle_register,  'POST' : handle_register},
    '/alloc/invoke'      : {'GET' : handle_invoke,    'POST' : handle_invoke},
    '/alloc/query'       : {'GET' : handle_query,     'POST' : handle_query},
  //'/alloc/test'        : {'GET' : handle_test,      'POST' : handle_test},
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

// restfull
function handle_deploy(params, res, req){  
    var body = {
        code : retCode.OK,
        msg: "OK",
        result: ""
    };

    chain.enroll(admin, adminPasswd, function (err, user) {
        if (err) {
            logger.error("Failed to register: error=%k",err.toString());
            body.code=retCode.ENROLL_ERR;
            body.msg="enroll error"
            res.send(body)
            return

        } else {
            var deployRequest = {
                fcn: "init",
                //args: [Date.now().toString()], //这里不输入当前时间参数，因为fabic0.6版本，如果init输入了变量参数，每次deploy出来的chainCodeId不一致。
                args: [],
                chaincodePath: "/usr/local/llwork/alloc/alloc_ccpath",
                confidential: isConfidential,
            };

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
                logger.error("deploy error: %s", err.toString());
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
    var body = {
        code : retCode.OK,
        msg: "OK",
        result: ""
    };
    
    var enrollUser = params.usr;
    
    chain.getUser(enrollUser, function (err, user) {
        if (err || !user.isEnrolled()) {
            logger.error("invoke: failed to get user: %s ",enrollUser, err);
            body.code=retCode.GETUSER_ERR;
            body.msg="tx error"
            res.send(body) 
            return
        }

        common.getCert(keyValStorePath, enrollUser, function (err, TCert) {
            if (err) {
                logger.error("invoke: failed to getUserCert: %s. err=%s",enrollUser, err);
                body.code=retCode.GETUSERCERT_ERR;
                body.msg="tx error"
                res.send(body) 
                return
            }

            var ccId = params.ccId;
            var func = params.func;
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
            }else if (func == "allocEarning") {
                var rackid = params.rid;
                var seller = params.slr;
                var platform = params.pfm;
                var fielder = params.fld;
                var delivery = params.dvy;
                var totalAmt = params.tamt;
                var allocKey = params.ak;  
                invokeRequest.args.push(rackid, seller, fielder, delivery, platform, allocKey, totalAmt)
            }
        

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
                invokeRequest.chaincodeID = "*"
                invokeRequest.userCert = "*"
                if (func == "account" || func == "accountCB")
                    invokeRequest.args[invokeRequest.args.length - 1] = "*"
                logger.info("Invoke success: request=%j, results=%s",invokeRequest, results.result.toString());
            });
            tx.on('error', function (error) {
                body.code=retCode.ERROR;
                body.msg="tx error"
                if (!isSend) {
                    isSend = true
                    res.send(body)
                }

                //去掉无用的信息,不打印
                invokeRequest.chaincodeID = "*"
                invokeRequest.userCert = "*"
                logger.error("Invoke failed : request=%j, error=%j",invokeRequest,error);
            });           
        });
    });
}

function handle_query(params, res, req) { 
    var body = {
        code : retCode.OK,
        msg: "OK",
        result: ""
    };

    var enrollUser = params.usr;  
    var func = params.func;

    chain.getUser(enrollUser, function (err, user) {
        if (err || !user.isEnrolled()) {
            //如果是查询账户是否存在，这里返回不存在"0"
            if (func == "queryAcc") {
                body.result = "0"
                res.send(body)
                return
            }

            logger.error("Query: failed to get user: %s",err);
            body.code=retCode.GETUSER_ERR;
            body.msg="tx error"
            res.send(body) 
            return
        }

        common.getCert(keyValStorePath, enrollUser, function (err, TCert) {
            if (err) {
                logger.error("Query: failed to getUserCert: %s",enrollUser);
                body.code=retCode.GETUSERCERT_ERR;
                body.msg="tx error"
                res.send(body) 
                return
            }
            
            logger.debug("**** query Enrolled ****");
  
            var ccId = params.ccId;
            
            var acc = params.acc;
            if (acc ==undefined)  //acc可能不需要
                acc = ""


            var queryRequest = {
                chaincodeID: ccId,
                fcn: func,
                attrs: getCertAttrKeys, //代码里会获取用户的attr，这里要开启
                userCert: TCert,
                args: [enrollUser, acc],
                confidential: isConfidential
            };   
            
            if (func == "queryRackAlloc") {
                var rackid = params.rid
                var allocKey = params.ak
                if (allocKey == undefined) 
                    allocKey = ""  //查询所有分配情况

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

                queryRequest.args.push(rackid, allocKey, begSeq, count, begTime, endTime)
                
            } else if (func == "queryRackAllocCfg") {
                var rackid = params.rid
                queryRequest.args.push(rackid)
                
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
                    /* 暂时还保留字符串格式
                    //如下几种函数的result返回json格式
                    if (func == "queryRackAllocCfg" || func == "queryRackAlloc") {
                        body.result = JSON.parse(resultStr)
                    } else {
                        body.result = resultStr
                    }
                    */
                    body.result = resultStr
                    res.send(body)
                }
                
                //去掉无用的信息,不打印
                queryRequest.userCert = "*"
                queryRequest.chaincodeID = "*"
                var maxPrtLen = 256
                if (resultStr.length > maxPrtLen)
                    resultStr = resultStr.substr(0, maxPrtLen) + "......"
                logger.info("Query success: request=%j, results=%s",queryRequest, resultStr);
            });

            tx.on('error', function (error) {

                body.code=retCode.ERROR;
                body.msg="query err"
                if (!isSend) {
                    isSend = true
                    res.send(body)
                }
                
                //去掉无用的信息,不打印
                queryRequest.userCert = "*"
                queryRequest.chaincodeID = "*"
                logger.error("Query failed : request=%j, error=%j",queryRequest,error.msg);
            });
        })
    });    
}

function handle_register(params, res, req) { 
    var userName = params.usr;

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
    path = req.route.path
    
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
var port = 8488
app.listen(port, "127.0.0.1");
logger.info("listen on %d...", port);

