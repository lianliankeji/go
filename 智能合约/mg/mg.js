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

const loulan = require('./loulan')


const common = require('./lib/common');


var logger = common.createLog("mg")
logger.setLogLevel(logger.logLevel.INFO)



process.on('exit', function (){
  logger.info(" ****  mg exit ****");
});

loulan.SetModuleName('mg')

loulan.SetLogger(logger)
loulan.UseDefaultRoute()


loulan.SetRequireParams(loulan.routeHandles.All,    {channame: 'mogaotest-channel', org: 'org1', ccname: 'mogao'})


loulan.SetRequireParams(loulan.routeHandles.ChaincodeInstall, {peers: 'peer0,peer1,peer2,peer3'})

loulan.SetRequireParams(loulan.routeHandles.ChaincodeInstantiate, {peers: 'peer0,peer1,peer2,peer3'})

loulan.SetRequireParams(loulan.routeHandles.ChaincodeUpgrade, {peers: 'peer0,peer1,peer2,peer3'})


loulan.SetRequireParams(loulan.routeHandles.Register,   {signature: 'LQ==', peers: 'peer0,peer1,peer2,peer3', useaccsys: '1'})

loulan.SetRequireParams(loulan.routeHandles.Query,      {signature: 'LQ==', peer: 'peer0', useaccsys: '1'})   //?????????,????????,???????  'LQ=='?'-'?base64??

loulan.SetRequireParams(loulan.routeHandles.Invoke,     {signature: 'LQ==', peers: 'peer0,peer1,peer2,peer3', useaccsys: '1'}) //?????????,????????,??????? 'LQ=='?'-'?base64??

//loulan.SetExportIntfPath('/usr/share/nginx/whtmls/exportIntf', 'https://www.lianlianchains.com', '/exportIntf')


//loulan.RegisterInvokeParamsFmtConvHandle(parseParams_Invoke)
//loulan.RegisterQueryParamsFmtConvHandle(parseParams_Query)

//loulan.RegisterInvokeResultFormatHandle(formatInvokeResult)
loulan.RegisterQueryResultFormatHandle(formatQueryResult)

loulan.Start('./cfg/subscribe.cfg')


function parseParams_Invoke(params) {
    var body = {
        code : loulan.retCode.OK,
        msg: "OK",
        result: ""
    };

    return null
}


function parseParams_Query(params) {
    var body = {
        code : loulan.retCode.OK,
        msg: "OK",
        result: ""
    };
  
    return null
}

function formatInvokeResult(params, req, body) {
}

function formatQueryResult(params, req, body) {
    var func = params.fcn || params.func
    var formatJson = params.jsonresult
    if (formatJson) {
        if (func == "getTransInfo" || func == "getRankingAndTopN") {
            body.result = JSON.parse(body.result)
        }
    }
}