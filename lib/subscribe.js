"use strict";

const util = require('util');
//const mysql = require('mysql');
const comm = require('./common');
//const hash = require('./hash');
const request = require('request');


var logger = comm.createLog("subscribe.js")

const respCode = {
    OK:                    200,
    INVLID_REQ:            400,    //错误请求
    
    ERROR:                  0xffffffff
}

const subscribeType {
    ChaincodeState : 1,   //合约状态，安装，部署，运行等状态
}

const subscribeFlag {
    Unsub   : 0, //取消订阅
    Sub     : 1, //订阅
}

const chaincodeStates {
    Invalide        : 0,    //
    Installing      : 1,    //正在安装
    InstallFailed   : 2,    //安装失败
    InstallSuccess  : 3,    //安装成功
    Deploying       : 4,    //正在部署
    DeployFailed    : 5,    //部署失败
    DeploySuccess   : 6,    //部署成功
    Upgrading       : 7,    //正在升级
    UpgradeFailed   : 8,    //升级失败
    UpgradeSuccess  : 9,    //升级成功
    Running         : 10,   //运行
    Stoped          : 11,   //停止
}


var subscribeConfig = {}  //先放文件中，这里做个cache，以后放数据库中

function subscribeState(params, res, req) {
    var sender  = params.sendId
    var subType = params.type   //订阅类型
    var subFlag = params.flag   //0 取消订阅  1 订阅
    var subIntf = params.intf

    var body = {
        statusCode  : respCode.OK,
        msg: "OK",
    };
    
    logger.info('subscribeState: ', params)
   
    if (subType == subscribeType.ChaincodeState) {
        return subChaincodeState(sender, subFlag, subIntf, params, res, req)
    } else {
        body.statusCode = respCode.INVLID_REQ
        body.msg = util.format("unknown subscribe type '%s'", subType)
        res.send(body)
    }
}

function getSubConfig(subType) {
    return subscribeConfig[subType]
}

function notifyChaincodeState(state, ccname, ccversion, msg) {

    var subConfig = getSubConfig(subscribeType.ChaincodeState)
    if (subConfig == undefined) {
        return Promise.resolve(null)
    }
    
    var notifyBody = {}
    notifyBody.ccname = ccname
    notifyBody.ccversion = ccversion
    notifyBody.state = state
    notifyBody.msg = msg
    
    
    var allReqPromises = []
    
    //use Promise, so must use 'let' keyword in for loop
    for (let sender in subConfig) {
        let subObj = subConfig[sender]
        
        let intf = subObj.intf
        
        let prom =  new Promise((resolve, reject)=>{
            request({
                url: intf,
                method: "POST",
                json: true,
                headers: {
                    "content-type": "application/json",
                },
                body: JSON.stringify(notifyBody)
                
            }, function(error, response, body) {
                if (!error && response.statusCode == 200) {
                    logger.debug('notifyChaincodeState: OK.')
                    return resolve(null)  //正常时resolve空，异常时resolve发送错误的sender
                } else {
                    logger.error('notifyChaincodeState: notify failed, sender=%s, err=%s.', sender, error)
                    return resolve(sender)
                    //如果这里reject，Promise.all只能获取到一个reject
                    //return reject(logger.errorf('notifyChaincodeState: notify failed, sender=%s, err=%s.', sender, error))
                }
            }); 
        })
        
        allReqPromises.push(prom)
    }
    
    return Promise.all(allReqPromises)
    .then((results)=>{
        var errSdrList = []
        for (var i in results) {
            var sender = results[i]
            if (sender) 
                errSdrList.push(sender)
        }
        
        if (errSdrList.length > 0) {
            return Promise.reject( logger.errorf('notifyChaincodeState: notify some target failed, target=%s',  errSdrList.join(',')))
        }
    })
    .catch((err)=>{
        return Promise.reject(err)
    })
}

function subChaincodeState(sender, subFlag, subIntf, params, res) {
    var body = {
        statusCode  : respCode.OK,
        msg: "OK",
    };

    if (subscribeFlag.Unsub == subFlag) {
        
        
    } else if (subscribeFlag.Sub == subFlag) {
        
        var subObj = {}
        subObj.intf = subIntf
        subObj.params = {} //目前不需要参数
        
        
        saveConfig(subscribeType.ChaincodeState, sender, subObj)
        
        res.send(body)
    }
}

var subCfgFile = ''

//先放文件中，这里保存一下
function saveConfig(type, sender, subObj) {
    var config = subscribeConfig[type]
    if (config == undefined)
        subscribeConfig[type] = {}
        subscribeConfig[type][sender] = subObj
    else {
        var obj = config[sender]
        if (obj == undefined) {
            config[sender] = subObj
        } else {
            logger.info('saveConfig: type=%d sender=%s exists already, ignore.', type, sender)
            return //do not writeFile
        }
        
    }
    
    fs.writeFileSync(subCfgFile, JSON.stringify(subscribeConfig))
}

function init(configFile) {
    //读取配置文件到cache
    
    subCfgFile = configFile
    
    var subConfig
    try {
        subConfig = fs.readFileSync(subCfgFile)
    } catch (err) {
        if (err.code !='ENOENT') {
            throw err
        }
    }
    
    if (subConfig) {
        //不用try-catch，如果异常直接抛出异常即可
        subscribeConfig = JSON.parse(subConfig.toString())
    }

    
}


exports.subscribeType = subscribeType
exports.chaincodeStates = chaincodeStates
exports.subscribeState = subscribeState
exports.notifyChaincodeState = notifyChaincodeState
exports.init = init
