"use strict";

const util = require('util');
const fs = require('fs');
const mpath = require('path');
const hfcCrypto = require('hfc/lib/crypto')
const crypto = require('crypto')

function getNowTime() {
    var now = new Date()
    var millis = now.getMilliseconds().toString()
    var millisLen = 3
    if (millis.length < millisLen) {
        millis = "000".substr(0, millisLen - millis.length) + millis
    }
    return util.format("%s-%s.%s", now.toLocaleDateString(), now.toTimeString().substr(0,8),  millis)
}
exports.getNowTime = getNowTime;


function md5sum(str) {
    return crypto.createHash('md5').update(str).digest('hex')
}
exports.md5sum = md5sum;

var Log = (function () {
    
    var __logLevel = {
        DEBUG   : 1,
        INFO    : 2,
        WARNING : 3,
        ERROR   : 4,
        FATAL   : 5,
        MAX     : 99
    };
    
    function Log(module) {
        this.logLevel = __logLevel  //api setLogLevel需要使用这个枚举值做为入参，所以放在属性里

        //这些变量属于私有变量，但是放在这里外部也能访问到。 如何控制使外部不能访问？
        this.__module = module
        this.__defaultLvl = this.logLevel.INFO
    }
    
    //内部函数，不暴露给外部，所以不做为Log的属性
    function __log (lvl, dftLvl, module, str) {
        if (lvl < dftLvl)
            return

        if (str == undefined || str == "")
            return

        var lvlStr

        switch (lvl) {
            case __logLevel.DEBUG:
                lvlStr = "debug"
                break
            case __logLevel.INFO:
                lvlStr = "info"
                break
            case __logLevel.WARNING:
                lvlStr = "warning"
                break
            case __logLevel.ERROR:
                lvlStr = "error"
                break
            case __logLevel.FATAL:
                lvlStr = "fatal"
                break
            default:
                lvlStr = "unknown"
                break
        }
        var header = util.format("%s [%s]%s: ", getNowTime(), module, lvlStr)
        console.log(header + str)
    };
    
   //arguments格式为{"0":xxx,"1":yyy,"2":zzzz,...}
   //如果没有输入参数，直接退出
    Log.prototype.fatal = function() {
        return  __log(this.logLevel.FATAL, this.__defaultLvl, this.__module, util.format.apply(this, arguments))
    };

    Log.prototype.error = function() {
        return  __log(this.logLevel.ERROR, this.__defaultLvl, this.__module, util.format.apply(this, arguments))
    };
    
    Log.prototype.warn = function() {
        return  __log(this.logLevel.WARNING, this.__defaultLvl, this.__module, util.format.apply(this, arguments))
    };

    Log.prototype.info = function() {
        return  __log(this.logLevel.INFO, this.__defaultLvl, this.__module, util.format.apply(this, arguments))
    };

    Log.prototype.debug = function() {
        return  __log(this.logLevel.DEBUG, this.__defaultLvl, this.__module, util.format.apply(this, arguments))
    };

    Log.prototype.errorf = function() {
        var errMsg = util.format.apply(this, arguments)
        __log(this.logLevel.ERROR, this.__defaultLvl, this.__module, errMsg)
        return Error(errMsg)
    };

    Log.prototype.fatalf = function() {
        var errMsg = util.format.apply(this, arguments)
        __log(this.logLevel.FATAL, this.__defaultLvl, this.__module, errMsg)
        return Error(errMsg)
    };
    
    Log.prototype.setLogLevel = function(lvl) {
        this.__defaultLvl = lvl
    };
        
    return Log;
}());

function createLog(moduleName) {
    return new Log(moduleName)
}
exports.createLog = createLog

//日志初始化
var logger = createLog("common")
logger.setLogLevel(logger.logLevel.INFO)
//logger.setLogLevel(logger.logLevel.DEBUG)


/*
   构造TCert桩类型。
   交易签名中，用了TCert.privateKey.getPrivate('hex')
   开户操作中，用了TCert.encode()函数获取cert
   只要桩类型能满足这个调用即可
*/
var Stub_PrivateKey = (function () {
    function Stub_PrivateKey(priv) {
        this.__priv = priv
    };
    
    Stub_PrivateKey.prototype.getPrivate = function(enc) {
        return this.__priv
    }
    
    return Stub_PrivateKey;
}());
var Stub_TCert = (function () {
    function Stub_TCert(priv, cert) {
        this.privateKey = new Stub_PrivateKey(priv)
        this.cert = cert
    };
    
    Stub_TCert.prototype.encode = function() {
        return this.cert
    }

    return Stub_TCert;
}());

const certFilePrefix = "cert."
var certCache = {}

//只能写入一次，因为chaincode中只会记录一次
function putCert(path, user, cert, cb) {
    var fileName = mpath.join(path, certFilePrefix + user)
    
    fs.open(fileName, 'wx', (err, fd) => {
        if (err) {
            if (err.code === 'EEXIST') {
                logger.warn('cert(%s) exists.', user)
                return cb(null)
            }

            logger.error('open file(%s) error. err=%s',  mpath.basename(fileName), err)
            return cb(err)
        }

        var priv = cert.privateKey.getPrivate('hex')
        var objSaved = {}
        objSaved.pk = (new Buffer(priv,'hex')).toString('base64') //存储的priv转换一下，暂时为base64
        objSaved.ct = cert.encode().toString('base64')
        objSaved.md5 = md5sum(objSaved.pk + objSaved.ct)
        
        fs.write(fd, JSON.stringify(objSaved), function(err, written, string){
            fs.close(fd)

            if (err) {
                logger.error('write file(%s) error. err=%s', mpath.basename(fileName), err)
                return cb(err)
            }
            
            var stubCert = new Stub_TCert(priv, cert.encode()) //cache中priv使用hex编码， cert使用原始的cert
            logger.debug("put %s's cert to cache. priv=[%s], cert=[%s]", user, stubCert.privateKey.getPrivate('hex'), stubCert.encode().toString('base64'))

            certCache[user] = stubCert
            cb(null)
        })
    })
    
    /*
    //flag设置为wx，如果文件已存在则报错
    var options = {encoding: 'utf8', mode: 438 , flag: 'wx'}
    fs.writeFile(fileName, JSON.stringify(objSaved), options, function(err){
    })
    */
}
function getCert(path, user, cb) {
    var stubCert = certCache[user]
    if (stubCert) {
        logger.debug("get %s's cert from cache. priv=[%s], cert=[%s]", user, stubCert.privateKey.getPrivate('hex'), stubCert.encode().toString('base64'))
        return cb(null, stubCert)
    }
    
    var fileName = mpath.join(path, certFilePrefix + user)
    fs.readFile(fileName, function(err, data) {
        if (err) {
            return cb(err)
        }

        var obj
        try {
           obj = JSON.parse(data.toString())
        } catch (err) {
            return cb(err)
        }
        //(new hfcCrypto.Crypto("SHA3", 256)).ecdsaKeyFromPrivate(priv, 'hex')
        //校验md5
        var newMd5 = md5sum(obj.pk + obj.ct)
        if (newMd5 != obj.md5) {
            return cb(logger.errorf('file [%s] md5(%s) check failed.', mpath.basename(fileName), newMd5))
        }
        
        var priv = (new Buffer(obj.pk,'base64')).toString('hex') //data为buffer类型，转为hex
        logger.debug("get %s's priv from file. priv=[%s]", user, priv)
        
        var cert = new Buffer(obj.ct,'base64')
        
        var stubCert = new Stub_TCert(priv, cert)
        logger.debug("get %s's cert from file. priv=[%s], cert=[%s]", user, stubCert.privateKey.getPrivate('hex'), stubCert.encode().toString('base64'))

        certCache[user] = stubCert
        cb(null, stubCert)
    })
}
exports.putCert = putCert
exports.getCert = getCert


 /*
function initAccPassCache() {
    if (fs.existsSync(passFile)) {
        logger.info("load passwd.");

        const rdLn = readline.createInterface({
            input: fs.createReadStream(passFile)
        });
        
        var rowCnt = 0;
        rdLn.on('line', function (line) {
            rowCnt++;
            var arr = line.split(delimiter)
            if (arr.length != 2)
                logger.info("line '%s' is invalid in '%s'.", line, passFile);
            else {
                if (!setUserPasswd(arr[0], arr[1]))
                    logger.info("initAccPassCache: set passwd(%s:%s) failed.", arr[0],  arr[1]);
            }
        });
        
        rdLn.on('close', function() {
            logger.info("read %d rows on Init.", rowCnt);
        })
    }
};
*/

