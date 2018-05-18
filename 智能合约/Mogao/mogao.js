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

const loulan = require('./loulan.js')


const common = require('./lib/common');


var logger = common.createLog("mogao")
logger.setLogLevel(logger.logLevel.DEBUG)



process.on('exit', function (){
  logger.info(" ****  mogao exit ****");
});

loulan.SetModuleName('mogao')

loulan.SetLogger(logger)
loulan.UseDefaultRoute()


loulan.SetRequireParams(loulan.routeHandles.All,    {channame: 'mogao-channel', org: 'org1'})


loulan.SetRequireParams(loulan.routeHandles.ChaincodeInstall, {peers: 'peer0,peer1,peer2,peer3'})

loulan.SetRequireParams(loulan.routeHandles.ChaincodeInstantiate, {peers: 'peer0,peer1,peer2,peer3'})

loulan.SetRequireParams(loulan.routeHandles.ChaincodeUpgrade, {peers: 'peer0,peer1,peer2,peer3'})


loulan.SetRequireParams(loulan.routeHandles.Register,   {peers: 'peer0,peer1,peer2,peer3'})

loulan.SetRequireParams(loulan.routeHandles.Query,      {signature: 'LQ==', peer: 'peer0'})   //测试链上不验证签名，不会发送这个参数，这里自动加一个  'LQ=='为'-'的base64编码

loulan.SetRequireParams(loulan.routeHandles.Invoke,     {signature: 'LQ==', peers: 'peer0,peer1,peer2,peer3'}) //测试链上不验证签名，不会发送这个参数，这里自动加一个 'LQ=='为'-'的base64编码

//loulan.SetExportIntfPath('/usr/share/nginx/whtmls/exportIntf', 'https://www.lianlianchains.com', '/exportIntf')

loulan.Start('./cfg/subscribe.cfg')
