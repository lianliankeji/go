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
var chain = hfc.newChain("smkChain");
var keyValStorePath = "/usr/local/llwork/hfc_keyValStore";

chain.setDevMode(false);
chain.setECDSAModeForGRPC(true);

chain.eventHubConnect(EVENTHUB_ADDRESS);

var eh = chain.getEventHub();

process.on('exit', function (){
  __myConsLog(" ****  smk exit **** ");
  chain.eventHubDisconnect();
  //fs.closeSync(wFd);
});

chain.setKeyValStore(hfc.newFileKeyValStore(keyValStorePath));
chain.setMemberServicesUrl(MEMBERSRVC_ADDRESS);
chain.addPeer(PEER_ADDRESS);


chain.setDeployWaitTime(55); //http请求默认超时是60s（nginx），所以这里的超时时间要少于60s，否则http请求会超时失败
chain.setInvokeWaitTime(30);

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

var admin = "admin"
var adminPasswd = "Xurw3yU9zI0l"

var getCertAttrKeys = [attrKeys.ROLE, attrKeys.USRNAME, attrKeys.USRTYPE]

var isConfidential = false;

// restfull
app.get('/smk/deploy',function(req, res){  

    res.set({'Content-Type':'text/json','Encodeing':'utf8'});  

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
                    chaincodePath: "/usr/local/llwork/smk/smkccpath",
                    confidential: isConfidential,
                };
                
                __myConsLog("===deploy begin===")

                // Trigger the deploy transaction
                var deployTx = user.deploy(deployRequest);
                
                // Print the deploy results
                deployTx.on('complete', function(results) {
                    __myConsLog("===deploy end===")
                    __myConsLog("results.chaincodeID=========="+results.chaincodeID);
                    res.send(body)
                });

                deployTx.on('error', function(err) {
                    __myConsLog("err==========%s", err.toString());
                    body.code=retCode.ERROR;
                    body.msg="deploy error"
                    res.send(body)
                });
                
                return
            })
        }
    });
});  


app.get('/smk/invoke', function(req, res) { 

    res.set({'Content-Type':'text/json','Encodeing':'utf8'});
    
    __execInvoke(req, res)
});

app.get('/smk/query', function(req, res) { 

    res.set({'Content-Type':'text/json','Encodeing':'utf8'});  

    var body = {
        code: retCode.OK,
        msg: "OK"
    };

    var enrollUser = req.query.usr;  

    chain.getUser(enrollUser, function (err, user) {
        if (err || !user.isEnrolled()) {
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
  
            var ccId = req.query.ccId;
            var func = req.query.func;


            var queryRequest = {
                
                chaincodeID: ccId,
                fcn: func,
                //attrs: getCertAttrKeys,
                userCert: TCert,
                args: [],
                confidential: isConfidential
            };   
            
            if (func == "query"){
                var acc = req.query.acc;
                queryRequest.args = [acc, enrollUser]

            } else if (func == "queryTx"){
                var begSeq = req.query.begSeq;
                if (begSeq == undefined) 
                    begSeq = "0"
                
                var endSeq = req.query.endSeq;
                if (endSeq == undefined) 
                    endSeq = "-1"
                
                var translvl = req.query.trsLvl;
                if (translvl == undefined) 
                    translvl = "2"
                
                queryRequest.args = [enrollUser, begSeq, endSeq, translvl]
                
            } else if (func == "queryTrace"){
                var wareId = req.query.wareId;
                queryRequest.args = [wareId, enrollUser]
            } else if (func == "queryDfid"){
                var dfId = req.query.dfId;
                queryRequest.args = [dfId, enrollUser]
            } 
            
            // query
            var tx = user.query(queryRequest);

            tx.on('complete', function (results) {
                body.code=retCode.OK;
                body.msg=results.result.toString()
                //var obj = JSON.parse(results.result.toString()); 
                //__myConsLog("obj=", obj)
                res.send(body)
                
                //去掉无用的信息,不打印
                queryRequest.userCert = "*"
                queryRequest.chaincodeID = "*"
                __myConsLog("Query success: request=%j, results=%s",queryRequest,results.result.toString());
            });

            tx.on('error', function (error) {

                body.code=retCode.ERROR;
                body.msg="query err"
                res.send(body)
                
                //去掉无用的信息,不打印
                queryRequest.userCert = "*"
                queryRequest.chaincodeID = "*"
                __myConsLog("Query failed : request=%j, error=%j",queryRequest,error.msg);
            });
        })
    });    
});

app.get('/smk/quotations', function(req, res) {
    res.set({'Content-Type':'text/json','Encodeing':'utf8'});
    
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
})

app.get('/smk/register', function(req, res) { 
    
    res.set({'Content-Type':'text/json','Encodeing':'utf8'});  

    var user = req.query.usr;

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
        
        var usrType = req.query.usrTp;
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
            var funcName = req.query.func
            if (funcName == "account" || funcName == "accountCB") {
                __execInvoke(req, res)
            }
        });
    });   
});

function __execInvoke(req, res) {
    var body = {
        code: retCode.OK,
        msg: "OK"
    };
    
    var enrollUser = req.query.usr;
    
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
            
            var ccId = req.query.ccId;
            var func = req.query.func;
            var acc = req.query.acc;
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
                var amt = req.query.amt;
                invokeRequest.args.push(amt)
                
            } else if (func == "transefer") {
                var reacc = req.query.reacc;
                var amt = req.query.amt;
                //var transType = req.query.transType;
                var transType = "" //暂时不用这个参数
                var dfId = req.query.dfId; //欠款融资id
                invokeRequest.args.push(reacc, transType, dfId, amt)
                
            } else if (func == "trace") { //物流溯源
                var wareId = req.query.acc;
                var msg = req.query.msg;
                invokeRequest.args.push(wareId, msg)
                
            } else if (func == "debt") { //供应商应收帐款
                var dfId = req.query.dfId; //欠款融资id
                var debtInfo = req.query.dtinfo; //欠款信息
                var contractNo = req.query.cttno; //欠款合同号
                var debtAmout = req.query.dtamt; //欠款金额
                invokeRequest.args.push(dfId, debtInfo, contractNo, debtAmout)
            } else if (func == "finance") { //金融机构申请贷款
                var dfId = req.query.dfId; //欠款融资id
                var finacInfo = req.query.fcinfo; //融资信息
                var contractNo = req.query.cttno; //融资合同号
                var finacAmout = req.query.fcamt; //融资金额
                invokeRequest.args.push(dfId, finacInfo, contractNo, finacAmout)
            }

            // invoke
            var tx = user.invoke(invokeRequest);

            tx.on('complete', function (results) {
                var retInfo = results.result.toString()  // like: "Tx 2eecbc7b-eb1b-40c0-818d-4340863862fe complete"
                var txId = retInfo.replace("Tx ", '').replace(" complete", '')
                body.msg=txId
                res.send(body)
                
                //去掉无用的信息,不打印
                invokeRequest.chaincodeID = "*"
                invokeRequest.userCert = "*"
                invokeRequest.args[2] = "*"
                __myConsLog("Invoke success: request=%j, results=%s",invokeRequest, results.result.toString());
            });
            tx.on('error', function (error) {
                body.code=retCode.ERROR;
                body.msg="tx error"
                res.send(body) 

                //去掉无用的信息,不打印
                invokeRequest.chaincodeID = "*"
                invokeRequest.userCert = "*"
                invokeRequest.args[2] = "*"
                __myConsLog("Invoke failed : request=%j, error=%j",invokeRequest,error);
            });           
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

   var header = util.format("%s [smk]: ", __getNowTime())
   arguments["0"] = header +  arguments["0"]
   console.log.apply(this, arguments)
};

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

 
app.listen(8288, "127.0.0.1");

__myConsLog("listen on 8288...");


//setTimeout(function A() {
//              process.exit(1)
//           }, 
//           2000);
