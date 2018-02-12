"use strict";

const crypto = require('crypto');

const hashLen = 128         //生成哈希的长度
const iterations  = 5000   //迭代次数

function genHash(pwd, cb) {
    //salt的长度使用生成的hash的长度
    crypto.randomBytes(hashLen, function(err, salt){ 
       if (err) 
           return cb(err); 
       salt = salt.toString('base64'); 
       crypto.pbkdf2(pwd, salt, iterations, hashLen, 'sha512', function(err, hash){ 
         if (err) 
             return cb(err); 
         cb(null, hash.toString('base64'), salt, iterations); 
       }); 
     }); 
}
exports.genHash = genHash;

function authenticate(pwd, salt, its, hash, cb) {
    crypto.pbkdf2(pwd, salt, its, hashLen, 'sha512', function(err, newHash){ 
         if (err)
             return cb(err); 
         if (newHash.toString('base64') == hash)
             cb(null, true)
         else
             cb(null, false)
    }); 
}
exports.authenticate = authenticate;