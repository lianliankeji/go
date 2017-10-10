// app index
var express = require('express');  
var app = express();  

var bodyParser = require('body-parser');  

var hfc = require('hfc');
var fs = require('fs');
var util = require('util');
const readline = require('readline');

// block
console.log(" **** starting HFC sample ****");

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
  console.log(" ****  frt exit **** at", __getNowTime());
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
app.get('/frt/deploy',function(req, res){  

    res.set({'Content-Type':'text/json','Encodeing':'utf8'});  

    var body = {
        code : retCode.OK,
        msg: "OK"
    };

    chain.enroll(admin, adminPasswd, function (err, user) {
        if (err) {
            console.log("Failed to register: error=%k",err.toString());
            body.code=retCode.ENROLL_ERR;
            body.msg="enroll error"
            res.send(body)
            return

        } else {

            var attr;
            
            user.getUserCert(attr, function (err, userCert) {

                console.log("enroll and getUserCert successfully!!!!!")

                if (err) {

                    console.log(err);
                }
            
                var deployRequest = {
                
                    fcn: "init",
                    args: [],
                    chaincodePath: "/usr/local/llwork/frt/frtccpath",
                    confidential: isConfidential,
                };
                
                console.log("deploy begin at" + __getNowTime())

                // Trigger the deploy transaction
                var deployTx = user.deploy(deployRequest);
                
                // Print the deploy results
                deployTx.on('complete', function(results) {
                    console.log("deploy end at" + __getNowTime())
                    console.log("results.chaincodeID=========="+results.chaincodeID);
                    res.send(body)
                });

                deployTx.on('error', function(err) {
                    console.log("err==========%s, at%s", err.toString(), __getNowTime());
                    body.code=retCode.ERROR;
                    body.msg="deploy error"
                    res.send(body)
                });
                
                return
            })
        }
    });
});  


app.get('/frt/invoke', function(req, res) { 

    res.set({'Content-Type':'text/json','Encodeing':'utf8'});
    
    __execInvoke(req, res)
});

app.get('/frt/query', function(req, res) { 

    res.set({'Content-Type':'text/json','Encodeing':'utf8'});  

    var body = {
        code: retCode.OK,
        msg: "OK"
    };

    var enrollUser = req.query.usr;  

    chain.getUser(enrollUser, function (err, user) {
        if (err || !user.isEnrolled()) {
            console.log("Query: failed to get user: %s",err);
            body.code=retCode.GETUSER_ERR;
            body.msg="tx error"
            res.send(body) 
            return
        }

        user.getUserCert(getCertAttrKeys, function (err, TCert) {
            if (err) {
                console.log("Query: failed to getUserCert: %s",enrollUser);
                body.code=retCode.GETUSERCERT_ERR;
                body.msg="tx error"
                res.send(body) 
                return
            }
            
            //console.log("user(%s)'s cert:", enrollUser, TCert.cert.toString('hex'));
            
            
            //console.log("**** query Enrolled ****");
  
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
                
            } 
            
            // query
            var tx = user.query(queryRequest);

            tx.on('complete', function (results) {
                body.code=retCode.OK;
                body.msg=results.result.toString()
                //var obj = JSON.parse(results.result.toString()); 
                //console.log("obj=", obj)
                res.send(body)

            });
            tx.on('error', function (error) {
                body.code=retCode.ERROR;
                body.msg="query err"
                res.send(body)
            });
        })
    });    
});

app.get('/frt/quotations', function(req, res) {
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

app.get('/frt/register', function(req, res) { 
    
    res.set({'Content-Type':'text/json','Encodeing':'utf8'});  

    var user = req.query.usr;

    var body = {
        code: retCode.OK,
        msg: "OK"
    };
    
    chain.enroll(admin, adminPasswd, function (err, adminUser) {
        
        if (err) {
            console.log("ERROR: register enroll failed. user: %s", user, err);
            body.code = retCode.ERROR
            body.msg = "register error"
            res.send(body) 
            return;
        }

        //console.log("admin affiliation: %s", adminUser.getAffiliation());
        
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
        
        //console.log("register: registrationRequest =", registrationRequest)
        
        chain.registerAndEnroll(registrationRequest, function(err) {
            if (err) {
                console.log("register: couldn't register name ", user, err)
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
            console.log("invoke: failed to get user: %s ",enrollUser, err);
            body.code=retCode.GETUSER_ERR;
            body.msg="tx error"
            res.send(body) 
            return
        }

        user.getUserCert(getCertAttrKeys, function (err, TCert) {
            if (err) {
                console.log("invoke: failed to getUserCert: %s",enrollUser);
                body.code=retCode.GETUSERCERT_ERR;
                body.msg="tx error"
                res.send(body) 
                return
            }

            //console.log("user(%s)'s cert:", enrollUser, TCert.cert.toString('hex'));
            
            var ccId = req.query.ccId;
            var func = req.query.func;
            var acc = req.query.acc;
            var invokeRequest = {
                chaincodeID: ccId,
                fcn: func,
                confidential: isConfidential,
                attrs: getCertAttrKeys,
                args: [enrollUser, acc, (new Date()).getTime() + ""],  //getTime()要转为字符串
                userCert: TCert
            }
            
            if (func == "account" || func == "accountCB") {
                                
            } else if (func == "issue") {
                var amt = req.query.amt;
                invokeRequest.args.push(amt)
                
            } else if (func == "transefer") {
                var reacc = req.query.reacc;
                var amt = req.query.amt;
                var transType = req.query.transType;
                invokeRequest.args.push(reacc, transType, amt)
                
            } else if (func == "support") {
                var movieId = req.query.movie
                var reacc = __getAccByMovieID(movieId)
                if (reacc == undefined) {
                    console.log("Failed to get account for movie ", movieId);
                    body.code=retCode.GETACCBYMVID_ERR;
                    body.msg="tx error"
                    res.send(body) 
                    return
                }
                
                var amt = req.query.amt;
                var transType = req.query.transType;
                invokeRequest.args.push(reacc, transType, amt)
                invokeRequest.fcn = "transefer"  //还是走的transefer
            }

            // invoke
            var tx = user.invoke(invokeRequest);

            tx.on('complete', function (results) {
                
                var retInfo = results.result.toString()  // like: "Tx 2eecbc7b-eb1b-40c0-818d-4340863862fe complete"
                //console.log("invoke completed successfully: request=%j, results=%j",invokeRequest, retInfo);
                
                var txId = retInfo.replace("Tx ", '').replace(" complete", '')
                body.msg=txId
                res.send(body)
            });
            tx.on('error', function (error) {
                
                console.log("Failed to invoke chaincode: request=%j, error=%k",invokeRequest,error);
                body.code=retCode.ERROR;
                body.msg="tx error"
                res.send(body) 
            });           
            return
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
        console.log("unknown user type:", usrType)
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
        console.log("load passwd. at ", __getNowTime());

        const rdLn = readline.createInterface({
            input: fs.createReadStream(passFile)
        });
        
        var rowCnt = 0;
        rdLn.on('line', function (line) {
            rowCnt++;
            var arr = line.split(delimiter)
            if (arr.length != 2)
                console.log("line '%s' is invalid in '%s'.", line, passFile);
            else {
                if (!setUserPasswd(arr[0], arr[1]))
                    console.log("initAccPassCache: set passwd(%s:%s) failed.", arr[0],  arr[1]);
            }
        });
        
        rdLn.on('close', function() {
            console.log("read %d rows on Init. at %s", rowCnt, __getNowTime());
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
    console.log("begin  at", new Date().getTime());
    var tetsObj={}
    for (var i=0; i<1000000; i++){
        tetsObj["testUserXXXXX" + i] = "Xurw3yU9zI0l"
    }
    console.log("after init obj at", new Date().getTime());
    
    fs.writeFileSync(passFile, JSON.stringify(tetsObj));
    
    console.log("end at", new Date().getTime());
};

function storePasswd(name, passwd) {
    var newLine = name + delimiter + passwd + endl;
    var ret = fs.writeSync(wFd, newLine)
    
    //writeSync返回的是写入字节数
    if (ret != newLine.length) {
        console.log("storePasswd: write %s failed (%d,%d).", newLine, ret, newLine.length);
        return false;
    }
    fs.fsyncSync(wFd);
    return true;
}
*/

function __getNowTime() {
    return ((new Date()).toLocaleString());
}

/*
initAccPassCache();

wFd = fs.openSync(passFile, "a")
if (wFd < 0) {
    console.log("open file %s failed", passFile);
    process.exit(1)
}
*/
//for (var i=0; i<1000000; i++){
//    fs.writeSync(wFd, "testUserXXXXX" + i + delimiter + "Xurw3yU9zI0l" + endl)
//}
//for (var i=0; i<500000; i++) 
//   fs.writeSync(wFd, "testUserIIIIIIII" + i + delimiter + "500000U9zI0l" + endl)
//fs.fsyncSync(wFd);

 
app.listen(8188, "127.0.0.1");

console.log("listen on 8188...");

