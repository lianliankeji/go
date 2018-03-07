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
//这里的path参数，client端connnect时也必须用这个参数，会拼在url请求中，io.connect("https://xxx/yyy", { path:'/kdsocketio' })。nginx中可以用它来匹配url。
const sockio = require('socket.io')(http, {path: '/kdsocketio'});  


//NOTE: run config.js  before require hfc_wrap.js
require('./config.js');
const hfc_wrap = require('./lib/hfc_wrap/hfc_wrap.js');


const user = require('./lib/user');
const hash = require('./lib/hash');
const common = require('./lib/common');

const host = hfc_wrap.getConfigSetting('host');
const port = hfc_wrap.getConfigSetting('port');

var logger = common.createLog("kd")
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
  logger.info(" ****  kd exit ****");
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
    '/llchain/blocks/:blockId'       : {'GET' : handle_queryBlockById,       'POST' : handle_queryBlockById},
    '/llchain/blocks'                : {'GET' : handle_queryBlockByHash,     'POST' : handle_queryBlockByHash},
    '/llchain/transactions/:trxnId'  : {'GET' : handle_queryTransactions,    'POST' : handle_queryTransactions},
    '/llchain/channels/:channelName' : {'GET' : handle_queryChannels,        'POST' : handle_queryChannels},
    '/llchain/chaincodes/:type'      : {'GET' : handle_queryChaincodes,      'POST' : handle_queryChaincodes},   //type = installed/instantiated
    '/llchain/chain'                 : {'GET' : handle_queryChains,          'POST' : handle_queryChains},
    '/llchain/setenv'                : {'GET' : handle_setenv,               'POST' : handle_setenv},
    '/llchain/test/'                 : {'GET' : handle_test,                 'POST' : handle_test},
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

//  Query Get Block by BlockNumber
function handle_queryBlockById(params, res, req) {
	logger.debug('==================== GET BLOCK BY NUMBER ==================');
    var body = {
        code : retCode.OK,
        msg : "OK",
    };

	let blockId = params.blockId;
	let peer = params.peer;
    if (!peer) {
        peer = 'peer1'
    }
	logger.debug('BlockID : ' + blockId);
	logger.debug('Peer : ' + peer);
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

	hfc_wrap.getBlockByNumber(peer, blockId, username, orgname)
	.then((response)=>{
            logger.debug('getBlockById success, response=', response)
            body.msg = 'ok'
            body.result = response
            res.send(body);
        },
        (err)=>{
            logger.error('getBlockById failed, err=%s', err)
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

	let hash = params.hash;
	let peer = params.peer;
    if (!peer) {
        peer = 'peer1'
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


	hfc_wrap.getBlockByHash(peer, hash, username, orgname)
	.then((response)=>{
            logger.debug('getBlockByHash success, response=', response)
            body.msg = 'ok'
            body.result = response
            res.send(body);
        },
        (err)=>{
            logger.error('getBlockByHash failed, err=%s', err)
            body.msg = '' + err
            res.send(body);
        });
}

// Query Get Transaction by Transaction ID
function handle_queryTransactions(params, res, req) {
	logger.debug(
		'================ GET TRANSACTION BY TRANSACTION_ID ======================'
	);
    var body = {
        code : retCode.OK,
        msg : "OK",
    };

	let trxnId = params.trxnId;
	let peer = params.peer;
    if (!peer) {
        peer = 'peer1'
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

	hfc_wrap.getTransactionByID(peer, trxnId, username, orgname)
	.then((response)=>{
            logger.debug('queryTransactions success, response=', response)
            body.msg = 'ok'
            body.result = response
            res.send(body);
        },
        (err)=>{
            logger.error('queryTransactions failed, err=%s', err)
            body.msg = '' + err
            res.send(body);
        });
}

//Query for Chains Information
function handle_queryChains(params, res, req) {
	logger.debug(
		'================ GET CHANNEL INFORMATION ======================');

    var body = {
        code : retCode.OK,
        msg : "OK",
    };
    
	let peer = params.peer;
    if (!peer) {
        peer = 'peer1'
    }

    var username = params.usr;
	if (!username) {
		username = 'admin'
	}

    var orgname = params.org;
	if (!orgname) {
        orgname = 'org1'
	}

	hfc_wrap.getChainInfo(peer, username, orgname)
	.then((response)=>{
            logger.debug('queryChains success, response=', response)
            body.msg = 'ok'
            body.result = response
            res.send(body);
        },
        (err)=>{
            logger.error('queryChains failed, err=%s', err)
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

	var peer = params.peer;
    if (!peer) {
        peer = 'peer1'
    }

	var installType = params.type;  //type = installed/instantiated
    if (installType != 'installed' && installType != 'instantiated') {
		return res.json(paraInvalidMessage('\'type\''));
    }
    
	if (installType === 'installed') {
		logger.debug(
			'================ GET INSTALLED CHAINCODES ======================');
	} else {
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

	hfc_wrap.getChaincodes(peer, installType, username, orgname)
	.then((response)=>{
            logger.debug('queryChaincodes(%s) success, response=', installType, response)
            body.msg = 'ok'
            body.result = response
            res.send(body);
        },
        (err)=>{
            logger.error('queryChaincodes(%s) failed, err=%s', installType, err)
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

    var channelName = params.channelName
	logger.debug('channelName : ' + channelName);
	if (!channelName) {
		return res.json(paraInvalidMessage('\'channelName\''));
	}

	var peer = params.peer;
	logger.debug('peer: ' + peer);
	if (!peer) {
		peer = 'peer1'
	}

    var username = params.usr;
	if (!username) {
		username = 'admin'
	}

    var orgname = params.org;
	if (!orgname) {
        orgname = 'org1'
	}

	hfc_wrap.getChannels(peer, username, orgname)
	.then((response)=>{
            logger.debug('queryChannels success, response=', response)
            body.msg = 'ok'
            body.result = response
            res.send(body);
        },
        (err)=>{
            logger.error('queryChannels failed, err=%s', err)
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