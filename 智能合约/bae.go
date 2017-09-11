package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"

	"github.com/hyperledger/fabric/core/chaincode/shim"
)

var mylog = InitMylog("bae")
var tblName = "BAE_AccountOwnership"

type EventSender struct {
}

func (t *EventSender) checkAccountOfUser(stub shim.ChaincodeStubInterface, userName string, account string, userCert []byte) (bool, error) {

	callerCert, err := stub.GetCallerCertificate()
	mylog.Debug("caller certificate: %x", callerCert)

	ok, err := t.isCaller(stub, callerCert)
	if err != nil {
		mylog.Error("call isCaller failed(callerCert).")
	}
	if !ok {
		mylog.Error("is not Caller(callerCert).")
	}

	ok, err = t.isCaller(stub, userCert)
	if err != nil {
		mylog.Error("call isCaller failed(userCert).")
	}
	if !ok {
		mylog.Error("is not userCert(userCert).")
	}

	return true, nil
}

func (t *EventSender) Init(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {

	_, err := stub.GetTable(tblName)
	if err == nil {
		mylog.Warn("table '%s' exists already.", tblName)
		return nil, nil
	}

	err = stub.CreateTable(tblName, []*shim.ColumnDefinition{
		&shim.ColumnDefinition{Name: "Asset", Type: shim.ColumnDefinition_STRING, Key: true},
		&shim.ColumnDefinition{Name: "Owner", Type: shim.ColumnDefinition_BYTES, Key: false},
	})

	if err != nil {
		mylog.Error("is not Caller.")
		//no return err
	}

	return nil, nil
}

// Transaction makes payment of X units from A to B
func (t *EventSender) Invoke(stub shim.ChaincodeStubInterface, function string, args []string) (
	[]byte, error) {

	var A, B string    // Entities
	var Aval, Bval int // Asset holdings
	var Gval int       // Asset holdings

	//verify user and account
	var userName = args[3]
	var accName = args[0]
	var userCert []byte

	userCert, _ = base64.StdEncoding.DecodeString(args[4])

	if ok, _ := t.checkAccountOfUser(stub, userName, accName, userCert); !ok {
		fmt.Println("verify user(%s) and account(%s) failed. \n", userName, accName)
		return nil, errors.New("user and account check failed.")
	}

	if function == "transfer" {

		A = args[0]
		B = args[2]

		// Perform the execution
		X, err := strconv.Atoi(args[1])
		if err != nil || X < 0 {

			//event
			stub.SetEvent("error", []byte("Invalid amount, expecting a positive integer value"))

			return nil, errors.New("Invalid amount, expecting a positive integer value")
		}

		// transfer 0, return ok.
		if X == 0 {
			return nil, nil
		}

		if A == B {
			stub.SetEvent("error", []byte("Two entities of transfer are same."))

			return nil, errors.New("Two entities of transfer are same.")
		}

		// Get the state from the ledger
		// TODO: will be nice to have a GetAllState call to ledger
		Avalbytes, err := stub.GetState(A)
		if err != nil {

			//event
			stub.SetEvent("error", []byte("Entity not found"))

			return nil, errors.New("Failed to get state")
		}
		if Avalbytes == nil {

			//event
			stub.SetEvent("error", []byte("Entity not found"))

			return nil, errors.New("Entity not found")
		}
		Aval, _ = strconv.Atoi(string(Avalbytes))

		Bvalbytes, err := stub.GetState(B)
		if err != nil {

			//event
			stub.SetEvent("error", []byte("Failed to get state"))

			return nil, errors.New("Failed to get state")
		}
		if Bvalbytes == nil {

			//event
			stub.SetEvent("error", []byte("Entity not found"))

			return nil, errors.New("Entity not found")
		}
		Bval, _ = strconv.Atoi(string(Bvalbytes))

		if Aval < X {
			//event
			stub.SetEvent("error", []byte("Balance not enough"))

			return nil, errors.New("Balance not enough")
		} else {
			Aval = Aval - X
			Bval = Bval + X
		}

		fmt.Printf("Aval = %d, Bval = %d\n", Aval, Bval)

		// Write the state back to the ledger
		err = stub.PutState(A, []byte(strconv.Itoa(Aval)))
		if err != nil {
			return nil, err
		}

		err = stub.PutState(B, []byte(strconv.Itoa(Bval)))
		if err != nil {
			return nil, err
		}
	}

	if function == "recharge" || function == "takeCash" {

		A = args[0]

		// Perform the execution
		X, err := strconv.Atoi(args[1])
		if err != nil || X < 0 {

			//event
			stub.SetEvent("error", []byte("Invalid amount, expecting a positive integer value"))

			return nil, errors.New("Invalid amount, expecting a integer value")
		}

		// Get the state from the ledger
		Avalbytes, err := stub.GetState(A)
		if err != nil {

			//event
			stub.SetEvent("error", []byte("Failed to get state"))

			return nil, errors.New("Failed to get state")
		}
		if Avalbytes == nil {

			err := stub.PutState(A, []byte(strconv.Itoa(0)))

			if err != nil {
				return nil, err
			}

		}
		Aval, _ = strconv.Atoi(string(Avalbytes))

		// gloab bae
		Gvalbytes, err := stub.GetState("gloab")
		if err != nil {

			//event
			stub.SetEvent("error", []byte("Failed to get state"))

			return nil, errors.New("Failed to get state")
		}
		if Gvalbytes == nil {

			err := stub.PutState("gloab", []byte(strconv.Itoa(0)))

			if err != nil {
				return nil, err
			}

		}
		Gval, _ = strconv.Atoi(string(Gvalbytes))

		if function == "recharge" {

			fmt.Printf("Aval = %d, X = %d\n", Aval, X)
			Aval = Aval + X

			Gval = Gval + X
		}

		if function == "takeCash" {

			if Aval < X {

				//event
				stub.SetEvent("error", []byte("Balance not enough"))

				return nil, errors.New("Balance not enough")
			}

			fmt.Printf("Aval = %d, X = %d\n", Aval, X)
			Aval = Aval - X

			Gval = Gval - X
		}

		// Write the state back to the ledger
		err = stub.PutState(A, []byte(strconv.Itoa(Aval)))
		if err != nil {
			return nil, err
		}

		err = stub.PutState("gloab", []byte(strconv.Itoa(Gval)))
		if err != nil {
			return nil, err
		}

	}

	//event
	stub.SetEvent("success", []byte("invoke success"))
	return nil, nil
}

// Query callback representing the query of a chaincode
func (t *EventSender) Query(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	if function != "query" {
		return nil, errors.New("Invalid query function name. Expecting \"query\"")
	}
	var A string // Entities
	var err error

	/*
		if len(args) != 3 {
			return nil,
				errors.New("Incorrect number of arguments. Expecting name of the personto query")
		}*/

	//verify user and account
	var userName = args[1]
	var accName = args[0]
	var userCert []byte

	userCert, err = base64.StdEncoding.DecodeString(args[2])

	if ok, _ := t.checkAccountOfUser(stub, userName, accName, userCert); !ok {
		fmt.Println("verify user(%s) and account(%s) failed. \n", userName, accName)
		return nil, errors.New("user and account check failed.")
	}

	A = args[0]

	// Get the state from the ledger
	Avalbytes, err := stub.GetState(A)
	if err != nil {
		jsonResp := "{\"Error\":\"Failed to get state for " + A + "\"}"
		return nil, errors.New(jsonResp)
	}

	if Avalbytes == nil {
		jsonResp := "{\"Error\":\"Nil amount for " + A + "\"}"
		return nil, errors.New(jsonResp)
	}

	jsonResp := "{\"Name\":\"" + A + "\",\"Amount\":\"" + string(Avalbytes) + "\"}"
	fmt.Printf("Query Response:%s\n", jsonResp)
	return Avalbytes, nil
}

func (t *EventSender) isCaller(stub shim.ChaincodeStubInterface, certificate []byte) (bool, error) {
	mylog.Debug("Check caller...")

	sigma, err := stub.GetCallerMetadata()
	if err != nil {
		return false, errors.New("Failed getting metadata")
	}
	payload, err := stub.GetPayload()
	if err != nil {
		return false, errors.New("Failed getting payload")
	}
	binding, err := stub.GetBinding()
	if err != nil {
		return false, errors.New("Failed getting binding")
	}

	mylog.Debug("passed certificate [% x]", certificate)
	mylog.Debug("passed sigma [% x]", sigma)
	mylog.Debug("passed payload [% x]", payload)
	mylog.Debug("passed binding [% x]", binding)

	ok, err := stub.VerifySignature(
		certificate,
		sigma,
		append(payload, binding...),
	)
	if err != nil {
		mylog.Error("Failed checking signature [%s]", err)
		return ok, err
	}
	if !ok {
		mylog.Error("Invalid signature")
		return ok, err
	}

	mylog.Debug("Check caller...Verified!")

	return ok, err
}

func main() {
	// for debug
	mylog.SetDefaultLvl(0)

	err := shim.Start(new(EventSender))
	if err != nil {
		fmt.Printf("Error starting EventSender chaincode: %s", err)
	}
}
