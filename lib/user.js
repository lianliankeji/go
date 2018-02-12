"use strict";

const util = require('util');
const mysql = require('mysql');
const comm = require('./common');
const hash = require('./hash');



var connPool

var log = comm.createLog("user.js")

function _createTableIdx(conn, dbName, tblName, indexName, uniqueOrNot, fieldArr, cb) {
    //mysql不支持 CREATE UNIQUE INDEX IF NOT EXISTS Idx_UserInfo_name ON UserInfo(name)语法，太垃圾了。需要先判断索引是否已存在
    conn.query(
       "SELECT COUNT(TABLE_NAME) as count FROM information_schema.statistics \
        WHERE TABLE_SCHEMA = ?  AND TABLE_NAME = ? AND INDEX_NAME = ?;", [dbName, tblName, indexName],
        function(err, results, fields) {
            if (err) {
                log.error("_createTableIdx query1 err=%s", err)
                return cb(err)
            }
            if (results.length != 1) {
                return cb(log.fatalf("_createTableIdx unexpect error,pls check."))
            } else {
                //如果不存在该索引，创建之；否则直接返回
                if (results[0].count == 0) {
                    var uniqueStr = ""
                    if (uniqueOrNot == true)
                        uniqueStr = "UNIQUE"
                        
                    var sql = util.format("CREATE %s INDEX %s ON %s(%s);COMMIT;", uniqueStr, indexName, tblName, fieldArr.join(','))
                    log.debug("sql=%s",sql)
                    conn.query(sql,  function(err, results, fields) {
                        if (err) {
                            log.error("_createTableIdx query2 err=%s", err)
                            return cb(err)
                        }
                    })
                }
                cb(null)
            }
    });
}

function init(cb) {
    var host = '192.168.10.101'
    var user = 'root'
    var pass = 'root'
    var dbName = 'rack'
    var connTmOut = 3000
    
    var connConfig = {
        host: host,
        user: user,
        password: pass,
        connectTimeout: connTmOut,
        multipleStatements: true
    }
    
    var connection = mysql.createConnection(connConfig);

    connection.connect( function(err) {
        if (err) {
            log.error("Init(connect) err=%s", err)
            return cb(err)
        }
        //先创建db，方便下面的连接池直接连接至目标 database
        connection.query('CREATE DATABASE IF NOT EXISTS ' + dbName + "; COMMIT;", function(err) {
            connection.end();
            if (err){
                log.error("Init(create db) err=%s", err)
                return cb(err)
            }

            if (connPool != undefined) {
                log.info("Init(create connPool) create already.")
                return cb(null)
            }
                
            //创建连接池
            connConfig.database = dbName //添加db参数
            connPool = mysql.createPool(connConfig);
            
            //初始化数据库表等
            connPool.getConnection(function (err, conn) {
                if (err) {
                    log.error("Init(getConnection) err=%s", err)
                    return cb(err)
                }
                conn.query(
                   "CREATE TABLE IF NOT EXISTS UserInfo (\
                        id          INT(10) NOT NULL AUTO_INCREMENT PRIMARY KEY,\
                        name        VARCHAR(64) NOT NULL,\
                        createTime  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP\
                    );\
                    CREATE TABLE IF NOT EXISTS UserShadow (\
                        id      INT(10) NOT NULL PRIMARY KEY,\
                        salt    VARCHAR(512) NOT NULL,\
                        its     INT(10) NOT NULL,\
                        hash    VARCHAR(512) NOT NULL\
                    );\
                    COMMIT;",    
                    function(err, results, fields) {
                        if (err) {
                            log.error("Init(query) err=%s", err)
                            conn.release();
                            return cb(err)
                        }
                        //log.debug("results=%j, fields=%j", results, fields)
                        _createTableIdx(conn, dbName, "UserInfo", "Idx_UserInfo_name", true, ["name"], function(err){
                            conn.release();
                            
                            if (err) {
                                log.error("Init(_createTableIdx) err=%s", err)
                                return cb(err)
                            }
                            cb(null)
                        })
                });
            });
            
        });
    });

}
exports.init = init

function userRegister(name, passwd, cb) {
    if (name == undefined || name == "" || passwd == undefined || passwd == ""){
        return cb(log.errorf("userRegister(args check) err=name or passwd is null"))
    }

    hash.genHash(passwd, function(err, encryt, salt, its) {
        if (err) {
            log.error("userRegister(genHash) err=%s", err)
            return cb(err)
        }

        connPool.getConnection(function (err, conn) {
            if (err) {
                log.error("userRegister(getConnection) err=%s", err)
                return cb(err)
            }
            conn.query(
               "INSERT INTO UserInfo SET name = ?;\
                INSERT INTO UserShadow SET id = (SELECT id FROM UserInfo where name = ?), salt = ?, its=?, hash=?;\
                COMMIT;", [name, name, salt, its, encryt], 
                function(err) {
                    conn.release();

                    if (err) {
                        log.error("userRegister(query) err=%s", err)
                        return cb(err)
                    }
                    cb(null)
                }
            )
        });
    });
}
exports.userRegister = userRegister

function userResetPasswd(name, passwd, cb) {
    if (name == undefined || name == "" || passwd == undefined || passwd == "") {
        return cb(log.errorf("userResetPasswd(args check) err=name or passwd is null"))
    }

    hash.genHash(passwd, function(err, encryt, salt, its) {
        if (err) {
            log.error("userResetPasswd(genHash) err=%s", err)
            return cb(err)
        }

        connPool.getConnection(function (err, conn) {
            if (err) {
                log.error("userResetPasswd(getConnection) err=%s", err)
                return cb(err)
            }
            conn.query(
               "UPDATE UserShadow SET  salt=?, its=?, hash=?\
                WHERE id = (SELECT id FROM UserInfo WHERE name=?);\
                COMMIT;", [salt, its, encryt, name], 
                function(err) {
                    conn.release();

                    if (err) {
                        log.error("userResetPasswd(query) err=%s", err)
                        return cb(err)
                    }
                    cb(null)
            })
        });
    });
}
exports.userResetPasswd = userResetPasswd

function userExists(name, cb) {
    if (name == undefined || name == ""){
        return cb(log.errorf("userExists(args check) err=name is null"))
    }
        
    connPool.getConnection(function (err, conn) {
        if (err) {
            log.error("userExists(getConnection) err=%s", err)
            return cb(err)
        }
        conn.query(
           "SELECT id FROM UserInfo WHERE name = ?", [name], 
            function(err, results, fields) {
                conn.release();

                if (err) {
                    log.error("userExists(query) err=%s", err)
                    return cb(err)
                }
                //log.info("err=%s, results=%j, fields=%j", err, results, fields)
                if (results.length == 0)
                    cb(null, false)
                else if (results.length == 1)
                    cb(null, true, results[0].id)
                else{
                    return cb(log.fatalf("userExists(query) unexpect error, pls check"))
                }
            }
        )
    });
}
exports.userExists = userExists

function userAuth(name, passwd, cb) {
    if (name == undefined || name == "" || passwd == undefined || passwd == ""){
        return cb(log.errorf("userAuth(args check) err=name or passwd is null"))
    }

    connPool.getConnection(function (err, conn) {
        if (err) {
            log.error("userAuth(getConnection) err=%s", err)
            return cb(err)
        }
        conn.query(
           "SELECT hash, salt, its FROM UserShadow WHERE id = (SELECT id FROM UserInfo where name = ?);", [name], 
            function(err, results, fields) {
                conn.release();
                
                if (err) {
                    log.error("userAuth(query) err=%s", err)
                    return cb(err)
                }
                if (results.length == 0) {
                    log.info("userAuth(query) user[%s] not exists.", name)
                    return cb(null, false)
                } else if (results.length == 1) {
                    log.debug("results=%s", results)
                    hash.authenticate(passwd, results[0].salt, results[0].its, results[0].hash,  function(err, ok) {
                        if (err) {
                            log.error("userAuth(authenticate) err=%s", err)
                            return cb(err)
                        }
                        cb (null, ok)
                    });
                } else {
                    
                    return cb(log.fatalf("userAuth(query) unexpect error, pls check."))
                }
            }
        )
    });
}
exports.userAuth = userAuth


function onExit() {
    if (connPool != undefined) {
        connPool.end();  //退出时使用，这里不输入callback
    }
}
exports.onExit = onExit

function __test() {
    Init(function(err){
        if (err) 
            return log.error("init error=%s", err)
        log.info("init ok.")
        
        if (true) { 
            userResetPasswd('LaoWang', '123456', function(err){
                if (err) 
                    return log.error("register error=%s", err) 
                log.info("register OK.") 
                
                onExit()

            })
        } else {
            userExists('LaoWang', function(err, ok) {
                log.info("userExists %s, %s", err,ok)
                
                userAuth("LaoWang", '123456', function(err, ok) {
                    log.info("userAuth %s, %s", err,ok)
                    onExit()
                })
            })

        }
        
    })
}
