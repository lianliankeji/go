const util = require('util');


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
        if (str == undefined || str == "")
            return
        
        if (lvl < dftLvl)
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
        if (arguments["0"] != undefined)
            return  __log(this.logLevel.FATAL, this.__defaultLvl, this.__module, util.format.apply(this, arguments))
    };

    Log.prototype.error = function() {
        if (arguments["0"] != undefined)
            return  __log(this.logLevel.ERROR, this.__defaultLvl, this.__module, util.format.apply(this, arguments))
    };
    
    Log.prototype.warn = function() {
        if (arguments["0"] != undefined)
            return  __log(this.logLevel.WARNING, this.__defaultLvl, this.__module, util.format.apply(this, arguments))
    };

    Log.prototype.info = function() {
        if (arguments["0"] != undefined)
            return  __log(this.logLevel.INFO, this.__defaultLvl, this.__module, util.format.apply(this, arguments))
    };

    Log.prototype.debug = function() {
        if (arguments["0"] != undefined)
            return  __log(this.logLevel.DEBUG, this.__defaultLvl, this.__module, util.format.apply(this, arguments))
    };

    Log.prototype.errorf = function() {
        if (arguments["0"] != undefined){
            var errMsg = util.format.apply(this, arguments)
            __log(this.logLevel.ERROR, this.__defaultLvl, this.__module, errMsg)
            return Error(errMsg)
        }
    };

    Log.prototype.fatalf = function() {
        if (arguments["0"] != undefined){
            var errMsg = util.format.apply(this, arguments)
            __log(this.logLevel.FATAL, this.__defaultLvl, this.__module, errMsg)
            return Error(errMsg)
        }
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


