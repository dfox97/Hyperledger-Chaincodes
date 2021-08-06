package chaincode

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

const TokenName = "MSc Token" //TOken name can be set to initialise a token name
const totalSupplyKey = "totalSupply"

// object names for prefix
const allowancePrefix = "allowance"

//provides function for transferring tokens between accounts using smart contract api.
type SmartContract struct {
	contractapi.Contract
}

// event used for transactions
type event struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Value int    `json:"value"`
}

//**********************************************************************************************
//****************ERC20 Contract Interface -- Common Functions From Ethereum*******************
//**********************************************************************************************
func (s *SmartContract) BalanceOf(ctx contractapi.TransactionContextInterface, account string) (int, error) {
	//nil means if empty e.g []string
	ownerBalance, err := ctx.GetStub().GetState(account) //read ledger used to access APIs and getstate retrives ledger of smartcontract struct.
	if err != nil {
		return 0, fmt.Errorf("failed to read balance from world state: %v", err)
	}
	if ownerBalance == nil {
		return 0, fmt.Errorf("the account %s doesnt exist", account)
	}
	balance, _ := strconv.Atoi(string(ownerBalance)) //converts datatype to string reprisentation, Atoi is equivalent to parseint (string to int)
	return balance, nil
}

//Transfer tokens from client account to recipient account triggering transfer event
//Recipient account must be a valid clientID as returned by the GetClientID() function reading the ledger
//Requires receiver address, and an amount
func (s *SmartContract) Transfer(ctx contractapi.TransactionContextInterface, receiver string, amount int) error {
	clientID, err := ctx.GetClientIdentity().GetID() //get the id of the client , verifying
	if err != nil {
		return fmt.Errorf("failed to get clientID:%v", err) //checking if clientid is valid
	}
	err = _transferCalc(ctx, clientID, receiver, amount) //we create an error and call the transferHelper function
	if err != nil {
		return fmt.Errorf("failed to transfer: %v", err)
	}

	transferEvent := event{clientID, receiver, amount}    //create a new event pass in updated variables
	transferEventJSON, err := json.Marshal(transferEvent) //json encoding
	if err != nil {
		return fmt.Errorf("failed to obtain JSON encoding: %v", err)

	}
	err = ctx.GetStub().SetEvent("Transfer", transferEventJSON) //check errors for events, read api and setEvent named transfer and pass in json
	if err != nil {
		return fmt.Errorf("failed to set event: %v", err)
	}
	return nil
}

//Delegated transfer
//The transferFrom() function transfers the tokens from an owner's account to the receiver account,
//but only if the transaction initiator has sufficient allowance that has been previously approved by the owner to the transaction initiator
func (s *SmartContract) TransferFrom(ctx contractapi.TransactionContextInterface, from string, receiver string, amount int) error {
	var currentAllowance int //needed to set allowance
	if amount <= 0 {
		return fmt.Errorf("failed amount must be positive integer") //check amount is correct
	}
	spender, err := ctx.GetClientIdentity().GetID() //get spenderID which is the person calling the function, e.g clientID
	if err != nil {
		return fmt.Errorf("failed to get clientID: %v", err)
	}
	//----------------------Current Allowance
	allowanceKey, err := ctx.GetStub().CreateCompositeKey(allowancePrefix, []string{from, spender}) //get allowancekey by creating composite
	if err != nil {
		return fmt.Errorf("failed to create the composite key for prefix %s: %v", allowancePrefix, err)
	}

	currAllowanceTemp, err := ctx.GetStub().GetState(allowanceKey) //getstate accesses the ledger pass in allowance key to verify
	if err != nil {
		return fmt.Errorf("failed to retrieve the allowance for %s from world state: %v", allowanceKey, err)
	}
	currentAllowance, _ = strconv.Atoi(string(currAllowanceTemp)) //error handling not needed since Itoa()
	if currentAllowance <= amount {
		return fmt.Errorf("spender does not have enough allowance to transfer") //check amount vs currentallowance
	}

	// -------------------Initiate the transfer
	err = _transferCalc(ctx, from, receiver, amount)
	if err != nil {
		return fmt.Errorf("failed to transfer:%v", err)
	}
	//decrease the allowance
	updatedAllowance := currentAllowance - amount
	err = ctx.GetStub().PutState(allowanceKey, []byte(strconv.Itoa(updatedAllowance))) //updating the leger with putstate setting allowances
	if err != nil {
		return err
	}
	//emit transfer event
	transferEvent := event{from, receiver, amount} //pass in event data
	transferEventJSON, err := json.Marshal(transferEvent)
	if err != nil {
		return fmt.Errorf("failed to obtain JSON encoding: %v", err)
	}
	err = ctx.GetStub().SetEvent("Transfer", transferEventJSON)
	if err != nil {
		return fmt.Errorf("failed to set event: %v", err)
	}

	log.Printf("spender %s allowance updated from %d to %d", spender, currentAllowance, updatedAllowance) //pring log to user

	return nil
}

//Approving transactions The allowance function tells how many tokens the ownerAddress has allowed the spender address to spend
func (s *SmartContract) Approve(ctx contractapi.TransactionContextInterface, spender string, amount int) error {
	owner, err := ctx.GetClientIdentity().GetID() //get owner id
	if err != nil {
		return fmt.Errorf("failed to get clientID : %v", err)

	}
	allowanceKey, err := ctx.GetStub().CreateCompositeKey(allowancePrefix, []string{owner, spender}) //create key
	if err != nil {
		return fmt.Errorf("failed to create composite key for prefix %s: %v", allowancePrefix, err)
	}
	// Update the state contract by adding the allowanceKey and value
	err = ctx.GetStub().PutState(allowanceKey, []byte(strconv.Itoa(amount)))
	if err != nil {
		return fmt.Errorf("failed to update state of smart contract for key %s: %v", allowanceKey, err)
	}
	//init event approve
	approvalEvent := event{owner, spender, amount}
	approvalEventJSON, err := json.Marshal(approvalEvent)
	if err != nil {
		return fmt.Errorf("failed to obtain JSON encoding: %v", err)
	}
	err = ctx.GetStub().SetEvent("Approval", approvalEventJSON)
	if err != nil {
		return fmt.Errorf("failed to set event: %v", err)
	}
	//log print
	log.Printf("client %s approved a withdrawal allowance of %d for spender %s", owner, amount, spender)

	return nil
}

//The allowance() function returns the token amount remaining
func (s *SmartContract) Allowance(ctx contractapi.TransactionContextInterface, owner string, spender string) (int, error) {
	var allowance int
	//get ledger data create comp key pass in allowancePrefix set above and input datastruct string owner,spender
	allowanceKey, err := ctx.GetStub().CreateCompositeKey(allowancePrefix, []string{owner, spender})
	if err != nil {
		return 0, fmt.Errorf("failed to create composite key fpr %s: %v", allowancePrefix, err)
	}

	//read the allowance amount from the world state
	allowanceTemp, err := ctx.GetStub().GetState(allowanceKey)
	if err != nil {
		return 0, fmt.Errorf("failed to read allowance for %s from world state: %v", allowanceKey, err)
	}
	//cjecl allowance value if nil then we set the allowance to 0 just like balance
	if allowanceTemp == nil {
		allowance = 0
	} else {
		allowance, _ = strconv.Atoi(string(allowanceTemp)) //if we have an allowance then convert to int and get value
	}

	log.Printf("The allowance left for spender %s to withdraw from owner %s: %d", spender, owner, allowance) //display values
	return allowance, nil
}

//**********************************************************************************************
//*********************************Other ERC20 Functions ***************************************
//**********************************************************************************************
//create/add a mintable token suply
func (s *SmartContract) Mint(ctx contractapi.TransactionContextInterface, amount int) error {
	var currentBalance int //setting variables
	var totalSupply int

	verifyClientID, err := ctx.GetClientIdentity().GetMSPID() //check authorization
	if err != nil {
		return fmt.Errorf("failed to verify clientID: %v", err)
	}
	//we assume that the verifying client is ORG1
	if verifyClientID != "Org1MSP" {
		return fmt.Errorf("client %s is not authorized to create new tokens", verifyClientID)
	}
	//we get the ID of the minter
	minter, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return fmt.Errorf("failed to get client id: %v", err)
	}
	if amount <= 0 {
		return fmt.Errorf("amount must be positive integer")
	}

	minterBalance, err := ctx.GetStub().GetState(minter) //get the balance of minter account
	if err != nil {
		return fmt.Errorf("failed to read minter account %s get current balance:%v", minter, err)
	}

	// If minter current balance doesn't yet exist, we'll create it with a current balance of 0
	if minterBalance == nil {
		currentBalance = 0
	} else {
		currentBalance, _ = strconv.Atoi(string(minterBalance)) //if we have a balance then read as string return as int
	}

	updatedBalance := currentBalance + amount                                  //update the balance
	err = ctx.GetStub().PutState(minter, []byte(strconv.Itoa(updatedBalance))) //check err is nil
	if err != nil {
		return err
	}

	//Updating Total supply
	totalSupplyBytes, err := ctx.GetStub().GetState(totalSupplyKey)
	if err != nil {
		return fmt.Errorf("failed to retrieve total token supply: %v", err)
	}
	//set total supply as 0 if no data shown
	if totalSupplyBytes == nil {
		totalSupply = 0
	} else {
		totalSupply, _ = strconv.Atoi(string(totalSupplyBytes))
	}
	//total suuply add
	totalSupply += amount
	err = ctx.GetStub().PutState(totalSupplyKey, []byte(strconv.Itoa(totalSupply)))
	if err != nil {
		return err
	}

	//pull transfer event
	transferEvent := event{"0x0", minter, amount} //0x0 is minter address
	transferEventJSON, err := json.Marshal(transferEvent)
	if err != nil {
		return fmt.Errorf("failed to obtain JSON encoding: %v", err)
	}
	err = ctx.GetStub().SetEvent("Transfer", transferEventJSON) //create event under transfer
	if err != nil {
		return fmt.Errorf("failed to set event: %v", err)
	}

	log.Printf("minter account %s balance updated from %d to %d", minter, currentBalance, updatedBalance)

	return nil
}

//remove from totalsupply deflation option, same as Mint function except we take away from total supply
func (s *SmartContract) Burn(ctx contractapi.TransactionContextInterface, amount int) error {
	var currentBalance int
	var totalSupply int

	verifyClientID, err := ctx.GetClientIdentity().GetMSPID() //check authorization

	if err != nil {
		return fmt.Errorf("failed to verify clientID: %v", err)
	}
	//we assume that the verifying client is ORG1
	if verifyClientID != "Org1MSP" {
		return fmt.Errorf("client %s is not authorized to burn tokens", verifyClientID)
	}
	//we get the ID of the minter/burner
	burner, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return fmt.Errorf("failed to get client id: %v", err)
	}
	if amount <= 0 {
		return fmt.Errorf("amount must be positive integer")
	}
	burnerBalance, err := ctx.GetStub().GetState(burner)
	if err != nil {
		return fmt.Errorf("failed to read burner account %s from state:%v", burner, err)
	}

	// If minter current balance doesn't yet exist, we'll create it with a current balance of 0
	if burnerBalance == nil {
		currentBalance = 0
	} else {
		currentBalance, _ = strconv.Atoi(string(burnerBalance))
	}
	updatedBalance := currentBalance - amount
	err = ctx.GetStub().PutState(burner, []byte(strconv.Itoa(updatedBalance)))
	if err != nil {
		return err
	}

	//UPDATE Total supply
	totalSupplyBytes, err := ctx.GetStub().GetState(totalSupplyKey)
	if err != nil {
		return fmt.Errorf("failed to retrieve total token supply: %v", err)
	}

	if totalSupplyBytes == nil {
		totalSupply = 0
	} else {
		totalSupply, _ = strconv.Atoi(string(totalSupplyBytes)) // Error handling not needed since Itoa() was used when setting the totalSupply, guaranteeing it was an integer.
	}
	//total suuply we TAKE AWAY (Burn)
	totalSupply -= amount
	err = ctx.GetStub().PutState(totalSupplyKey, []byte(strconv.Itoa(totalSupply)))
	if err != nil {
		return err
	}

	//pull transfer event
	//in Ethereum Solidity means 0x0 is the value returned for not-yet created accounts in this case 0x0 would be the main orgs from: json:"from" address. geneis block 0x0
	//FROM, TO , AMOUNT = creation account at 0x0 , to burner account, specified amount
	transferEvent := event{"0x0", burner, amount}
	transferEventJSON, err := json.Marshal(transferEvent)
	if err != nil {
		return fmt.Errorf("failed to obtain JSON encoding: %v", err)
	}
	err = ctx.GetStub().SetEvent("Transfer", transferEventJSON)
	if err != nil {
		return fmt.Errorf("failed to set event: %v", err)
	}

	log.Printf("burner account %s balance updated from %d to %d", burner, currentBalance, updatedBalance)

	return nil
}

//get and verify accountid
// Users can use this function to get their own account id, which they can then give to others as the payment address
func (s *SmartContract) ClientAccountID(ctx contractapi.TransactionContextInterface) (string, error) {
	clientAccountID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return "", fmt.Errorf("failed to get client id: %v", err)
	}

	return clientAccountID, nil
}

//Used to help with transfer function and transferfrom, works out neccessary calcs.
func _transferCalc(ctx contractapi.TransactionContextInterface, from string, receiver string, amount int) error {
	var toCurrentBalance int
	//check to make sure addresses are different
	if from == receiver {
		return fmt.Errorf("failed to and from are both the same addresses ")
	}
	//check values is not negative
	if amount < 0 {
		return fmt.Errorf("failed, amount less than zero")
	}

	//read ledger get currentbalancebytes
	//read client account pass in getstate from address
	//check currentbalance is not nil
	fromCurrentBalanceBytes, err := ctx.GetStub().GetState(from)
	if err != nil {
		return fmt.Errorf("failed to get client account balance: %v", err)
	}
	//convert fromcurrentbalancebytes using strconv.atoi to create fromcurrentbalance
	if fromCurrentBalanceBytes == nil {
		return fmt.Errorf("client account %s has no balance", from)
	}
	fromCurrentBalance, _ := strconv.Atoi(string(fromCurrentBalanceBytes))

	//if fromcurrentbalance less than value fail
	if fromCurrentBalance < amount {
		return fmt.Errorf("failed, client account %s has insufficient funds", from)
	}
	//receiver address read GetStub.Get.State(to)
	//check err
	toCurrentBalanceBytes, err := ctx.GetStub().GetState(receiver)
	if err != nil {
		return fmt.Errorf("failed to get receiver account %s from world state:%v", receiver, err)
	}

	//if no balance for client create a empty one and set to 0
	//toCurrentBalanceBytes =nil then tocurrentbalance=0
	//else toCurrentBalance = atoi .. tocurrentbalancebytes
	if toCurrentBalanceBytes == nil {
		toCurrentBalance = 0
	} else {
		toCurrentBalance, _ = strconv.Atoi(string(toCurrentBalanceBytes))
	}

	//update balances
	//fromupdatedblance fromcurrentbalance - value
	//toupdatedbalance tocurrentbalance + value

	fromUpdatedBalance := fromCurrentBalance - amount
	toUpdatedBalance := toCurrentBalance + amount

	err = ctx.GetStub().PutState(from, []byte(strconv.Itoa(fromUpdatedBalance)))
	if err != nil {
		return err
	}

	err = ctx.GetStub().PutState(receiver, []byte(strconv.Itoa(toUpdatedBalance)))
	if err != nil {
		return err
	}

	log.Printf("client %s %s balance updated from %d to %d", from, TokenName, fromCurrentBalance, fromUpdatedBalance)
	log.Printf("recipient %s %s balance updated from %d to %d", receiver, TokenName, toCurrentBalance, toUpdatedBalance)

	return nil
}
