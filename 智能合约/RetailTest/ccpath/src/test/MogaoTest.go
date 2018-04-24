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
	"crypto/rand"
	"fmt"
	"strconv"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
)

var logger = shim.NewLogger("test")

// SimpleChaincode example simple Chaincode implementation
type SimpleChaincode struct {
	Test int
}

func (t *SimpleChaincode) Init(stub shim.ChaincodeStubInterface) (pbResponse pb.Response) {
	logger.Info("########### example_cc0 Init ###########")

	function, args := stub.GetFunctionAndParameters()

	defer func() {
		if excption := recover(); excption != nil {
			pbResponse = shim.Error(fmt.Sprintf("Init(%s) got excption:%s", function, excption))
		}
	}()

	//合约实例化时，默认会执行init函数，除非在调用合约实例化接口时指定了其它的函数
	if function == "init" {
		var A string // Entities
		var Aval int // Asset holdings
		var err error

		for i := 1; i < len(args); i += 2 {

			// Initialize the chaincode
			A = args[i]
			Aval, err = strconv.Atoi(args[i+1])
			if err != nil {
				return shim.Error("Expecting integer value for asset holding")
			}
			logger.Infof("A=%s, Aval = %d\n", A, Aval)

			// Write the state to the ledger
			err = stub.PutState(A, []byte(strconv.Itoa(Aval)))
			if err != nil {
				return shim.Error(err.Error())
			}
		}

		return shim.Success(nil)

	} else if function == "upgrade" { //升级时默认会执行upgrade函数，除非在调用合约升级接口时指定了其它的函数
		return shim.Success(nil)

	} else if function == "errTest" {
		return shim.Error("err: test error.")
	} else if function == "initExcp" {
		var pp *SimpleChaincode = nil
		pp.Test = 0
		return shim.Error("err: initExcp.")
	}

	return shim.Error(fmt.Sprintf("unkonwn function '%s'", function))

}

// Transaction makes payment of X units from A to B
func (t *SimpleChaincode) Invoke(stub shim.ChaincodeStubInterface) (pbResponse pb.Response) {
	logger.Info("########### example_cc0 Invoke ###########")

	function, args := stub.GetFunctionAndParameters()

	defer func() {
		if excption := recover(); excption != nil {
			pbResponse = shim.Error(fmt.Sprintf("Invoke(%s) got excption:%s", function, excption))
		}
	}()

	if function == "delete" {
		// Deletes an entity from its state
		return t.delete(stub, args)
	}

	if function == "query" {
		// queries an entity state
		return t.query(stub, args)
	}
	if function == "move" {
		// Deletes an entity from its state
		return t.move(stub, args)
	}
	if function == "writeWhenQuery" {
		// Deletes an entity from its state
		var A string // Entities
		var err error

		if len(args) != 1 {
			return shim.Error("Incorrect number of arguments. Expecting name of the person to query")
		}

		A = args[0]

		// Get the state from the ledger
		Avalbytes, err := stub.GetState(A)
		if err != nil {
			jsonResp := "{\"Error\":\"Failed to get state for " + A + "\"}"
			return shim.Error(jsonResp)
		}

		if Avalbytes == nil {
			jsonResp := "{\"Error\":\"Nil amount for " + A + "\"}"
			return shim.Error(jsonResp)
		}

		err = stub.PutState(A, []byte(strconv.Itoa(99999)))
		if err != nil {
			jsonResp := "{\"Error\":\"Failed to put state for " + A + "\"}"
			return shim.Error(jsonResp)
		}

		avTemp, err := stub.GetState(A)
		fmt.Printf("avTemp=%s\n", string(avTemp))

		jsonResp := "{\"Name\":\"" + A + "\",\"Amount\":\"" + string(Avalbytes) + "\"}"
		logger.Infof("Query Response:%s\n", jsonResp)
		return shim.Success(Avalbytes)
	}
	if function == "writeDiffValue" {

		salt := make([]byte, 16)
		rand.Read(salt)
		err := stub.PutState("writeDiffValue", salt)
		if err != nil {
			jsonResp := "{\"Error\":\"Failed to put state for writeDiffValue\"}"
			return shim.Error(jsonResp)
		}

		avTemp, err := stub.GetState("writeDiffValue")
		fmt.Printf("avTemp=%v\n", avTemp)
		fmt.Printf("put salt ok, =%v\n", salt)

		return shim.Success([]byte(fmt.Sprintf("avTemp=%v\n", avTemp)))
	}
	if function == "queryDiffValue" {

		avTemp, err := stub.GetState("writeDiffValue")
		if err != nil {
			jsonResp := "{\"Error\":\"Failed to GetState for writeDiffValue\"}"
			return shim.Error(jsonResp)
		}
		fmt.Printf("avTemp=%v\n", avTemp)

		return shim.Success([]byte(fmt.Sprintf("avTemp=%v\n", avTemp)))
	}
	if function == "writeTxTimestamp" {

		timestamp, err := stub.GetTxTimestamp()
		if err != nil {
			jsonResp := "{\"Error\":\"Failed to GetTxTimestamp\"}"
			return shim.Error(jsonResp)
		}
		fmt.Printf("timestamp=%+v\n", timestamp)
		fmt.Printf("timestamp.Nanos=%+v\n", timestamp.Nanos)
		fmt.Printf("timestamp.Seconds=%+v\n", timestamp.Seconds)
		fmt.Printf("timestamp.String()=%+v\n", timestamp.String())
		fmt.Printf("timestamp.XXX_WellKnownType()=%+v\n", timestamp.XXX_WellKnownType())

		invokeTime := timestamp.Seconds*1000 + int64(timestamp.Nanos/1000000)

		err = stub.PutState("TxTimestamp", []byte(strconv.FormatInt(invokeTime, 10)))
		if err != nil {
			jsonResp := "{\"Error\":\"Failed to put state for GetTxTimestamp\"}"
			return shim.Error(jsonResp)
		}

		return shim.Success([]byte(fmt.Sprintf("TxTimestamp=%v\n", invokeTime)))
	}
	if function == "queryTxTimestamp" {

		avTemp, err := stub.GetState("TxTimestamp")
		if err != nil {
			jsonResp := "{\"Error\":\"Failed to GetState for writeDiffValue\"}"
			return shim.Error(jsonResp)
		}
		fmt.Printf("avTemp=%v\n", avTemp)

		return shim.Success([]byte(fmt.Sprintf("TxTimestamp=%v\n", avTemp)))
	}
	if function == "invokeExcp" {
		var pp *SimpleChaincode = nil
		pp.Test = 0
		return shim.Error("err: invokeExcp.")
	}
	if function == "invokeTest" {
		if len(args) < 1 {
			return shim.Error("invokeTest need 1 arg at least.")
		}
		err := stub.PutState("invokeTest", []byte(args[0]))
		if err != nil {
			jsonResp := "{\"Error\":\"Failed to PutState\"}"
			return shim.Error(jsonResp)
		}
		return shim.Success(nil)
	}
	if function == "queryTest" {
		avTemp, err := stub.GetState("invokeTest")
		if err != nil {
			jsonResp := "{\"Error\":\"Failed to GetState for writeDiffValue\"}"
			return shim.Error(jsonResp)
		}

		return shim.Success(avTemp)
	}

	logger.Errorf("Unknown action, check the first argument, must be one of 'delete', 'query', or 'move'. But got: %v", args[0])
	return shim.Error(fmt.Sprintf("Unknown action, check the first argument, must be one of 'delete', 'query', or 'move'. But got: %v", args[0]))
}

func (t *SimpleChaincode) move(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	// must be an invoke
	var A, B string    // Entities
	var Aval, Bval int // Asset holdings
	var X int          // Transaction value
	var err error

	if len(args) != 3 {
		return shim.Error("Incorrect number of arguments. Expecting 4, function followed by 2 names and 1 value")
	}

	A = args[0]
	B = args[1]

	// Get the state from the ledger
	// TODO: will be nice to have a GetAllState call to ledger
	Avalbytes, err := stub.GetState(A)
	if err != nil {
		return shim.Error("Failed to get state")
	}
	if Avalbytes == nil {
		return shim.Error("Entity not found")
	}
	Aval, _ = strconv.Atoi(string(Avalbytes))

	Bvalbytes, err := stub.GetState(B)
	if err != nil {
		return shim.Error("Failed to get state")
	}
	if Bvalbytes == nil {
		return shim.Error("Entity not found")
	}
	Bval, _ = strconv.Atoi(string(Bvalbytes))

	// Perform the execution
	X, err = strconv.Atoi(args[2])
	if err != nil {
		return shim.Error("Invalid transaction amount, expecting a integer value")
	}

	if Aval < X {
		return shim.Error(fmt.Sprintf("balance of '%s' less than %d", A, X))
	}

	Aval = Aval - X
	Bval = Bval + X
	logger.Infof("Aval = %d, Bval = %d\n", Aval, Bval)

	// Write the state back to the ledger
	err = stub.PutState(A, []byte(strconv.Itoa(Aval)))
	if err != nil {
		return shim.Error(err.Error())
	}

	err = stub.PutState(B, []byte(strconv.Itoa(Bval)))
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success([]byte(fmt.Sprintf("%s:%d, %s:%d", A, Aval, B, Bval)))
}

// Deletes an entity from state
func (t *SimpleChaincode) delete(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	if len(args) != 1 {
		return shim.Error("Incorrect number of arguments. Expecting 1")
	}

	A := args[0]

	// Delete the key from the state in ledger
	err := stub.DelState(A)
	if err != nil {
		return shim.Error("Failed to delete state")
	}

	return shim.Success(nil)
}

// Query callback representing the query of a chaincode
func (t *SimpleChaincode) query(stub shim.ChaincodeStubInterface, args []string) pb.Response {

	var A string // Entities
	var err error

	if len(args) != 1 {
		return shim.Error("Incorrect number of arguments. Expecting name of the person to query")
	}

	A = args[0]

	// Get the state from the ledger
	Avalbytes, err := stub.GetState(A)
	if err != nil {
		jsonResp := "{\"Error\":\"Failed to get state for " + A + "\"}"
		return shim.Error(jsonResp)
	}

	if Avalbytes == nil {
		jsonResp := "{\"Error\":\"Nil amount for " + A + "\"}"
		return shim.Error(jsonResp)
	}

	jsonResp := "{\"Name\":\"" + A + "\",\"Amount\":\"" + string(Avalbytes) + "\"}"
	logger.Infof("Query Response:%s\n", jsonResp)
	return shim.Success(Avalbytes)
}

func main() {
	err := shim.Start(new(SimpleChaincode))
	if err != nil {
		logger.Errorf("Error starting Simple chaincode: %s", err)
	}
}
