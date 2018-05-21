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

const fs = require('fs')

const loulan = require('./loulan')


const common = require('./lib/common');
const hash = require('./lib/hash');


var logger = common.createLog("kd")
logger.setLogLevel(logger.logLevel.INFO)



process.on('exit', function (){
  logger.info(" ****  kd exit ****");
});

loulan.SetModuleName('kd')

loulan.SetLogger(logger)
loulan.UseDefaultRoute()


loulan.SetRequireParams(loulan.routeHandles.All,    {channame: 'retail-channel', org: 'org1', ccname: 'accoutsys'})


loulan.SetRequireParams(loulan.routeHandles.ChaincodeInstall, {peers: 'peer0,peer1,peer2,peer3'})

loulan.SetRequireParams(loulan.routeHandles.ChaincodeInstantiate, {peers: 'peer0,peer1,peer2,peer3'})

loulan.SetRequireParams(loulan.routeHandles.ChaincodeUpgrade, {peers: 'peer0,peer1,peer2,peer3'})


loulan.SetRequireParams(loulan.routeHandles.Register,   {signature: 'LQ==', peers: 'peer0,peer1,peer2,peer3'})

loulan.SetRequireParams(loulan.routeHandles.Query,      {signature: 'LQ==', peer: 'peer0'})   //�������ϲ���֤ǩ�������ᷢ����������������Զ���һ��  'LQ=='Ϊ'-'��base64����

loulan.SetRequireParams(loulan.routeHandles.Invoke,     {signature: 'LQ==', peers: 'peer0,peer1,peer2,peer3'}) //�������ϲ���֤ǩ�������ᷢ����������������Զ���һ�� 'LQ=='Ϊ'-'��base64����

//loulan.SetExportIntfPath('/usr/share/nginx/whtmls/exportIntf', 'https://www.lianlianchains.com', '/exportIntf')

var cfgStr = fs.readFileSync('./kd.cfg', 'utf-8')
var kd_cfg = JSON.parse(cfgStr)

loulan.RegisterInvokeParamsFmtConvHandle(parseParams_Invoke)
loulan.RegisterQueryParamsFmtConvHandle(parseParams_Query)

loulan.RegisterInvokeResultFormatHandle(formatInvokeResult)
loulan.RegisterQueryResultFormatHandle(formatQueryResult)

loulan.TurnoffErrorJson() //error�ݲ�֧��json��ʽ

loulan.Start('./cfg/subscribe.cfg')


function parseParams_Invoke(params) {
    var body = {
        code : loulan.retCode.OK,
        msg: "OK",
        result: ""
    };

    var func = params.fcn || params.func

    if (!func) {
        return loulan.paraInvalidMessage("'fcn'")
    }
    
    params.fcn = func
    
    var user = params.usr
    var acc = params.acc
    if (!user) {
        return loulan.paraInvalidMessage("'user'")
    }
    if (!acc) {
        return loulan.paraInvalidMessage("'acc'")
    }
    
    if (params.args) {
        body.code = loulan.retCode.INVLID_PARA
        body.msg = "must no args in params."
        return body
    }
    
    params.args = []
    var args = params.args
    
    args.push(user, acc)
    
    
    if (func == "account" || func == "accountCB") {
        var pubKeyHash = ''
        args.push(pubKeyHash)
    } else if (func == "issue") {
        var amt = params.amt;
        args.push(amt)

    } else if (func == "transefer") {
        var reacc = params.reacc;
        var amt = params.amt;
        var transType = params.tstp;
        if (transType == undefined)
            transType = ""
        var description = params.desc;
        if (description == undefined)
            description = ""
        var sameEntSaveTrans = params.sest; //���ת����ת���˻���ͬ���Ƿ��¼���� 0��ʾ����¼ 1��ʾ��¼
        if (sameEntSaveTrans == undefined)
            sameEntSaveTrans = "1" //Ĭ�ϼ�¼
        //�Ƿ�preCheck
        if (params.pck == undefined)
            params.pck = "0" //Ĭ�ϲ�Ԥ���

        args.push(reacc, amt, description, transType, sameEntSaveTrans)
    } else if (func == "transeferUsePwd") {
        params.fcn = "transefer2" //�ڲ���transefer2
        params.func = "transefer2" //�ڲ���transefer2
       
        var reacc = params.reacc;
        var amt = params.amt;
        var transType = params.tstp;
        if (transType == undefined)
            transType = ""
        var description = params.desc;
        if (description == undefined)
            description = ""
        var sameEntSaveTrans = params.sest; //���ת����ת���˻���ͬ���Ƿ��¼���� 0��ʾ����¼ 1��ʾ��¼
        if (sameEntSaveTrans == undefined)
            sameEntSaveTrans = "1" //Ĭ�ϼ�¼
        
        var pwd = params.pwd
        if (pwd == undefined || pwd.length == 0) {
            body.code=loulan.retCode.ERROR;
            body.msg="tx error: pwd is empty."
            logger.errorf("invoke(%s): failed, err=pwd is empty.", func)
            return body
        }
        //���ܺ�תΪbase64
        var encryptedPwd = hash.aes_encrypt(256, kd_cfg.PWD_ENCRYPT_KEY, kd_cfg.PWD_ENCRYPT_IV, pwd)
        if (encryptedPwd == undefined) {
            body.code=loulan.retCode.ERROR;
            body.msg="tx error: pwd encrypt failed."
            logger.errorf("invoke(%s): failed, err=pwd encrypt failed.", func)
            return body
        }
        
        //�Ƿ�preCheck
        if (params.pck == undefined) {
            params.pck = "0" //Ĭ�ϲ�Ԥ���
        }
        
        args.push(reacc, amt, encryptedPwd, description, transType, sameEntSaveTrans)
    } else if (func == "transeferAndLock") {
        params.fcn = "transefer3" //�ڲ���transefer3
        params.func = "transefer3" //�ڲ���transefer3
        
        var reacc = params.reacc;
        var amt = params.amt;
        var transType = params.tstp;
        if (transType == undefined)
            transType = ""
        var description = params.desc;
        if (description == undefined)
            description = ""
        var sameEntSaveTrans = params.sest; //���ת����ת���˻���ͬ���Ƿ��¼���� 0��ʾ����¼ 1��ʾ��¼
        if (sameEntSaveTrans == undefined)
            sameEntSaveTrans = "1" //Ĭ�ϼ�¼

        //�Ƿ�preCheck
        if (params.pck == undefined)
            params.pck = "0" //Ĭ�ϲ�Ԥ���
        
        var lockEndtmAmtMap = {}
        var parseError = __parseLockAmountCfg(params.lcfg, invokeTime, lockEndtmAmtMap)
        if (parseError) {
            body.code=loulan.retCode.ERROR;
            body.msg= util.format("tx error: %s", parseError)
            logger.errorf("invoke(%s): failed, err=%s", func, parseError)
            return body
        }
        
        var newLockCfgs = ""
        var totalLockAmt = 0
        for (var lockEndTime in lockEndtmAmtMap){
            var lamt = lockEndtmAmtMap[lockEndTime]
            totalLockAmt += lamt
            if (lockEndTime <=  invokeTime) {
                body.code=loulan.retCode.ERROR;
                body.msg="tx error: lock end time must big than now."
                logger.errorf("invoke(%s): failed, err=lock end time must big than now.", func)
                return body
            }
            newLockCfgs += util.format("%d:%d;", lamt, lockEndTime)
        }
        
        if (totalLockAmt > amt) {
            body.code=loulan.retCode.ERROR;
            body.msg="tx error: lock amount big than transefer-amount."
            logger.errorf("invoke(%s): failed, err=lock amount big than transefer-amount.", func)
            return body
        }

        args.push(reacc, amt, newLockCfgs, description, transType, sameEntSaveTrans)
    } else if (func == "updateEnv") {
        var key = params.key;
        var value = params.val;
        args.push(key, value)
    } else if (func == "setAllocCfg") {
        var rackid = params.rid;
        var seller = params.slr;
        var platform = params.pfm;
        var fielder = params.fld;
        var delivery = params.dvy;
        args.push(rackid, seller, fielder, delivery, platform)
    } else if (func == "allocEarning") {
        var rackid = params.rid;
        var seller = params.slr;
        var platform = params.pfm;
        var fielder = params.fld;
        var delivery = params.dvy;
        var totalAmt = params.tamt;
        var allocKey = params.ak;  
        args.push(rackid, seller, fielder, delivery, platform, allocKey, totalAmt)
    } else if (func == "setSESCfg") {
        var rackid = params.rid;
        var cfg = params.cfg;
        args.push(rackid, cfg)
        
    } else if (func == "encourageScoreForSales" || func == "encourageScoreForNewRack") {
        var cfg = params.cfg;
        var transType = params.tstp;
        if (transType == undefined)
            transType = ""
        var description = params.desc;
        if (description == undefined)
            description = ""
        var sameEntSaveTrans = params.sest; //���ת����ת���˻���ͬ���Ƿ��¼���� 0��ʾ����¼ 1��ʾ��¼
        if (sameEntSaveTrans == undefined)
            sameEntSaveTrans = "1" //Ĭ�ϼ�¼
        
        args.push(cfg, transType, description, sameEntSaveTrans)
        
    } else if (func == "buyFinance") {
        var rackid = params.rid;
        var financid = params.fid;
        var payee = params.pee;
        var amout = params.amt;
        var transType = params.tstp;
        if (transType == undefined)
            transType = ""
        var description = params.desc;
        if (description == undefined)
            description = ""
        var sameEntSaveTrans = params.sest; //���ת����ת���˻���ͬ���Ƿ��¼���� 0��ʾ����¼ 1��ʾ��¼
        if (sameEntSaveTrans == undefined)
            sameEntSaveTrans = "1" //Ĭ�ϼ�¼
        
        args.push(rackid, financid, payee, amout, transType, description, sameEntSaveTrans)
        
    } else if (func == "financeIssueFinish") {
        var financid = params.fid;
        args.push(financid)
        
    } else if (func == "payFinance") {
        var rackid = params.rid;
        var payee = params.pee;
        var transType = params.tstp;
        if (transType == undefined)
            transType = ""
        var description = params.desc;
        if (description == undefined)
            description = ""
        var sameEntSaveTrans = params.sest; //���ת����ת���˻���ͬ���Ƿ��¼���� 0��ʾ����¼ 1��ʾ��¼
        if (sameEntSaveTrans == undefined)
            sameEntSaveTrans = "1" //Ĭ�ϼ�¼
        
        args.push(rackid, payee, transType, description, sameEntSaveTrans)
        
    } else if (func == "financeBouns") {
        var financid = params.fid;
        var rackSalesCfg = params.rscfg;
        args.push(financid, rackSalesCfg)
        
    } else if (func == "setFinanceCfg") {
        var rackid = params.rid;
        var profitsPercent = params.prop;
        var investProfitsPercent = params.ivpp;
        var investCapacity = params.ivc;
        args.push(rackid, profitsPercent, investProfitsPercent, investCapacity)
        
    } else if (func == "updateCert") {
        var upUser = params.uusr;
        var upAcc = params.uacc;
        var upCert = params.ucert;
        args.push(upUser, upAcc, upCert)
        
    } else if (func == "AuthCert") {
        var authAcc = params.aacc;
        var authUser = params.ausr;
        var authCert = params.acert;
        args.push(authAcc, authUser, authCert)
        
    } else if (func == "setWorldState") {
        params.fcn = "updateState"
        params.func = "updateState"

        var fileName = params.fnm;
        var needHash = params.hash;
        if (needHash == undefined)
            needHash = "0"
        var sameKeyOverwrite = params.skow;
        if (sameKeyOverwrite == undefined)
            sameKeyOverwrite = "1"  //Ĭ����ͬ��key����
        
        var srcCcid = params.sccid;
        
        args.push(fileName, needHash, sameKeyOverwrite, srcCcid)
    } else if (func == "lockAccAmt") {
        var lockedAccName = params.lacc;
        var lockedCfgs = params.lcfg;
        var overwriteOld = params.owo;
        if (overwriteOld == undefined)
            overwriteOld = "0"  //Ĭ�ϲ��������м�¼
        var canLockMoreThanRest = params.clmtr
        if (canLockMoreThanRest == undefined)
            canLockMoreThanRest = "0"  //Ĭ�ϲ���lock������Ľ��
        
        var lockEndtmAmtMap = {}
        var parseError = __parseLockAmountCfg(lockedCfgs, invokeTime, lockEndtmAmtMap)
        if (parseError) {
            body.code=loulan.retCode.ERROR;
            body.msg= util.format("tx error: %s", parseError)
            logger.errorf("invoke(%s): failed, err=%s", func, parseError)
            return body
        }
        
        var newLockCfgs = ""
        var totalLockAmt = 0
        for (var lockEndTime in lockEndtmAmtMap){
            var lamt = lockEndtmAmtMap[lockEndTime]
            totalLockAmt += lamt
            if (lockEndTime <=  invokeTime) {
                body.code=loulan.retCode.ERROR;
                body.msg="tx error: lock end time must big than now."
                logger.errorf("invoke(%s): failed, err=lock end time must big than now.", func)
                return body
            }
            newLockCfgs += util.format("%d:%d;", lamt, lockEndTime)
        }
        
        args.push(lockedAccName, newLockCfgs, overwriteOld, canLockMoreThanRest)
        
    } else if (func == "setAccPwd" || func == "resetAccPwd") {
        if (func == "setAccPwd"){
            params.fcn = "setAccCfg1"
            params.func = "setAccCfg1"
        }
        else if (func == "resetAccPwd"){
            params.fcn = "setAccCfg2"
            params.func = "setAccCfg2"
        }
        
        var pwd = params.pwd
        if (pwd == undefined || pwd.length == 0) {
            body.code=loulan.retCode.ERROR;
            body.msg="tx error: pwd is empty."
            logger.errorf("invoke(%s): failed, err=pwd is empty.", func)
            return body
        }
        //���ܺ�תΪbase64
        var encryptedPwd = hash.aes_encrypt(256, kd_cfg.PWD_ENCRYPT_KEY, kd_cfg.PWD_ENCRYPT_IV, pwd)
        if (encryptedPwd == undefined) {
            body.code=loulan.retCode.ERROR;
            body.msg="tx error: pwd encrypt failed."
            logger.errorf("invoke(%s): failed, err=pwd encrypt failed.", func)
            return body
        }
        
        args.push(encryptedPwd)
        
    } else if (func == "changeAccPwd") {
        params.fcn = "setAccCfg3"
        params.func = "setAccCfg3"
        
        var oldpwd = params.opwd
        var newpwd = params.npwd
        if (oldpwd == undefined || oldpwd.length == 0 || newpwd == undefined || newpwd.length == 0) {
            body.code=loulan.retCode.ERROR;
            body.msg="tx error: pwd is empty."
            logger.errorf("invoke(%s): failed, err=pwd is empty.", func)
            return body
        }
        //���ܺ�תΪbase64
        var encryptedOldPwd = hash.aes_encrypt(256, kd_cfg.PWD_ENCRYPT_KEY, kd_cfg.PWD_ENCRYPT_IV, oldpwd)
        if (encryptedOldPwd == undefined) {
            body.code=loulan.retCode.ERROR;
            body.msg="tx error: pwd encrypt failed."
            logger.errorf("invoke(%s): failed, err=pwd encrypt failed.", func)
            return body
        }
        var encryptedNewPwd = hash.aes_encrypt(256, kd_cfg.PWD_ENCRYPT_KEY, kd_cfg.PWD_ENCRYPT_IV, newpwd)
        if (encryptedNewPwd == undefined) {
            body.code=loulan.retCode.ERROR;
            body.msg="tx error: pwd encrypt failed."
            logger.errorf("invoke(%s): failed, err=pwd encrypt failed.", func)
            return body
        }
        
        args.push(encryptedOldPwd, encryptedNewPwd)
        
    }
    
    return null
}

function __parseLockAmountCfg(lockCfgs, currTime, tmAmtMap) {
    var cfgArr = lockCfgs.split(';')
    var newLockCfgs = ""
    for (var i=0; i<cfgArr.length; i++){
        var cfg = cfgArr[i]
        if (cfg.length == 0)
            continue
        
        var pair=cfg.split(':')
        if (pair.length != 2) {
            return Error("lock config format error1.")
        }
        var lockAmt = parseInt(pair[0])
        if (lockAmt == NaN) {
            return Error("lock config format error2.")
        }
        
        var lockEndTimeStr = pair[1]
        var lockEndTime = 0
        var daysIdx = lockEndTimeStr.indexOf('days')
        if (daysIdx > 0) {
            var days = parseInt(lockEndTimeStr.substr(0, daysIdx))
            if (days == NaN) {
                return Error("lock config format error3.")
            }
            lockEndTime = currTime + days*24*3600*1000 //��λ����
        } else {
            lockEndTime = parseInt(lockEndTimeStr)
            if (lockEndTime == NaN) {
                return Error("lock config format error4.")
            }
        }
        
        if (tmAmtMap[lockEndTime] == undefined)
            tmAmtMap[lockEndTime] = lockAmt
        else
            tmAmtMap[lockEndTime] += lockAmt
    }
    
    return null
}

function parseParams_Query(params) {
    var func = params.fcn || params.func

    var body = {
        code : loulan.retCode.OK,
        msg: "OK",
        result: ""
    };

    if (!func) {
        return loulan.paraInvalidMessage("'fcn'")
    }

    var user = params.usr
    var acc = params.acc
    if (!user) {
        return loulan.paraInvalidMessage("'user'")
    }
    if (!acc) {
        return loulan.paraInvalidMessage("'acc'")
    }

    if (params.args) {
        body.code = loulan.retCode.INVLID_PARA
        body.msg = "must no args in params."
        return body
    }
    
    params.args = []
    var args = params.args
    
    args.push(user, acc)
    
    if (func == "getTransInfo"){
        var begSeq = params.bsq;
        if (begSeq == undefined) 
            begSeq = "0"
        
        var count = params.cnt;
        if (count == undefined) 
            count = "-1"  //-1��ʾ��ѯ����
        
        var translvl = params.trsLvl;
        if (translvl == undefined) 
            translvl = "2"
        
        var begTime = params.btm;
        if (begTime == undefined) 
            begTime = "0"

        var endTime = params.etm;
        if (endTime == undefined) 
            endTime = "-1"  //-1��ʾ��ѯ�����µ�ʱ��

        var qAcc = params.qacc;
        if (qAcc == undefined) 
            qAcc = ""

        var maxSeq = params.msq;
        if (maxSeq == undefined) 
            maxSeq = "-1" //������Ĭ��Ϊ-1

        var order = params.ord;
        if (order == undefined) 
            order = "desc" //������Ĭ��Ϊ���򣬼������µ����ݲ���

        args.push(begSeq, count, translvl, begTime, endTime, qAcc, maxSeq, order)
        
    } else if (func == "queryRackAlloc") {
        var rackid = params.rid
        var allocKey = params.ak
        if (allocKey == undefined) 
            allocKey = ""  //��ֵ˵����ѯĳ�εķ������

        var begSeq = params.bsq;
        if (begSeq == undefined) 
            begSeq = "0"
        
        var count = params.cnt;
        if (count == undefined) 
            count = "-1"  //-1��ʾ��ѯ����

        var begTime = params.btm;
        if (begTime == undefined)
            begTime = "0"

        var endTime = params.etm;
        if (endTime == undefined) 
            endTime = "-1"  //-1��ʾ��ѯ�����µ�ʱ��

        var qAcc = params.qacc;
        if (qAcc == undefined) 
            qAcc = ""    //��ֵ˵����ѯĳ���˻��ķ������
        
        args.push(rackid, allocKey, begSeq, count, begTime, endTime, qAcc)
        
    } else if (func == "getRackAllocCfg" || func == "getSESCfg" || func == "getRackFinanceCfg") {
        var rackid = params.rid
        args.push(rackid)
        
    } else if (func == "queryState"){
        var key = params.key
        args.push(key)
    } else if (func == "getRackFinanceProfit") {
        var rackid = params.rid
        args.push(rackid)
    } else if (func == "getRackRestFinanceCapacity") {
        var rackid = params.rid
        var fid = params.fid
        args.push(rackid, fid)
    } else if (func == "getWorldState") {
        params.fcn = "getDataState"
        params.func = "getDataState"
        
        var needHash = params.hash
        if (needHash == undefined) 
            needHash = "0"    //Ĭ�ϲ���hash
        var flushLimit = params.flmt
        if (flushLimit == undefined) 
            flushLimit = "-1"    //Ĭ�ϲ���hash
        args.push(needHash, flushLimit, params.ccname)
    } else if (func == "transPreCheck") {
        var reacc = params.reacc
        var amt = params.amt
        var pwd = params.pwd
        if (pwd == undefined)
            pwd = ""

        args.push(reacc, pwd, amt)

    } else if (func == "getInfoForWeb") {
        args.push("kdcoinpool") //Ŀǰ������ͨ���ҵ��˻�
    }
  
    return null
}

function formatInvokeResult(params, req, body) {
}

function formatQueryResult(params, req, body) {
    var func = params.fcn || params.func
    if (func == "getTransInfo" || func == "getBalanceAndLocked") { //���¼��ֺ�����result����json��ʽ
        body.result = JSON.parse(body.result)
    }
}