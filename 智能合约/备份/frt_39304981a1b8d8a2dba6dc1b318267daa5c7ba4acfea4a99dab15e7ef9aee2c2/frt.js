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

var logger = common.createLog("frt")

// block
logger.info(" **** starting HFC sample ****");

var MEMBERSRVC_ADDRESS = "grpc://127.0.0.1:7054";
var PEER_ADDRESS = "grpc://127.0.0.1:7051";
var EVENTHUB_ADDRESS = "grpc://127.0.0.1:7053";

// var pem = fs.readFileSync('./cert/us.blockchain.ibm.com.cert'); 
var chain = hfc.newChain("frtChain");
var keyValStorePath = "/usr/local/llwork/hfc_keyValStore";

chain.setDevMode(false);
chain.setECDSAModeForGRPC(true);

chain.eventHubConnect(EVENTHUB_ADDRESS);

var eh = chain.getEventHub();

process.on('exit', function (){
  logger.info(" ****  frt exit ****");
  chain.eventHubDisconnect();
  //fs.closeSync(wFd);
});

chain.setKeyValStore(hfc.newFileKeyValStore(keyValStorePath));
chain.setMemberServicesUrl(MEMBERSRVC_ADDRESS);
chain.addPeer(PEER_ADDRESS);


chain.setDeployWaitTime(55); //http请求默认超时是60s（nginx），所以这里的超时时间要少于60s，否则http请求会超时失败
chain.setInvokeWaitTime(20);

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
    ROLE: "role",
    USRNAME: "usrname",
    USRTYPE: "usertype"
}

var admin = "WebAppAdmin"
var adminPasswd = "DJY27pEnl16d"

var getCertAttrKeys = [attrKeys.ROLE, attrKeys.USRNAME, attrKeys.USRTYPE]

var isConfidential = false;

//注册处理函数
var routeTable = {
    '/frt/deploy'      : {'GET' : handle_deploy,    'POST' : handle_deploy},
    '/frt/register'    : {'GET' : handle_register,  'POST' : handle_register},
    '/frt/invoke'      : {'GET' : handle_invoke,    'POST' : handle_invoke},
    '/frt/query'       : {'GET' : handle_query,     'POST' : handle_query},
    '/frt/quotations'  : {'GET' : handle_quotations,'POST' : handle_quotations},
  //'/frt/test'        : {'GET' : handle_test,      'POST' : handle_test},
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

            var attr;
            
            user.getUserCert(attr, function (err, userCert) {
                if (err) {
                    logger.error("getUserCert err, ", err);
                    body.code=retCode.GETUSERCERT_ERR;
                    body.msg="getUserCert error"
                    res.send(body)
                    return
                }

                var deployRequest = {
                    fcn: "init",
                    args: [],
                    chaincodePath: "/usr/local/llwork/frt/ccpath",
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
                
                return
            })
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

        user.getUserCert(getCertAttrKeys, function (err, TCert) {
            if (err) {
                logger.error("invoke: failed to getUserCert: %s",enrollUser);
                body.code=retCode.GETUSERCERT_ERR;
                body.msg="tx error"
                res.send(body) 
                return
            }

            //logger.info("user(%s)'s cert:", enrollUser, TCert.cert.toString('hex'));
            
            var ccId = params.ccId;
            var func = params.func;
            var acc = params.acc;
            var invokeRequest = {
                chaincodeID: ccId,
                fcn: func,
                confidential: isConfidential,
                attrs: getCertAttrKeys,
                args: [enrollUser, acc,  Date.now() + ""],  //getTime()要转为字符串
                userCert: TCert
            }
            
            if (func == "account" || func == "accountCB") {
                                
            } else if (func == "issue") {
                var amt = params.amt;
                invokeRequest.args.push(amt)
                
            } else if (func == "transefer") {
                var reacc = params.reacc;
                var amt = params.amt;
                var transType = params.transType;
                if (transType == undefined)
                    transType = ""
                invokeRequest.args.push(reacc, transType, amt)
                
            } else if (func == "support") {
                var movieId = params.movie
                var reacc = __getAccByMovieID(movieId)
                if (reacc == undefined) {
                    logger.error("Failed to get account for movie ", movieId);
                    body.code=retCode.GETACCBYMVID_ERR;
                    body.msg="tx error"
                    res.send(body) 
                    return
                }
                
                var amt = params.amt;
                var transType = params.transType;
                invokeRequest.args.push(reacc, transType, amt)
                invokeRequest.fcn = "transefer"  //还是走的transefer
            }

            // invoke
            var tx = user.invoke(invokeRequest);

            var isSend = false;  //判断是否已发过回应。 有时操作比较慢时，可能超时等原因先走了'error'的流程，但是当操作完成之后，又会走‘complete’流程再次发回应，此时会发生内部错误，导致脚本异常退出
            tx.on('complete', function (results) {
                var retInfo = results.result.toString()  // like: "Tx 2eecbc7b-eb1b-40c0-818d-4340863862fe complete"
                //logger.info("invoke completed successfully: request=%j, results=%j",invokeRequest, retInfo);
                
                var txId = retInfo.replace("Tx ", '').replace(" complete", '')
                if (!isSend) {
                    isSend = true
                    body.result = {'txid': txId}
                    res.send(body)
                }
                
                //去掉无用的信息,不打印
                invokeRequest.chaincodeID = "*"
                invokeRequest.userCert = "*"
                //invokeRequest.args[2] = "*"
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
                //invokeRequest.args[2] = "*"
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

        user.getUserCert(getCertAttrKeys, function (err, TCert) {
            if (err) {
                logger.error("Query: failed to getUserCert: %s",enrollUser);
                body.code=retCode.GETUSERCERT_ERR;
                body.msg="tx error"
                res.send(body) 
                return
            }
            
            //logger.info("user(%s)'s cert:", enrollUser, TCert.cert.toString('hex'));
            
            
            //logger.info("**** query Enrolled ****");
  
            var ccId = params.ccId;
            
            var acc = params.acc;
            if (acc ==undefined)  //acc可能不需要
                acc = ""


            var queryRequest = {
                chaincodeID: ccId,
                fcn: func,
                //attrs: getCertAttrKeys,
                userCert: TCert,
                args: [enrollUser, acc],
                confidential: isConfidential
            };   
            
            if (func == "queryTx"){
                var begSeq = params.begSeq;
                if (begSeq == undefined) 
                    begSeq = "0"
                
                var endSeq = params.endSeq;
                if (endSeq == undefined) 
                    endSeq = "-1"
                
                var translvl = params.trsLvl;
                if (translvl == undefined) 
                    translvl = "2"
                
                queryRequest.args.push(begSeq, endSeq, translvl)
                
            }
            
            // query
            var tx = user.query(queryRequest);

            var isSend = false;  //判断是否已发过回应。 有时操作比较慢时，可能超时等原因先走了'error'的流程，但是当操作完成之后，又会走‘complete’流程再次发回应，此时会发生内部错误，导致脚本异常退出
            tx.on('complete', function (results) {
                body.code=retCode.OK;
                //var obj = JSON.parse(results.result.toString()); 
                //logger.info("obj=", obj)
                if (!isSend) {
                    isSend = true
                    body.result = results.result.toString()
                    res.send(body)
                }
                
                //去掉无用的信息,不打印
                queryRequest.userCert = "*"
                queryRequest.chaincodeID = "*"
                var maxPrtLen = 256
                if (body.result.length > maxPrtLen)
                    body.result = body.result.substr(0, maxPrtLen) + "......"
                logger.info("Query success: request=%j, results=%s",queryRequest, body.result);
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

function handle_quotations(params, res, req) {
    var quotations = {
        exchangeRate:   '1',
        increase:       '0.23',
        increaseRate:   '23%'
    };
    
    var body = {
        code: retCode.OK,
        msg: "OK",
        result: quotations
    };
    
    res.send(body) 
}

function handle_register(params, res, req) { 
    var user = params.usr;

    var body = {
        code : retCode.OK,
        msg: "OK",
        result: ""
    };
    
    chain.enroll(admin, adminPasswd, function (err, adminUser) {
        
        if (err) {
            logger.error("ERROR: register enroll failed. user: %s", user, err);
            body.code = retCode.ERROR
            body.msg = "register error"
            res.send(body) 
            return;
        }

        //logger.info("admin affiliation: %s", adminUser.getAffiliation());
        
        chain.setRegistrar(adminUser);
        
        var usrType = params.usrTp;
        if (usrType == undefined) {
            usrType = userType.PERSON + ""      //转为字符串格式
        }
        
        var registrationRequest = {
            roles: [ 'client' ],
            enrollmentID: user,
            registrar: adminUser,
            affiliation: __getUserAffiliation(usrType),
            //此处的三个属性名需要和chainCode中的一致
            attributes: [{name: attrKeys.ROLE, value: __getUserAttrRole(usrType)}, 
                         {name: attrKeys.USRNAME, value: user}, 
                         {name: attrKeys.USRTYPE, value: usrType}]
        };
        
        //logger.info("register: registrationRequest =", registrationRequest)
        
        chain.registerAndEnroll(registrationRequest, function(err) {
            if (err) {
                logger.error("register: couldn't register name ", user, err)
                body.code = retCode.ERROR
                body.msg = "register error"
                res.send(body) 
                return
            }

            //如果需要同时开户，则执行开户
            var funcName = params.func
            if (funcName == "account" || funcName == "accountCB") {
                handle_invoke(params, res, req)
            }
        });
    });   
}

var movieIdAccMap = {
    "0001": "lianlian",
    "0002": "lianlian",
    "unknown": "unknown"
}
function __getAccByMovieID(id) {
    return movieIdAccMap[id]
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

/*
var passFile = "/usr/local/llwork/hfc_keyValStore/user.enrollpasswd"
var wFd = -1
var delimiter = ":"
var endl = "\n"

/**
 * cache for acc and passwd.
 */
//var accPassCache={}


/**
 * init accPassCache
 */
 /*
function initAccPassCache() {
    if (fs.existsSync(passFile)) {
        logger.info("load passwd.");

        const rdLn = readline.createInterface({
            input: fs.createReadStream(passFile)
        });
        
        var rowCnt = 0;
        rdLn.on('line', function (line) {
            rowCnt++;
            var arr = line.split(delimiter)
            if (arr.length != 2)
                logger.info("line '%s' is invalid in '%s'.", line, passFile);
            else {
                if (!setUserPasswd(arr[0], arr[1]))
                    logger.info("initAccPassCache: set passwd(%s:%s) failed.", arr[0],  arr[1]);
            }
        });
        
        rdLn.on('close', function() {
            logger.info("read %d rows on Init.", rowCnt);
        })
    }
};
*/

/**
 * Set the passwd.
 * @returns error.
 */
/*
function setUserPasswd(name, passwd, isStored) {
    accPassCache[name] = passwd
    if (isStored == true) {
        return storePasswd(name, passwd)
    }
    return true;
};
*/

/**
 * Get the passwd.
 * @returns passwd
 */
/*
function getUserPasswd(name) {
    return accPassCache[name];
};


function writeManyUsers() {
    logger.info("begin  at", new Date().getTime());
    var tetsObj={}
    for (var i=0; i<1000000; i++){
        tetsObj["testUserXXXXX" + i] = "Xurw3yU9zI0l"
    }
    logger.info("after init obj at", new Date().getTime());
    
    fs.writeFileSync(passFile, JSON.stringify(tetsObj));
    
    logger.info("end at", new Date().getTime());
};

function storePasswd(name, passwd) {
    var newLine = name + delimiter + passwd + endl;
    var ret = fs.writeSync(wFd, newLine)
    
    //writeSync返回的是写入字节数
    if (ret != newLine.length) {
        logger.info("storePasswd: write %s failed (%d,%d).", newLine, ret, newLine.length);
        return false;
    }
    fs.fsyncSync(wFd);
    return true;
}
*/


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
initAccPassCache();

wFd = fs.openSync(passFile, "a")
if (wFd < 0) {
    logger.info("open file %s failed", passFile);
    process.exit(1)
}
*/
//for (var i=0; i<1000000; i++){
//    fs.writeSync(wFd, "testUserXXXXX" + i + delimiter + "Xurw3yU9zI0l" + endl)
//}
//for (var i=0; i<500000; i++) 
//   fs.writeSync(wFd, "testUserIIIIIIII" + i + delimiter + "500000U9zI0l" + endl)
//fs.fsyncSync(wFd);

 
var port = 8188
app.listen(port, "127.0.0.1");

logger.info("listen on %d...", port);

