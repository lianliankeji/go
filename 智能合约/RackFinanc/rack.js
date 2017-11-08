// app index
var express = require('express');  
var app = express();  

var bodyParser = require('body-parser');  

var hfc = require('hfc');
var fs = require('fs');
var util = require('util');
const readline = require('readline');

// block
__myConsLog(" **** starting HFC sample ****");

var MEMBERSRVC_ADDRESS = "grpc://127.0.0.1:7054";
var PEER_ADDRESS = "grpc://127.0.0.1:7051";
var EVENTHUB_ADDRESS = "grpc://127.0.0.1:7053";

// var pem = fs.readFileSync('./cert/us.blockchain.ibm.com.cert'); 
var chain = hfc.newChain("rackChain");
var keyValStorePath = "/usr/local/llwork/hfc_keyValStore";

chain.setDevMode(false);
chain.setECDSAModeForGRPC(true);

chain.eventHubConnect(EVENTHUB_ADDRESS);

var eh = chain.getEventHub();

process.on('exit', function (){
  __myConsLog(" ****  rack exit **** ");
  chain.eventHubDisconnect();
  //fs.closeSync(wFd);
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
    '/rack/deploy'      : handle_deploy,
    '/rack/invoke'      : handle_invoke,
    '/rack/query'       : handle_query,
    '/rack/quotations'  : handle_quotations,
    '/rack/register'    : handle_register,
}


// restfull
function handle_deploy(params, res, req){  
    var body = {
        code : retCode.OK,
        msg: "OK"
    };

    chain.enroll(admin, adminPasswd, function (err, user) {
        if (err) {
            __myConsLog("Failed to register: error=%k",err.toString());
            body.code=retCode.ENROLL_ERR;
            body.msg="enroll error"
            res.send(body)
            return

        } else {

            var attr;
            
            user.getUserCert(attr, function (err, userCert) {
                if (err) {
                    __myConsLog("getUserCert err, ", err);
                    body.code=retCode.GETUSERCERT_ERR;
                    body.msg="getUserCert error"
                    res.send(body)
                    return
                }

                var deployRequest = {
                    fcn: "init",
                    args: [],
                    chaincodePath: "/usr/local/llwork/rack/ccpath",
                    confidential: isConfidential,
                };
                
                __myConsLog("===deploy begin===")

                // Trigger the deploy transaction
                var deployTx = user.deploy(deployRequest);
                
                var isSend = false;  //判断是否已发过回应。 有时操作比较慢时，可能超时等原因先走了'error'的流程，但是当操作完成之后，又会走‘complete’流程再次发回应，此时会发生内部错误，导致脚本异常退出
                // Print the deploy results
                deployTx.on('complete', function(results) {
                    __myConsLog("===deploy end===")
                    __myConsLog("results.chaincodeID=========="+results.chaincodeID);
                    if (!isSend) {
                        isSend = true
                        res.send(body)
                    }
                });

                deployTx.on('error', function(err) {
                    __myConsLog("deploy error: %s", err.toString());
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
        code: retCode.OK,
        msg: "OK"
    };
    
    var enrollUser = params.usr;
    
    chain.getUser(enrollUser, function (err, user) {
        if (err || !user.isEnrolled()) {
            __myConsLog("invoke: failed to get user: %s ",enrollUser, err);
            body.code=retCode.GETUSER_ERR;
            body.msg="tx error"
            res.send(body) 
            return
        }

        user.getUserCert(getCertAttrKeys, function (err, TCert) {
            if (err) {
                __myConsLog("invoke: failed to getUserCert: %s",enrollUser);
                body.code=retCode.GETUSERCERT_ERR;
                body.msg="tx error"
                res.send(body) 
                return
            }

            //__myConsLog("user(%s)'s cert:", enrollUser, TCert.cert.toString('hex'));
            
            var ccId = params.ccId;
            var func = params.func;
            var acc = params.acc;
            var invokeRequest = {
                chaincodeID: ccId,
                fcn: func,
                confidential: isConfidential,
                attrs: getCertAttrKeys,
                args: [enrollUser, acc, TCert.encode().toString('base64'),  Date.now() + ""],  //getTime()要转为字符串  时间从服务器取？  万一服务器时间不对怎么办？
                userCert: TCert
            }
            
            if (func == "account" || func == "accountCB") {
                                
            } else if (func == "issue") {
                var amt = params.amt;
                invokeRequest.args.push(amt)
                
            } else if (func == "transefer") {
                var reacc = params.reacc;
                var amt = params.amt;
                //var transType = params.transType;
                var transType = "" //暂时不用这个参数
                invokeRequest.args.push(reacc, transType, amt)
                
            } else if (func == "addRack") {
                var rackid = params.rid;
                var desc = params.desc;
                if (desc == undefined)
                    desc = ""
                var addr = params.addr;
                if (addr == undefined)
                    addr = ""
                var capacity = params.cap;
                invokeRequest.args.push(rackid, desc, addr, capacity)
            } else if (func == "finacIssue") { 
                var finacid = params.fid; 
                var finacAcc = params.fac; 
                var amtPerUnit = params.apu; 
                var totalUnit = params.tu; 
                var deadline = params.dl; 
                invokeRequest.args.push(finacid, finacAcc, amtPerUnit, totalUnit, deadline)
            } else if (func == "userBuyFinac") { 
                var finacid = params.fid; 
                var rackid = params.rid;
                var amt = params.amt; 
                invokeRequest.args.push(finacid, rackid, amt)
            } else if (func == "finacBonus") {
                var finacid = params.fid;
                var rackid = params.rid;
                var cost = params.cost;
                var earning = params.earn;
                invokeRequest.args.push(finacid, rackid, cost, earning)
            }

            // invoke
            var tx = user.invoke(invokeRequest);

            var isSend = false;  //判断是否已发过回应。 有时操作比较慢时，可能超时等原因先走了'error'的流程，但是当操作完成之后，又会走‘complete’流程再次发回应，此时会发生内部错误，导致脚本异常退出
            tx.on('complete', function (results) {
                var retInfo = results.result.toString()  // like: "Tx 2eecbc7b-eb1b-40c0-818d-4340863862fe complete"
                var txId = retInfo.replace("Tx ", '').replace(" complete", '')
                body.msg=txId
                if (!isSend) {
                    isSend = true
                    res.send(body)
                }
                
                //去掉无用的信息,不打印
                invokeRequest.chaincodeID = "*"
                invokeRequest.userCert = "*"
                invokeRequest.args[2] = "*"
                __myConsLog("Invoke success: request=%j, results=%s",invokeRequest, results.result.toString());
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
                invokeRequest.args[2] = "*"
                __myConsLog("Invoke failed : request=%j, error=%j",invokeRequest,error);
            });           
        });
    });
}

function handle_query(params, res, req) { 
    var body = {
        code: retCode.OK,
        msg: "OK"
    };

    var enrollUser = params.usr;  
    var func = params.func;

    chain.getUser(enrollUser, function (err, user) {
        if (err || !user.isEnrolled()) {
            //如果是查询账户是否存在，这里返回不存在"0"
            if (func == "queryAcc") {
                body.code = retCode.OK;
                body.msg = "0"
                res.send(body)
                return
            }

            __myConsLog("Query: failed to get user: %s",err);
            body.code=retCode.GETUSER_ERR;
            body.msg="tx error"
            res.send(body) 
            return
        }

        user.getUserCert(getCertAttrKeys, function (err, TCert) {
            if (err) {
                __myConsLog("Query: failed to getUserCert: %s",enrollUser);
                body.code=retCode.GETUSERCERT_ERR;
                body.msg="tx error"
                res.send(body) 
                return
            }
            
            //__myConsLog("user(%s)'s cert:", enrollUser, TCert.cert.toString('hex'));
            
            
            //__myConsLog("**** query Enrolled ****");
  
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
            
            if (func == "query"){
            } else if (func == "queryTx"){
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
                
            } else if (func == "queryFinac"){
                var finacid = params.fid;
                var rackid = params.rid;
                if (rackid == undefined )
                    rackid = ""
               queryRequest.args.push(finacid, rackid)
            } else if (func == "queryRack"){
                var rackid = params.rid;
                var finacid = params.fid;
                if (finacid == undefined )
                    finacid = ""
               queryRequest.args.push(rackid, finacid)
            } 
            
            // query
            var tx = user.query(queryRequest);

            var isSend = false;  //判断是否已发过回应。 有时操作比较慢时，可能超时等原因先走了'error'的流程，但是当操作完成之后，又会走‘complete’流程再次发回应，此时会发生内部错误，导致脚本异常退出
            tx.on('complete', function (results) {
                body.code=retCode.OK;
                body.msg=results.result.toString()
                //var obj = JSON.parse(results.result.toString()); 
                //__myConsLog("obj=", obj)
                if (!isSend) {
                    isSend = true
                    res.send(body)
                }
                
                //去掉无用的信息,不打印
                queryRequest.userCert = "*"
                queryRequest.chaincodeID = "*"
                var maxPrtLen = 256
                if (body.msg.length > maxPrtLen)
                    body.msg = body.msg.substr(0, maxPrtLen) + "......"
                __myConsLog("Query success: request=%j, results=%s",queryRequest, body.msg);
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
                __myConsLog("Query failed : request=%j, error=%j",queryRequest,error.msg);
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
        msg: JSON.stringify(quotations)
    };
    
    res.send(body) 
}

function handle_register(params, res, req) { 
    var user = params.usr;

    var body = {
        code: retCode.OK,
        msg: "OK"
    };
    
    chain.enroll(admin, adminPasswd, function (err, adminUser) {
        
        if (err) {
            __myConsLog("ERROR: register enroll failed. user: %s", user, err);
            body.code = retCode.ERROR
            body.msg = "register error"
            res.send(body) 
            return;
        }

        //__myConsLog("admin affiliation: %s", adminUser.getAffiliation());
        
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
        
        //__myConsLog("register: registrationRequest =", registrationRequest)
        
        chain.registerAndEnroll(registrationRequest, function(err) {
            if (err) {
                __myConsLog("register: couldn't register name ", user, err)
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
        __myConsLog("unknown user type:", usrType)
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
        __myConsLog("load passwd.");

        const rdLn = readline.createInterface({
            input: fs.createReadStream(passFile)
        });
        
        var rowCnt = 0;
        rdLn.on('line', function (line) {
            rowCnt++;
            var arr = line.split(delimiter)
            if (arr.length != 2)
                __myConsLog("line '%s' is invalid in '%s'.", line, passFile);
            else {
                if (!setUserPasswd(arr[0], arr[1]))
                    __myConsLog("initAccPassCache: set passwd(%s:%s) failed.", arr[0],  arr[1]);
            }
        });
        
        rdLn.on('close', function() {
            __myConsLog("read %d rows on Init.", rowCnt);
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
    __myConsLog("begin  at", new Date().getTime());
    var tetsObj={}
    for (var i=0; i<1000000; i++){
        tetsObj["testUserXXXXX" + i] = "Xurw3yU9zI0l"
    }
    __myConsLog("after init obj at", new Date().getTime());
    
    fs.writeFileSync(passFile, JSON.stringify(tetsObj));
    
    __myConsLog("end at", new Date().getTime());
};

function storePasswd(name, passwd) {
    var newLine = name + delimiter + passwd + endl;
    var ret = fs.writeSync(wFd, newLine)
    
    //writeSync返回的是写入字节数
    if (ret != newLine.length) {
        __myConsLog("storePasswd: write %s failed (%d,%d).", newLine, ret, newLine.length);
        return false;
    }
    fs.fsyncSync(wFd);
    return true;
}
*/

function __getNowTime() {
    var now = new Date()
    var millis = now.getMilliseconds().toString()
    var millisLen = 3
    if (millis.length < millisLen) {
        millis = "000".substr(0, millisLen - millis.length) + millis
    }
    return util.format("%s-%s.%s", now.toLocaleDateString(), now.toTimeString().substr(0,8),  millis)
}

function __myConsLog () {
   //arguments格式为{"0":xxx,"1":yyy,"2":zzzz,...}
   //如果没有输入参数，直接退出
   if (arguments["0"] == undefined) {
      return
   }

   var header = util.format("%s [rack]: ", __getNowTime())
   arguments["0"] = header +  arguments["0"]
   console.log.apply(this, arguments)
};





//公共处理
function __handle_comm__(req, res) {
    res.set({'Content-Type':'text/json','Encodeing':'utf8', 'Access-Control-Allow-Origin':'*'});

    var params
    if (req.method == "GET")
        params = req.query
    else if (req.method == "POST")
        params = req.body
    else {
        var body = {
            code : retCode.ERROR,
            msg: "only support 'GET' and 'POST' method",
            result: ""
        };
        res.send(body)
        return
    }
    path = req.route.path
    
    //调用处理函数
    return routeTable[path](params, res, req)
}

for (var path in routeTable) {
    app.get(path, __handle_comm__)
    app.post(path, __handle_comm__)
}

/*
initAccPassCache();

wFd = fs.openSync(passFile, "a")
if (wFd < 0) {
    __myConsLog("open file %s failed", passFile);
    process.exit(1)
}
*/
//for (var i=0; i<1000000; i++){
//    fs.writeSync(wFd, "testUserXXXXX" + i + delimiter + "Xurw3yU9zI0l" + endl)
//}
//for (var i=0; i<500000; i++) 
//   fs.writeSync(wFd, "testUserIIIIIIII" + i + delimiter + "500000U9zI0l" + endl)
//fs.fsyncSync(wFd);


var port = 8388
app.listen(port, "127.0.0.1");

__myConsLog("listen on %d...", port);


//setTimeout(function A() {
//              process.exit(1)
//           }, 
//           2000);
