/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
)

var logger = shim.NewLogger("controller")

// SimpleChaincode example simple Chaincode implementation
type Controller struct {
}

const ()

func (t *Controller) Init(stub shim.ChaincodeStubInterface) pb.Response {
	return shim.Success(nil)
}

// Transaction makes payment of X units from A to B
func (t *Controller) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	logger.Info("########### example_cc0 Invoke ###########")

	function, args := stub.GetFunctionAndParameters()

	logger.Infof("Invoke function=%s args=%+v.", function, args)

	if function == "delete" {
		// Deletes an entity from its state
		return t.delete(stub, args)
	}

	logger.Errorf("Unknown action, check the first argument, must be one of 'delete', 'query', or 'move'. But got: %v", args[0])
	return shim.Error(fmt.Sprintf("Unknown action, check the first argument, must be one of 'delete', 'query', or 'move'. But got: %v", args[0]))
}

func main() {
	err := shim.Start(new(Controller))
	if err != nil {
		logger.Errorf("Error starting chaincode: %s", err)
	}
}
