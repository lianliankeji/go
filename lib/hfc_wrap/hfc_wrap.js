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

var grpc = require('grpc');
var _ccProto = grpc.load(__dirname + '/protos/peer/chaincode.proto').protos;  //需要在本文件所在目录创建一个软连接，指向 node_modules/fabric-client/lib/protos/


var ByteBuffer = require('fabric-client/node_modules/bytebuffer/dist/bytebuffer-node.js')


var ORGS = hfc.getConfigSetting('network-config');



/*****************************************************************************************/
/********************************  Query  ************************************************/
/*****************************************************************************************/
var queryChaincode = function(peer, channelName, chaincodeName, args, fcn, username, org, payloadJson) {
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
                    if (qResult.details)
                        return Promise.reject(qResult.details);
                    else
                        return Promise.reject(qResult.toString());
                }

                qResult = qResult.toString('utf8')
                if (qResult.length > 0 && payloadJson == true) {
                    try {
                        qResult = JSON.parse(qResult)
                    } catch(err) {
                        //如果转换JSON失败，仍然返回字符串格式，这里记录个日志即可
                        logger.error('queryChaincode: parse payload to JSON failed, err=%s', err)
                    }
                }
                
                let response = {
                    success: true,
                    message: 'Query OK.',
                    result: qResult
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



/*****************************************************************************************/
/********************************  Invoke  ***********************************************/
/*****************************************************************************************/
var invokeChaincode = function(peerNames, channelName, chaincodeName, fcn, args, username, org, payloadJson, errMsgJson, eventHubTimeout) {
    logger.debug(util.format('\n============ invoke transaction on organization %s ============\n', org));
    
    if (!eventHubTimeout) 
        eventHubTimeout = 30000
    
    var client = helper.getClientForOrg(org);
    var channel = helper.getChannelForOrg(org);
    var targets = (peerNames) ? helper.newPeers(peerNames, org) : undefined;
    var tx_id = null;
    var invokeResult = null;

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
        logger.debug('invokeChaincode: proposal=%j', results)
        var proposalResponses = results[0];
        var proposal = results[1];
        var errDetails = ''
        var all_good = true;

        logger.debug('invokeChaincode: proposalResponses=', proposalResponses)
        for (var i in proposalResponses) {
            let one_good = false;
            if (proposalResponses && proposalResponses[i].response && proposalResponses[i].response.status === 200) {
                one_good = true;
                if (!invokeResult) {
                    invokeResult = proposalResponses[i].response.payload.toString('utf8')
                    if (invokeResult.length > 0 && payloadJson == true) {
                        try {
                            invokeResult = JSON.parse(invokeResult)
                        } catch(err) {
                            //如果转换JSON失败，仍然返回字符串格式，这里记录个日志即可
                            logger.error('invokeChaincode: parse payload to JSON failed, err=%s', err)
                        }
                    }
                }
            } else {
                logger.error('invokeChaincode: channel.sendTransactionProposal proposalResponses err: %s', proposalResponses[i].details); //不要删除这个日志
                //details: 'chaincode error (status: 500, message: balance of 'a' less than 1000)'
                if (!errDetails) { //目前只保留一条错误信息
                    errDetails = __getErrMsgFromInvokeResponse(proposalResponses[i].details)
                    if (errDetails.length > 0 && errMsgJson == true) {
                        try {
                            errDetails = JSON.parse(errDetails)
                        } catch(err) {
                            //如果转换JSON失败，仍然返回字符串格式，这里记录个日志即可
                            logger.error('invokeChaincode: parse errDetails to JSON failed, err=%s', err)
                        }
                    }
                }
            }
            all_good = all_good & one_good;
        }
        if (all_good) {
            /*
            logger.debug(util.format(
                'Successfully sent Proposal and received ProposalResponse: Status - %s, message - "%s", metadata - "%s", endorsement signature: %s',
                proposalResponses[0].response.status, proposalResponses[0].response.message,
                proposalResponses[0].response.payload, proposalResponses[0].endorsement
                .signature));
            */
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
                        reject(logger.errorf('invokeChaincode: TxEvent timeout, peer=%s', eh.getPeerAddr()));
                    }, eventHubTimeout);

                    eh.registerTxEvent(transactionID, (tx, code) => {
                        clearTimeout(handle);
                        eh.unregisterTxEvent(transactionID);
                        eh.disconnect();

                        if (code !== 'VALID') {
                            reject(logger.errorf('invokeChaincode: The transaction was invalid, code =%s, peer=%s', code, eh.getPeerAddr()));
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
            if (errDetails)
                return Promise.reject(errDetails);
            else
                return Promise.reject('invoke response null or status is not 200.');
        }
    })
    .then((result) => {
        logger.debug('invokeChaincode: result=', result);
        var resp = result[0]
        if (resp.status === 'SUCCESS') {
            logger.info('Successfully sent transaction to the orderer.');
            let response = {
                success: true,
                message: 'Invoke OK.',
                result: {payload: invokeResult,  txid: tx_id.getTransactionID()}
            };
            return response
        } else {
            return Promise.reject(logger.errorf('invokeChaincode: Failed to order the transaction. response: %s', resp));
        }
    })
    .catch((err) => {
        return Promise.reject(err); //上面的err已经有错误信息了， 这里不需要再组装一次错误原因，直接返回
    });
};



/*****************************************************************************************/
/********************************  Chaincode *********************************************/
/*****************************************************************************************/
var installChaincode = function(peers, chaincodeName, chaincodePath, chaincodeVersion, username, org) {
    logger.debug('============ Install chaincode on organizations ============');
    helper.setupChaincodeDeploy();
    var client = helper.getClientForOrg(org);
    
    //先检查chaincodePath是否存在。 如果不存在， client.installChaincode会出现异常，导致程序退出
    var realPath =  path.join(process.env.GOPATH, 'src', chaincodePath)
    
    return new Promise((resolve, reject)=>{
            fs.access(realPath, fs.constants.R_OK, (err) => {
                if (err) {
                    return reject(logger.errorf('access chaincode path [%s] failed, err=%s ', realPath, err));
                }
                
                return resolve()
            })
        })
    .then(()=>{
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
        })
    .then((results) => {
        logger.debug('installChaincode results=', results)
        var proposalResponses = results[0];
        var proposal = results[1];
        var errDetails = ''
        var all_good = true;
        for (var i in proposalResponses) {
            let one_good = false;
            if (proposalResponses && proposalResponses[i].response && proposalResponses[i].response.status === 200) {
                one_good = true;
                logger.debug('install proposal was good');
            } else {
                logger.debug('client.installChaincode proposalResponses err: %s', proposalResponses[i].details); //不要删除这个日志
                //details: chaincode error (status: 500, message: XXXXXXX)
                if (!errDetails) //目前只保留一条错误信息
                    errDetails = __getErrMsgFromInvokeResponse(proposalResponses[i].details)
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
            if (errDetails)
                return Promise.reject(errDetails);
            else
                return Promise.reject('install response null or status is not 200.');
        }
    })
    .catch((err) => {
        return Promise.reject(err);
    });
};

var __instantiateOrUpdateChaincode = function(peers, type, chaincodeName, chaincodeVersion, functionName, args, username, org, cctimeout, eventHubTimeout) {
    logger.debug('\n============ instantiateOrUpdate chaincode on organization ' + org + ' ============\n');
    
    if (!cctimeout)
        cctimeout = 100000
    
    if (!eventHubTimeout)
        eventHubTimeout = 30000

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
            targets: helper.newPeers(peers, org),
            chaincodeId: chaincodeName,
            chaincodeVersion: chaincodeVersion,
            args: args,
            txId: tx_id
        };

        if (functionName)
            request.fcn = functionName;
        
        if (type == "upgrade") {
            return channel.sendUpgradeProposal(request, cctimeout)
            .then((success) => { return success; },
                  (err)     => { return Promise.reject(logger.errorf('channel.sendUpgradeProposal failed, err=%s', err)); })
        } else {
            return channel.sendInstantiateProposal(request, cctimeout)
            .then((success) => { return success; },
                  (err)     => { return Promise.reject(logger.errorf('channel.sendInstantiateProposal failed, err=%s', err)); })
        }
    })
    .then((results) => {
        logger.debug('__instantiateOrUpdateChaincode: sendProposal results=', results)
        var proposalResponses = results[0];
        var proposal = results[1];
        var all_good = true;
        var errDetails = ''
        for (var i in proposalResponses) {
            let one_good = false;
            if (proposalResponses && proposalResponses[i].response && proposalResponses[i].response.status === 200) {
                one_good = true;
                logger.debug('instantiate proposal was good');
            } else {
                if (proposalResponses[i].details) {
                    logger.debug('__instantiateOrUpdateChaincode: sendProposal proposalResponses err: %s', proposalResponses[i].details); //不要删除这个日志
                    if (!errDetails) //目前只保留一条错误信息
                        errDetails = proposalResponses[i].details  //details信息没有固定的格式，这里原样返回
                } else if (proposalResponses[i] instanceof Error) { //有时向peer发送请求，会出现超时的情况，此时不会返回 details， 会返回一个Error
                    if (!errDetails)
                        errDetails = proposalResponses[i].toString()
                }
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
            var ehPromises = []
            
            var eventhubs = helper.newEventHubs(peers, org);
            for (let key in eventhubs) {
                let eh = eventhubs[key];
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
                ehPromises.push(txPromise)
            }

            var sendPromise = channel.sendTransaction(request);
            return Promise.all([sendPromise].concat(ehPromises))
        } else {
            if (errDetails)
                return Promise.reject(errDetails);
            else
                return Promise.reject(type + ' response null or status is not 200.');
        }
    })
    .then((results) => {
        let resp = results[0];
        if (resp.status === 'SUCCESS') {
            logger.debug('Successfully sent transaction to the orderer.');
            let response = {
                success: true,
                message: type + ' OK.',
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

var instantiateChaincode = function(peers, chaincodeName, chaincodeVersion, functionName, args, username, org, cctimeout, eventHubTimeout) {
    if (!functionName)
        functionName = 'init'
    
    return __instantiateOrUpdateChaincode(peers, 'instantiate', chaincodeName, chaincodeVersion, functionName, args, username, org, cctimeout, eventHubTimeout)
}
var upgradeChaincode = function(peers, chaincodeName, chaincodeVersion, functionName, args, username, org, cctimeout, eventHubTimeout) {
    if (!functionName)
        functionName = 'upgrade'

    return __instantiateOrUpdateChaincode(peers, 'upgrade', chaincodeName, chaincodeVersion, functionName, args, username, org, cctimeout, eventHubTimeout)
}

/*****************************************************************************************/
/********************************   Channel  *********************************************/
/*****************************************************************************************/
//Attempt to send a request to the orderer with the sendCreateChain method
var createChannel = function(channelName, channelConfigPath, username, orgName) {
    logger.debug('\n====== Creating Channel \'' + channelName + '\' ======\n');
    var client = helper.getClientForOrg(orgName);

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
                    orderer: helper.newOrderer(),
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
    if (channel.getName() != channelName) {
        channel = client.newChannel(channelName);
        channel.addOrderer(helper.newOrderer());
    }
    
    
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
        var timerHandles = []
        eventhubs.forEach((eh) => {
            let txPromise = new Promise((resolve, reject) => {
                let handle = setTimeout(()=>{
                    eh.disconnect();
                    reject(logger.errorf('Join Channel, wait for eventHub timeout, peer=%s', eh.getPeerAddr()))
                }, eventWaitTime);
                timerHandles.push(handle);

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
        
        let sendPromise = channel.joinChannel(request)  // channel.joinChannel失败后，不会返回reject，所以在这里检测一下。如果这里不检测而是通过Promise.all检测，只有定时器超时才能被检测到
            .then((result)=>{
                    logger.debug('channel.joinChannel result=', result)
                    var all_good = true
                    var one_good = false
                    var errDetails = ''

                    for (i in result) {
                        if (result[i] && result[i].response && result[i].response.status == 200) {
                            one_good = true
                        } else {
                            logger.error('joinChannel: channel.joinChannel error: %s ', result[i].details)
                            if (!errDetails) { //目前只保留一条错误信息
                                errDetails = __getErrMsgFromInvokeResponse(result[i].details)
                            }
                        }     
                        all_good = all_good & one_good;                        
                    }
                    if (all_good) {
                        return null
                    } else {
                        //stop timers
                        for (t in timerHandles) {
                            clearTimeout(timerHandles[t])
                        }
                         
                        if (errDetails) {
                            return Promise.reject('joinChannel failed: ' + errDetails)
                        } else {
                            return Promise.reject('joinChannel: channel.joinChannel response null or status is not 200.');
                        }
                    }
                },
                (err)=>{
                    return Promise.reject(logger.errorf('joinChannel error: %s ', err))
                })
            
        return Promise.all([sendPromise].concat(eventPromises));
    })
    .then((results) => {
        let response = {
            success: true,
            message: util.format('Successfully joined peers in organization %s to the channel \'%s\'', org, channelName)
        };
        return response;
    })
    .catch((err) => {
        return  Promise.reject(err)
    });
};




/*****************************************************************************************/
/********************************     etc    *********************************************/
/*****************************************************************************************/
function __getErrMsgFromInvokeResponse(invokeResp) {
    //invokeResp:  chaincode error (status: 500, message: XXX)
    var msgKey = "message:"
    var keyIdx = invokeResp.indexOf(msgKey)
    if (keyIdx<=0)
        return invokeResp
    
    return invokeResp.substring(keyIdx+msgKey.length, invokeResp.length-1).trim()
}


function getNumFromNumStruct(numStruct) {
    /* numStruct
     {
        "low": 12,
        "high": 0,
        "unsigned": true
    }
    */

    //high如何处理？  目前先返回low
    return numStruct.low
}

function getChainHight(chainInfo) {
    /* chainInfo
    {
		"height": {
			"low": 12,
			"high": 0,
			"unsigned": true
		},
		"currentBlockHash": {...},
		"previousBlockHash": {...}
    }
    */
    
    if (!chainInfo || !chainInfo.height || !chainInfo.height.low){
        return 'chainInfo format error.'
    }
    
    //high如何处理？  目前先返回low
    return getNumFromNumStruct(chainInfo.height)
}


function getTxInfo(blockInfo, isDesc) {
    
    if (!blockInfo || !blockInfo.header || !blockInfo.data){
        return 'blockInfo format error.'
    }
    
    var blockIdx = getNumFromNumStruct(blockInfo.header.number)

    var txList = []

    var txPayloads = blockInfo.data.data

    var txIdxList = []
    //block中的tx是由旧到新排序的？
    if (isDesc == true) {
        for (var i = txPayloads.length-1; i>=0; i--) {
            txIdxList.push(i)
        }
    } else {
        for (var i = 0; i < txPayloads.length; i++) {
            txIdxList.push(i)
        }
    }
    
    for (var i in txIdxList) {
        var onePayload = txPayloads[txIdxList[i]]
        var txObj = {}
        txObj.block = blockIdx
        var chnlHdr = onePayload.payload.header.channel_header
        if (chnlHdr.type == 'ENDORSER_TRANSACTION') { //还有一种类型ORDERER_TRANSACTION，是否属于交易？
            txObj.txid = chnlHdr.tx_id
            txObj.seconds = new Date(chnlHdr.timestamp).getTime() / 1000
            //下面的actions会有多个的情况吗？
            //input为Buffer类型
            txObj.input = onePayload.payload.data.actions[0].payload.chaincode_proposal_payload.input
            txList.push(txObj)
        }
    }
    
    return txList
}

function getInvokeArgs(inputHexStr) {
    var chaincodeInvocationSpec =_ccProto.ChaincodeInvocationSpec.decode(ByteBuffer.fromHex(inputHexStr))
    var chaincodeSpec = chaincodeInvocationSpec.getChaincodeSpec()
    var tmpArgs = chaincodeSpec.getInput().getArgs()
    var args = []
    for (var i in tmpArgs)
        args.push(tmpArgs[i].toBuffer().toString())
    
    return args
}

//导出接口
exports.createChannel = createChannel;
exports.joinChannel = joinChannel;

exports.installChaincode = installChaincode;
exports.instantiateChaincode = instantiateChaincode;
exports.upgradeChaincode = upgradeChaincode;

exports.invokeChaincode = invokeChaincode;

exports.queryChaincode = queryChaincode;
exports.getBlockByNumber = getBlockByNumber;
exports.getTransactionByID = getTransactionByID;
exports.getBlockByHash = getBlockByHash;
exports.getChainInfo = getChainInfo;
exports.getChaincodes = getChaincodes;
exports.getChannels = getChannels;


exports.getInvokeArgs = getInvokeArgs;
exports.getTxInfo = getTxInfo;
exports.getChainHight = getChainHight;
exports.registerAndEnroll = helper.registerAndEnroll;
exports.getConfigSetting = hfc.getConfigSetting;
exports.initNetworkTopo = helper.initNetworkTopo;