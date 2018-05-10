/**
 * Copyright 2017 IBM All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the 'License');
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an 'AS IS' BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */
'use strict';

const express = require('express');
//const session = require('express-session');
const cookieParser = require('cookie-parser');
const bodyParser = require('body-parser');
const fs = require('fs');
const util = require('util');
const expressJWT = require('express-jwt');
const jwt = require('jsonwebtoken');
const bearerToken = require('express-bearer-token');
const cors = require('cors');
const crypto = require('crypto')
const secp256k1 = require('secp256k1/elliptic');
const mpath = require('path');


//socketio功能需要使用如下的http和sockio
const app = express();
const http = require('http').Server(app);
//这里的path参数，client端connnect时也必须用这个参数，会拼在url请求中，io.connect("https://xxx/yyy", { path:'/loulansocketio' })。nginx中可以用它来匹配url。
const sockio = require('socket.io')(http, {path: '/loulansocketio'});  


//NOTE: run config.js  before require hfc_wrap.js
require('./config.js');
const hfc_wrap = require('./lib/hfc_wrap/hfc_wrap.js');


const user = require('./lib/user');
const hash = require('./lib/hash');
const common = require('./lib/common');
const subscribe = require('./lib/subscribe');

const host = hfc_wrap.getConfigSetting('host');
const port = hfc_wrap.getConfigSetting('port');

var logger = common.createLog("loulan")
logger.setLogLevel(logger.logLevel.INFO)

const attrKeys = {
    USRROLE: "usrrole",
    USRNAME: "usrname",
    USRTYPE: "usrtype"
}
const certAttrKeys = [attrKeys.USRROLE, attrKeys.USRNAME, attrKeys.USRTYPE]

const accountSysCC = "accountsys"

const crossCCCallFlag = "^_^~"

const retCode = {
    OK:                     0,
    ACCOUNT_NOT_EXISTS:     101,
    ENROLL_ERR:             102,
    GETUSER_ERR:            103,
    GETUSERCERT_ERR:        104,
    USER_EXISTS:            105,
    GETACCBYMVID_ERR:       106,
    GET_CERT_ERR:           107,
    PUT_CERT_ERR:           108,
    INVLID_PARA:            109,
    
    ERROR:                  0xffffffff
}


///////////////////////////////////////////////////////////////////////////////
//////////////////////////////// SET CONFIGURATONS ////////////////////////////
///////////////////////////////////////////////////////////////////////////////
app.options('*', cors());
app.use(cors());
//support parsing of application/json type post data
app.use(bodyParser.json());
//support parsing of application/x-www-form-urlencoded post data
app.use(bodyParser.urlencoded({
	extended: false
}));

process.on('exit', function (){
  logger.info(" ****  loulan exit ****");
  //fs.closeSync(wFd);
  user.onExit();
});




/*
// set secret variable
app.set('secret', 'thisismysecret');
app.use(expressJWT({
	secret: 'thisismysecret'
}).unless({
	path: ['/users']
}));
app.use(bearerToken());
app.use(function(req, res, next) {
	if (req.originalUrl.indexOf('/users') >= 0) {
		return next();
	}

	var token = req.token;
	jwt.verify(token, app.get('secret'), function(err, decoded) {
		if (err) {
			res.send({
				success: false,
				message: 'Failed to authenticate token. Make sure to include the ' +
					'token returned from /users call in the authorization header ' +
					' as a Bearer token'
			});
			return;
		} else {
			// add the decoded user name and org name to the request object
			// for the downstream code to use
			req.username = decoded.username;
			req.orgname = decoded.orgname;
			logger.debug(util.format('Decoded from JWT token: username - %s, orgname - %s', decoded.username, decoded.orgname));
			return next();
		}
	});
});
*/

const routeHandles = {
    All                     : 'all',                    //所有的路由处理函数都添加某些参数
    ChannelCreate           : 'channelCreate',
    ChannelJoin             : 'channelJoin',
    ChannelUpdate           : 'channelUpdate',
    ChaincodeInstall        : 'chaincodeInstall',
    ChaincodeInstantiate    : 'chaincodeInstantiate',
    ChaincodeUpgrade        : 'chaincodeUpgrade',
    Register                : 'register',
    Invoke                  : 'invoke',
    Query                   : 'query',
}
exports.routeHandles = routeHandles

//注入参数的变量
var externReqParams_All
var externReqParams_Register
var externReqParams_Invoke
var externReqParams_Query
var externReqParams_ChannelCreate
var externReqParams_ChannelJoin
var externReqParams_ChannelUpdate
var externReqParams_ChaincodeInstall
var externReqParams_ChaincodeInstantiate
var externReqParams_ChaincodeUpgrade


//注册参数解析的handle
var paramsFmtConvHandle_Invoke
var paramsFmtConvHandle_Query

var resultFormatHandle_Invoke
var resultFormatHandle_Query

var module_name

const templateModuleName = '_loulan__'

var walletPath = '/usr/local/llwork/wallet'

//注册路由处理函数。数组的第一个参数为get/post对应的处理函数， 第二个参数为处理路由中参数的函数
const defaultRouteTable = {
    '/_loulan__/register'              : {'GET' : handle_register,                'POST' : handle_register},
    '/_loulan__/invoke'                : {'GET' : handle_invoke,                  'POST' : handle_invoke},
    '/_loulan__/query'                 : {'GET' : handle_query,                   'POST' : handle_query},
 
    '/_loulan__/subscribe'             : {'GET' : subscribe.subscribeState,       'POST' : subscribe.subscribeState},

    
    '/_loulan__/setenv'                : {'GET' : handle_setenv,                  'POST' : handle_setenv},
    '/_loulan__/test/'                 : {'GET' : handle_test,                    'POST' : handle_test},
    
    '/_loulan__/channel/create'             : {'GET' : handle_channel_create,               'POST' : handle_channel_create},
    '/_loulan__/channel/join'               : {'GET' : handle_channel_join,                 'POST' : handle_channel_join},
  //'/_loulan__/channel/update'             : {'GET' : handle_channel_update,               'POST' : handle_channel_update},
    '/_loulan__/chaincode/install'          : {'GET' : handle_chaincode_install,            'POST' : handle_chaincode_install},
    '/_loulan__/chaincode/deploy'           : {'GET' : handle_chaincode_instantiate,        'POST' : handle_chaincode_instantiate},
    '/_loulan__/chaincode/upgrade'          : {'GET' : handle_chaincode_upgrade,            'POST' : handle_chaincode_upgrade},
    '/_loulan__/chaincode/installAndDeploy' : {'GET' : handle_chaincode_instll_instantiate, 'POST' : handle_chaincode_instll_instantiate},
    '/_loulan__/chaincode/installAndUpgrade': {'GET' : handle_chaincode_instll_upgrade,     'POST' : handle_chaincode_instll_upgrade},

    //查询channel上的信息
    '/_loulan__/chains/blocks/:blockId'       : {'GET' : handle_queryBlockById,       'POST' : handle_queryBlockById},
    '/_loulan__/chains/blocks'                : {'GET' : handle_queryBlockByHash,     'POST' : handle_queryBlockByHash},
    '/_loulan__/chains/transactions/:trxnId'  : {'GET' : handle_queryTransactions,    'POST' : handle_queryTransactions},
    '/_loulan__/chains/channels/'             : {'GET' : handle_queryChannels,        'POST' : handle_queryChannels},        //查询所有的channel信息
    '/_loulan__/chains/chaincodes/:type'      : {'GET' : handle_queryChaincodes,      'POST' : handle_queryChaincodes},      //查询chaincodes的相关信息 type = installed/instantiated
    '/_loulan__/chains/chain'                 : {'GET' : handle_queryChains,          'POST' : handle_queryChains},
    '/_loulan__/chains/getsometransonce'      : {'GET' : handle_queryTransInBlockOnce,'POST' : handle_queryTransInBlockOnce},
}

function getCrossCallCcname(chaincodeName) {
    return crossCCCallFlag + chaincodeName
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
    } else {
        body.code=retCode.ENROLL_ERR;
        body.msg="unknown env key."
        logger.error("unknown env key=%s",key);
    }
    
    res.send(body)
    return
}


///////////////////////////////////////////////////////////////////////////////
///////////////////////// REST ENDPOINTS START HERE ///////////////////////////
///////////////////////////////////////////////////////////////////////////////
// Register and enroll user
function handle_register(params, res, req, serialno){
    logger.debug('enter handle_register')
    
    for (var p in externReqParams_Register) {
        if (params[p] == undefined)
            params[p] = externReqParams_Register[p]
    }
    
    return __execRegister(params, req, true, serialno)
    .then((response)=>{
            res.send(response)
        }, 
        (err)=> {
            res.send(err)
        })
}

function __execRegister(params, req, outputRResult, serialno) {
    var body = {
        code : retCode.OK,
        msg: "OK",
    };

    if (outputRResult == true) 
        logger.info("Enter Register.%d", serialno)

	logger.debug('params= \n', params);

	var username = params.usr;
	var orgname = params.org;
    var funcName = params.fcn || params.func; //兼容老接口,老接口为func
    var args = params.args

    if (args) {
        if (typeof(args) == 'string') {
            var delim = params.argsdelim ? params.argsdelim : ","
            args = args.split(delim)
        } else {
            if (!(args instanceof Array)) {
                return Promise.reject(paraInvalidMessage('\'args\''));
            }
        } 
    }

	if (!username) {
        if (!args || !args[0]) {
            return Promise.reject(paraInvalidMessage('\'usr\''))
        } else {
            username = args[0] //args的第一个参数为用户名
        }
	}

	if (!orgname) {
        orgname = 'org1'
	}

    /*
	var token = jwt.sign({
		exp: Math.floor(Date.now() / 1000) + parseInt(hfc_wrap.getConfigSetting('jwt_expiretime')),
		username: username,
		orgname: orgname
	}, app.get('secret'));
    */
    var attributes = [  {name: attrKeys.USRROLE, value: 'role'}, 
                        {name: attrKeys.USRNAME, value: username}, 
                        {name: attrKeys.USRTYPE, value: 'user'}]
                        
                        
    var passwd = params.passwd
    
    if (funcName == 'setPasswd' || funcName == 'changePasswd' || funcName == "accountPasswd") {
        if (!passwd) {
            return Promise.reject(paraInvalidMessage('\'passwd\''));
        }
    }

    var promise1
    if (params.useOLW && funcName == 'setPasswd') {
        promise1 = new Promise((resolve, reject)=>{
            hash.encryteWallet(username, passwd, walletPath, (err)=>{
                if (err) {
                    logger.errorf('encryteWallet failed, err=%s ', err)
                    return reject(err);
                }
                return resolve(null)
            })
        })
    } else if (params.useOLW && funcName == 'changePasswd') {
        var oldpasswd = params.oldpasswd
        if (!oldpasswd) {
            return Promise.reject(paraInvalidMessage('\'oldpasswd\''));
        }
        
        promise1 = new Promise((resolve, reject)=>{
            hash.changeWalletPassphrase(username, oldpasswd, passwd, walletPath, (err)=>{
                if (err) {
                    logger.error('changeWalletPassphrase failed, err=%s ', err)
                    return reject(err);
                }
                return resolve(null)
            })
        })
    } else {
        promise1 = hfc_wrap.registerAndEnroll(username, orgname, true, attributes)
        .then((response)=> {
                //OK
                logger.debug('registerAndEnroll responsed ok, response=', response)
                
                if (funcName == "account" || funcName == "accountCB" || funcName == "accountPasswd") {
                    
                    var promise2
                    if (params.useOLW) { //useOLW 使用线上钱包
                        promise2 = new Promise((resolve, reject)=>{
                            hash.creatWallet(username, walletPath, (err, pubKeyHash)=>{
                                if (err) {
                                    logger.errorf('creatWallet failed, err=%s ', err)
                                    return reject(err);
                                }
                                
                                if (funcName == "accountPasswd") {
                                    params.fcn = 'account' //还是account函数，只是在这里加密一下
                                    hash.encryteWallet(username, passwd, walletPath, (err)=>{
                                        if (err) {
                                            logger.errorf('encryteWallet failed, err=%s ', err)
                                            return reject(err);
                                        }
                                        return resolve(pubKeyHash)
                                    })
                                } else {
                                    return resolve(pubKeyHash)
                                }
                            })
                        })
                    } else {
                        promise2 = Promise.resolve()
                    }
                    
                    return promise2.then((pubKeyHash)=>{

                        if (pubKeyHash) {
                            params.addPubKeyHash = pubKeyHash   //参数中添加pubKeyHash， invoke时会处理
                        }
                        
                        var indentity = hash.Hash160(response.cert).toString('base64')
                        logger.debug('indentity=  : ', indentity);
                        params.addusrindentity = indentity  //参数中添加indentity， invoke时会处理
                        return __execInvoke(params, req, outputRResult, serialno)
                        .then((response)=>{
                               return response
                            }, 
                            (err)=> {
                                return Promise.reject(err.msg) //__execInvoke出错返回的是body，这里只取其msg，会在下面的catch捕获到
                        })
                    
                    })
                } else {
                    return null
                }
            })
    }

	return promise1.then((invokeResp)=> {
            //OK
            if (funcName)
                body.msg = util.format('Register(%s) OK.', funcName)
            else 
                body.msg = util.format('Register OK.')
            if (invokeResp) {
                body.result = invokeResp.result
            }
            
            if (outputRResult == true) {
                logger.info("Register.%d success: user=%s", serialno, username);
            }
            
            return (body)
        })
    .catch((err)=>{
            logger.debug('register responsed error, err=%s', err)
            body.code = retCode.ERROR
            body.msg = util.format('Register: %s err, %s.', funcName ? funcName : '', err)
            
            if (outputRResult == true) {
                logger.info("Register.%d failed: user=%s, err=%j", serialno, username, err);
            }
            
            return Promise.reject(body)
        });
}




// Invoke transaction on chaincode on target peers
function handle_invoke(params, res, req, serialno) {

    for (var p in externReqParams_Invoke) {
        if (params[p] == undefined)
            params[p] = externReqParams_Invoke[p]
    }

	logger.debug('==================== INVOKE ON CHAINCODE ==================');
    return __execInvoke(params, req, true, serialno)
    .then((response)=>{
            res.send(response)
        }, 
        (err)=> {
            res.send(err)
        })
}

function __execInvoke(params, req, outputQReslt, serialno) {
    var body = {
        code : retCode.OK,
        msg: "OK",
    };

    if (outputQReslt == true)
        logger.info("Enter Invoke.%d", serialno)

    if (paramsFmtConvHandle_Invoke) {
        var rbody = paramsFmtConvHandle_Invoke(params)
        if (rbody)
            return Promise.reject(rbody);
    }

	logger.debug('params= \n', params);

	var peers = params.peers;
	var chaincodeName = params.ccname;
	var channelName = params.channame;
	var fcn = params.fcn || params.func; //兼容老接口,老接口为func
	var args = params.args;

	if (!chaincodeName) {
		return Promise.reject(paraInvalidMessage("'ccname'"));
	}
	if (!channelName) {
		return Promise.reject(paraInvalidMessage("'channame'"));
	}
	if (!fcn) {
		return Promise.reject(paraInvalidMessage('\'fcn\''));
	}

    if (args) {
        if (typeof(args) == 'string') {
            var delim = params.argsdelim ? params.argsdelim : ","
            args = args.split(delim)
        } else {
            if (!(args instanceof Array)) {
                return Promise.reject(paraInvalidMessage('\'args\''));
            }
        } 
    }

    var username = params.usr;
	if (!username) {
        if (!args || !args[0]) {
            return Promise.reject(paraInvalidMessage('\'usr\''))
        } else {
            username = args[0] //args的第一个参数为用户名
        }
	}
    var orgname = params.org;
	if (!orgname) {
        orgname = 'org1'
	}
	logger.debug('username  : ' + username);
	logger.debug('orgname  : ' + orgname);
    

    var inputArgs = []
    
    //使用线上钱包时才能自动添加参数，因为签名是在本程序中做； 否则不能自动添加，因为签名是在客户端做的，会对参数做签名
    if (params.useOLW && params.autoadd) {
        inputArgs.push(username) 
        if (params.accequsr) { //账户和用户名相同
            inputArgs.push(username)
        } else {
            var acc = params.acc
            if (!acc) {
                return Promise.reject(paraInvalidMessage('\'acc\''));
            }
            inputArgs.push(acc)
        }
    } 

    
    if (args) {
        inputArgs = inputArgs.concat(args)
    }

    //这个参数只有在开户时才会设置
    if (params.useOLW && params.addPubKeyHash) {
        inputArgs.push(params.addPubKeyHash)
    }
    
    /* 可以没有参数
    if (inputArgs.length == 0) {
        return Promise.reject(paraInvalidMessage('\'args\''));
    }
    */
    logger.debug('args  : ', inputArgs);

    if (typeof(peers) == 'string') {
        peers = peers.split(',')
    } else {
        if (!(peers instanceof Array)) {
            return Promise.reject(paraInvalidMessage('\'peers\''));
        }
    }

    //
    var passwd = params.passwd
    if (!passwd) {
        passwd = ''
    }
    
    var promise0
    if (params.useOLW) { //useOLW 使用线上钱包
        //追加数字签名
        promise0 = new Promise((resolve, reject)=>{
            hash.decryptWallet(username, passwd, walletPath, (err, priKey)=>{
                if (err) {
                    return reject(err);
                }

                var allArgs = fcn + ',' + inputArgs.join(',')
                var msg = crypto.createHash('sha256').update(allArgs).digest()
                var sign = hash.makeSignatureBuffer(priKey, msg).toString('base64')

                logger.debug('invoke allArgs=', allArgs)
                logger.debug('invoke msg=', msg)
                logger.debug('invoke sign=', sign)
                return resolve(sign)
            })
        })
    } else {
        if (!params.signature) {
            return Promise.reject(paraInvalidMessage('\'signature\''));
        }
        
        promise0 = Promise.resolve(params.signature)
    }
    
    
    
    var invokeRequest = {}
    invokeRequest.fcn = fcn
    invokeRequest.args = inputArgs
    invokeRequest.org = orgname
    invokeRequest.peers = peers
    invokeRequest.user = username
    invokeRequest.channame = channelName
    invokeRequest.ccname = chaincodeName
    
    return promise0.then((sign)=>{
        inputArgs.push(sign)

        var promise1
        
        //这个参数只有在开户时才会设置。用户身份是平台自动添加的，所以要放在签名之后，因为签名可能是在客户端做的
        if (params.addusrindentity) {
            inputArgs.push(params.addusrindentity)
            promise1 = Promise.resolve()
        } else {
            //如果是开户操作，加上 usrindentity   在楼兰平台上的测试链中，可能会直接执行 account
            if (fcn == "account") {
                promise1 = new Promise((resolve, reject)=>{
                    return hfc_wrap.getUserCert(username, orgname, true)
                    .then((cert)=>{
                        var indentity = hash.Hash160(cert).toString('base64')
                        logger.debug('indentity=  : ', indentity);
                        inputArgs.push(indentity)
                        return resolve()
                    })
                    .catch((err)=>{
                        return reject(err)
                    })
                })
            } else {
                promise1 = Promise.resolve()
            }
        }
        
        return promise1.then(()=>{
            if (params.useaccsys &&  accountSysCC  != chaincodeName) {
                inputArgs.push(getCrossCallCcname(chaincodeName))
                chaincodeName = accountSysCC
            }
            
            logger.debug('args  : ', inputArgs);


            return hfc_wrap.invokeChaincode(peers, channelName, chaincodeName, fcn, inputArgs, username, orgname)
            .then((response)=>{
                    logger.debug('invoke success, response=', response)
                    body.msg = util.format("Invoke(%s) OK.", fcn)
                    body.result = response.result

                    if (params.exportInvokeIntf) {
                        exportInvokeIntf(username, req, body);
                        body.exportIntfUrl = getExportIntfUrl(username);
                    }
                    
                    if (outputQReslt == true) {
                        //去掉无用的信息,不打印
                        if (fcn == "account" || fcn == "accountCB") {
                            
                        } else {
                            
                        }                        
                            
                        logger.info("Invoke.%d success: request=%j, results=%j", serialno, invokeRequest, response.result);
                    }

                    if (resultFormatHandle_Invoke) {
                        resultFormatHandle_Invoke(params, req, body)
                    }

                    return body;
            })
        })
    })
    .catch((err)=>{
            logger.debug('invoke failed, err=%s', err)
            body.code = retCode.ERROR
            body.msg = '' + err
            
            if (outputQReslt == true) {
                //去掉无用的信息,不打印
                if (fcn == "account" || fcn == "accountCB") {
                    
                } else {
                    
                }                        
                logger.error("Invoke.%d failed : request=%j, error=%j", serialno, invokeRequest, err);
            }
            
            
            return Promise.reject(body);
        });
    
}

// Query on chaincode on target peers
function handle_query(params, res, req, serialno) {
	logger.debug('==================== QUERY BY CHAINCODE ==================');
    var body = {
        code : retCode.OK,
        msg: "OK",
    };

    var outputQReslt = true
    if (outputQReslt == true) {    
        logger.info("Enter Query.%d", serialno)
    }

    for (var p in externReqParams_Query) {
        if (params[p] == undefined)
            params[p] = externReqParams_Query[p]
    }
    
    
    if (paramsFmtConvHandle_Query) {
        var rbody = paramsFmtConvHandle_Query(params)
        if (rbody)
            return res.send(rbody);
    }
        
	logger.debug('params= \n', params);

	var channelName = params.channame;
	var chaincodeName = params.ccname;
	let args = params.args;
	let fcn = params.fcn || params.func; //兼容老接口,老接口为func
	let peer = params.peer;


	if (!chaincodeName) {
		res.send(paraInvalidMessage("'ccname'"));
		return;
	}
	if (!channelName) {
		res.send(paraInvalidMessage("'channame'"));
		return;
	}
	if (!fcn) {
		res.send(paraInvalidMessage('\'fcn\''));
		return;
	}

    if (args) {
        if (typeof(args) == 'string') {
            var delim = params.argsdelim ? params.argsdelim : ","
            args = args.split(delim)
        } else {
            if (!(args instanceof Array)) {
                return res.send(paraInvalidMessage('\'args\''));
            }
        } 
    }

    var username = params.usr;
	if (!username) {
        if (!args || !args[0]) {
            return res.send(paraInvalidMessage('\'usr\''))
        } else {
            username = args[0] //args的第一个参数为用户名
        }
	}
    
    var orgname = params.org;
	if (!orgname) {
        orgname = 'org1'
	}
	logger.debug('username  : ' + username);
	logger.debug('orgname  : ' + orgname);


    var inputArgs = []
    //使用线上钱包时才能自动添加参数，因为签名是在本程序中做； 否则不能自动添加，因为签名是在客户端做的，会对参数做签名
    if (params.useOLW && params.autoadd) {
        inputArgs.push(username) 
        if (params.accequsr) { //账户和用户名相同
            inputArgs.push(username)
        } else {
            var acc = params.acc
            if (!acc) {
                return res.send(paraInvalidMessage('\'acc\''));
            }
            inputArgs.push(acc)
        }
    }
    if (args) {
        inputArgs = inputArgs.concat(args)
    }

    /* 可以没有参数
	if (inputArgs.length == 0) {
		res.send(paraInvalidMessage('\'args\''));
		return;
	}
    */
	logger.debug('args  : ', inputArgs);

    var passwd = params.passwd
    if (!passwd) {
        passwd = ''
    }
   
    var promise0
    if (params.useOLW) { //useOLW 使用线上钱包
         //追加数字签名
         promise0 = new Promise((resolve, reject)=>{
            hash.decryptWallet(username, passwd, walletPath, (err, priKey)=>{
                if (err) {
                    return reject(err);
                }
                
                var allArgs = fcn + ',' + inputArgs.join(',')
                var msg = crypto.createHash('sha256').update(allArgs).digest()
                var sign = hash.makeSignatureBuffer(priKey, msg).toString('base64')
                
                logger.debug('invoke allArgs=', allArgs)
                logger.debug('invoke msg=', msg)
                logger.debug('invoke sign=', sign)
                return resolve(sign)
            })
        })
    } else {
        if (!params.signature) {
            return res.send(paraInvalidMessage('\'signature\''));
        }
        
        promise0 = Promise.resolve(params.signature)
    }

            
    var queryRequest = {}
    queryRequest.fcn = fcn
    queryRequest.args = inputArgs
    queryRequest.org = orgname
    queryRequest.peer = peer
    queryRequest.user = username
    queryRequest.channame = channelName
    queryRequest.ccname = chaincodeName

    return promise0.then((sign)=>{
        inputArgs.push(sign)
    
        if (params.useaccsys &&  accountSysCC  != chaincodeName) {
            inputArgs.push(getCrossCallCcname(chaincodeName))
            chaincodeName = accountSysCC
        }

        logger.debug('args  : ', inputArgs);
    

        return hfc_wrap.queryChaincode(peer, channelName, chaincodeName, inputArgs, fcn, username, orgname)
        .then((response)=>{
                logger.debug('query success, response=', response)
                body.msg = util.format("Query(%s) OK.", fcn)
                body.result = response.result
                
                if (params.exportInvokeIntf) {
                    exportInvokeIntf(username, req, body);
                    body.exportIntfUrl = getExportIntfUrl(username);
                }
                
                var promise1
                if (fcn == "getInfoForWeb") {
                    promise1 = new Promise((resolve, reject)=>{
                        return __queryTransInBlockOnce(params, req, true)
                        .then((response)=>{
                                return resolve(response.result)
                            }, 
                            (err)=> {
                                return reject(err.msg) //只取错误信息
                            })
                    })
                    
                } else {
                    promise1 = Promise.resolve()
                }
                
                return promise1.then((txInfos)=>{
                    if (fcn == "getInfoForWeb") {
                        var queryObj = JSON.parse(response.result)
                        txInfos.accountCnt = queryObj.accountcount
                        txInfos.issuedAmt = queryObj.issueamt
                        txInfos.totalAmt = queryObj.issuetotalamt
                        txInfos.circulateAmt = queryObj.circulateamt
                        txInfos.nodesCnt = 4
                        
                        body.result = txInfos
                    }
                    
                    logger.debug("resultFormatHandle_Query =",resultFormatHandle_Query)
                    if (resultFormatHandle_Query) {
                        resultFormatHandle_Query(params, req, body)
                    }
                    
                    res.send(body);
                   

                    if (outputQReslt == true) {
                        var resultStr = response.result
                        //去掉无用的信息,不打印
                        var maxPrtLen = 256
                        if (resultStr.length > maxPrtLen)
                            resultStr = resultStr.substr(0, maxPrtLen) + "......"
                        logger.info("Query.%d success: request=%j, results=%s", serialno, queryRequest, resultStr);
                    }

                })
            })
    })
    .catch((err)=>{
            //logger.error('query failed, err=%s', err)
            body.code = retCode.ERROR
            body.msg = '' + err
            res.send(body);
            
            if (outputQReslt == true) {
                //去掉无用的信息,不打印
                logger.error("Query.%d failed : request=%j, error=%j", serialno, queryRequest, err);
            }
        });
}


function handle_queryBlockById(params, res, req) {
	logger.debug('==================== GET BLOCK BY NUMBER ==================');
    var body = {
        code : retCode.OK,
        msg : "OK",
    };

	logger.debug('params= \n', params);

    var channelName = params.channame;
    if (!channelName) {
        return res.json(paraInvalidMessage("'channame'"));
    }

	let blockId = params.blockId;
	let peer = params.peer;
    if (!peer) {
        peer = 'peer0'
    }

	if (!blockId) {
		res.json(paraInvalidMessage('\'blockId\''));
		return;
	}

    var username = params.usr;
	if (!username) {
		username = 'admin'
	}

    var orgname = params.org;
	if (!orgname) {
        orgname = 'org1'
	}

	return hfc_wrap.getBlockByNumber(peer, blockId, username, orgname, channelName)
	.then((response)=>{
            logger.debug('getBlockById success, response=', response)
            body.msg = 'ok'
            body.result = response
            res.send(body);
        })
    .catch((err)=>{
            logger.error('getBlockById failed, err=%s', err)
            body.code = retCode.ERROR
            body.msg = '' + err
            res.send(body);
        });
}

// Query Get Block by Hash
function handle_queryBlockByHash(params, res, req) {
	logger.debug('================ GET BLOCK BY HASH ======================');
    var body = {
        code : retCode.OK,
        msg : "OK",
    };

	logger.debug('params= \n', params);

    var channelName = params.channame;
    if (!channelName) {
        return res.json(paraInvalidMessage("'channame'"));
    }

	let hash = params.hash;
	let peer = params.peer;
    if (!peer) {
        peer = 'peer0'
    }

	if (!hash) {
		res.json(paraInvalidMessage('\'hash\''));
		return;
	}

    var username = params.usr;
	if (!username) {
		username = 'admin'
	}

    var orgname = params.org;
	if (!orgname) {
        orgname = 'org1'
	}


	return hfc_wrap.getBlockByHash(peer, hash, username, orgname, channelName)
	.then((response)=>{
            logger.debug('getBlockByHash success, response=', response)
            body.msg = 'ok'
            body.result = response
            res.send(body);
        })
    .catch((err)=>{
            logger.error('getBlockByHash failed, err=%s', err)
            body.code = retCode.ERROR
            body.msg = '' + err
            res.send(body);
        });
}

// Query Get Transaction by Transaction ID
function handle_queryTransactions(params, res, req) {
	logger.debug('================ GET TRANSACTION BY TRANSACTION_ID ======================');
    var body = {
        code : retCode.OK,
        msg : "OK",
    };

	logger.debug('params= \n', params);

    var channelName = params.channame;
    if (!channelName) {
        return res.json(paraInvalidMessage("'channame'"));
    }

	let trxnId = params.trxnId;
	let peer = params.peer;
    if (!peer) {
        peer = 'peer0'
    }
    
	if (!trxnId) {
		res.json(paraInvalidMessage('\'trxnId\''));
		return;
	}

    var username = params.usr;
	if (!username) {
		username = 'admin'
	}

    var orgname = params.org;
	if (!orgname) {
        orgname = 'org1'
	}

	return hfc_wrap.getTransactionByID(peer, trxnId, username, orgname, channelName)
	.then((response)=>{
            logger.debug('queryTransactions success, response=', response)
            body.msg = 'ok'
            body.result = response
            res.send(body);
        })
    .catch((err)=>{
            logger.error('queryTransactions failed, err=%s', err)
            body.code = retCode.ERROR
            body.msg = '' + err
            res.send(body);
        });
}

//Query for Chains Information
function handle_queryChains(params, res, req) {
	logger.debug('================ GET CHANNEL INFORMATION ======================');

    var body = {
        code : retCode.OK,
        msg : "OK",
    };
    
	logger.debug('params= \n', params);

    var channelName = params.channame;
    if (!channelName) {
        return res.json(paraInvalidMessage("'channame'"));
    }

	let peer = params.peer;
    if (!peer) {
        peer = 'peer0'
    }

    var username = params.usr;
	if (!username) {
		username = 'admin'
	}

    var orgname = params.org;
	if (!orgname) {
        orgname = 'org1'
	}

	return hfc_wrap.getChainInfo(peer, username, orgname, channelName)
	.then((response)=>{
            logger.debug('queryChains success, response=', response)
            body.msg = 'ok'
            body.result = response
            res.send(body);
        })
    .catch((err)=>{
            logger.error('queryChains failed, err=%s', err)
            body.code = retCode.ERROR
            body.msg = '' + err
            res.send(body);
        });
}

// Query to fetch all Installed/instantiated chaincodes
function handle_queryChaincodes(params, res, req) {

    var body = {
        code : retCode.OK,
        msg : "OK",
    };
    
	logger.debug('params= \n', params);

	var peer = params.peer;
    if (!peer) {
        peer = 'peer0'
    }

	var installType = params.type;  //type = installed/instantiated
    if (installType != 'installed' && installType != 'instantiated') {
		return res.json(paraInvalidMessage('\'type\''));
    }
    
    var channelName = params.channame;

    
	if (installType === 'installed') {
		logger.debug(
			'================ GET INSTALLED CHAINCODES ======================');
	} else {
        
        if (!channelName) {
            return res.json(paraInvalidMessage("'channame'"));
        }
        
		logger.debug(
			'================ GET INSTANTIATED CHAINCODES ======================');
	}

    var username = params.usr;
	if (!username) {
		username = 'admin'
	}

    var orgname = params.org;
	if (!orgname) {
        orgname = 'org1'
	}

	return hfc_wrap.getChaincodes(peer, installType, username, orgname, channelName)
	.then((response)=>{
            logger.debug('queryChaincodes(%s) success, response=', installType, response)
            body.msg = 'ok'
            body.result = response
            res.send(body);
        })
    .catch((err)=>{
            logger.error('queryChaincodes(%s) failed, err=%s', installType, err)
            body.code = retCode.ERROR
            body.msg = '' + err
            res.send(body);
        });
}

// Query to fetch channels
function handle_queryChannels(params, res, req) {
	logger.debug('================ GET CHANNELS ======================');

    var body = {
        code : retCode.OK,
        msg : "OK",
    };

	logger.debug('params= \n', params);

	var peer = params.peer;

	if (!peer) {
		peer = 'peer0'
	}

    var username = params.usr;
	if (!username) {
		username = 'admin'
	}

    var orgname = params.org;
	if (!orgname) {
        orgname = 'org1'
	}

	return hfc_wrap.getChannels(peer, username, orgname)
	.then((response)=>{
            logger.debug('queryChannels success, response=', response)
            body.msg = 'ok'
            body.result = response
            res.send(body);
        })
    .catch((err)=>{
            logger.error('queryChannels failed, err=%s', err)
            body.code = retCode.ERROR
            body.msg = '' + err
            res.send(body);
        });
}


//  Query Get Trans in blocks
function handle_queryTransInBlockOnce(params, res, req) {
	logger.debug('==================== GET BLOCK BY NUMBER ==================');

	return __queryTransInBlockOnce(params, req)
    .then((response)=>{
            res.send(response)
        }, 
        (err)=> {
            res.send(err)
        })
}

function __queryTransInBlockOnce(params, req, gotChainHight) {
	logger.debug('==================== GET BLOCK BY NUMBER ==================');
    var body = {
        code : retCode.OK,
        msg : "OK",
    };

	logger.debug('params= \n', params);
    
    var channelName = params.channame;
    if (!channelName) {
        return res.json(paraInvalidMessage("'channame'"));
    }

	let peer = params.peer;
    if (!peer) {
        peer = 'peer0'
    }

    var username = params.usr;
	if (!username) {
		username = 'admin'
	}

    var orgname = params.org;
	if (!orgname) {
        orgname = 'org1'
	}
    
    var queryTxCount = params.qtc;
	if (!queryTxCount) {
		queryTxCount = 5
	}

    var order = params.ord;
    if (!order) 
        order = "desc" //不输入默认为降序，即从最新的数据查起
    
    var isDesc = false
    if (order == "desc")
        isDesc = true
    
    //获取区块高度
    return hfc_wrap.getChainInfo(peer, username, orgname)
	.then((response)=>{
            logger.debug('queryTransInBlock: queryChains success, response=', response)

            var chainHight = hfc_wrap.getChainHight(response)
            if (typeof(chainHight) != 'number') {
                return Promise.reject('get hight of chain failed.')
            }
            
            var latestBlockNum = chainHight - 1  //最新的block编号，从0开始到chainHight-1个
            
            
            var txRecords = []
            
            var startBlockNum = 0
            if (isDesc)
                startBlockNum = latestBlockNum

            return __getTxInfoInBlockOnce(latestBlockNum, startBlockNum, queryTxCount, isDesc, 1, txRecords, peer, username, orgname, channelName)
            .then(()=>{
                    logger.debug('txRecords=', txRecords)
                    if (gotChainHight == true){
                        var retObj = {}
                        retObj.latestBlock = latestBlockNum
                        retObj.txRecords = txRecords
                        body.result = retObj
                    } else {
                        body.result = txRecords
                    }
                    return (body)
                },
                (err)=>{
                    return Promise.reject('get tx info failed, err=%s', err)
                })
        })
    .catch((err)=>{
            logger.error('queryTransInBlock: getChainInfo failed, err=%s', err)
            body.code = retCode.ERROR
            body.msg = '' + err
            return Promise.reject(body)
    })


}

//txRecords作为入参传入，因为里面有递归调用，如果在本函数里用局部变量定义无法在递归中传递
function __getTxInfoInBlockOnce(latestBlockNum, startBlockNum, queryTxCnt, isDesc, accIdxInArgs, txRecords, peer, username, orgname, channelName) {

    var queryBlockCnt = queryTxCnt  //每个区块可能含有0个或多个交易信息，所以查询的区块数默认等于交易数

    var blockNumList = [] //待查询的block列表
    var endBlockNum

    if (isDesc == true) { //降序
        if (startBlockNum < 0) {
            return Promise.resolve()
        }
        
        endBlockNum = startBlockNum - queryBlockCnt + 1
        if (endBlockNum < 0 )
            endBlockNum = 0
        
        for (var i=startBlockNum; i>=endBlockNum; i--) {
            blockNumList.push(i)
        } 
        
    } else {
        if (startBlockNum > latestBlockNum) {
            return Promise.resolve()
        }
        
        endBlockNum = startBlockNum + queryBlockCnt - 1
        if (endBlockNum > latestBlockNum)
            endBlockNum = latestBlockNum
        
        for (var i=startBlockNum; i<=endBlockNum; i++) {
            blockNumList.push(i)
        }
    }

    var tmpRecds = {}
    var keyList = []
    
    var queryPromises = []

    logger.debug("__getTxInfoInBlockOnce: begin get blocks(%j)", blockNumList)

    //并行查询
    blockNumList.forEach((blockIdx)=>{
        let qPromise = hfc_wrap.getBlockByNumber(peer, blockIdx, username, orgname, channelName)
            .then((response)=>{
                    logger.debug('getBlockById success, response=', response)
                    return Promise.resolve(response)
                },
                (err)=>{
                    logger.error('getBlockById failed, blockIdx=%d err=%s', blockIdx, err)
                    return Promise.reject(err)
                })
            .catch((err)=>{
                logger.error('getBlockByNumber has some error. err=%s', err)
                return reject(err)
            });
        queryPromises.push(qPromise)
    })

    //结果处理
    return Promise.all(queryPromises)
    .then((results)=>{
            for (var i in results) {
                var oneBlock = results[i]
                var txInfo = hfc_wrap.getTxInfo(oneBlock, isDesc)
                if (typeof(txInfo) != 'object') {
                    return Promise.reject(logger.errorf('getTxInfo failed. err=%s', txInfo))
                }
                var txInfoList = txInfo
                
                for (var j in txInfoList) {
                    var txObj = txInfoList[j]
                    var args = hfc_wrap.getInvokeArgs(txObj.input.toString('hex'))
                    
                    logger.debug('block %d tx[%d], args=%j' , txObj.block, j, args)
                    //accIdxInArgs指明账户名是第args中的几个参数
                    if (args.length <= accIdxInArgs) {
                        continue
                    }

                    var accountName = args[accIdxInArgs]
                    //先过滤centerBank和kdcoinpool的交易
                    if (accountName.indexOf("centerBank") >= 0 || accountName.indexOf("kdcoinpool") >= 0) {
                        accountName = (new Buffer(common.md5sum(accountName), 'hex')).toString('base64')
                    }
                    
                    txObj.txInfo = txObj.input.toString('base64')
                    txObj.node = accountName
                    delete txObj.input

                    txRecords.push(txObj)

                    //最多记录 queryTxCnt 条
                    if (txRecords.length >= queryTxCnt)
                        break
                }

                //最多记录 queryTxCnt 条
                if (txRecords.length >= queryTxCnt)
                    break
            }

            //记录不够，再查一次
            if (txRecords.length < queryTxCnt) {
                //从上次查到的最小序列号开始
                var nextStart
                if (isDesc == true) { //降序
                    nextStart = endBlockNum - 1
                } else {
                    nextStart = endBlockNum + 1
                }

                return __getTxInfoInBlockOnce(latestBlockNum, nextStart, queryTxCnt, isDesc, accIdxInArgs, txRecords, peer, username, orgname, channelName)
            } else {
                return Promise.resolve()
            }
            
        },
        (err)=>{
            logger.error('get blokcs info has some err. err=%s', err)
            return Promise.reject(err)
        })
    .catch((err)=>{
        return Promise.reject(err)
    })
}


// Create Channel
function handle_channel_create(params, res, req) {

    for (var p in externReqParams_ChannelCreate) {
        if (params[p] == undefined)
            params[p] = externReqParams_ChannelCreate[p]
    }

    var body = {
        code : retCode.OK,
        msg: "OK",
    };

	logger.debug('params= \n', params);

	var channelName = params.channame;
	var channelConfigPath = params.channelConfigPath;
    var username = params.usr;
	if (!username) {
		return res.send(paraInvalidMessage('\'usr\''))
	}
    var orgname = params.org;
	if (!orgname) {
        orgname = 'org1' //随便找一个org
	}

	if (!channelName) {
		return res.send(paraInvalidMessage("'channame'"));
	}
	if (!channelConfigPath) {
		return res.send(paraInvalidMessage('\'channelConfigPath\''));
	}

	return hfc_wrap.createChannel(channelName, channelConfigPath, username, orgname)
	.then((response)=>{
            logger.debug('createChannel success, response=', response)
            body.msg = response.message
            res.send(body)
        })
    .catch((err)=>{
            logger.debug('createChannel failed, err=%s', err)
            body.code = retCode.ERROR
            body.msg = '' + err
            res.send(body);
        });
}


// Join Channel
function handle_channel_join(params, res, req) {
    var body = {
        code : retCode.OK,
        msg: "OK",
    };

    for (var p in externReqParams_ChannelJoin) {
        if (params[p] == undefined)
            params[p] = externReqParams_ChannelJoin[p]
    }

	logger.debug('params= \n', params);

	var channelName = params.channame;
	var peers = params.peers;
	if (!channelName) {
		res.send(paraInvalidMessage("'channame'"));
		return;
	}
	if (!peers) {
		res.send(paraInvalidMessage('\'peers\''));
		return;
	}
    var username = params.usr;
	if (!username) {
		return res.send(paraInvalidMessage('\'usr\''))
	}
    var orgname = params.org;
	if (!orgname) {
        //orgname = 'org1'   join时 org不能省
		return res.send(paraInvalidMessage('\'org\''))
	}

	return hfc_wrap.joinChannel(channelName, peers.split(','), username, orgname)
	.then((response)=>{
            logger.debug('joinChannel success, response=', response)
            body.msg = response.message
            res.send(body);
        })
    .catch((err)=>{
            logger.debug('joinChannel failed, err=%s', err)
            body.code = retCode.ERROR
            body.msg = '' + err
            res.send(body);
        });
}

function handle_chaincode_install(params, res, req) {
    for (var p in externReqParams_ChaincodeInstall) {
        if (params[p] == undefined)
            params[p] = externReqParams_ChaincodeInstall[p]
    }
    
    return __chaincode_install(params, req)
    .then((okResp)=>{
        res.send(okResp)
    })
    .catch((errResp)=>{
        res.send(errResp)
    })
}

// Install chaincode on target peers
function __chaincode_install(params, req, outputRResult) {
    var body = {
        code : retCode.OK,
        msg: "OK",
    };

	logger.debug('params= \n', params);

	var peers = params.peers;
	var chaincodeName = params.ccname;
	var chaincodePath = params.ccpath;
	var chaincodeVersion = params.ccvers;

	if (!peers || peers.length == 0) {
		return Promise.reject(paraInvalidMessage('\'peers\''));
	}
    if (typeof(peers) == 'string') {
        peers = peers.split(',')
    } else {
        if (!(peers instanceof Array)) {
            return Promise.reject(paraInvalidMessage('\'peers\''));
        }
    }
    
    
	if (!chaincodeName) {
		return Promise.reject(paraInvalidMessage("'ccname'"));
	}
	if (!chaincodePath) {
		return Promise.reject(paraInvalidMessage("'ccpath'"));
	}
	if (!chaincodeVersion) {
		return Promise.reject(paraInvalidMessage("'ccvers'"));
	}
    
    var username = params.usr;
	if (!username) {
		return Promise.reject(paraInvalidMessage('\'usr\''))
	}
    var orgname = params.org;
	if (!orgname) {
        return Promise.reject(paraInvalidMessage('\'org\''))
	}
	logger.debug('username  : ' + username);
	logger.debug('orgname  : ' + orgname);
    
    
    
    var proms0
    
    if (params.needStateNotify) {
        proms0 = subscribe.notifyChaincodeState(subscribe.chaincodeStates.Installing, chaincodeName, chaincodeVersion, '')
    } else {
        proms0 = Promise.resolve()
    }
    
    return proms0
    .then(()=>{}, //成功什么也不干
          (err)=>{ logger.error('install notifyChaincodeState(%d) failed: err=%s', subscribe.chaincodeStates.Installing, err) //失败了只记录日志，部署或升级继续 
          })
    .then(()=>{
        return hfc_wrap.installChaincode(peers, chaincodeName, chaincodePath, chaincodeVersion, username, orgname)
        })
	.then((response)=>{
            logger.debug('installChaincode success, response=', response)
            body.msg = response.message
            
            var proms1
            if (params.needStateNotify) {
                proms1 = subscribe.notifyChaincodeState(subscribe.chaincodeStates.InstallSuccess, chaincodeName, chaincodeVersion, '')
            } else {
                proms1 = Promise.resolve()
            }
            
            return proms1
            .then(()=>{}, //成功什么也不干
                  (err)=>{ logger.error('install notifyChaincodeState(%d) failed: err=%s', subscribe.chaincodeStates.InstallSuccess, err) //失败了只记录日志，部署或升级继续 
                  })
            .then(()=>{
                return (body);
            })
        })
    .catch((err)=>{
            logger.error('installChaincode failed, err=%s', err)
            body.code = retCode.ERROR
            body.msg = '' + err
            
            var proms1
            if (params.needStateNotify) {
                proms1 = subscribe.notifyChaincodeState(subscribe.chaincodeStates.InstallFailed, chaincodeName, chaincodeVersion, body.msg)
            } else {
                proms1 = Promise.resolve()
            }
            
            return proms1
            .then(()=>{}, //成功什么也不干
                  (err)=>{ logger.error('install notifyChaincodeState(%d) failed: err=%s', subscribe.chaincodeStates.InstallFailed, err) //失败了只记录日志，部署或升级继续 
                  })
            .then(()=>{
                return Promise.reject(body);
            })
        });
}

function __chaincode_instantiateOrUpgrade(params, req, type, serialno) {
    var body = {
        code : retCode.OK,
        msg: "OK",
    };
    
    logger.info("Enter %s.%d", type, serialno);

	logger.debug('params= \n', params);

	var chaincodeName = params.ccname;
	var chaincodeVersion = params.ccvers;
	var channelName = params.channame;
	var fcn = params.fcn;
	var args = params.args;
	var peers = params.peers;

	if (!peers || peers.length == 0) {
		return Promise.reject(paraInvalidMessage('\'peers\''));
	}
    if (typeof(peers) == 'string') {
        peers = peers.split(',')
    } else {
        if (!(peers instanceof Array)) {
            return Promise.reject(paraInvalidMessage('\'peers\''));
        }
    }

	if (!chaincodeName) {
		return Promise.reject(paraInvalidMessage("'ccname'"));
	}
	if (!chaincodeVersion) {
		return Promise.reject(paraInvalidMessage("'ccvers'"));
	}
	if (!channelName) {
		return Promise.reject(paraInvalidMessage("'channame'"));
	}

    var username = params.usr;
	if (!username) {
		return Promise.reject(paraInvalidMessage('\'usr\''))
	}
    var orgname = params.org;
	if (!orgname) {
        orgname = 'org1'
	}
	logger.debug('username  : ' + username);
	logger.debug('orgname  : ' + orgname);
    
    //init 或者 upgrade 可能不需要args
    if (args) {
        if (typeof(args) == 'string') {
            var delim = params.argsdelim ? params.argsdelim : ","
            args = args.split(delim)
        } else {
            if (!(args instanceof Array)) {
                return Promise.reject(paraInvalidMessage('\'args\''));
            }
        }
    }
    
    var inputArgs = []
    if (params.addtm) {
        inputArgs.push(Date.now().toString())  // 第一个参数为调用时间
    }
    if (args) {
        inputArgs = inputArgs.concat(args)
    }

    logger.debug('args=', inputArgs)
    
    var requestObj = {}
    requestObj.peers = peers
    requestObj.channame = channelName
    requestObj.ccname = chaincodeName
    requestObj.ccver = chaincodeVersion
    requestObj.fcn = fcn
    requestObj.inputArgs = inputArgs
    requestObj.username = username
    requestObj.orgname = orgname
    
    if (type == 'instantiate') {
        
        var proms0
        
        if (params.needStateNotify) {
            proms0 = subscribe.notifyChaincodeState(subscribe.chaincodeStates.Deploying, chaincodeName, chaincodeVersion, '')
        } else {
            proms0 = Promise.resolve()
        }
        
        return proms0
        .then(()=>{}, //成功什么也不干
              (err)=>{ logger.error('%s notifyChaincodeState(%d) failed: err=%s', type, subscribe.chaincodeStates.Deploying, err) //失败了只记录日志，部署或升级继续 
              })
        .then(()=>{
            return hfc_wrap.instantiateChaincode(peers, channelName, chaincodeName, chaincodeVersion, fcn, inputArgs, username, orgname, 180000, 50000)
        })
        .then((response)=>{
                logger.debug('instantiateChaincode success, response=', response)
                body.msg = response.message
                body.result = response.result
                
                var proms1
                if (params.needStateNotify) {
                    proms1 = subscribe.notifyChaincodeState(subscribe.chaincodeStates.DeploySuccess, chaincodeName, chaincodeVersion, '')
                } else {
                    proms1 = Promise.resolve()
                }
                
                return proms1
                .then(()=>{}, //成功什么也不干
                      (err)=>{ logger.error('%s notifyChaincodeState(%d) failed: err=%s', type, subscribe.chaincodeStates.DeploySuccess, err) //失败了只记录日志，部署或升级继续 
                      })
                .then(()=>{
                    logger.info("%s.%d success: request=%j", type, serialno, requestObj);
                    return (body);
                })
            })
        .catch((err)=>{
                body.code = retCode.ERROR
                body.msg = '' + err
                
                var proms1
                if (params.needStateNotify) {
                    proms1 = subscribe.notifyChaincodeState(subscribe.chaincodeStates.DeployFailed, chaincodeName, chaincodeVersion, body.msg)
                } else {
                    proms1 = Promise.resolve()
                }
                
                return proms1
                .then(()=>{}, //成功什么也不干
                      (err)=>{ logger.error('%s notifyChaincodeState(%d) failed: err=%s', type, subscribe.chaincodeStates.DeployFailed, err) //失败了只记录日志，部署或升级继续 
                      })
                .then(()=>{
                    logger.error("%s.%d failed: request=%j, err=%j", type, serialno, requestObj, err);
                    return Promise.reject(body);
                })
            });
    } else {
        
        var proms0
        
        if (params.needStateNotify) {
            proms0 = subscribe.notifyChaincodeState(subscribe.chaincodeStates.Upgrading, chaincodeName, chaincodeVersion, '')
        } else {
            proms0 = Promise.resolve()
        }

        return proms0
        .then(()=>{}, //成功什么也不干
              (err)=>{ logger.error('%s notifyChaincodeState(%d) failed: err=%s', type, subscribe.chaincodeStates.Upgrading, err) //失败了只记录日志，部署或升级继续 
              })
        .then(()=>{
            return hfc_wrap.upgradeChaincode(peers, channelName, chaincodeName, chaincodeVersion, fcn, inputArgs, username, orgname, 180000, 50000)
        })
        .then((response)=>{
                logger.debug('upgradeChaincode success, response=', response)
                body.msg = response.message
                body.result = response.result
                
                var proms1
                if (params.needStateNotify) {
                    proms1 = subscribe.notifyChaincodeState(subscribe.chaincodeStates.UpgradeSuccess, chaincodeName, chaincodeVersion, '')
                } else {
                    proms1 = Promise.resolve()
                }
                
                return proms1
                .then(()=>{}, //成功什么也不干
                      (err)=>{ logger.error('%s notifyChaincodeState(%d) failed: err=%s', type, subscribe.chaincodeStates.UpgradeSuccess, err) //失败了只记录日志，部署或升级继续 
                      })
                .then(()=>{
                    logger.info("%s.%d success: request=%j", type, serialno, requestObj);
                    return (body);
                })
            })
        .catch((err)=>{
                body.code = retCode.ERROR
                body.msg = '' + err
                
                var proms1
                if (params.needStateNotify) {
                    proms1 = subscribe.notifyChaincodeState(subscribe.chaincodeStates.UpgradeFailed, chaincodeName, chaincodeVersion, body.msg)
                } else {
                    proms1 = Promise.resolve()
                }
                
                return proms1
                .then(()=>{}, //成功什么也不干
                      (err)=>{ logger.error('%s notifyChaincodeState(%d) failed: err=%s', type, subscribe.chaincodeStates.UpgradeFailed, err) //失败了只记录日志，部署或升级继续 
                      })
                .then(()=>{
                    logger.error("%s.%d failed: request=%j, err=%j", type, serialno, requestObj, err);
                    return Promise.reject(body);
                })
            });
    }
}

// Instantiate chaincode on target peers
function handle_chaincode_instantiate(params, res, req, serialno) {
    for (var p in externReqParams_ChaincodeInstantiate) {
        if (params[p] == undefined)
            params[p] = externReqParams_ChaincodeInstantiate[p]
    }

    var body = {
        code : retCode.OK,
        msg: "OK",
    };
    
    var respSent = false
    //如果需要状态通知，则这里直接返回成功，表示收到了请求，后续会发送状态通知
    if (params.needStateNotify) {
        res.send(body)
        respSent = true
    }

    return __chaincode_instantiateOrUpgrade(params, req, 'instantiate', serialno)
    .then((okResp)=>{
        if (!respSent) {
            res.send(okResp)
            respSent = true
        }
    })
    .catch((errResp)=>{
        if (!respSent) {
            res.send(errResp)
            respSent = true
        }
    })
}

// Upgrade chaincode on target peers
function handle_chaincode_upgrade(params, res, req, serialno) {
    for (var p in externReqParams_ChaincodeUpgrade) {
        if (params[p] == undefined)
            params[p] = externReqParams_ChaincodeUpgrade[p]
    }
    
    
    var body = {
        code : retCode.OK,
        msg: "OK",
    };

    var respSent = false
    
    //如果需要状态通知，则这里直接返回成功，表示收到了请求，后续会发送状态通知
    if (params.needStateNotify) {
        res.send(body)
        respSent = true
    }

    return __chaincode_instantiateOrUpgrade(params, req, 'upgrade', serialno)
    .then((okResp)=>{
        if (!respSent) {
            res.send(okResp)
            respSent = true
        }
    })
    .catch((errResp)=>{
        if (!respSent) {
            res.send(errResp)
            respSent = true
        }
    })
}

function handle_chaincode_instll_instantiate(params, res, req, serialno) {
    var body = {
        code : retCode.OK,
        msg: "OK",
    };

    for (var p in externReqParams_ChaincodeInstall) {
    if (params[p] == undefined)
        params[p] = externReqParams_ChaincodeInstall[p]
    }

    var respSent = false
    
    return __chaincode_install(params, req)
    .then(()=>{
            logger.debug('install OK.');
            
            for (var p in externReqParams_ChaincodeInstantiate) {
                if (params[p] == undefined)
                    params[p] = externReqParams_ChaincodeInstantiate[p]
            }
            
            //如果需要状态通知，则这里直接返回成功，表示收到了请求，后续会发送状态通知
            if (params.needStateNotify) {
                res.send(body)
                respSent = true
            }

            return __chaincode_instantiateOrUpgrade(params, req, 'instantiate', serialno)
            .then((response)=>{
                    logger.debug('instantiate OK.');
                    body.msg = 'install and instantiate OK.'
                    body.result = response.result
                    if (!respSent) {
                        res.send(body)
                        respSent = true
                    }
                },
                (errResp)=>{ // instantiate error
                    if (!respSent) {
                        res.send(errResp) 
                        respSent = true
                    }
                })
        },
        (errResp)=>{ // install error 直接返回
            if (!respSent) {
                res.send(errResp) 
                respSent = true
            }
        })
    .catch((err)=>{
        logger.error('handle_chaincode_instll_instantiate: catch err=%s.', err);
        body.code = retCode.ERROR
        body.msg = 'unexpect err:'+err
        if (!respSent) {
            res.send(body)
            respSent = true
        }
    })
}


function handle_chaincode_instll_upgrade(params, res, req, serialno) {
    var body = {
        code : retCode.OK,
        msg: "OK",
    };

    for (var p in externReqParams_ChaincodeInstall) {
    if (params[p] == undefined)
        params[p] = externReqParams_ChaincodeInstall[p]
    }
 
    var respSent = false
 
    return __chaincode_install(params, req)
    .then(()=>{
            logger.debug('install OK.');
            
            for (var p in externReqParams_ChaincodeUpgrade) {
                if (params[p] == undefined)
                    params[p] = externReqParams_ChaincodeUpgrade[p]
            }
            
            //如果需要状态通知，则这里直接返回成功，表示收到了请求，后续会发送状态通知
            if (params.needStateNotify) {
                res.send(body)
                respSent = true
            }

            return __chaincode_instantiateOrUpgrade(params, req, 'upgrade', serialno)
            .then((response)=>{
                    logger.debug('upgrade OK.');
                    body.msg = 'install and upgrade OK.'
                    body.result = response.result
                    
                    if (!respSent) {
                        res.send(body)
                        respSent = true
                    }
                },
                (errResp)=>{
                if (!respSent) {
                    res.send(errResp)
                    respSent = true
                }
            })
        },
        (errResp)=>{ // install error 直接返回
            if (!respSent) {
                res.send(errResp)
                respSent = true
            }
        })
    .catch((err)=>{
        logger.error('handle_chaincode_instll_upgrade: catch err=%s.', err);
        body.code = retCode.ERROR
        body.msg = 'unexpect err:'+err
        if (!respSent) {
            res.send(body)
            respSent = true
        }
    })
}

function paraInvalidMessage(field) {
	var response = {
        code: retCode.INVLID_PARA,
		msg: field + ' field is missing or Invalid in the request'
	};
	return response;
}

var exportIntfFileRootPath
var exportIntfAddr
var exportIntfBaseUrl

function exportInvokeIntf(usr, req, response) {
    var filePath = getExportInfoFileAbsolutePath(usr)
    
    var intf = {}
    intf.input = {}
    intf.input.url = exportIntfAddr + req.url
    intf.input.method = req.method
    if (req.method == 'POST') {
        intf.input.body = req.body
    }
    
    intf.output = response
    
    logger.debug('exportInvokeIntf: file=%s.', filePath);
    
    fs.writeFileSync(filePath, JSON.stringify(intf, null, 2))
}

function getExportInfoFileAbsolutePath(usr) {
    return mpath.join(exportIntfFileRootPath, getExportInfoFileRelativePath(usr))
}
function getExportInfoFileRelativePath(usr) {
    return mpath.join(module_name, usr+'.txt')
}

function getExportIntfUrl(usr) {
    return exportIntfAddr + exportIntfBaseUrl + '/' + getExportInfoFileRelativePath(usr)
}

var accessSeq = 0
//公共处理
function __handle_comm__(req, res) {
    //logger.info('new http req=%d, res=%d', req.socket._idleStart, res.socket._idleStart)
    res.set({'Content-Type':'text/json','Encodeing':'utf8', 'Access-Control-Allow-Origin':'*'});

    var params
    var method = req.method
    
    if (method == "GET")
        params = JSON.parse(JSON.stringify(req.query))  //deep copy
    else if (method == "POST")
        params = JSON.parse(JSON.stringify(req.body))  //deep copy
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
    
    //获取url中参数，如果有的话
    for (var p in req.params) {
        params[p] = req.params[p]
    }
    
    for (var p in externReqParams_All) {
        if (params[p] == undefined)
            params[p] = externReqParams_All[p]
    }
    
    //调用处理函数
    accessSeq++
    return handle(params, res, req, accessSeq)
}


function Loulan_SetRequireParams(routeHandle, params) {
    if (routeHandle == routeHandles.All) {
        externReqParams_All = params
    } else if (routeHandle == routeHandles.Register) {
        externReqParams_Register = params
    } else if (routeHandle == routeHandles.Invoke) {
        externReqParams_Invoke = params
    } else if (routeHandle == routeHandles.Query) {
        externReqParams_Query = params
    } else if (routeHandle == routeHandles.ChannelCreate) {
        externReqParams_ChannelCreate = params
    } else if (routeHandle == routeHandles.ChannelJoin) {
        externReqParams_ChannelJoin = params
    } else if (routeHandle == routeHandles.ChannelUpdate) {
        externReqParams_ChannelUpdate = params
    } else if (routeHandle == routeHandles.ChaincodeInstall) {
        externReqParams_ChaincodeInstall = params
    } else if (routeHandle == routeHandles.ChaincodeInstantiate) {
        externReqParams_ChaincodeInstantiate = params
    } else if (routeHandle == routeHandles.ChaincodeUpgrade) {
        externReqParams_ChaincodeUpgrade = params
    }
}

function Loulan_SetModuleName(mdname) {
    module_name = mdname
}

function Loulan_SetLogger(log) {
    logger = log
}

function Loulan_SetLogLevel(lvl) {
    logger.setLogLevel(lvl)
}

function Loulan_SetExportIntfPath(pth, addr, urlPrefix) {
    exportIntfFileRootPath = pth
    exportIntfAddr = addr
    
    exportIntfBaseUrl = urlPrefix
}


var routeTable = {}

function Loulan_RegisterRoute(route, getHandel, postHandle) {
    routeTable[route] = {'GET' : getHandel, 'POST' : postHandle}
}

function Loulan_UseDefaultRoute() {

    if (!module_name) {
        throw new Error("please call 'loulan.SetModuleName(moduleName)' first.");
    }
    
    for (var route in defaultRouteTable) {
        var newRoute = route.replace(templateModuleName, module_name)
        routeTable[newRoute] = defaultRouteTable[route]
    }
}

function Loulan_SetWalletPath(p) {
    walletPath = p
}

function Loulan_RegisterInvokeParamsFmtConvHandle(fn) {
    paramsFmtConvHandle_Invoke = fn
}

function Loulan_RegisterQueryParamsFmtConvHandle(fn) {
    paramsFmtConvHandle_Query = fn
}




function Loulan_RegisterQueryResultFormatHandle(fn) {
    resultFormatHandle_Query = fn
}
function Loulan_RegisterInvokeResultFormatHandle(fn) {
    resultFormatHandle_Invoke = fn
}


function Loulan_Start(subCfgFile) {

    if (!module_name) {
        throw new Error("please call 'loulan.SetModuleName(moduleName)' first.");
    }
    
    subscribe.init(subCfgFile)
    
    hfc_wrap.initNetworkTopo()
    .then(()=>{
        for (var path in routeTable) {
            app.get(path, __handle_comm__)
            app.post(path, __handle_comm__)
        }

        http.listen(port, host);
        logger.info("listen on %s:%d...", host, port);
        
    })
    .catch((err)=>{
        logger.error("Loulan_Start: initNetworkTopo error, err=%s", err)
        process.exit(1)
    })
    
}

exports.retCode = retCode
exports.paraInvalidMessage = paraInvalidMessage
exports.SetModuleName = Loulan_SetModuleName
exports.SetWalletPath = Loulan_SetWalletPath
exports.SetRequireParams = Loulan_SetRequireParams
exports.SetLogger = Loulan_SetLogger
exports.UseDefaultRoute = Loulan_UseDefaultRoute
exports.RegisterRoute = Loulan_RegisterRoute
exports.SetExportIntfPath = Loulan_SetExportIntfPath
exports.RegisterInvokeParamsFmtConvHandle = Loulan_RegisterInvokeParamsFmtConvHandle
exports.RegisterQueryParamsFmtConvHandle = Loulan_RegisterQueryParamsFmtConvHandle
exports.RegisterQueryResultFormatHandle = Loulan_RegisterQueryResultFormatHandle
exports.RegisterInvokeResultFormatHandle = Loulan_RegisterInvokeResultFormatHandle
exports.Start = Loulan_Start
