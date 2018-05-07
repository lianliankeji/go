"use strict";

const crypto = require('crypto');
const util = require('util');
const mpath = require('path');
const fs = require('fs');
const common = require('./common');

const secp256k1 = require('secp256k1/elliptic');

const hashLen = 128        //生成哈希的长度
const iterations  = 5000   //迭代次数


var logger = common.createLog("hash.js")
logger.setLogLevel(logger.logLevel.DEBUG)


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

function Hash160(data) {
    return crypto.createHash('ripemd160').update(crypto.createHash('sha256').update(data).digest()).digest()
}
exports.Hash160 = Hash160;

function md5sum(data, encode) {
    if (!encode) {
        encode = 'hex'
    }
    return crypto.createHash('md5').update(data).digest(encode)
}

exports.md5sum = md5sum;

/**
 * aes加密方法  (cfb模式)
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
            return logger.errorf(err.toString())
    }

	if (key.length*8 < bits) {
        var err = Error(util.format("aes_encrypt: key must longer than %d bytes.", bits/8))
        if (cb)
            return cb(err)
        else 
            return logger.errorf(err.toString())
	}
	var newKey = key.slice(0, bits/8)

	if (iv.length < aes_BlockSize) {
		var err = Error(util.format("aes_encrypt: iv must longer than %d bytes.", aes_BlockSize))
        if (cb)
            return cb(err)
        else 
            return logger.errorf(err.toString())
	}
	var newIv = iv.slice(0, aes_BlockSize)
    
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


var ErrAesDecryptError = new Error('aes decrypt error')
/**
 * aes解密方法 (cfb模式)
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
            return logger.errorf(err.toString())
    }

	if (key.length*8 < bits) {
        var err = Error(util.format("aes_decrypt: key must longer than %d bytes.", bits/8))
        if (cb)
            return cb(err)
        else 
            return logger.errorf(err.toString())
	}
	var newKey = key.slice(0, bits/8)
    
	if (iv.length < aes_BlockSize) {
		var err = Error(util.format("aes_decrypt: iv must longer than %d bytes.", aes_BlockSize))
        if (cb)
            return cb(err)
        else
            return logger.errorf(err.toString())
	}
	var newIv = iv.slice(0, aes_BlockSize)
    
    var algorithm = util.format('aes-%d-cfb', bits)

    try{ //解密时，如果key不对会报错，用try捕获
        var decipher = crypto.createDecipheriv(algorithm, newKey, newIv);
        var decoded = decipher.update(crypted, 'base64', 'base64');
        decoded += decipher.final('base64');
        if (cb)
           return cb(null, decoded)
        else
           return decoded
    } catch (err) {
        if (cb)
           return cb(ErrAesDecryptError)
        else
           return logger.errorf(ErrAesDecryptError.toString())
    }
};
exports.aes_decrypt = aes_decrypt;


function createSecp256k1PrivKey(bytes, cb) {
    crypto.randomBytes(bytes, function(err, privKey) {
        if (err)
            return cb(err)
        
        if (secp256k1.privateKeyVerify(privKey)){
            return cb(null, privKey)
        } else {
            logger.info('createSecp256k1PrivKey invalid, retry.')
            createSecp256k1PrivKey(bytes, cb)
        }
    })
}


function __walletMd5sum(wallet) {
    return md5sum(wallet.p0 + 
                  wallet.p1 + 
                  wallet.p2 + 
                  wallet.p3 + 
                  wallet.p4 + 
                  wallet.p5 + 
                  wallet.p6 + 
                  wallet.p7 +
                  wallet.p8, 
                  'base64')
}

function __walletFullPath(user, walletPath) {
    return mpath.join(walletPath, user+'.wallet')
}


var ErrWalletInvalid    = new Error('wallet invalid.')
var ErrWalletExists     = new Error('wallet exists.')
var ErrWalletNotExists  = new Error('wallet not exists.')
var ErrWalletEncrypted  = new Error('wallet has been encrypted.')
var ErrPasswdIsnull     = new Error('passwd is null.')
var ErrPasswdInvalid    = new Error('passwd invalid.')

exports.ErrWalletInvalid = ErrWalletInvalid;
exports.ErrWalletExists = ErrWalletExists;
exports.ErrWalletNotExists = ErrWalletNotExists;
exports.ErrPasswdIsnull = ErrPasswdIsnull;

function creatWallet(user, walletPath, cb) {
    
    var fileName = __walletFullPath(user, walletPath)
    fs.open(fileName, 'wx', (err, fd) => {
        if (err) {
            if (err.code === 'EEXIST') {
                
                //可能出现钱包生成后，开户失败的情况，此时也返回pubkey
                fs.readFile(fileName, (err, data) => {
                    if (err) {
                        if (err.code ==='ENOENT') {
                            return cb(ErrWalletNotExists)
                        }
                        return cb(err)
                    }
                    
                    var wallet
                    
                    try {
                       wallet = JSON.parse(data.toString())
                    } catch (err) {
                        return cb(err)
                    }
                    
                    var md5 = __walletMd5sum(wallet)
                    if (md5 != wallet.p9) {
                        logger.error('user %s wallet invalid.', user)
                        return cb(ErrWalletInvalid)
                    }
                    
                    return cb(null, wallet.p1)
                })
            } else { //因为上面err处理里有异步函数，所以这里一定要加else
                logger.error('open file(%s) error. err=%s',  mpath.basename(fileName), err)
                return cb(err)
            }
        } else { //因为上面err处理里有异步函数，所以这里一定要加else

            //生成公私钥
            createSecp256k1PrivKey(32, function(err, privateKey){
                if (err) {
                    fs.close(fd)
                    fs.unlinkSync(fileName); //出错删除文件
                    return cb(err)
                }

                var publicKey = secp256k1.publicKeyCreate(privateKey)
                
                logger.info('creatWallet privateKey=', privateKey.toString('hex'))
                logger.info('creatWallet publicKey=', publicKey.toString('hex'))
                
                var publicKeyHash = Hash160(publicKey).toString('base64')

                var wallet = {}
                wallet.p0 = '1.0.0'
                wallet.p1 = publicKeyHash  //hash160存储pubkey
                wallet.p2 = privateKey.toString('base64')
                wallet.p3 = 0   //是否加密
                wallet.p4 = ''
                wallet.p5 = ''
                wallet.p6 = 0
                wallet.p7 = 0
                wallet.p8 = ''
                wallet.p9 = __walletMd5sum(wallet)
                
                logger.info('creatWallet wallet=', wallet)
                
                fs.write(fd, JSON.stringify(wallet), function(err, written, string){
                    fs.close(fd)

                    if (err) {
                        logger.error('write file(%s) error. err=%s', mpath.basename(fileName), err)
                        fs.unlinkSync(fileName); //出错删除文件
                        return cb(err)
                    }
                    
                    return cb(null, publicKeyHash)
                })
            })
        }
    })    
}
exports.creatWallet = creatWallet;


function __InnerEncryteWallet(user, passwd, walletPath, decryptedWalletObj, cb) {
    
    if (!passwd) {
        return cb(ErrPasswdIsnull)
    }
    
    var wallet = decryptedWalletObj
    
    //不是解密状态
    if (wallet.p3 != 0) {
        return cb(ErrWalletEncrypted)
    }
    
    //生成256位salt
    crypto.randomBytes(256, function(err, salt){
        if (err)
            return cb(err)

        var hashLen = 48 //must longger than 48
        var its = 16000

        var salt1 = salt.slice(0,128).toString('base64')
        var salt2 = salt.slice(128).toString('base64')

        logger.info('encryteWallet salt1=', salt1) 
        logger.info('encryteWallet salt2=', salt2) 
       
        crypto.pbkdf2(passwd+salt1, salt2, its, hashLen, 'sha512', function(err, pshash){
            if (err)
                return cb(err)

            logger.info('encryteWallet pshash=', pshash.toString('hex'))        
            
            //生成主密钥
            crypto.randomBytes(48, function(err, mainKey){
                if (err)
                    return cb(err)
                
                logger.info('encryteWallet mainKey=', mainKey.toString('hex'))
                
                
                var mkey = pshash.slice(0,32)   //key 32位
                var  miv = pshash.slice(32,48)  //iv  16位   所以获取的hash值为48位
                
                
                logger.info('encryteWallet mkey=', mkey.toString('hex'))        
                logger.info('encryteWallet  miv=', miv.toString('hex'))
                
                //加密主密钥
                aes_encrypt(256, mkey, miv, mainKey, function(err, mainKeyEncrypted){
                    if (err)
                        return cb(err)

                    logger.info('encryteWallet mainKeyEncrypted=', new Buffer(mainKeyEncrypted, 'base64').toString('hex'))
                    
                    var privateKey = new Buffer(wallet.p2, 'base64')

                    logger.info('encryteWallet privateKey=', privateKey.toString('hex'))

                    var pkey = mainKey.slice(0,32)
                    //var  piv = mainKey.slice(32)
                    var publicKeyHash160 = wallet.p1
                    var piv = new Buffer(publicKeyHash160, 'base64').slice(0,16)
                    
                    logger.info('encryteWallet pkey=', pkey.toString('hex'))        
                    logger.info('encryteWallet  piv=', piv.toString('hex'))
                    
                    //用主密钥加密私钥
                    aes_encrypt(256, pkey, piv, privateKey, function(err, privateKeyEncrypted){
                        if (err)
                            return cb(err)
                        
                        logger.info('encryteWallet privateKeyEncrypted=', new Buffer(privateKeyEncrypted,'base64').toString('hex'))

                        //wallet.p0 = '1.0.0'                       //no change, no need update
                        //wallet.p1 = publicKey.toString('base64')  //no change, no need update
                        wallet.p2 = privateKeyEncrypted
                        wallet.p3 = 1   //是否加密
                        wallet.p4 = salt1
                        wallet.p5 = salt2
                        wallet.p6 = hashLen * 10000
                        wallet.p7 = its * 10
                        wallet.p8 = mainKeyEncrypted
                        wallet.p9 = __walletMd5sum(wallet)

                        logger.info('encryteWallet wallet=', wallet)

                        //先写到临时文件，防止写的过程中出错，导致原来文件损坏
                        var walletFullPath = __walletFullPath(user, walletPath)
                        var tmpWalletFullPath = walletFullPath + '.tmp'
                        fs.writeFile(tmpWalletFullPath, JSON.stringify(wallet), (err) => {
                            if (err)
                                return cb(err)

                            fs.rename(tmpWalletFullPath, walletFullPath, (err)=>{
                                if (err)
                                    return cb(err)

                                return cb(null)
                            })
                        })
                    })
                })
            })
        })
    })
}

function encryteWallet(user, passwd, walletPath, cb) {
    
    var walletFullPath = __walletFullPath(user, walletPath)
    
    if (!passwd) {
        return cb(ErrPasswdIsnull)
    }
    
    fs.readFile(walletFullPath, (err, data) => {
        if (err) {
            if (err.code ==='ENOENT') {
                return cb(ErrWalletNotExists)
            }
            return cb(err)
        }
        
        var wallet
        
        try {
           wallet = JSON.parse(data.toString())
        } catch (err) {
            return cb(err)
        }
        
        var md5 = __walletMd5sum(wallet)
        if (md5 != wallet.p9) {
            logger.error('user %s wallet invalid.', user)
            return cb(ErrWalletInvalid)
        }
        
        //已加密
        if (wallet.p3 === 1) {
            return cb(ErrWalletEncrypted)
        }

        __InnerEncryteWallet(user, passwd, walletPath, wallet, (err)=>{
            if (err)
                return cb(err)
            
            return cb(null)
        })
    })
}

exports.encryteWallet = encryteWallet;


function creatWalletAndEncryte(user, passwd, walletPath, cb) {
    if (!passwd) {
        return cb(ErrPasswdIsnull)
    }
    
    creatWallet(user, walletPath, (err, pubKey)=>{
        if (err)
            return cb(err)
        
        encryteWallet(user, passwd, walletPath, (err)=>{
            if (err)
                return cb(err)
            
            return cb(null, pubKey)
        })
    })
}

exports.creatWalletAndEncryte = creatWalletAndEncryte;


function decryptWallet(user, passwd, walletPath, cb) {
    
    var walletFullPath = __walletFullPath(user, walletPath)

    fs.readFile(walletFullPath, (err, data) => {
        if (err) {
            if (err.code ==='ENOENT') {
                return cb(ErrWalletNotExists)
            }
            return cb(err)
        }

        var wallet
        try {
           wallet = JSON.parse(data.toString())
        } catch (err) {
            return cb(err)
        }

        var md5 = __walletMd5sum(wallet)
        if (md5 != wallet.p9) {
            logger.error('user %s wallet invalid.', user)
            return cb(ErrWalletInvalid)
        }
        
       
        //没有加密
        if (wallet.p3 === 0) {
            var privateKey = new Buffer(wallet.p2, 'base64')
            logger.info('decryptWallet privateKey=', privateKey.toString('hex'))
            return cb(null, privateKey, wallet)
        }
        
        //可能没有密码， 所以在这里检查passwd参数
        if (!passwd) {
            return cb(ErrPasswdIsnull)
        }

        var publicKeyHash160 = wallet.p1
        var privateKeyEncrypted = wallet.p2
        var salt1   = wallet.p4
        var salt2   = wallet.p5
        var hashLen = wallet.p6 / 10000
        var its     = wallet.p7 / 10
        var mainKeyEncrypted = wallet.p8
        
        
        crypto.pbkdf2(passwd+salt1, salt2, its, hashLen, 'sha512', function(err, pshash){
            if (err)
                return cb(err)

            logger.info('decryptWallet pshash=', pshash.toString('hex'))
            
            var mkey = pshash.slice(0,32)
            var  miv = pshash.slice(32)
            
            logger.info('decryptWallet mkey=', mkey.toString('hex'))
            logger.info('decryptWallet  miv=', miv.toString('hex'))
            
            //解密主密钥
            aes_decrypt(256, mkey, miv, mainKeyEncrypted, function(err, mainKey){
                if (err) {
                    return cb(ErrPasswdInvalid) //解密时出错，除了参数错误，就是解密的key和iv不对，这里返回密码错误.
                }
                
                logger.info('decryptWallet mainKey=', mainKey)
                mainKey = new Buffer(mainKey, 'base64')
                logger.info('decryptWallet mainKey=', mainKey.toString('hex'))
                
                var pkey = mainKey.slice(0,32)
                //var  piv = mainKey.slice(32)
                var piv = new Buffer(publicKeyHash160, 'base64').slice(0,16)

                logger.info('decryptWallet pkey=', pkey.toString('hex'))        
                logger.info('decryptWallet  piv=', piv.toString('hex'))

                //用主密钥解密私钥
                aes_decrypt(256, pkey, piv, privateKeyEncrypted, function(err, privateKey){
                    if (err) {
                        return cb(ErrPasswdInvalid) //解密时出错，除了参数错误，就是解密的key和iv不对，这里返回密码错误.
                    }
                    
                    privateKey = new Buffer(privateKey, 'base64')
                    logger.info('decryptWallet privateKey=', privateKey.toString('hex'))
                    logger.info('decryptWallet privateKey=%j', privateKey)
                    
                    
                    var publicKey = secp256k1.publicKeyCreate(privateKey)
                    
                    if (Hash160(publicKey).toString('base64') != publicKeyHash160) {
                        logger.info('passwd invalid.')
                        return cb(ErrPasswdInvalid)
                    }
                    
                    logger.info('passwd ok.')
                    

                            //wallet.p0 = '1.0.0'                       //no change, no need update
                            //wallet.p1 = publicKey.toString('base64')  //no change, no need update
                            wallet.p2 = privateKeyEncrypted
                            wallet.p3 = 1   //是否加密
                            wallet.p4 = salt1
                            wallet.p5 = salt2
                            wallet.p6 = hashLen * 10000
                            wallet.p7 = its * 10
                            wallet.p8 = mainKeyEncrypted
                            wallet.p9 = __walletMd5sum(wallet)
                    
                    var decryptdWallet = wallet
                    decryptdWallet.p2 = privateKey
                    decryptdWallet.p3 = 0
                    decryptdWallet.p8 = mainKey
                    decryptdWallet.p9 = __walletMd5sum(wallet)
                        
                    return cb(null, privateKey, decryptdWallet)
                    
                    /*
                    //验证私钥是否正确。即验证密码是否正确
                    crypto.randomBytes(32, function(err, msg){
                        const sigObj = secp256k1.sign(msg, privateKey)
                        logger.info('decryptWallet sigObj=%j', sigObj)
                        logger.info('decryptWallet msg=%j', msg)
                        
                        var pubKey = new Buffer(publicKey, 'base64')
                        logger.info('decryptWallet pubKey=%j', pubKey)

                        logger.info('decryptWallet pubKey=%s, len=%d', pubKey.toString('hex'), pubKey.length)
                        if (!secp256k1.verify(msg, sigObj.signature, pubKey)) {
                            logger.info('passwd invalid.')
                            return cb(ErrPasswdInvalid)
                        }
                        logger.info('passwd ok.')
                        
                        return cb(null, privateKey)
                    })
                    */
                })
            })
        })
    })
}

exports.decryptWallet = decryptWallet;

function changeWalletPassphrase(user, oldpasswd, newpasswd, walletPath, cb) {
    
    //这里只检查新密码是否输入。因为可能没给钱包加密，不用检测oldpasswd，此时也可以修改密码
    if (!newpasswd) {
        return cb(ErrPasswdIsnull)
    }

    decryptWallet(user, oldpasswd, walletPath, (err, privateKey, decryptedWallet)=>{
        if (err)
            return cb(err)
        
        __InnerEncryteWallet(user, newpasswd, walletPath, decryptedWallet, (err)=>{
            if (err)
                return cb(err)
            
            return cb(null)
        })
    })
}

exports.changeWalletPassphrase = changeWalletPassphrase;

//该signature是发送给区块链用的，不是json格式的。
function makeSignatureBuffer(privateKey, msg) {
    var sigObj = secp256k1.sign(msg, privateKey)
    logger.info('makeSignatureBuffer, sigObj=%j', sigObj)
    return Buffer.concat([sigObj.signature, new Buffer([sigObj.recovery])])
}

exports.makeSignatureBuffer = makeSignatureBuffer;

/*
creatWallet('user0001', '/usr/local/llwork/wallet', function(err){
    if (err) {
        logger.info('###########creatWallet failed, err=', err)
        return
    }

    logger.info('###########creatWallet OK')
    
    encryteWallet('user0001', 'nicai', '/usr/local/llwork/wallet', function(err){
        if (err) {
            logger.info('###########encryteWallet failed, err=', err)
            return
        }
        
        logger.info('###########encryteWallet OK')
        
        decryptWallet('user0001', 'nicai', '/usr/local/llwork/wallet', (err, privKey, decryptedWallet)=>{
            if (err) {
                logger.info('###########decryptWallet failed, err=%s', err)
                return
            }
            
            logger.info('###########decryptWallet ok, privKey=', privKey)
            
            changeWalletPassphrase('user0001', 'nicai', 'caigemao', '/usr/local/llwork/wallet', (err)=>{
                if (err) {
                    logger.info('###########changeWalletPassphrase failed, err=%s', err)
                    return
                }
                
                logger.info('###########changeWalletPassphrase ok')
                
                decryptWallet('user0001', 'caigemao', '/usr/local/llwork/wallet', (err)=>{
                    if (err) {
                        logger.info('###########decryptWallet failed, err=%s', err)
                        return
                    }
                    
                    logger.info('###########decryptWallet ok, privKey=', privKey)
                })
            })
        })
    })
})


var map = {'a':1, 'b':2, 'c':3, 'd':4}
var plist=[]
for (let k in map) {
    let v = map[k]
    
    let prom =  new Promise((resolve, reject)=>{
        
        if (k != 'b' && k != 'd') {
            setTimeout(()=>{
                
                    return resolve()
                
            }, 4000)
        } else {
            setTimeout(()=>{

                    return reject(k+v)
                
            }, 1000)
        }
    })
    plist.push(prom)
}

Promise.all(plist)
.then((results)=>{
    console.log('results=', results)
})
.catch((err)=>{
    console.log('err=', err)
})

//*/
