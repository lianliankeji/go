// app index
var express = require('express');  
var app = express();  

var bodyParser = require('body-parser');  

var hfc = require('hfc');
var fs = require('fs');
var util = require('util');

// block
__myConsLog(" **** starting HFC sample ****");

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
  __myConsLog(" ****  app exit ****");
  chain.eventHubDisconnect();
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

var adminUser = "admin"
var adminPasswd = "Xurw3yU9zI0l"

// restfull
app.get('/app/deploy',function(req, res){  

    res.set({'Content-Type':'text/json','Encodeing':'utf8'});  

    chain.enroll(adminUser, adminPasswd, function (err, user) {

        if (err) {
            __myConsLog("Failed to register: error=%s",err.toString());
            res.send(err.toString()) 
        
        } else {

            var attr;
            
            user.getUserCert(attr, function (err, userCert) {

                __myConsLog("enroll and getUserCert successfully!!!!!")

                if (err) {

                    __myConsLog(err);
                }
            
                var deployRequest = {
                
                    fcn: "init",
                    args: [],
                    chaincodePath: "/usr/local/llwork/api/apiccpath"
                };

                // Trigger the deploy transaction
                var deployTx = user.deploy(deployRequest);

                // Print the deploy results
                deployTx.on('complete', function(results) {
                    
                    __myConsLog("results.chaincodeID=========="+results.chaincodeID);

                });

                deployTx.on('error', function(err) {
                    
                    __myConsLog("err=========="+err.toString());
                });

                var body = {

                    "results": "OK"
                };

                res.send(body)
            })

        }

    });

});  

app.get('/app/invoke', function(req, res) { 

    res.set({'Content-Type':'text/json','Encodeing':'utf8'});
    
    /*
    var enrollUser = req.query.acc;  //use account
    
    var enrollPasswd, err = getUserPasswd(acc);
    if (err) {
        res.send("error"); 
        return
    }
    */


    chain.enroll(adminUser, adminPasswd, function (err, user) {
        
        if (err) {
            __myConsLog("ERROR: failed to register user: %s",err);
            res.send("admin" + ' not regist or pw error')
        }
                
        var ccId = req.query.ccId;
        var func = req.query.func;

        var acc = req.query.acc;
        if (acc == undefined || acc.length == 0) {
            __myConsLog("ERROR: acc is null");
            if (func == "transfer") {
                var body = {
                    code: retCode.ERROR,
                    msg: "tx error"
                };

                res.send(body)
            } else {
                res.send("tx error");
            }
            return
        } 
        var reacc = req.query.reacc;
        var amt = req.query.amt;

        var invokeRequest = {
            
            chaincodeID: ccId,
            fcn: func,
            args: [acc, amt, reacc]
        };   
        
        chain.setInvokeWaitTime(30) 
        // invoke
        var tx = user.invoke(invokeRequest);

        tx.on('complete', function (results) {
            
            var retInfo = results.result.toString()  // like: "Tx 2eecbc7b-eb1b-40c0-818d-4340863862fe complete"
            
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
            
            //不打印的内容置为*
            invokeRequest.chaincodeID = "*"
            __myConsLog("Invoke success: request=%j, results=%j",invokeRequest, retInfo);
        });
        tx.on('error', function (error) {
            if (func == "transfer") {
                var body = {
                    code: retCode.ERROR,
                    msg: "tx error"
                };

                res.send(body)
            } else {            
                res.send("tx error"); 
            }
            
            //不打印的内容置为*
            invokeRequest.chaincodeID = "*"
            __myConsLog("Invoke failed : request=%j, error=%j",invokeRequest, error);
        });

    });   
});

app.get('/app/query', function(req, res) { 

    res.set({'Content-Type':'text/json','Encodeing':'utf8'});  

    /*
    var enrollUser = req.query.acc;  //use account
    
    var enrollPasswd, err = getUserPasswd(acc);
    if (err) {
        res.send("error"); 
        return
    }
    */

    chain.enroll(adminUser, adminPasswd, function (err, user) {
        
        if (err) {
            __myConsLog("ERROR: failed to register user: %s",err);
            res.send("admin" + ' not regist or pw error')
        }
                
  
        var ccId = req.query.ccId;
        var func = req.query.func;

        var acc = req.query.acc;

        var queryRequest = {
            
            chaincodeID: ccId,
            fcn: func,
            args: [acc]
        };   
        
        // invoke
        var tx = user.query(queryRequest);

        tx.on('complete', function (results) {
            var rslt = results.result.toString()
            res.send(rslt)

            //不打印的内容置为*
            queryRequest.chaincodeID = "*"
            __myConsLog("Query success: request=%j, results=%s", queryRequest, rslt);
        });
        tx.on('error', function (error) {
            res.send("tx error"); 

            //不打印的内容置为*
            queryRequest.chaincodeID = "*"
            __myConsLog("Query failed : request=%j, error=%s", queryRequest, error.msg);
        });

    });   
});

app.get('/app/register', function(req, res) { 
    
    res.set({'Content-Type':'text/json','Encodeing':'utf8'});  

    /*
    chain.getMember("admin", function(err, member) {
        if (err) {
            __myConsLog("could not get member for admin", err);
            res.send("getMember error")
        }
        __myConsLog("I did find this member", member)
    });
    */

    chain.enroll(adminUser, adminPasswd, function (err, adminUser) {
        
        if (err) {
            __myConsLog("ERROR: failed to register user: %s",err);
            res.send("admin" + ' not regist or pw error')
        }

        var acc = req.query.acc;

        chain.setRegistrar(adminUser);
        
        var registrationRequest = {
            roles: [ 'client' ],
            enrollmentID: acc,
            affiliation: "bank_a",
            //attributes: attributes,
            registrar: adminUser
        };

        chain.register(registrationRequest, function(err, enrollmentPassword) {
            if (err) {
                __myConsLog("register: couldn't register name ", acc, err)
                res.send("register error") 
            }
            // Fetch name's member so we can set the Registrar
            setUserPasswd(acc, enrollmentPassword)
            
            //res.send(enrollmentPassword)
            var body = {
                code: retCode.OK,
                msg: "OK"
            };

            res.send(body)
            
       });


    });   
});


/**
 * cache for acc and passwd.
 */
var accPassCache={}


/**
 * init accPassCache
 */
function initAccPassCache() {
    
};


/**
 * Set the passwd.
 * @returns error.
 */
function setUserPasswd(name, passwd) {
    accPassCache[name] = passwd
};


/**
 * Get the passwd.
 * @returns passwd, error.
 */
function getUserPasswd(name) {
    passwd = accPassCache[name];
    if (passwd)
        return passwd
    else
        return null, "get user passwd failed, not regisger?"
};



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

   var header = util.format("%s [app]: ", __getNowTime())
   arguments["0"] = header +  arguments["0"]
   console.log.apply(this, arguments)
};


 
app.listen(8088, "127.0.0.1");

__myConsLog("listen on 8088...");

