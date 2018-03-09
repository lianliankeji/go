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

const host = hfc_wrap.getConfigSetting('host');
const port = hfc_wrap.getConfigSetting('port');

var logger = common.createLog("loulan")
logger.setLogLevel(logger.logLevel.DEBUG)


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

const globalCcid = "a71bc9939c8774ff6ebbea6984110e4a8307db002a31d40b50cefce2fe3342da"


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



//注册路由处理函数。数组的第一个参数为get/post对应的处理函数， 第二个参数为处理路由中参数的函数
const routeTable = {
    '/loulan/channel/create'        : {'GET' : handle_channel_create,          'POST' : handle_channel_create},
    '/loulan/channel/join'          : {'GET' : handle_channel_join,            'POST' : handle_channel_join},
  //'/loulan/channel/update'        : {'GET' : handle_channel_update,          'POST' : handle_channel_update},
    '/loulan/chaincode/install'     : {'GET' : handle_chaincode_install,       'POST' : handle_chaincode_install},
    '/loulan/chaincode/instantiate' : {'GET' : handle_chaincode_instantiate,   'POST' : handle_chaincode_instantiate},
    '/loulan/chaincode/upgrade'     : {'GET' : handle_chaincode_upgrade,       'POST' : handle_chaincode_upgrade},
    '/loulan/register'              : {'GET' : handle_register,                'POST' : handle_register},
    '/loulan/invoke'                : {'GET' : handle_invoke,                  'POST' : handle_invoke},
    '/loulan/query'                 : {'GET' : handle_query,                   'POST' : handle_query},
    '/loulan/setenv'                : {'GET' : handle_setenv,                  'POST' : handle_setenv},
    '/loulan/test/'                 : {'GET' : handle_test,                    'POST' : handle_test},
}

//for test
function test_paramParser(params, req) {
    
}
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
function handle_register(params, res, req){
    logger.debug('enter handle_register')
    
    return __execRegister(params, req, true)
    .then((response)=>{
            res.send(response)
        }, 
        (err)=> {
            res.send(err)
        })
}

function __execRegister(params, req, outputRResult) {
    var body = {
        code : retCode.OK,
        msg: "OK",
    };

	var username = params.usr;
	var orgname = params.org;
    var funcName = params.func;
	logger.debug('User name : ' + username);
	logger.debug('Org name  : ' + orgname);
	if (!username) {
		return Promise.reject(paraInvalidMessage('\'username\''))
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
	return hfc_wrap.registerAndEnroll(username, orgname, true)
    .then((response)=> {
            //OK
            logger.debug('registerAndEnroll responsed ok, response=', response)
            if (funcName == "account" || funcName == "accountCB") {
                ///TODO:
                body.msg = response.message
                return (body)
            } else {
                body.msg = response.message
                return (body)
            }
        },
        (err)=> {
            logger.debug('registerAndEnroll responsed error, err=%s', err)
            body.code = retCode.ERROR
            body.msg = '' + err
            return Promise.reject(body)
        });
}


// Create Channel
function handle_channel_create(params, res, req) {
    var body = {
        code : retCode.OK,
        msg: "OK",
    };

	logger.debug('End point : /channels');
	var channelName = params.channelName;
	var channelConfigPath = params.channelConfigPath;
    var username = params.usr;
	if (!username) {
		return res.send(paraInvalidMessage('\'usr\''))
	}
    var orgname = params.org;
	if (!orgname) {
        orgname = 'org1'
	}

	logger.debug('Channel name : ' + channelName);
	logger.debug('channelConfigPath : ' + channelConfigPath); //../artifacts/channel/mychannel.tx
	if (!channelName) {
		return res.send(paraInvalidMessage('\'channelName\''));
	}
	if (!channelConfigPath) {
		return res.send(paraInvalidMessage('\'channelConfigPath\''));
	}

	hfc_wrap.createChannel(channelName, channelConfigPath, username, orgname)
	.then((response)=>{
            logger.debug('createChannel success, response=', response)
            body.msg = response.message
            res.send(body)
        },
        (err)=>{
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

	var channelName = params.channelName;
	var peers = params.peers;
	logger.debug('channelName : ' + channelName);
	logger.debug('peers : ' + peers);
	if (!channelName) {
		res.send(paraInvalidMessage('\'channelName\''));
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
        orgname = 'org1'
	}

	hfc_wrap.joinChannel(channelName, peers.split(','), username, orgname)
	.then((response)=>{
            logger.debug('joinChannel success, response=', response)
            body.msg = response.message
            res.send(body);
        },
        (err)=>{
            logger.debug('joinChannel failed, err=%s', err)
            body.code = retCode.ERROR
            body.msg = '' + err
            res.send(body);
        });
}



// Install chaincode on target peers
function handle_chaincode_install(params, res, req) {
    var body = {
        code : retCode.OK,
        msg: "OK",
    };

	var peers = params.peers;
	var chaincodeName = params.ccname;
	var chaincodePath = params.ccpath;
	var chaincodeVersion = params.ccvers;
	logger.debug('peers : ' + peers); // target peers list
	logger.debug('chaincodeName : ' + chaincodeName);
	logger.debug('chaincodePath  : ' + chaincodePath);
	logger.debug('chaincodeVersion  : ' + chaincodeVersion);
	if (!peers || peers.length == 0) {
		res.send(paraInvalidMessage('\'peers\''));
		return;
	}
	if (!chaincodeName) {
		res.send(paraInvalidMessage('\'chaincodeName\''));
		return;
	}
	if (!chaincodePath) {
		res.send(paraInvalidMessage('\'chaincodePath\''));
		return;
	}
	if (!chaincodeVersion) {
		res.send(paraInvalidMessage('\'chaincodeVersion\''));
		return;
	}
    
    var username = params.usr;
	if (!username) {
		return res.send(paraInvalidMessage('\'usr\''))
	}
    var orgname = params.org;
	if (!orgname) {
        orgname = 'org1'
	}

	hfc_wrap.installChaincode(peers.split(','), chaincodeName, chaincodePath, chaincodeVersion, username, orgname)
	.then((response)=>{
            logger.debug('installChaincode success, response=', response)
            body.msg = response.message
            res.send(body);
        },
        (err)=>{
            logger.error('installChaincode failed, err=%s', err)
            body.code = retCode.ERROR
            body.msg = '' + err
            res.send(body);
        });
}

function __handle_chaincode_instantiateOrUpgrade(params, res, req, type) {
    var body = {
        code : retCode.OK,
        msg: "OK",
    };

	var chaincodeName = params.ccname;
	var chaincodeVersion = params.ccvers;
	var channelName = params.channame;
	var fcn = params.fcn;
	var args = params.args;
	logger.debug('channelName  : ' + channelName);
	logger.debug('chaincodeName : ' + chaincodeName);
	logger.debug('chaincodeVersion  : ' + chaincodeVersion);
	logger.debug('fcn  : ' + fcn);
	logger.debug('args  : ' + args);
	if (!chaincodeName) {
		res.send(paraInvalidMessage('\'chaincodeName\''));
		return;
	}
	if (!chaincodeVersion) {
		res.send(paraInvalidMessage('\'chaincodeVersion\''));
		return;
	}
	if (!channelName) {
		res.send(paraInvalidMessage('\'channelName\''));
		return;
	}
	if (!args) {
		res.send(paraInvalidMessage('\'args\''));
		return;
	}

    var username = params.usr;
	if (!username) {
		return res.send(paraInvalidMessage('\'usr\''))
	}
    var orgname = params.org;
	if (!orgname) {
        orgname = 'org1'
	}

    args = args.split(',')
    logger.debug('args=', args)
    
    if (type == 'instantiate') {
        hfc_wrap.instantiateChaincode(channelName, chaincodeName, chaincodeVersion, fcn, args, username, orgname, 50000)
        .then((response)=>{
                logger.debug('instantiateChaincode success, response=', response)
                body.msg = response.message
                body.result = response.result
                res.send(body);
            },
            (err)=>{
                logger.error('instantiateChaincode failed, err=%s', err)
                body.code = retCode.ERROR
                body.msg = '' + err
                res.send(body);
            });
    } else {
        hfc_wrap.upgradeChaincode(channelName, chaincodeName, chaincodeVersion, fcn, args, username, orgname, 50000)
        .then((response)=>{
                logger.debug('upgradeChaincode success, response=', response)
                body.msg = response.message
                body.result = response.result
                res.send(body);
            },
            (err)=>{
                logger.error('upgradeChaincode failed, err=%s', err)
                body.code = retCode.ERROR
                body.msg = '' + err
                res.send(body);
            });
    }
    
}

// Instantiate chaincode on target peers
function handle_chaincode_instantiate(params, res, req) {
    return __handle_chaincode_instantiateOrUpgrade(params, res, req, 'instantiate')
}

// Upgrade chaincode on target peers
function handle_chaincode_upgrade(params, res, req) {
    return __handle_chaincode_instantiateOrUpgrade(params, res, req, 'upgrade')
}

// Invoke transaction on chaincode on target peers
function handle_invoke(params, res, req) {
	logger.debug('==================== INVOKE ON CHAINCODE ==================');
    var body = {
        code : retCode.OK,
        msg: "OK",
    };

	var peers = params.peers;
	var chaincodeName = params.ccname;
	var channelName = params.channame;
	var fcn = params.fcn;
	var args = params.args;
	logger.debug('channelName  : ' + channelName);
	logger.debug('chaincodeName : ' + chaincodeName);
	logger.debug('fcn  : ' + fcn);
	logger.debug('args  : ' + args);
	if (!chaincodeName) {
		res.send(paraInvalidMessage('\'chaincodeName\''));
		return;
	}
	if (!channelName) {
		res.send(paraInvalidMessage('\'channelName\''));
		return;
	}
	if (!fcn) {
		res.send(paraInvalidMessage('\'fcn\''));
		return;
	}
	if (!args) {
		res.send(paraInvalidMessage('\'args\''));
		return;
	}

    var username = params.usr;
	if (!username) {
		return res.send(paraInvalidMessage('\'usr\''))
	}
    var orgname = params.org;
	if (!orgname) {
        orgname = 'org1'
	}

    args = args.split(',')
    peers = peers.split(',')
    
	hfc_wrap.invokeChaincode(peers, channelName, chaincodeName, fcn, args, username, orgname)
	.then((response)=>{
            logger.debug('invoke success, response=', response)
            body.msg = response.message
            body.result = response.result
            res.send(body);
        },
        (err)=>{
            logger.error('invoke failed, err=%s', err)
            body.code = retCode.ERROR
            body.msg = '' + err
            res.send(body);
        });
}

// Query on chaincode on target peers
function handle_query(params, res, req) {
	logger.debug('==================== QUERY BY CHAINCODE ==================');
    var body = {
        code : retCode.OK,
        msg: "OK",
    };

	var channelName = params.channame;
	var chaincodeName = params.ccname;
	let args = params.args;
	let fcn = params.fcn;
	let peer = params.peer;

	logger.debug('channelName : ' + channelName);
	logger.debug('chaincodeName : ' + chaincodeName);
	logger.debug('fcn : ' + fcn);
	logger.debug('args : ' + args);

	if (!chaincodeName) {
		res.send(paraInvalidMessage('\'chaincodeName\''));
		return;
	}
	if (!channelName) {
		res.send(paraInvalidMessage('\'channelName\''));
		return;
	}
	if (!fcn) {
		res.send(paraInvalidMessage('\'fcn\''));
		return;
	}
	if (!args) {
		res.send(paraInvalidMessage('\'args\''));
		return;
	}

    var username = params.usr;
	if (!username) {
		return res.send(paraInvalidMessage('\'usr\''))
	}
    var orgname = params.org;
	if (!orgname) {
        orgname = 'org1'
	}

    args = args.split(',')

    
	hfc_wrap.queryChaincode(peer, channelName, chaincodeName, args, fcn, username, orgname)
	.then((response)=>{
            logger.debug('invoke success, response=', response)
            body.msg = response.message
            body.result = response.result
            res.send(body);
        },
        (err)=>{
            logger.error('invoke failed, err=%s', err)
            body.code = retCode.ERROR
            body.msg = '' + err
            res.send(body);
        });
}


function paraInvalidMessage(field) {
	var response = {
        code: retCode.INVLID_PARA,
		msg: field + ' field is missing or Invalid in the request'
	};
	return response;
}



//公共处理
function __handle_comm__(req, res) {
    //logger.info('new http req=%d, res=%d', req.socket._idleStart, res.socket._idleStart)
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
    
    //获取url中参数，如果有的话
    for (var p in req.params) {
        params[p] = req.params[p]
    }
    
    //调用处理函数
    return handle(params, res, req)
}

for (var path in routeTable) {
    app.get(path, __handle_comm__)
    app.post(path, __handle_comm__)
}


http.listen(port, "127.0.0.1");
logger.info("default ccid : %s", globalCcid);
logger.info("listen on %d...", port);