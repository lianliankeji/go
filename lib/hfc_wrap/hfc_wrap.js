/**
 * Copyright 2017 IBM All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */
 
const common = require('../common');
var logger = common.createLog("hfc_wrap")
logger.setLogLevel(logger.logLevel.DEBUG)

var path = require('path');
var fs = require('fs');
var util = require('util');
var hfc = require('fabric-client');
hfc.setLogger(logger);
var Peer = require('fabric-client/lib/Peer.js');
var EventHub = require('fabric-client/lib/EventHub.js');
var BlockDecoder = require('fabric-client/lib/BlockDecoder.js');
var helper = require('./helper.js');



var ORGS = hfc.getConfigSetting('network-config');

/*****************************************************************************************/
/********************************  Query  ************************************************/
/*****************************************************************************************/
var queryChaincode = function(peer, channelName, chaincodeName, args, fcn, username, org) {
    var channel = helper.getChannelForOrg(org);
    var client = helper.getClientForOrg(org);
    var target = peer ? helper.buildTarget(peer, org) : undefined

    return helper.registerAndEnroll(username, org)
    .then((user) => {
            // send query
            var request = {
                chaincodeId: chaincodeName,
                targets: target,
                fcn: fcn,
                args: args
            };
            return channel.queryByChaincode(request)
            .then((success) => { return success; }, 
                  (err)     => { return Promise.reject(logger.errorf('channel.queryByChaincode failed, err=%s', err)); })
        }, 
        (err) => {
            return Promise.reject(logger.errorf('Failed to get submitter %s', username))
        })
    .then((response_payloads) => {
            logger.debug('queryChaincode response_payloads =',response_payloads)
            if (response_payloads) {
                //只给一个peer发送查询，如果返回多个，这里记录一下
                if (response_payloads.length > 1) {
                    logger.warn("queryChaincode retun more than one response(%d).", response_payloads.length)
                    for (let i = 0; i < response_payloads.length; i++) {
                        logger.debug('query result is : %s on peer %d', response_payloads[i].toString('utf8'), i);
                    }
                }
                
                //channel.queryByChaincode会返回错误信息，但是没有reject，这里处理一下
                let qResult = response_payloads[0]
                if (qResult instanceof Error) {
                    return Promise.reject(qResult);
                } 
                
                let response = {
                    success: true,
                    message: 'Chaincode query is SUCCESS on ' + peer,
                    result: response_payloads[0].toString('utf8')
                };
                return response;
                
            } else {
                return Promise.reject(logger.errorf('queryChaincode response_payloads is null'));
            }
        })
    .catch((err) => {
        return Promise.reject(err);
    });
};

var getBlockByNumber = function(peer, blockNumber, username, org) {
    var target = peer ? helper.buildTarget(peer, org) : undefined;
    var channel = helper.getChannelForOrg(org);

    return helper.registerAndEnroll(username, org)
    .then((member) => {
            return channel.queryBlock(parseInt(blockNumber), target)
            .then((success) => { return success; },
                  (err)     => { return Promise.reject(logger.errorf('channel.queryBlock failed, err=%s', err)); })
        }, 
        (err) => {
            return Promise.reject(logger.errorf('getBlockByNumber: Failed to get submitter %s, err=%s', username, err));
    })
    .then((response_payloads) => {
            if (response_payloads) {
                logger.debug('getBlockByNumber: response_payloads=', response_payloads);
                return response_payloads;
            } else {
                return Promise.reject(logger.errorf('getBlockByNumber: response_payloads is null'))
            }
        })
    .catch((err) => {
        return Promise.reject(err)
    });
};

var getBlockByHash = function(peer, hash, username, org) {
    var target = peer ? helper.buildTarget(peer, org) : undefined;
    var channel = helper.getChannelForOrg(org);

    return helper.registerAndEnroll(username, org)
    .then((member) => {
            return channel.queryBlockByHash(Buffer.from(hash), target)
            .then((success) => { return success; },
                  (err)     => { return Promise.reject(logger.errorf('channel.queryBlockByHash failed, err=%s', err)); })
        }, 
        (err) => {
            return Promise.reject(logger.errorf('getBlockByHash: Failed to get submitter %s, err=%s', username, err));
        })
    .then((response_payloads) => {
            if (response_payloads) {
                logger.debug('queryBlockByHash: response_payloads=', response_payloads);
                return response_payloads;
            } else {
                return Promise.reject(logger.errorf('queryBlockByHash: response_block is null'))
            }
        })
    .catch((err) => {
        return Promise.reject(err)
    });
};

var getTransactionByID = function(peer, trxnID, username, org) {
    var target = peer ? helper.buildTarget(peer, org) : undefined;
    var channel = helper.getChannelForOrg(org);

    return helper.registerAndEnroll(username, org)
    .then((member) => {
            return channel.queryTransaction(trxnID, target)
            .then((success) => { return success; },
                  (err)     => { return Promise.reject(logger.errorf('channel.queryTransaction failed, err=%s', err)); })
        }, 
        (err) => {
            return Promise.reject(logger.errorf('getTransactionByID: Failed to get submitter %s, err=%s', username, err));
        })
    .then((response_payloads) => {
            if (response_payloads) {
                logger.debug('getTransactionByID: response_payloads=', response_payloads);
                return response_payloads;
            } else {
                return Promise.reject(logger.errorf('getTransactionByID: response_block is null'))
            }
        })
    .catch((err) => {
        return Promise.reject(err)
    });
};

var getChainInfo = function(peer, username, org) {
    var target = peer ? helper.buildTarget(peer, org) : undefined;
    var channel = helper.getChannelForOrg(org);

    return helper.registerAndEnroll(username, org)
    .then((member) => {
            return channel.queryInfo(target)
            .then((success) => { return success; },
                  (err)     => { return Promise.reject(logger.errorf('getChainInfo: channel.queryInfo failed, err=%s', err)); })
        }, 
        (err) => {
            return Promise.reject(logger.errorf('getChainInfo: Failed to get submitter %s, err=%s', username, err));
        })
    .then((blockchainInfo) => {
            if (blockchainInfo) {
                logger.debug('getChainInfo: blockchainInfo=', blockchainInfo);
                return blockchainInfo;
            } else {
                return Promise.reject(logger.errorf('getChainInfo: response_block is null'))
            }
        })
    .catch((err) => {
        return Promise.reject(err)
    });
};

//getInstalledChaincodes
var getChaincodes = function(peer, type, username, org) {
    var target = peer ? helper.buildTarget(peer, org) : undefined;
    var channel = helper.getChannelForOrg(org);
    var client = helper.getClientForOrg(org);

    return helper.getOrgAdmin(org)
    .then((member) => {
            if (type === 'installed') {
                return client.queryInstalledChaincodes(target)
                .then((success) => { return success; },
                      (err)     => { return Promise.reject(logger.errorf('getChaincodes: client.queryInstalledChaincodes failed, err=%s', err)); })
            } else {
                return channel.queryInstantiatedChaincodes(target)
                .then((success) => { return success; },
                      (err)     => { return Promise.reject(logger.errorf('getChaincodes: channel.queryInstantiatedChaincodes failed, err=%s', err)); })
            }
        }, 
        (err) => {
            return Promise.reject(logger.errorf('getChainInfo(%s): Failed to get submitter %s, err=%s', type, username, err));
        })
    .then((response) => {
            if (response) {
                if (type === 'installed') {
                    logger.debug('<<< Installed Chaincodes >>>');
                } else {
                    logger.debug('<<< Instantiated Chaincodes >>>');
                }
                logger.debug('getChaincodes response=', response);
                /*
                var details = [];
                for (let i = 0; i < response.chaincodes.length; i++) {
                    details.push('name: ' + response.chaincodes[i].name + ', version: ' + response.chaincodes[i].version + ', path: ' + response.chaincodes[i].path);
                }
                */
                return response;
            } else {
                return Promise.reject(logger.errorf('getChaincodes(%s): response_block is null', type))
            }
        })
    .catch((err) => {
        return Promise.reject(err)
    });
};

var getChannels = function(peer, username, org) {
    var target = peer ? helper.buildTarget(peer, org) : undefined;
    var channel = helper.getChannelForOrg(org);
    var client = helper.getClientForOrg(org);

    return helper.registerAndEnroll(username, org)
    .then((member) => {
            return client.queryChannels(target)
            .then((success) => { return success; },
                  (err)     => { return Promise.reject(logger.errorf('getChannels: client.queryChannels failed, err=%s', err)); })
        }, 
        (err) => {
            return Promise.reject(logger.errorf('getChannels: Failed to get submitter %s, err=%s', username, err));
        })
    .then((response) => {
            if (response) {
                logger.debug('<<< channels >>>, response=', response);
                /*
                var channelNames = [];
                for (let i = 0; i < response.channels.length; i++) {
                    channelNames.push('channel id: ' + response.channels[i].channel_id);
                }
                logger.debug(channelNames);
                */
                return response;
            } else {
                return Promise.reject(logger.errorf('getChannels: response is null'))
            }
        })
    .catch((err) => {
        return Promise.reject(err)
    });
};

exports.queryChaincode = queryChaincode;
exports.getBlockByNumber = getBlockByNumber;
exports.getTransactionByID = getTransactionByID;
exports.getBlockByHash = getBlockByHash;
exports.getChainInfo = getChainInfo;
exports.getChaincodes = getChaincodes;
exports.getChannels = getChannels;


/*****************************************************************************************/
/********************************  Invoke  ***********************************************/
/*****************************************************************************************/
var invokeChaincode = function(peerNames, channelName, chaincodeName, fcn, args, username, org, eventHubTimeout) {
    logger.debug(util.format('\n============ invoke transaction on organization %s ============\n', org));
    
    if (!eventHubTimeout) 
        eventHubTimeout = 30000
    
    var client = helper.getClientForOrg(org);
    var channel = helper.getChannelForOrg(org);
    var targets = (peerNames) ? helper.newPeers(peerNames, org) : undefined;
    var tx_id = null;

    return helper.registerAndEnroll(username, org)
    .then((user) => {
            tx_id = client.newTransactionID();
            logger.debug(util.format('Sending transaction "%j"', tx_id));
            // send proposal to endorser
            var request = {
                chaincodeId: chaincodeName,
                fcn: fcn,
                args: args,
                chainId: channelName,
                txId: tx_id
            };

            if (targets)
                request.targets = targets;

            return channel.sendTransactionProposal(request)
            .then((success) => { return success; },
                  (err)     => { return Promise.reject(logger.errorf('channel.sendTransactionProposal failed, err=%s', err)); })
        },
        (err) => {
            return Promise.reject(logger.errorf('invokeChaincode: Failed to enroll user %s, err=%s', username, err))
        })
    .then((results) => {
        var proposalResponses = results[0];
        var proposal = results[1];
        var all_good = true;
        for (var i in proposalResponses) {
            let one_good = false;
            if (proposalResponses && proposalResponses[i].response && proposalResponses[i].response.status === 200) {
                one_good = true;
                logger.debug('transaction proposal was good');
            } else {
                logger.error('invokeChaincode: channel.sendInstantiateProposal proposalResponses err: %s', proposalResponses[i].details); //不要删除这个日志
            }
            all_good = all_good & one_good;
        }
        if (all_good) {
            logger.debug(util.format(
                'Successfully sent Proposal and received ProposalResponse: Status - %s, message - "%s", metadata - "%s", endorsement signature: %s',
                proposalResponses[0].response.status, proposalResponses[0].response.message,
                proposalResponses[0].response.payload, proposalResponses[0].endorsement
                .signature));
            var request = {
                proposalResponses: proposalResponses,
                proposal: proposal
            };
            // set the transaction listener and set a timeout of 30sec
            // if the transaction did not get committed within the timeout period,
            // fail the test
            var transactionID = tx_id.getTransactionID();
            var eventPromises = [];

            if (!peerNames) {
                peerNames = channel.getPeers().map(function(peer) {
                    return peer.getName();
                });
            }

            var eventhubs = helper.newEventHubs(peerNames, org);
            for (let key in eventhubs) {
                let eh = eventhubs[key];
                eh.connect();

                let txPromise = new Promise((resolve, reject) => {
                    let handle = setTimeout(() => {
                        eh.disconnect();
                        reject(logger.errorf('invokeChaincodeThe: TxEvent timeout, peer=%s', eh.getPeerAddr()));
                    }, eventHubTimeout);

                    eh.registerTxEvent(transactionID, (tx, code) => {
                        clearTimeout(handle);
                        eh.unregisterTxEvent(transactionID);
                        eh.disconnect();

                        if (code !== 'VALID') {
                            reject(logger.errorf('invokeChaincodeThe: The transaction was invalid, code =%s, peer=%s', code, eh.getPeerAddr()));
                        } else {
                            logger.info('The transaction has been committed on peer ' + eh.getPeerAddr());
                            resolve();
                        }
                    });
                });
                eventPromises.push(txPromise);
            };
            var sendPromise = channel.sendTransaction(request);
            return Promise.all([sendPromise].concat(eventPromises));
        } else {
            return Promise.reject(logger.errorf('invokeChaincodeThe: Failed to send Proposal or receive valid response. Response null or status is not 200'));
        }
    })
    .then((result) => {
        var resp = result[0]
        if (resp.status === 'SUCCESS') {
            logger.info('Successfully sent transaction to the orderer.');
            let response = {
                success: true,
                message: 'Successfully sent transaction to the orderer.',
                result: tx_id.getTransactionID()
            };
            return response
        } else {
            return Promise.reject(logger.errorf('invokeChaincodeThe: Failed to order the transaction. response: %s', resp));
        }
    })
    .catch((err) => {
        return Promise.reject(err); //上面的err已经有错误信息了， 这里不需要再组装一次错误原因，直接返回
    });
};

exports.invokeChaincode = invokeChaincode;


/*****************************************************************************************/
/********************************  Chaincode *********************************************/
/*****************************************************************************************/
var installChaincode = function(peers, chaincodeName, chaincodePath, chaincodeVersion, username, org) {
    logger.debug(
        '\n============ Install chaincode on organizations ============\n');
    helper.setupChaincodeDeploy();
    var channel = helper.getChannelForOrg(org);
    var client = helper.getClientForOrg(org);

    return helper.getOrgAdmin(org)
    .then((user) => {
            var request = {
                targets: helper.newPeers(peers, org),
                chaincodePath: chaincodePath,
                chaincodeId: chaincodeName,
                chaincodeVersion: chaincodeVersion
            };
            return client.installChaincode(request)
            .then((results)=>{
                    return results;
                }, (err)=>{
                    return Promise.reject(logger.errorf('client.installChaincode failed, err=%s ', err))
                })
        },
        (err) => {
            return Promise.reject(logger.errorf('Failed to enroll admin. err=%s ', err))
        })
    .then((results) => {
        logger.debug('installChaincode results=', results)
        var proposalResponses = results[0];
        var proposal = results[1];
        var all_good = true;
        for (var i in proposalResponses) {
            let one_good = false;
            if (proposalResponses && proposalResponses[i].response && proposalResponses[i].response.status === 200) {
                one_good = true;
                logger.debug('install proposal was good');
            } else {
                logger.error('client.installChaincode proposalResponses err: %s', proposalResponses[i].details); //不要删除这个日志
            }
            all_good = all_good & one_good;
        }
        if (all_good) {
            logger.debug('Successfully sent install Proposal and received ProposalResponse on org %s, Status - %s', org, proposalResponses[0].response.status);
            let response = {
                success: true,
                message: 'Successfully Installed chaincode on organization ' + org
            };
            return response;
        } else {
            return Promise.reject(logger.errorf('Failed to send install Proposal or receive valid response. Response null or status is not 200.'));
        }
    })
    .catch((err) => {
        return Promise.reject(err);
    });
};
exports.installChaincode = installChaincode;

var __instantiateOrUpdateChaincode = function(type, channelName, chaincodeName, chaincodeVersion, functionName, args, username, org, eventHubTimeout) {
    logger.debug('\n============ instantiateOrUpdate chaincode on organization ' + org + ' ============\n');
    
    if (!eventHubTimeout)
        eventHubTimeout = 55000

    var channel = helper.getChannelForOrg(org);
    var client = helper.getClientForOrg(org);
    var tx_id = null;
    var eh = null;

    return helper.getOrgAdmin(org)
    .then((user) => {
            // read the config block from the orderer for the channel
            // and initialize the verify MSPs based on the participating
            // organizations
            return channel.initialize()
            .then((success) => { return success },
                  (err)     => { return Promise.reject(logger.errorf('channel.initialize failed, err=%s', err)) })
        },
        (err) => {
            return Promise.reject(logger.errorf('Failed to enroll admin. err=%s', err))
        })
    .then((success) => {
        tx_id = client.newTransactionID();
        // send proposal to endorser
        var request = {
            chaincodeId: chaincodeName,
            chaincodeVersion: chaincodeVersion,
            args: args,
            txId: tx_id
        };

        if (functionName)
            request.fcn = functionName;
        
        if (type == "upgrade") {
            return channel.sendUpgradeProposal(request)
            .then((success) => { return success; },
                  (err)     => { return Promise.reject(logger.errorf('channel.sendUpgradeProposal failed, err=%s', err)); })
        } else {
            return channel.sendInstantiateProposal(request)
            .then((success) => { return success; },
                  (err)     => { return Promise.reject(logger.errorf('channel.sendInstantiateProposal failed, err=%s', err)); })
        }
    })
    .then((results) => {
        logger.debug('sendProposal results=', results)
        var proposalResponses = results[0];
        var proposal = results[1];
        var all_good = true;
        for (var i in proposalResponses) {
            let one_good = false;
            if (proposalResponses && proposalResponses[i].response && proposalResponses[i].response.status === 200) {
                one_good = true;
                logger.debug('instantiate proposal was good');
            } else {
                logger.error('sendProposal proposalResponses err: %s', proposalResponses[i].details); //不要删除这个日志
            }
            all_good = all_good & one_good;
        }
        if (all_good) {
            logger.debug(
                'Successfully sent Proposal and received ProposalResponse: Status - %s, message - "%s", metadata - "%s", endorsement signature: %s',
                proposalResponses[0].response.status, proposalResponses[0].response.message,
                proposalResponses[0].response.payload, proposalResponses[0].endorsement.signature);
            var request = {
                proposalResponses: proposalResponses,
                proposal: proposal
            };
            // set the transaction listener and set a timeout of 30sec
            // if the transaction did not get committed within the timeout period,
            // fail the test
            var deployId = tx_id.getTransactionID();

            eh = client.newEventHub();
            //这里为什么只注册peer1的EventHub ？  
            let data = fs.readFileSync(path.join('', ORGS[org].peers['peer1']['tls_cacerts']));
            eh.setPeerAddr(ORGS[org].peers['peer1']['events'], {
                pem: Buffer.from(data).toString(),
                'ssl-target-name-override': ORGS[org].peers['peer1']['server-hostname']
            });
            eh.connect();

            let txPromise = new Promise((resolve, reject) => {
                let handle = setTimeout(() => {
                    eh.disconnect();
                    reject(logger.errorf('The chaincode instantiate/upgrade TxEvent timeout, peer=%s', eh.getPeerAddr()));
                }, eventHubTimeout);

                eh.registerTxEvent(deployId, (tx, code) => {
                    logger.debug('The chaincode instantiate/upgrade transaction has been committed on peer ' + eh.getPeerAddr());
                    clearTimeout(handle);
                    eh.unregisterTxEvent(deployId);
                    eh.disconnect();

                    if (code !== 'VALID') {
                        reject(logger.errorf('The chaincode instantiate/upgrade transaction was invalid, code = %s, peer=%s', code, eh.getPeerAddr()));
                    } else {
                        logger.debug('The chaincode instantiate/upgrade transaction was valid.');
                        resolve();
                    }
                });
            });

            var sendPromise = channel.sendTransaction(request);
            return Promise.all([sendPromise].concat([txPromise]))
        } else {
            return Promise.reject('Failed to send instantiate/upgrade Proposal or receive valid response. Response null or status is not 200.');
        }
    })
    .then((results) => {
        let resp = results[0];
        if (resp.status === 'SUCCESS') {
            logger.debug('Successfully sent transaction to the orderer.');
            let response = {
                success: true,
                message: util.format('Chaincode %s is SUCCESS on %s', type == "upgrade" ? 'upgrade' : 'instantiation',  org),
                result: tx_id.getTransactionID()
            };
            return response;
        } else {
            return Promise.reject(logger.errorf('Failed to order the transaction. response: %s', resp));
        }
    })
    .catch((err) => {
        return Promise.reject(err)
    });
};

var instantiateChaincode = function(channelName, chaincodeName, chaincodeVersion, functionName, args, username, org, eventHubTimeout) {
    return __instantiateOrUpdateChaincode('instantiate', channelName, chaincodeName, chaincodeVersion, functionName, args, username, org, eventHubTimeout)
}
var upgradeChaincode = function(channelName, chaincodeName, chaincodeVersion, functionName, args, username, org, eventHubTimeout) {
    return __instantiateOrUpdateChaincode('upgrade', channelName, chaincodeName, chaincodeVersion, functionName, args, username, org, eventHubTimeout)
}
exports.instantiateChaincode = instantiateChaincode;
exports.upgradeChaincode = upgradeChaincode;

/*****************************************************************************************/
/********************************   Channel  *********************************************/
/*****************************************************************************************/
//Attempt to send a request to the orderer with the sendCreateChain method
var createChannel = function(channelName, channelConfigPath, username, orgName) {
    logger.debug('\n====== Creating Channel \'' + channelName + '\' ======\n');
    var client = helper.getClientForOrg(orgName);
    var channel = helper.getChannelForOrg(orgName);

    // read in the envelope for the channel config raw bytes
    var envelope = fs.readFileSync(path.join('', channelConfigPath));
    // extract the channel config bytes from the envelope to be signed
    var channelConfig = client.extractChannelConfig(envelope);

    //Acting as a client in the given organization provided with "orgName" param
    return helper.getOrgAdmin(orgName)
        .then((admin) => {
                logger.debug(util.format('Successfully acquired admin user for the organization "%s"', orgName));
                // sign the channel config bytes as "endorsement", this is required by
                // the orderer's channel creation policy
                let signature = client.signChannelConfig(channelConfig);

                let request = {
                    config: channelConfig,
                    signatures: [signature],
                    name: channelName,
                    orderer: channel.getOrderers()[0],
                    txId: client.newTransactionID()
                };

                // send to orderer
                return client.createChannel(request)
                .then((success)=>{ return success; },
                      (err)    =>{ return Promise.reject(logger.errorf('createChannel Failed, Error: %s ', err)); })
            }, 
            (err) => {
                return Promise.reject(logger.errorf('getOrgAdmin Failed, Error: %s ', err))
            })
        .then((result) => {
            logger.debug(' result ::%j', result);
            if (result && result.status === 'SUCCESS') {
                logger.debug('Successfully created the channel.');
                let response = {
                    success: true,
                    message: 'Channel \'' + channelName + '\' created Successfully'
                };
                return response;
            } else {
                return Promise.reject(logger.errorf('createChannel status invalid, result: %s ', result));
            }
        })
        .catch((err) => {
            return Promise.reject(err);
        });
};

exports.createChannel = createChannel;

//
//Attempt to send a request to the orderer with the sendCreateChain method
//

var joinChannel = function(channelName, peers, username, org, eventWaitTime) {
    if (!eventWaitTime)
        eventWaitTime = 15000

    //logger.debug('\n============ Join Channel ============\n')
    logger.info(util.format(
        'Calling peers in organization "%s" to join the channel', org));

    var client = helper.getClientForOrg(org);
    var channel = helper.getChannelForOrg(org);
    var eventhubs = [];
    var tx_id = null;

    return helper.getOrgAdmin(org)
    .then((admin) => {
        logger.info('received member object for admin of the organization "%s": ', org);
        tx_id = client.newTransactionID();
        let request = {
            txId :  tx_id
        };

        return channel.getGenesisBlock(request)
        .then((genesis_block) => {
                return genesis_block
            },
            (err)=>{
                return Promise.reject('channel.getGenesisBlock failed, err=%s', err)
            })
    })
    .then((genesis_block) => {
        tx_id = client.newTransactionID();
        var request = {
            targets: helper.newPeers(peers, org),
            txId: tx_id,
            block: genesis_block
        };

        eventhubs = helper.newEventHubs(peers, org);
        for (let key in eventhubs) {
            let eh = eventhubs[key];
            eh.connect();
        }

        //这里为什么注册BlockEvent？ 
        var eventPromises = [];
        eventhubs.forEach((eh) => {
            let txPromise = new Promise((resolve, reject) => {
                let handle = setTimeout(()=>{ 
                    eh.disconnect();
                    reject(logger.errorf('Join Channel, wait for eventHub timeout, peer=%s', eh.getPeerAddr()))
                }, eventWaitTime);
                
                let blkRegNum = eh.registerBlockEvent((block) => {
                    clearTimeout(handle);
                    eh.unregisterBlockEvent(blkRegNum);
                    eh.disconnect();
                    // in real-world situations, a peer may have more than one channels so
                    // we must check that this block came from the channel we asked the peer to join
                    if (block.data.data.length === 1) {
                        // Config block must only contain one transaction
                        var channel_header = block.data.data[0].payload.header.channel_header;
                        if (channel_header.channel_id === channelName) {
                            resolve();
                        } else {
                            reject(logger.errorf('Join Channel, channel_id missmatch, channel_id=%s', channel_header.channel_id));
                        }
                    }
                });
            });
            eventPromises.push(txPromise);
        });
        let sendPromise = channel.joinChannel(request);
        return Promise.all([sendPromise].concat(eventPromises));
    })
    .then((results) => {
        logger.debug(util.format('Join Channel R E S P O N S E : %j', results));
        if (results[0] && results[0][0] && results[0][0].response && results[0][0].response.status == 200) {
            logger.info(util.format('Successfully joined peers in organization %s to the channel \'%s\'', org, channelName));
            let response = {
                success: true,
                message: util.format('Successfully joined peers in organization %s to the channel \'%s\'', org, channelName)
            };
            return response;
        } else {
            return  Promise.reject(logger.errorf('Failed to join channel due to error: %s ', err))
        }
    })
    .catch((err) => {
        return  Promise.reject(err)
    });
};
exports.joinChannel = joinChannel;




/*****************************************************************************************/
/********************************     etc    *********************************************/
/*****************************************************************************************/
function registerAndEnroll(username, userOrg, isJson) {
    return helper.registerAndEnroll(username, userOrg, isJson)
}

function getConfigSetting(name, default_value) {
    return hfc.getConfigSetting(name, default_value)
}

function setRelativePath(path) {
    return helper.setRelativePath(path)
}
function getRelativePath() {
    return helper.getRelativePath()
}

exports.registerAndEnroll = registerAndEnroll;
exports.getConfigSetting = getConfigSetting;
exports.setRelativePath = setRelativePath;
exports.getRelativePath = getRelativePath;
