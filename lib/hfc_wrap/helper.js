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
logger.setLogLevel(logger.logLevel.DEBUG)


var path = require('path');
var util = require('util');
var fs = require('fs-extra');
var User = require('fabric-client/lib/User.js');
var crypto = require('crypto');
var caClient = require('fabric-ca-client');

var hfc = require('fabric-client');
hfc.setLogger(logger);


var ORGS = hfc.getConfigSetting('network-config');

var clients = {};
var channels = {};
var caClients = {};

function initNetworkTopo () {

    // set up the client and channel objects for each org
    for (let key in ORGS) {
        if (key.indexOf('org') === 0) {
            let client = new hfc();

            let cryptoSuite = hfc.newCryptoSuite();
            cryptoSuite.setCryptoKeyStore(hfc.newCryptoKeyStore({path: getKeyStoreForOrg(ORGS[key].name)}));
            client.setCryptoSuite(cryptoSuite);

            let channel = client.newChannel(hfc.getConfigSetting('channelName'));
            channel.addOrderer(newOrderer());

            clients[key] = client;
            channels[key] = channel;

            setupPeers(channel, key);

            let caUrl = ORGS[key].ca;
            caClients[key] = new caClient(caUrl, null /*defautl TLS opts*/, '' /* default CA */, cryptoSuite);
        }
    }
}


function setupPeers(channel, org) {
    var client = new hfc()
    for (let key in ORGS[org].peers) {
        let data = fs.readFileSync(path.join('', ORGS[org].peers[key]['tls_cacerts']));
        let peer = client.newPeer(
            ORGS[org].peers[key].requests,
            {
                pem: Buffer.from(data).toString(),
                'ssl-target-name-override': ORGS[org].peers[key]['server-hostname']
            }
        );
        peer.setName(key);

        channel.addPeer(peer);
    }
}

function newOrderer() {
    var client = new hfc()
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

function newRemotes(names, forPeers, userOrg) {
    let client = getClientForOrg(userOrg);

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
var getChannelForOrg = function(org) {
    return channels[org];
};

var getClientForOrg = function(org) {
    return clients[org];
};

var newPeers = function(names, org) {
    return newRemotes(names, true, org);
};

var newEventHubs = function(names, org) {
    return newRemotes(names, false, org);
};

var getMspID = function(org) {
    logger.debug('Msp ID : ' + ORGS[org].mspid);
    return ORGS[org].mspid;
};

var getAdminUser = function(userOrg) {
    var users = hfc.getConfigSetting('admins');
    var username = users[0].username;
    var password = users[0].secret;
    var member;
    var client = getClientForOrg(userOrg);

    return hfc.newDefaultKeyValueStore({path: getKeyStoreForOrg(getOrgName(userOrg))})
    .then((store) => {
            client.setStateStore(store);
            return client.getUserContext(username, true)
            .then((user) => {
                if (user && user.isEnrolled()) {
                    return user;
                } else {
                    let caClient = caClients[userOrg];
                    // need to enroll it with CA server
                    return caClient.enroll({
                        enrollmentID: username,
                        enrollmentSecret: password
                    })
                    .then((enrollment) => {
                            logger.info('Successfully enrolled user \'' + username + '\'');
                            member = new User(username);
                            member.setCryptoSuite(client.getCryptoSuite());
                            return member.setEnrollment(enrollment.key, enrollment.certificate, getMspID(userOrg));
                        },
                        (err)=>{  // enroll err
                            return Promise.reject(logger.errorf('getAdminUser: enroll failed, err=%s', err));
                        })
                    .then(() => {
                        return client.setUserContext(member);
                    })
                    .then(() => {
                        return member;
                    });
                }
            },
            (err)=>{  // getUserContext err
                return Promise.reject(logger.errorf('getAdminUser: getUserContext failed, err=%s', err));
            });
        },
        (err)=>{  // newDefaultKeyValueStore err
            return Promise.reject(logger.errorf('getAdminUser: newDefaultKeyValueStore failed, err=%s', err));
        })
    .catch((err) => {
            return Promise.reject(err)
        });
};

/*
    attrs如果没有，可以不输入
*/
var registerAndEnroll = function(username, userOrg, carryCert, attrs) {
    var member;
    var client = getClientForOrg(userOrg);
    var enrollmentSecret = null;
    return hfc.newDefaultKeyValueStore({path: getKeyStoreForOrg(getOrgName(userOrg))})
    .then((store) => {
            logger.debug("registerAndEnroll: user %s's KeyValueStore=%j", username, store);
            client.setStateStore(store);
            return client.getUserContext(username, true)
            .then((user) => {
                if (user && user.isEnrolled()) {
                    return user;
                } else {
                    let caClient = caClients[userOrg];
                    return getAdminUser(userOrg)
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
                                  (err)   =>{ return Promise.reject(logger.errorf('registerAndEnroll: %s failed to register, err=%s', username, err));})
                            
                        },
                        (err)=>{
                            return Promise.reject(logger.errorf('registerAndEnroll: getAdminUser failed, err=%s', err));
                        })
                    .then((secret) => {
                            enrollmentSecret = secret;
                            logger.debug(username + ' registered successfully');
                            return caClient.enroll({enrollmentID: username, enrollmentSecret: secret})
                            .then((enrollMsg)=>{ return enrollMsg},
                                  (err)      =>{ return Promise.reject(logger.errorf('registerAndEnroll: %s failed to enroll, err=%s', username, err));})
                        })
                    .then((message) => {
                            logger.debug(username + ' enrolled successfully');
                            member = new User(username);
                            member._enrollmentSecret = enrollmentSecret;
                            return member.setEnrollment(message.key, message.certificate, getMspID(userOrg));
                        })
                    .then(() => {
                            client.setUserContext(member);
                            return member;
                        })
                }
            },
            (err)=>{  // getUserContext err
                return Promise.reject(logger.errorf('registerAndEnroll: getUserContext failed, err=%s', err));
            });
        },
        (err)=>{  // newDefaultKeyValueStore err
            return Promise.reject(logger.errorf('registerAndEnroll: newDefaultKeyValueStore failed, err=%s', err));
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


var getOrgAdmin = function(userOrg) {
    var admin = ORGS[userOrg].admin;
    var keyPath = path.join('', admin.key);
    var keyPEM = Buffer.from(readAllFiles(keyPath)[0]).toString();
    var certPath = path.join('', admin.cert);
    var certPEM = readAllFiles(certPath)[0].toString();

    var client = getClientForOrg(userOrg);
    var cryptoSuite = hfc.newCryptoSuite();
    if (userOrg) {
        cryptoSuite.setCryptoKeyStore(hfc.newCryptoKeyStore({path: getKeyStoreForOrg(getOrgName(userOrg))}));
        client.setCryptoSuite(cryptoSuite);
    }

    return hfc.newDefaultKeyValueStore({path: getKeyStoreForOrg(getOrgName(userOrg))})
    .then((store) => {
            let username = 'peer'+userOrg+'Admin'
            logger.debug("getOrgAdmin: user %s's KeyValueStore=%j", username, store);
            client.setStateStore(store);
            return client.getUserContext(username, true)
            .then((user) => {
                    logger.debug("getOrgAdmin: getUserContext user.isEnrolled : %s", user ? user.isEnrolled() : false);
                    if (user && user.isEnrolled()) {
                        return user;
                    } else {
                        return client.createUser({
                            username: username,
                            mspid: getMspID(userOrg),
                            cryptoContent: {
                                privateKeyPEM: keyPEM,
                                signedCertPEM: certPEM
                            }
                        })
                        .then((success)=>{ return success}, 
                              (err)    =>{ return Promise.reject(logger.errorf('getOrgAdmin: %s failed to createUser, err=%s', username, err));})
                    }
                },
                (err)=>{
                    return Promise.reject(logger.errorf('getOrgAdmin: getUserContext failed, err=%s', err));
                });
        },
        (err)=>{
            return Promise.reject(logger.errorf('getOrgAdmin: newDefaultKeyValueStore failed, err=%s', err));
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


exports.getChannelForOrg = getChannelForOrg;
exports.getClientForOrg = getClientForOrg;
exports.getLogger = getLogger;
exports.setupChaincodeDeploy = setupChaincodeDeploy;
exports.getMspID = getMspID;
exports.ORGS = ORGS;
exports.newPeers = newPeers;
exports.newEventHubs = newEventHubs;
exports.registerAndEnroll = registerAndEnroll;
exports.getOrgAdmin = getOrgAdmin;
exports.buildTarget = buildTarget;
exports.newOrderer = newOrderer;
exports.initNetworkTopo = initNetworkTopo;