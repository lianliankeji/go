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

const common = require('../common');
var logger = common.createLog("helper")
logger.setLogLevel(logger.logLevel.INFO)


var path = require('path');
var util = require('util');
var fs = require('fs-extra');
var User = require('fabric-client/lib/User.js');
var crypto = require('crypto');
var caClient = require('fabric-ca-client');

var hfc = require('fabric-client');
//hfc.setLogger(logger);


var ORGS

var caClients = {};

var kvStores = {}

var globalClient = new hfc()

function initNetworkTopo () {
    
    ORGS = hfc.getConfigSetting('network-config');

    var orgList = []
    var kvStorePromises = []
    // set up the client and channel objects for each org
    for (let key in ORGS) {
        if (key.indexOf('org') === 0) {

            let cryptoSuite = hfc.newCryptoSuite();
            cryptoSuite.setCryptoKeyStore(hfc.newCryptoKeyStore({path: getKeyStoreForOrg(ORGS[key].name)}));
            
            let caUrl = ORGS[key].ca;
            caClients[key] = new caClient(caUrl, null /*defautl TLS opts*/, '' /* default CA */, cryptoSuite);
            
            let proms = hfc.newDefaultKeyValueStore({path: getKeyStoreForOrg(getOrgName(key))})
            kvStorePromises.push(proms)
            orgList.push(key)
        }
    }
    
    return Promise.all(kvStorePromises)
    .then((results)=>{
        for (let i in orgList) {
            let org = orgList[i]
            let kvstore = results[i]
            kvStores[org] = kvstore
        }
    })
    .catch((err)=>{
        logger.error("initNetworkTopo err=%s", err)
        return Promise.reject(err)
    })
}


function newOrderer() {
    var client = globalClient
    var caRootsPath = ORGS.orderer.tls_cacerts;
    let data = fs.readFileSync(path.join('', caRootsPath));
    let caroots = Buffer.from(data).toString();
    return client.newOrderer(ORGS.orderer.url, {
        'pem': caroots,
        'ssl-target-name-override': ORGS.orderer['server-hostname']
    });
}

function readAllFiles(dir) {
    var files = fs.readdirSync(dir);
    var certs = [];
    files.forEach((file_name) => {
        let file_path = path.join(dir,file_name);
        let data = fs.readFileSync(file_path);
        certs.push(data);
    });
    return certs;
}

function getOrgName(org) {
    return ORGS[org].name;
}

function getKeyStoreForOrg(org) {
    return hfc.getConfigSetting('keyValueStore') + '_' + org;
}

function newRemotes(names, forPeers, userOrg, client) {

    let targets = [];
    // find the peer that match the names
    for (let idx in names) {
        let peerName = names[idx];
        if (ORGS[userOrg].peers[peerName]) {
            // found a peer matching the name
            let data = fs.readFileSync(path.join('', ORGS[userOrg].peers[peerName]['tls_cacerts']));
            let grpcOpts = {
                pem: Buffer.from(data).toString(),
                'ssl-target-name-override': ORGS[userOrg].peers[peerName]['server-hostname']
            };

            if (forPeers) {
                if (!client)
                    client = globalClient
                targets.push(client.newPeer(ORGS[userOrg].peers[peerName].requests, grpcOpts));
            } else {
                let eh = client.newEventHub();
                eh.setPeerAddr(ORGS[userOrg].peers[peerName].events, grpcOpts);
                targets.push(eh);
            }
        }
    }

    if (targets.length === 0) {
        logger.error(util.format('Failed to find peers matching the names %s', names));
    }

    return targets;
}

//-------------------------------------//
// APIs
//-------------------------------------//

var newPeers = function(names, org) {
    return newRemotes(names, true, org);
};

var newEventHubs = function(names, org, client) {
    return newRemotes(names, false, org, client);
};

var getMspID = function(org) {
    logger.debug('Msp ID : ' + ORGS[org].mspid);
    return ORGS[org].mspid;
};

function getCaAdminClient(username, userOrg) {
    var users = hfc.getConfigSetting('admins');
    var password = users[0].secret;
    var member;
    
    var client = CCCache_getUserClient(username, userOrg)
    if (client){
        logger.debug('#####getCaAdminClient: use cache client.')
        return Promise.resolve(client)
    }
    
    client = newUserClient(userOrg)
    
    return client.getUserContext(username, true)
    .then((user) => {
            if (user && user.isEnrolled()) {
                logger.debug('#####getCaAdminClient: enter getUserContext client.')
                CCCache_setUserClientChan(username, userOrg, client)
                return client
            } else {
                let caClient = caClients[userOrg];
                // need to enroll it with CA server
                return caClient.enroll({
                    enrollmentID: username,
                    enrollmentSecret: password
                })
                .then((enrollment) => {
                        logger.debug('Successfully enrolled user \'' + username + '\'');
                        member = new User(username);
                        member.setCryptoSuite(client.getCryptoSuite());
                        return member.setEnrollment(enrollment.key, enrollment.certificate, getMspID(userOrg));
                    },
                    (err)=>{  // enroll err
                        return Promise.reject(logger.errorf('getCaAdminUser: enroll failed, err=%s', err));
                    })
                .then(() => {
                    return client.setUserContext(member);
                })
                .then(() => {
                    logger.debug('#####getCaAdminClient: enter caClient.enroll.')
                    CCCache_setUserClientChan(username, userOrg, client)
                    return client;
                });
            }
        },
        (err)=>{  // getUserContext err
            return Promise.reject(logger.errorf('getCaAdminClient: getUserContext failed, err=%s', err));
        })
    .catch((err)=>{
        return Promise.reject(err)
    })
}

var getCaAdminUser = function(userOrg) {
    var users = hfc.getConfigSetting('admins');
    var username = users[0].username;

    return getCaAdminClient(username, userOrg)
    .then((client) => {
            var user = client.getUserContext(username)
            return user
        },
        (err)=>{  // getUserContext err
            return Promise.reject(logger.errorf('getCaAdminUser: getCaAdminClient failed, err=%s', err));
        })
    .catch((err) => {
        return Promise.reject(err)
    });
};


function newUserRegisterAndEnroll(username, userOrg, attrs) {
    var member
    var enrollmentSecret = null;   
    var caClient = caClients[userOrg];
    
    return getCaAdminUser(userOrg)
    .then((adminUserObj)=> {
            member = adminUserObj;
            let regReq = {
                enrollmentID: username,
                //enrollmentSecret: xxxx,   不指定secret，由系统自动生成
                role: 'client',
                attrs: attrs,
                affiliation: userOrg + '.department1'
            }
            return caClient.register(regReq, member)
            .then((secret)=>{ return secret}, 
                  (err)   =>{ return Promise.reject(logger.errorf('newUserRegisterAndEnroll: %s failed to register, err=%s', username, err));})
            
        },
        (err)=>{
            return Promise.reject(logger.errorf('newUserRegisterAndEnroll: getCaAdminUser failed, err=%s', err));
        })
    .then((secret) => {
            enrollmentSecret = secret;
            logger.debug(username + ' registered successfully');
            return caClient.enroll({enrollmentID: username, enrollmentSecret: secret})
            .then((enrollMsg)=>{ return enrollMsg},
                  (err)      =>{ return Promise.reject(logger.errorf('newUserRegisterAndEnroll: %s failed to enroll, err=%s', username, err));})
        })
    .then((message) => {
            logger.debug(username + ' enrolled successfully');
            member = new User(username);
            member._enrollmentSecret = enrollmentSecret;
            return member.setEnrollment(message.key, message.certificate, getMspID(userOrg));
        })
    .then(() => {
            return member;
        })
    .catch((err)=>{
        return Promise.reject(err)
    })
    
}

function newUserClient(org) {
    //这里要用new新建一个， 每个user一个client
    var client = new hfc();

    var cryptoSuite = hfc.newCryptoSuite();
    cryptoSuite.setCryptoKeyStore(hfc.newCryptoKeyStore({path: getKeyStoreForOrg(getOrgName(org))}));
    client.setCryptoSuite(cryptoSuite);
    
    client.setStateStore(kvStores[org]);
    
    return client
}

// return Promise(obj). obj={'client':client, 'chan': channel}
function getUserClientAndChannel(username, userOrg, newUserWhenNone) {
    
    var obj = CCCache_getUserClientChan(username, userOrg)
    if (obj){
        logger.debug('#####getUserClientAndChannel: use cache client and chan.')
        return Promise.resolve(obj)
    }
    
    return getUserClient(username, userOrg, newUserWhenNone)
    .then(()=>{
        return CCCache_getUserClientChan(username, userOrg)
    })
    .catch((err)=>{
        return Promise.reject(err)
    })
}

function getUserClient(username, userOrg, newUserWhenNone, attrs) {
    
    var client = CCCache_getUserClient(username, userOrg)
    if (client){
        logger.debug('#####getUserClient: use cache client.')
        return Promise.resolve(client)
    }
    
    client = newUserClient(userOrg)
    
    return client.getUserContext(username, true)
    .then((user) => {
            if (user && user.isEnrolled()) {
                logger.debug('#####getUserClient: enter getUserContext.')
                CCCache_setUserClientChan(username, userOrg, client);
                return client
            } else if (newUserWhenNone === true) {
                return newUserRegisterAndEnroll(username, userOrg, attrs)
                .then((member) => {
                        return client.setUserContext(member)
                        .then(()=>{
                            logger.debug('#####getUserClient: enter newUserRegisterAndEnroll.')
                            CCCache_setUserClientChan(username, userOrg, client);
                            return client
                        })
                    },
                    (err)=>{
                        return Promise.reject(logger.errorf('getUserClient: newUserRegisterAndEnroll failed, err=%s', err));
                    })
            } else {
                return Promise.reject(logger.errorf('getUserClient: getUserContext get usr nil'));
            }
        },
        (err)=>{  // getUserContext err
            return Promise.reject(logger.errorf('getUserClient: getUserContext failed, err=%s', err));
        })
    .catch((err)=>{
        return Promise.reject(err)
    })
}

var registerAndEnroll = function(username, userOrg, carryCert, attrs) {
    return getUserClient(username, userOrg, true, attrs)
    .then((client) => {
            var user = client.getUserContext(username)
            return user
        },
        (err)=>{  
            return Promise.reject(logger.errorf('registerAndEnroll: getUserClient failed, err=%s', err));
        })
    .then((user) => {
            var response = {
                success: true,
                secret: user._enrollmentSecret,
                message: username + ' enrolled Successfully',
            };
            
            if (carryCert && carryCert === true) {
                response.cert = user._identity._certificate
            }

            return response;
        })
    .catch((err) => {
            return Promise.reject(err)
        });
};

function getUserCert(username, userOrg, newUserWhenNone) {
    return getUserClient(username, userOrg, newUserWhenNone)
    .then((client) => {
            var user = client.getUserContext(username)
            return user._identity._certificate
        },
        (err)=>{  
            return Promise.reject(logger.errorf('getUserCert: getUserClient failed, err=%s', err));
        })
    .catch((err) => {
            return Promise.reject(err)
        });
}


function getOrgAdminUserName(userOrg){
    return 'peer'+userOrg+'Admin'
}
function getOrgAdminClient(userOrg) {
    
    let username = getOrgAdminUserName(userOrg)
    
    var client = CCCache_getUserClient(username, userOrg)
    if (client){
        logger.debug('#####getOrgAdminClient: use cache client.')
        return Promise.resolve(client)
    }
    
    client = newUserClient(userOrg)

    return client.getUserContext(username, true)
    .then((user) => {

        if (user && user.isEnrolled()) {
                logger.debug('#####getOrgAdminClient: enter getUserContext user.')
                CCCache_setUserClientChan(username, userOrg, client);
                return client
            } else {
                
                logger.debug('#####getOrgAdminClient: 1.')
                var admin = ORGS[userOrg].admin;
                var keyPath = path.join('', admin.key);
                var keyPEM = Buffer.from(readAllFiles(keyPath)[0]).toString();
                var certPath = path.join('', admin.cert);
                var certPEM = readAllFiles(certPath)[0].toString();

                logger.debug('#####getOrgAdminClient: 2.')
                
                return client.createUser({
                    username: username,
                    mspid: getMspID(userOrg),
                    cryptoContent: {
                        privateKeyPEM: keyPEM,
                        signedCertPEM: certPEM
                    }
                })
                .then(()=>{
                        logger.debug('#####getOrgAdminClient: enter create user.')
                        CCCache_setUserClientChan(username, userOrg, client);
                        return client 
                    }, 
                    (err)=>{ return Promise.reject(logger.errorf('getOrgAdminClient: %s failed to createUser, err=%s', username, err));})
            }
        },
        (err)=>{  // getUserContext err
            return Promise.reject(logger.errorf('getOrgAdminClient: getUserContext failed, err=%s', err));
        })
    .catch((err)=>{
        return Promise.reject(err)
    })
}

function getOrgAdminClientAndChannel(userOrg) {
    let username = getOrgAdminUserName(userOrg)
    
    var obj = CCCache_getUserClientChan(username, userOrg)
    if (obj){
        logger.debug('#####getOrgAdminClientAndChannel: use cache client and chan.')
        return Promise.resolve(obj)
    }
    
    return getOrgAdminClient(userOrg)
    .then(()=>{
        return CCCache_getUserClientChan(username, userOrg)
    })
    .catch((err)=>{
        return Promise.reject(err)
    })
}


var getOrgAdmin = function(userOrg) {

    return getOrgAdminClient(userOrg)
    .then((client) => {
            var user = client.getUserContext()
            return user
        },
        (err)=>{
            return Promise.reject(logger.errorf('getOrgAdmin: getUserContext failed, err=%s', err));
        })
    .catch((err)=>{
        return Promise.reject(err);
    })
};

var setupChaincodeDeploy = function() {
    process.env.GOPATH = path.join('', hfc.getConfigSetting('CC_SRC_PATH'));
};

var getLogger = function(moduleName) {
    var logger = log4js.getLogger(moduleName);
    logger.setLevel('DEBUG');
    return logger;
};


var buildTarget = function(peer, org) {
    var target = null;
    if (typeof peer !== 'undefined') {
        let targets = newPeers([peer], org);
        if (targets && targets.length > 0) target = targets[0];
    }

    return target;
}


exports.getUserClientAndChannel = getUserClientAndChannel;
exports.getUserClient = getUserClient;
exports.getLogger = getLogger;
exports.setupChaincodeDeploy = setupChaincodeDeploy;
exports.getMspID = getMspID;
exports.ORGS = ORGS;
exports.newPeers = newPeers;
exports.newEventHubs = newEventHubs;
exports.registerAndEnroll = registerAndEnroll;
exports.getOrgAdmin = getOrgAdmin;
exports.getOrgAdminClient = getOrgAdminClient;
exports.getOrgAdminClientAndChannel = getOrgAdminClientAndChannel;
exports.buildTarget = buildTarget;
exports.newOrderer = newOrderer;
exports.initNetworkTopo = initNetworkTopo;
exports.getUserCert = getUserCert;


/*******************************************************************/
/****************************   client cache  **********************/
/*******************************************************************/
//后续用户数量多时，考虑改为LRU cache
var userClientChanCache = {}
var cacheSize = 0

function CCCache_getCacheKey(username, userOrg) {
    return username + '_' + userOrg
}


function CCCache_getUserClientChan(username, userOrg) {
    return userClientChanCache[CCCache_getCacheKey(username, userOrg)]
}

function CCCache_getUserClient(username, userOrg) {
    var obj = CCCache_getUserClientChan(username, userOrg)
    if (obj)
        return obj.client
    
    return null
}

function CCCache_setUserClientChan(username, userOrg, client) {
    var channel = client.newChannel(hfc.getConfigSetting('channelName'));
    channel.addOrderer(newOrderer());
    
    userClientChanCache[CCCache_getCacheKey(username, userOrg)] = {'client': client, 'chan': channel}
    cacheSize++
    if (cacheSize > 1000)
        logger.info("CCCache size:", cacheSize)
}
/*******************************************************************/
/****************************   client cache  **********************/
/*******************************************************************/



function test() {
    require('/usr/local/llwork/KuaiDian1.0/config.js');
    
    return initNetworkTopo()
    .then(()=>{
        
        return registerAndEnroll("testUser1", 'org1', true)
        .then((ok)=>{
            logger.info("test: registerAndEnroll OK:%j", ok)
            
            return getOrgAdmin('org1')
            .then((ok)=>{
                logger.info("test: getOrgAdmin OK:%j", ok)
            })
        })
        
        
    })
    .catch((err)=>{
        return Promise.reject(logger.errorf("test: error, err=%s", err))
    })
}
/*
test().then((ok)=>{
    logger.info("test OK:%j", ok)
},
(err)=>{
    logger.error("test error, err=%s", err)
})
*/