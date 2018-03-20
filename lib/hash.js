"use strict";

const crypto = require('crypto');
const util = require('util');
const secp256k1 = require('secp256k1/elliptic');

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
 * aes加密方法  (cbc模式)
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
	var newKey = key.slice(0, bits/8)

	if (iv.length < aes_BlockSize) {
		var err = Error(util.format("aes_encrypt: iv must longer than %d bytes.", aes_BlockSize))
        if (cb)
            return cb(err)
        else 
            return console.log(err.toString())
	}
	var newIv = iv.slice(0, aes_BlockSize)
    
    var algorithm = util.format('aes-%d-cbc', bits)
    
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
 * aes解密方法 (cbc模式)
 * @param key      解密的key
 * @param iv       向量
 * @param crypted  密文 string-base64
 * @returns string-base64
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
	var newKey = key.slice(0, bits/8)
    
	if (iv.length < aes_BlockSize) {
		var err = Error(util.format("aes_decrypt: iv must longer than %d bytes.", aes_BlockSize))
        if (cb)
            return cb(err)
        else
            return console.log(err.toString())
	}
	var newIv = iv.slice(0, aes_BlockSize)
    
    var algorithm = util.format('aes-%d-cbc', bits)

    var decipher = crypto.createDecipheriv(algorithm, newKey, newIv);
    var decoded = decipher.update(crypted, 'base64', 'base64');
    decoded += decipher.final('base64');
    if (cb)
       return cb(null, decoded)
    else
       return decoded
};
exports.aes_decrypt = aes_decrypt;


function createSecp256k1PrivKey(bytes, cb) {
    crypto.randomBytes(bytes, function(err, privKey) {
        if (err)
            return cb(err)
        
        if (secp256k1.privateKeyVerify(privKey)){
            return cb(null, privKey)
        } else {
            console.log('createSecp256k1PrivKey invalid, retry.')
            createSecp256k1PrivKey(bytes, cb)
        }
    })
}


function test(passwd, cb) {
    
    //生成256位salt
    crypto.randomBytes(256, function(err, salt){
        if (err)
            return cb(err)

        
        var hashLen = 48
        var its = 24000

        var salt1 = salt.slice(0,128).toString('base64')
        var salt2 = salt.slice(128).toString('base64')

        console.log('sha512 salt1=', salt1) 
        console.log('sha512 salt2=', salt2) 
       
        crypto.pbkdf2(passwd+salt1, salt2, its, hashLen, 'sha512', function(err, pshash){
            if (err)
                return cb(err)

            console.log('sha512 pshash=', pshash.toString('hex'))        
            
            //生成主密钥
            crypto.randomBytes(48, function(err, mainKey){
                if (err)
                    return cb(err)
                
                console.log('sha512 mainKey=', mainKey.toString('hex'))
                
                
                var mkey = pshash.slice(0,32)   //key 32位
                var  miv = pshash.slice(32)     //iv  16位   所以获取的hash值为48位
                
                console.log('sha512 mkey=', mkey.toString('hex'))        
                console.log('sha512  miv=', miv.toString('hex'))
                
                //加密主密钥
                aes_encrypt(256, mkey, miv, mainKey, function(err, mainKeyEncrypted){
                    if (err)
                        return cb(err)

                    console.log('sha512 mainKeyEncrypted=', new Buffer(mainKeyEncrypted, 'base64').toString('hex'))
                    

                    //生成公私钥
                    createSecp256k1PrivKey(32, function(err, privateKey){
                        if (err)
                            return cb(err)
                        
                        var publicKey = secp256k1.publicKeyCreate(privateKey)
                        
                        console.log('sha512 privateKey=', privateKey.toString('hex'))
                        console.log('sha512 publicKey=', publicKey.toString('hex'))

                        var pkey = mainKey.slice(0,32)
                        var  piv = mainKey.slice(32)
                        console.log('sha512 pkey=', pkey.toString('hex'))        
                        console.log('sha512  piv=', piv.toString('hex'))

                        //用主密钥加密私钥
                        aes_encrypt(256, pkey, piv, privateKey, function(err, privateKeyEncrypted){
                            if (err)
                                return cb(err)
                            
                            console.log('sha512 privateKeyEncrypted=', new Buffer(privateKeyEncrypted,'base64').toString('hex'))

                            var retObj = {}
                            retObj.p0 = '1.0.0'
                            retObj.p1 = salt1
                            retObj.p2 = salt2
                            retObj.p3 = hashLen * 10000
                            retObj.p4 = its * 10
                            retObj.p5 = mainKeyEncrypted
                            retObj.p6 = privateKeyEncrypted
                            retObj.p7 = publicKey.toString('base64')
                            
                            return cb (null, retObj)
                        })
                    })
                }) 
            })
        })
    })
}

function test2(passwd, inObj, cb) {
    crypto.pbkdf2(passwd+inObj.p1, inObj.p2, inObj.p4/10, inObj.p3 / 10000, 'sha512', function(err, pshash){
        if (err)
            return cb(err)

        console.log('test2 pshash=', pshash.toString('hex'))
        
        var mkey = pshash.slice(0,32)
        var  miv = pshash.slice(32)
        
        console.log('test2 mkey=', mkey.toString('hex'))
        console.log('test2  miv=', miv.toString('hex'))
        
        //解密主密钥
        aes_decrypt(256, mkey, miv, inObj.p5, function(err, mainKey){
            if (err)
                return cb(err)
            
            console.log('test2 mainKey=', mainKey)
            mainKey = new Buffer(mainKey, 'base64')
            console.log('test2 mainKey=', mainKey.toString('hex'))
            
            var pkey = mainKey.slice(0,32)
            var  piv = mainKey.slice(32)
            console.log('test2 pkey=', pkey.toString('hex'))        
            console.log('test2  piv=', piv.toString('hex'))

            //用主密钥解密私钥
            aes_decrypt(256, pkey, piv, inObj.p6, function(err, privateKey){
                if (err)
                    return cb(err)
                
                privateKey = new Buffer(privateKey, 'base64')
                console.log('test2 privateKey=%j', privateKey)
                
                var msg = crypto.randomBytes(32)
                const sigObj = secp256k1.sign(msg, privateKey)
                console.log('test2 sigObj=%j', sigObj)
                console.log('test2 msg=%j', msg)
                
                var pubKey = new Buffer(inObj.p7, 'base64')
                console.log('test2 pubKey=%j', pubKey)

                console.log('test2 pubKey=%s, len=%d', pubKey.toString('hex'), pubKey.length)
                console.log('test2 verify result=', secp256k1.verify(msg, sigObj.signature, pubKey))
                
                return cb(null)
            })
        })
        
    })
}

test('nicai', function(err, result){
    if (err) {
        console.log('test failed, err=', err)
        return
    }
    
    console.log('result=', result)
    
    test2('nicai', result, (err)=>{
        if (err) {
            console.log('test2 failed, err=', err)
            return
        }
    })
})

var msg = new Buffer([174, 227, 41, 69, 31, 62, 64, 23, 236, 217, 7, 242, 53, 210, 102, 151, 101, 224, 147, 149, 50, 120, 237, 102, 33, 168, 0, 154, 83, 222, 92, 252])
var pubKey = new Buffer([2, 225, 251, 114, 60, 134, 181, 127, 162, 101, 51, 69, 3, 225, 190, 225, 57, 139, 142, 25, 248, 88, 66, 240, 27, 228, 89, 33, 243, 38, 21, 136, 56])
var signature = new Buffer([113, 176, 81, 228, 238, 8, 60, 22, 168, 5, 120, 55, 138, 216, 20, 62, 168, 113, 139, 46, 50, 80, 195, 166, 160, 96, 48, 200, 203, 205, 250, 60, 11, 120, 249, 16, 20, 102, 233, 15, 128, 3, 236, 153, 109, 88, 227, 18, 58, 144, 33, 61, 137, 126, 202, 39, 156, 217, 80, 90, 5, 19, 228, 61])
console.log('XXXXXXXXX=', secp256k1.verify(msg, signature, pubKey))
console.log('XXXXXXXXX2=%j', secp256k1.recover(msg, signature, 1, true))