// app index
var express = require('express');  
var app = express();  

var bodyParser = require('body-parser');  

var hfc = require('hfc');
var fs = require('fs');
var util = require('util');
//const readline = require('readline');

// block
console.log(" **** starting HFC sample ****");

var MEMBERSRVC_ADDRESS = "grpc://127.0.0.1:7054";
var PEER_ADDRESS = "grpc://127.0.0.1:7051";
var EVENTHUB_ADDRESS = "grpc://127.0.0.1:7053";

// var pem = fs.readFileSync('./cert/us.blockchain.ibm.com.cert'); 
var chain = hfc.newChain("testChain");
var keyValStorePath = "/usr/local/llwork/hfc_keyValStore";

chain.setDevMode(false);
chain.setECDSAModeForGRPC(true);

chain.eventHubConnect(EVENTHUB_ADDRESS);

var eh = chain.getEventHub();

process.on('exit', function (){
  console.log(" ****  app exit ****");
  chain.eventHubDisconnect();
  //fs.closeSync(wFd);
});

chain.setKeyValStore(hfc.newFileKeyValStore(keyValStorePath));
chain.setMemberServicesUrl(MEMBERSRVC_ADDRESS);
chain.addPeer(PEER_ADDRESS);

// parse application/x-www-form-urlencoded  
app.use(bodyParser.urlencoded({ extended: false }))  
// parse application/json  
app.use(bodyParser.json())  

var retCode = {
    OK:                     0,
    ACCOUNT_NOT_EXISTS:     1001,
    
    ERROR:                  0xffffffff
}

var admin = "admin"
var adminPasswd = "Xurw3yU9zI0l"

// restfull
app.get('/app/deploy',function(req, res){  

    res.set({'Content-Type':'text/json','Encodeing':'utf8'});  

    chain.enroll(admin, adminPasswd, function (err, user) {

        if (err) {
            console.log("Failed to register: error=%k",err.toString());
            res.send(err.toString()) 
        
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
                    chaincodePath: "/usr/local/llwork/api/apiccpath",
                    confidential: true,
                };
                
                chain.setDeployWaitTime(55); //http请求默认超时是60s（nginx），所以这里的超时时间要少于60s，否则http请求会超时失败
                
                console.log("deploy begin at" + getNowTime())

                // Trigger the deploy transaction
                var deployTx = user.deploy(deployRequest);
                
                
                
                var body = {
                    code : retCode.OK,
                    msg: "OK"
                };

                // Print the deploy results
                deployTx.on('complete', function(results) {
                    console.log("deploy end at" + getNowTime())
                    console.log("results.chaincodeID=========="+results.chaincodeID);
                    res.send(body)
                });

                deployTx.on('error', function(err) {
                    
                    console.log("err==========%s, at%s", err.toString(), getNowTime());
                    body.code=retCode.ERROR;
                    body.msg="deploy error"
                    res.send(body)
                });
            })
        }
    });
});  


app.get('/app/invoke', function(req, res) { 

    res.set({'Content-Type':'text/json','Encodeing':'utf8'});
    
    var enrollUser = req.query.usr;
    
    /*
    var enrollPasswd = getUserPasswd(enrollUser);
    if (!enrollPasswd) {
        console.log("Invoke: Failed to get passwd: %s",enrollUser);
        res.send("tx error"); 
        return
    }


    chain.enroll(enrollUser, enrollPasswd, function (err, user) {
        
        if (err) {
            console.log("ERROR: failed to register user: %s",err);
            res.send("admin" + ' not regist or pw error')
            return
        }

        //console.log("**** invoke Enrolled ****");

        var ccId = req.query.ccId;
        var func = req.query.func;

        var acc = req.query.acc;
        var reacc = req.query.reacc;
        var amt = req.query.amt;

        var invokeRequest = {
            
            chaincodeID: ccId,
            fcn: func,
            args: [acc, amt, reacc, enrollUser]
        };   
        
        // invoke
        var tx = user.invoke(invokeRequest);

        tx.on('complete', function (results) {
            
            var retInfo = results.result.toString()  // like: "Tx 2eecbc7b-eb1b-40c0-818d-4340863862fe complete"
            //console.log("invoke completed successfully: request=%j, results=%j",invokeRequest, retInfo);
            
            if (func == "transfer") {
                var txId = retInfo.replace("Tx ", '').replace(" complete", '')
                var body = {
                    code: retCode.OK,
                    msg: txId
                };

                res.send(body)
            } else {
                res.send(retInfo); 
            }

        });
        tx.on('error', function (error) {
            
            console.log("Failed to invoke chaincode: request=%j, error=%k",invokeRequest,error);

            if (func == "transfer") {
                var body = {
                    code: retCode.ERROR,
                    msg: "tx error"
                };

                res.send(body)
            } else {            
                res.send("tx error"); 
            }

        });

    });   
    */
    chain.getUser(enrollUser, function (err, user) {
        if (err || !user.isEnrolled()) {
            console.log("Query: failed to get user: %s",err);
            res.send("tx error")
            return
        }

        user.getUserCert(null, function (err, TCert) {
            if (err) {
                console.log("Query: failed to getUserCert: %s",enrollUser);
                res.send("tx error")
            }

            console.log("user(%s)'s cert:", enrollUser, TCert.cert.toString('hex'));
            
            var ccId = req.query.ccId;
            var func = req.query.func;

            var acc = req.query.acc;
            var reacc = req.query.reacc;
            var amt = req.query.amt;

            var invokeRequest = {
                
                chaincodeID: ccId,
                fcn: func,
                args: [acc, amt, reacc, enrollUser, TCert.encode().toString('base64')],
                confidential: true,
                userCert: TCert
            };   
            
            // invoke
            var tx = user.invoke(invokeRequest);

            tx.on('complete', function (results) {
                
                var retInfo = results.result.toString()  // like: "Tx 2eecbc7b-eb1b-40c0-818d-4340863862fe complete"
                //console.log("invoke completed successfully: request=%j, results=%j",invokeRequest, retInfo);
                
                if (func == "transfer") {
                    var txId = retInfo.replace("Tx ", '').replace(" complete", '')
                    var body = {
                        code: retCode.OK,
                        msg: txId
                    };

                    res.send(body)
                } else {
                    res.send(retInfo); 
                }

            });
            tx.on('error', function (error) {
                
                console.log("Failed to invoke chaincode: request=%j, error=%k",invokeRequest,error);

                if (func == "transfer") {
                    var body = {
                        code: retCode.ERROR,
                        msg: "tx error"
                    };

                    res.send(body)
                } else {            
                    res.send("tx error"); 
                }
            });           
            
        });
    });
});

app.get('/app/query', function(req, res) { 

    res.set({'Content-Type':'text/json','Encodeing':'utf8'});  

    var enrollUser = req.query.usr;  
    
    
    /*
    var enrollPasswd = getUserPasswd(enrollUser);
    if (!enrollPasswd) {
        console.log("Query: Failed to get passwd: %s",enrollUser);
        res.send("tx error"); 
        return
    }

    //console.log("query: enroll with %s : %s", enrollUser, enrollPasswd)

    chain.enroll(enrollUser, enrollPasswd, function (err, user) {
        
        if (err) {
            console.log("ERROR: failed to register user: %s",err);
            res.send("admin" + ' not regist or pw error')
            return
        }

        //console.log("**** query Enrolled ****");
  
        var ccId = req.query.ccId;
        var func = req.query.func;

        var acc = req.query.acc;

        var queryRequest = {
            
            chaincodeID: ccId,
            fcn: func,
            metadata: "133909",
            args: [acc, enrollUser]
        };   
        
        // invoke
        var tx = user.query(queryRequest);

        tx.on('complete', function (results) {
            
            //console.log("query completed successfully: request=%j, results=%j",queryRequest,results);

            res.send(results.result.toString())

        });
        tx.on('error', function (error) {
            
            console.log("Failed to query chaincode: request=%j, error=%k",queryRequest,error);

            res.send("tx error"); 

        });

    });   
    */
    chain.getUser(enrollUser, function (err, user) {
        if (err || !user.isEnrolled()) {
            console.log("Query: failed to register user: %s",err);
            res.send("tx error")
            return
        }

        user.getUserCert(null, function (err, TCert) {
            if (err) {
                console.log("Query: failed to getUserCert: %s",enrollUser);
                res.send("tx error")
            }
            
            console.log("user(%s)'s cert:", enrollUser, TCert.cert.toString('hex'));
            
            
            //console.log("**** query Enrolled ****");
  
            var ccId = req.query.ccId;
            var func = req.query.func;

            var acc = req.query.acc;

            var queryRequest = {
                
                chaincodeID: ccId,
                fcn: func,
                args: [acc, enrollUser, TCert.encode().toString('base64')],
                userCert: TCert,
                confidential: true
           };   
            
            // invoke
            var tx = user.query(queryRequest);

            tx.on('complete', function (results) {
                
                //console.log("query completed successfully: request=%j, results=%j",queryRequest,results);

                res.send(results.result.toString())

            });
            tx.on('error', function (error) {
                
                console.log("Failed to query chaincode: request=%j, error=%k",queryRequest,error);

                res.send("tx error"); 

            });
        })


    });
    
});

app.get('/app/register', function(req, res) { 
    
    res.set({'Content-Type':'text/json','Encodeing':'utf8'});  

    /*
    chain.getMember("admin", function(err, member) {
        if (err) {
            console.log("could not get member for admin", err);
            res.send("getMember error")
        }
        console.log("I did find this member", member)
    });
    */
    
    var user = req.query.usr;

    var body = {
        code: retCode.OK,
        msg: "OK"
    };
    
    /*
    if (getUserPasswd(user)) {
        console.log("register: user '%s' exists. ", user)
        res.send(body)
        return
    }*/
    
    chain.enroll(admin, adminPasswd, function (err, adminUser) {
        
        if (err) {
            console.log("ERROR: failed to register user: %s", user, err);
            body.code = retCode.ERROR
            body.msg = "register error"
            res.send(body) 
            return;
        }


        chain.setRegistrar(adminUser);
        
        var registrationRequest = {
            roles: [ 'client' ],
            enrollmentID: user,
            affiliation: "bank_a",
            //attributes: [{name:'role',value:'client'},{name:'account',value:"123-456"}],
            registrar: adminUser
        };

        /*
        chain.register(registrationRequest, function(err, enrollmentPassword) {
            if (err) {
                console.log("register: couldn't register name ", user, err)
                body.code = retCode.ERROR
                body.msg = "register error"
                res.send(body) 
                return
            }
            
            chain.enroll(user, enrollmentPassword, function(err, member) {
                if (err) {
                    console.log("register: enroll failed", err);
                    body.code = retCode.ERROR
                    body.msg = "enroll error"
                    res.send(body)
                    return
                }
                
                console.log("user(%s)'s enrollment:", user, member.getEnrollment());
                
                member.getUserCert(null, function (err, userCert) {
                    if (err) {
                        fail(t, "Failed getting Application certificate.");
                        // Exit the test script after a failure
                        process.exit(1);
                    }
                    console.log("user(%s)'s userCert:", user, userCert);
                })
                
                if (!setUserPasswd(user, enrollmentPassword, true)) {
                    console.log("register: set passwd error.")
                    body.code = retCode.ERROR
                    body.msg = "register error"
                }
                
                res.send(body)
                
            });

            
       });
       */
       
       chain.registerAndEnroll(registrationRequest, function(err) {
            if (err) {
                console.log("register: couldn't register name ", user, err)
                body.code = retCode.ERROR
                body.msg = "register error"
                res.send(body) 
                return
            }
            
            res.send(body)
                
       });

            
    });   
});


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
        console.log("load passwd. at ", getNowTime());

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
            console.log("read %d rows on Init. at %s", rowCnt, getNowTime());
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

function getNowTime() {
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

 
app.listen(8088, "127.0.0.1");

console.log("listen on 8088...");

