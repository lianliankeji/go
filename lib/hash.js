"use strict";

const crypto = require('crypto');
const util = require('util');

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


/**
 * aes加密方法
 * @param bits 加密长度
 * @param key 加密key  string
 * @param iv       向量 string
 * @param data     需要加密的数据
 * @returns string-base64
 */
var aes_BlockSize = 16
function aes_encrypt(bits, key, iv, data, cb) {
    if (bits != 128 && bits != 192 && bits != 256) {
        var err = Error('aes_encrypt: bits must be 128 or 192 or 256.')
        if (cb)
            return cb(err)
        else 
            return console.log(err.toString())
    }

	if (key.length*8 < bits) {
        var err = Error(util.format("aes_encrypt: key must longer than %d bytes.", bits/8))
        if (cb)
            return cb(err)
        else 
            return console.log(err.toString())
	}
	var newKey = key.substr(0, bits/8)

    
	if (iv.length < aes_BlockSize) {
		var err = Error(util.format("aes_encrypt: iv must longer than %d bytes.", aes_BlockSize))
        if (cb)
            return cb(err)
        else 
            return console.log(err.toString())
	}
	var newIv = iv.substr(0, aes_BlockSize)
    
    var algorithm = util.format('aes-%d-cfb', bits)
    
    var cipher = crypto.createCipheriv(algorithm, newKey, newIv);
    var crypted = cipher.update(data, 'utf8', 'base64');
    crypted += cipher.final('base64');
    if (cb)
       return cb(null, crypted)
    else 
       return crypted
};
exports.aes_encrypt = aes_encrypt;

/**
 * aes解密方法
 * @param key      解密的key
 * @param iv       向量
 * @param crypted  密文 string-base64
 * @returns string
 */
function aes_decrypt(bits, key, iv, crypted, cb) {
    if (bits != 128 && bits != 192 && bits != 256) {
        var err = Error('aes_decrypt: bits must be 128 or 192 or 256.')
        if (cb)
            return cb(err)
        else 
            return console.log(err.toString())
    }

	if (key.length*8 < bits) {
        var err = Error(util.format("aes_decrypt: key must longer than %d bytes.", bits/8))
        if (cb)
            return cb(err)
        else 
            return console.log(err.toString())
	}
	var newKey = key.substr(0, bits/8)

    
	if (iv.length < aes_BlockSize) {
		var err = Error(util.format("aes_decrypt: iv must longer than %d bytes.", aes_BlockSize))
        if (cb)
            return cb(err)
        else 
            return console.log(err.toString())
	}
	var newIv = iv.substr(0, aes_BlockSize)
    
    var algorithm = util.format('aes-%d-cfb', bits)

    var decipher = crypto.createDecipheriv(algorithm, newKey, newIv);
    var decoded = decipher.update(crypted, 'base64', 'utf8');
    decoded += decipher.final('utf8');
    if (cb)
       return cb(null, decoded)
    else 
       return decoded
};
exports.aes_decrypt = aes_decrypt;
