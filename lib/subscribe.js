"use strict";

const util = require('util');
//const mysql = require('mysql');
const comm = require('./common');
//const hash = require('./hash');
const request = require('request');
const fs = require('fs');


var logger = comm.createLog("subscribe.js")
logger.setLogLevel(logger.logLevel.DEBUG)

const respCode = {
    OK:                    0,
    INVLID_REQ:            1001,    //错误请求
    
    ERROR:                  0xffffffff
}

const subscribeType = {
    ChaincodeState :  'ccstate',   //合约状态，安装，部署，运行等状态
}

const subscribeFlag = {
    Unsub   : 0, //取消订阅
    Sub     : 1, //订阅
}

const chaincodeStates = {
    Invalide        : 0,    //上传完成
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

function subChaincodeState(sender, subFlag, subIntf, params, res) {
    var body = {
        statusCode  : respCode.OK,
        msg: "OK",
    };

    if (subscribeFlag.Unsub == subFlag) {
        
        
    } else if (subscribeFlag.Sub == subFlag) {
        
        var subObj = {}
        subObj.intf = subIntf
        subObj.params = {}
        if (params.format) //调用接口的数据post格式。 如 Json/formData等等
            subObj.params.format = params.format
        
        
        saveConfig(subscribeType.ChaincodeState, sender, subObj)
        
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
        let sendMode = subObj.params.format
        
        let prom =  new Promise((resolve, reject)=>{
            
            ///TODO：根据sendMode来选择不同的数据发送方式， 如 json、formData等格式
            
            /*
            let requestObj = {
                url: intf,
                method: "POST",
                json: true,
                headers: {
                    "content-type": "application/json",
                },
                body: JSON.stringify(notifyBody)
            }
            */
            let requestObj = {
                url: intf,
                formData: notifyBody
            }
            
            logger.debug('notifyChaincodeState: requestObj=', requestObj)
            
            request.post(requestObj, function(error, response, body) {
                
                if (error) {
                    logger.error('notifyChaincodeState: notify failed, request.post error, sender=%s, err=%s.', sender, error)
                    return resolve(sender)  //出错返回sender，在下面处理
                }
                //logger.debug('notifyChaincodeState: response=', response)
                body = JSON.parse(body)
                logger.debug('notifyChaincodeState: body=', body)
                logger.debug('notifyChaincodeState: body.ec=', body.ec)

                if (body.ec == "000000") {
                    logger.debug('notifyChaincodeState OK: recv=%s, ccname=%s, ccver=%s, state=%d.', sender, ccname, ccversion, state)
                    return resolve(null)  //正常时resolve空，异常时resolve发送错误的sender
                } else {
                    logger.error('notifyChaincodeState: notify failed, sender=%s, body=%j.', sender, body)
                    return resolve(sender)  //出错返回sender，在下面处理
                    //如果这里reject，Promise.all会被任一reject打断，从而无法获取其它结果
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

var subscribeConfigFile = ''

//先放文件中，这里保存一下
function saveConfig(type, sender, subObj) {
    var config = subscribeConfig[type]
    if (config == undefined) {
        subscribeConfig[type] = {}
        subscribeConfig[type][sender] = subObj
    } else {
         /*
        var obj = config[sender]
        
        if (obj == undefined) {
            config[sender] = subObj
        } else {
            logger.info('saveConfig: type=%s sender=%s exists already, ignore.', type, sender)
            return //do not writeFile
        }
        */
        //由最新的订阅接口覆盖老的
        config[sender] = subObj
        
    }
    
    fs.writeFileSync(subscribeConfigFile, JSON.stringify(subscribeConfig))
}

function init(subCfgFile) {
    //读取配置文件到cache
    
    if (!subCfgFile) {
        throw Error('miss subCfgFile.')
    }
    
    subscribeConfigFile = subCfgFile
    
    var subConfig
    try {
        subConfig = fs.readFileSync(subscribeConfigFile)
    } catch (err) {
        if (err.code !='ENOENT') {
            throw err
        }
    }
    
    if (subConfig) {
        //不用try-catch，如果异常直接抛出异常即可
        subscribeConfig = JSON.parse(subConfig.toString())
        logger.info('load subConfig:', subscribeConfig)
    }

    
}


exports.subscribeType = subscribeType
exports.chaincodeStates = chaincodeStates
exports.subscribeState = subscribeState
exports.notifyChaincodeState = notifyChaincodeState
exports.init = init
