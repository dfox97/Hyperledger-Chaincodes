//Function calls:

/*
Chaincode Invoke Functions:
Create Asset
DeleteAsset
UpdateAsset
ReadAsset

AssetExists
TransferAsset
GetAllAssets


//NEED TO ADD ANOTHER ORG WHCIH INSPECTS AND APPROVES
//func Inspection (ctx , assetID, clientOrgID){
	check asset is owned by clientorgID
	if asset is owned Set approval == true{
		return
	}
}

*/
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-contract-api-go/contractapi" //Provides the smart contract api interface
)

const (
	sellerPrice = "S"
	bidderPrice = "B"
)

//Init SmartContract
type SmartContract struct {
	contractapi.Contract
}

// Asset details (start with capitals) to work with contract api metadata
/*
e.g
ObjectType: "asset"
AssetID: "asset1"
OwnerOrg: "Org1"
PublicDescription: "This asset is owned by ORG1" / "This asset is for sale"
*/
type Asset struct {
	ObjectType        string `json:"objectType"` // ObjectType is used to distinguish different object types in the same chaincode namespace
	ID                string `json:"assetID"`
	OwnerOrg          string `json:"ownerOrg"`
	PublicDescription string `json:"publicDescription"`
}

// ****************************  CreateAsset  *********************************************

/*Creates an asset and sets it as owned by the client's org.
The function checks to see if the asset already exist before taking any action in creating the contract.
Transient data is private to the application-smart contract interaction. It is not recorded on the ledger and is often used in conjunction with private data collections
Transient data is confidental and excluded from the ledger.
Each time the contractapi is passed in a function a transaction context "ctx" is used, from which you can get the chaincode api functions e.g GetStub() , GetState() .
*/
func (s *SmartContract) CreateAsset(ctx contractapi.TransactionContextInterface, assetID, publicDescription string) error {

	transientMap, err := ctx.GetStub().GetTransient() // Transient data is private to the application-smart contract interaction.
	if err != nil {
		return fmt.Errorf("error getting transient: %v", err)
	}

	// Must use transient to access Asset properties  as they are private
	privatePropertiesJSON, key := transientMap["asset_properties"]
	if !key {
		return fmt.Errorf("asset_properties key not found in the private transient map")
	}

	// Verify client id of org and verify it matches peer org id.
	// Client is only authorized to read/write private data from its own peer for this contract.
	clientOrgID, err := _getClientOrgID(ctx, true) //get the client org id from transaction context
	if err != nil {
		return fmt.Errorf("failed to get verified OrgID: %v", err)
	}
	//create asset data from struct Asset
	assetCreate := Asset{
		ObjectType:        "asset",
		ID:                assetID,
		OwnerOrg:          clientOrgID,
		PublicDescription: publicDescription,
	}
	assetBytes, err := json.Marshal(assetCreate) // JSON string from a data structure
	if err != nil {
		return fmt.Errorf("failed to create asset JSON: %v", err)
	}
	//getStub accesses the ledger and requests to update the state to ledger
	err = ctx.GetStub().PutState(assetCreate.ID, assetBytes) //check and verify assetCreated.ID
	if err != nil {
		return fmt.Errorf("failed to put asset in public data: %v", err)
	}

	// add private immutable asset properties to owner's private data collection
	collection := _buildClientOrgName(clientOrgID) //_buildClientOrgName function passing in ownerOrg: clientOrgID
	//call ledger add private data
	err = ctx.GetStub().PutPrivateData(collection, assetCreate.ID, privatePropertiesJSON)

	if err != nil {
		return fmt.Errorf("failed to put Asset private details: %v", err)
	}
	return nil
}

// ******************************* Update Asset  ******************************************

/*Update the asset e.g change public description only callable by the current owner of the asset.
Must verify the ID of the org */

func (s *SmartContract) UpdateAsset(ctx contractapi.TransactionContextInterface, assetID string, newDescription string) error {
	// check client org id matches peer org id not needed, use asset ownership check instead.
	clientOrgID, err := _getClientOrgID(ctx, false)
	if err != nil {
		return fmt.Errorf("failed to get verified OrgID: %v", err)
	}

	assetUpdate, err := s.ReadAsset(ctx, assetID) //Read smartcontract ledger passing in CTX and assetID to modify data
	if err != nil {
		return fmt.Errorf("failed to get asset: %v , check if exists", err)
	}

	// verify to ensure that client org owns the asset
	if clientOrgID != assetUpdate.OwnerOrg {
		return fmt.Errorf("a client from %s cannot update the description of a asset owned by %s", clientOrgID, assetUpdate.OwnerOrg)
	}

	assetUpdate.PublicDescription = newDescription     //set new description
	updatedAssetJSON, err := json.Marshal(assetUpdate) //change json to string
	if err != nil {
		return fmt.Errorf("failed to marshal asset: %v", err)
	}

	return ctx.GetStub().PutState(assetID, updatedAssetJSON) //update ledger changing id and updated description
}

// ******************************* Private functions  ******************************************
//INSPECTOR AND APPROVAL
func _getClientOrgID(ctx contractapi.TransactionContextInterface, verifyOrg bool) (string, error) {
	clientOrgID, err := ctx.GetClientIdentity().GetMSPID() //membershipservice provider ID of organisation e.g {mspid:Org1MSP}
	if err != nil {
		return "", fmt.Errorf("failed getting client's orgID: %v", err)
	}

	if verifyOrg {
		err = _verifyClientOrgMatchesPeerOrg(clientOrgID) //pass into function to verify client
		if err != nil {
			return "", err
		}
	}

	return clientOrgID, nil
}

// *Aproval of transactions, assets and pricing *
// _verifyClientOrgMatchesPeerOrg checks the client org id matches the peer org id.
func _verifyClientOrgMatchesPeerOrg(clientOrgID string) error {
	peerOrgID, err := shim.GetMSPID() //returns the local mspid of the peer by checking the CORE_PEER_LOCALMSPID env var and returns an error if the env var is not set
	if err != nil {
		return fmt.Errorf("failed getting peer's orgID: %v", err)
	}

	if clientOrgID != peerOrgID {
		return fmt.Errorf("client from org %s is not authorized to read or write private data from an org %s peer", clientOrgID, peerOrgID)
	}
	return nil
}

// _setApproval checks that client org currently owns asset and that both parties have agreed on price
//privatePropertiesJSON makes object unable to change
func _SetApproval(ctx contractapi.TransactionContextInterface, asset *Asset, privatePropertiesJSON []byte, clientOrgID string, buyerOrgID string, priceJSON []byte) error {

	// CHECK1: Auth check to ensure that client's org actually owns the asset

	if clientOrgID != asset.OwnerOrg {
		return fmt.Errorf("a client from %s cannot transfer a asset owned by %s", clientOrgID, asset.OwnerOrg)
	}

	// CHECK2: Verify that the hash of the passed immutable properties matches the on-chain hash

	collectionSeller := _buildClientOrgName(clientOrgID)
	setImmutableDataOnChainHash, err := ctx.GetStub().GetPrivateDataHash(collectionSeller, asset.ID)
	if err != nil {
		return fmt.Errorf("failed to read asset private properties hash from seller's collection: %v", err)
	}
	if setImmutableDataOnChainHash == nil {
		return fmt.Errorf("asset private properties hash does not exist: %s", asset.ID)
	}

	hash := sha256.New()
	hash.Write(privatePropertiesJSON)
	calculatedDataHash := hash.Sum(nil)

	// verify that the hash of the passed immutable properties matches the on-chain hash
	if !bytes.Equal(setImmutableDataOnChainHash, calculatedDataHash) {
		return fmt.Errorf("hash %x for passed immutable properties %s does not match on-chain hash %x",
			calculatedDataHash,
			privatePropertiesJSON,
			setImmutableDataOnChainHash,
		)
	}

	// CHECK3: Verify that seller and buyer agreed on the same price

	// Get sellers asking price
	assetForSaleKey, err := ctx.GetStub().CreateCompositeKey(sellerPrice, []string{asset.ID})
	if err != nil {
		return fmt.Errorf("failed to create composite key: %v", err)
	}
	sellerPriceHash, err := ctx.GetStub().GetPrivateDataHash(collectionSeller, assetForSaleKey)
	if err != nil {
		return fmt.Errorf("failed to get seller price hash: %v", err)
	}
	if sellerPriceHash == nil {
		return fmt.Errorf("seller price for %s does not exist", asset.ID)
	}

	// Get buyers bid price
	collectionBuyer := _buildClientOrgName(buyerOrgID)
	assetBidKey, err := ctx.GetStub().CreateCompositeKey(bidderPrice, []string{asset.ID})
	if err != nil {
		return fmt.Errorf("failed to create composite key: %v", err)
	}
	buyerPriceHash, err := ctx.GetStub().GetPrivateDataHash(collectionBuyer, assetBidKey)
	if err != nil {
		return fmt.Errorf("failed to get buyer price hash: %v", err)
	}
	if buyerPriceHash == nil {
		return fmt.Errorf("buyer price for %s does not exist", asset.ID)
	}

	hash = sha256.New()
	hash.Write(priceJSON)
	calculatedPriceHash := hash.Sum(nil)

	// Verify that the hash of the passed price matches the on-chain sellers price hash
	if !bytes.Equal(calculatedPriceHash, sellerPriceHash) {
		return fmt.Errorf("hash %x for passed price JSON %s does not match on-chain hash %x, seller hasn't agreed to the passed trade id and price",
			calculatedPriceHash,
			priceJSON,
			sellerPriceHash,
		)
	}

	// Verify that the hash of the passed price matches the on-chain buyer price hash
	if !bytes.Equal(calculatedPriceHash, buyerPriceHash) {
		return fmt.Errorf("hash %x for passed price JSON %s does not match on-chain hash %x, buyer hasn't agreed to the passed trade id and price",
			calculatedPriceHash,
			priceJSON,
			buyerPriceHash,
		)
	}

	return nil
}

//Get clientorg name used to add and verify to private data collection
func _buildClientOrgName(clientOrgID string) string {
	return fmt.Sprintf("_implicit_org_%s", clientOrgID)
}

//Set State
// _SetTransferAssetState performs the public and private state updates for the transferred asset
//privatePropertiesJSON makes object unable to change
func _SetTransferAssetState(ctx contractapi.TransactionContextInterface, asset *Asset, privatePropertiesJSON []byte, clientOrgID string, buyerOrgID string, price int) error {

	asset.OwnerOrg = buyerOrgID              //set the buyerorgid to the owner in the struct asset
	updatedAsset, err := json.Marshal(asset) //Marshal JSON string from a data structure which will add to the asset structure.
	if err != nil {
		return err
	}

	err = ctx.GetStub().PutState(asset.ID, updatedAsset) //write state PutState(ID, updated asset)
	if err != nil {
		return fmt.Errorf("failed to write asset for buyer: %v", err)
	}

	// Transfer the private properties (delete from seller collection, create in buyer collection)
	collectionSeller := _buildClientOrgName(clientOrgID)
	err = ctx.GetStub().DelPrivateData(collectionSeller, asset.ID)
	if err != nil {
		return fmt.Errorf("failed to delete Asset private details from seller: %v", err)
	}

	collectionBuyer := _buildClientOrgName(buyerOrgID)
	err = ctx.GetStub().PutPrivateData(collectionBuyer, asset.ID, privatePropertiesJSON)
	if err != nil {
		return fmt.Errorf("failed to put Asset private properties for buyer: %v", err)
	}

	// Delete the price records for seller
	assetPriceKey, err := ctx.GetStub().CreateCompositeKey(sellerPrice, []string{asset.ID})
	if err != nil {
		return fmt.Errorf("failed to create composite key for seller: %v", err)
	}

	err = ctx.GetStub().DelPrivateData(collectionSeller, assetPriceKey)
	if err != nil {
		return fmt.Errorf("failed to delete asset price from implicit private data collection for seller: %v", err)
	}

	// Delete the price records for buyer
	assetPriceKey, err = ctx.GetStub().CreateCompositeKey(bidderPrice, []string{asset.ID})
	if err != nil {
		return fmt.Errorf("failed to create composite key for buyer: %v", err)
	}

	err = ctx.GetStub().DelPrivateData(collectionBuyer, assetPriceKey)
	if err != nil {
		return fmt.Errorf("failed to delete asset price from implicit private data collection for buyer: %v", err)
	}

	// Keep record for a 'receipt' in both buyers and sellers private data collection to record the sale price and date.
	// Add function here ???
	//receiptBuyKey, err := ctx.GetStub().CreateCompositeKey(typeAssetBuyReceipt, []string{asset.ID, ctx.GetStub().GetTxID()})

	return nil
}

// * Transactions and pricing *
// approvePrice adds a bid or ask price to caller's implicit private data collection
func approvePrice(ctx contractapi.TransactionContextInterface, assetID string, priceType string) error {
	// client is only authorized to read/write private data from its own data.
	clientOrgID, err := _getClientOrgID(ctx, true) //verify
	if err != nil {
		return fmt.Errorf("failed to verify OrgID: %v", err)
	}

	transMap, err := ctx.GetStub().GetTransient() //get private data
	if err != nil {
		return fmt.Errorf("error getting transient data: %v", err)
	}

	// Asset price must be retrieved from the transient field as they are private
	price, ok := transMap["asset_price"]
	if !ok {
		return fmt.Errorf("asset_price key not found transient map")
	}

	collection := _buildClientOrgName(clientOrgID) //get org id

	// set the agreed price in a collection of sub-namespace on priceType key prefix,
	// Compositekey to avoid collisions between private asset properties, sell price, and buy price
	assetPriceKey, err := ctx.GetStub().CreateCompositeKey(priceType, []string{assetID})
	if err != nil {
		return fmt.Errorf("failed creating composite key: %v", err)
	}

	// The Price hash will be verified later, the persist price bytes are passed as is,
	// so there is no risk of nondeterministic marshaling.
	err = ctx.GetStub().PutPrivateData(collection, assetPriceKey, price)
	if err != nil {
		return fmt.Errorf("failed to put asset bid: %v", err)
	}
	return nil
}

// ******************************* AgreeToSell  ******************************************

// AgreeToSell adds seller's asking price to seller's private data
//Make sure noone authorised can list the item to sell, only the owner can
func (s *SmartContract) AgreeToSell(ctx contractapi.TransactionContextInterface, assetID string) error {
	asset, err := s.ReadAsset(ctx, assetID) //read asset from ledger
	if err != nil {
		return err
	}
	//make sure org is verified for payment
	clientOrgID, err := _getClientOrgID(ctx, true)
	if err != nil {
		return fmt.Errorf("failed to get verified OrgID: %v", err)
	}

	// Verify (inspect and aproval) that this clientOrgId actually owns the asset.
	if clientOrgID != asset.OwnerOrg {
		return fmt.Errorf("a client from %s cannot sell an asset owned by %s", clientOrgID, asset.OwnerOrg)
	}

	return approvePrice(ctx, assetID, sellerPrice)
}

// ******************************* AgreeToBuy ******************************************

// AgreeToBuy adds buyer's bid price to buyer's private data collection
func (s *SmartContract) AgreeToBuy(ctx contractapi.TransactionContextInterface, assetID string) error {
	return approvePrice(ctx, assetID, bidderPrice)
}

// SetInspection verifies asset and allows buyer to validate the properties of
// an asset against the owners private data collection
func (s *SmartContract) SetInspection(ctx contractapi.TransactionContextInterface, assetID string) (bool, error) {
	transMap, err := ctx.GetStub().GetTransient() //private data
	if err != nil {
		return false, fmt.Errorf("error getting transient: %v", err)
	}

	/// Asset properties retrieved from the transient field as they are private
	privatePropertiesJSON, ok := transMap["asset_properties"]
	if !ok {
		return false, fmt.Errorf("asset_properties key not found in the transient map")
	}

	asset, err := s.ReadAsset(ctx, assetID) //find and read asset from ledger
	if err != nil {
		return false, fmt.Errorf("failed to get asset: %v", err)
	}

	collectionOwner := _buildClientOrgName(asset.OwnerOrg) //verifty client org with the asset.ownerOrg from readAsset function
	setImmutableDataOnChainHash, err := ctx.GetStub().GetPrivateDataHash(collectionOwner, assetID)
	if err != nil {
		return false, fmt.Errorf("failed to read asset private properties hash from seller's collection: %v", err)
	}
	//set secure hash e.g the salt tag, so people cant attack and guess the chaincode asset.
	if setImmutableDataOnChainHash == nil {
		return false, fmt.Errorf("asset private properties hash does not exist: %s", assetID)
	}

	hash := sha256.New()
	hash.Write(privatePropertiesJSON) //create hash256 for private data
	calculatedDataHash := hash.Sum(nil)

	// verify hash of the passed immutable properties matches the on-chain hash
	if !bytes.Equal(setImmutableDataOnChainHash, calculatedDataHash) {
		return false, fmt.Errorf("hash %x for passed immutable data %s does not match on-chain hash %x",
			calculatedDataHash,
			privatePropertiesJSON,
			setImmutableDataOnChainHash,
		)
	}

	return true, nil
}

// ******************************* TransferAsset ******************************************

func getClientImplicitCollectionName(ctx contractapi.TransactionContextInterface) (string, error) {
	clientOrgID, err := _getClientOrgID(ctx, true)
	if err != nil {
		return "", fmt.Errorf("failed to get verified OrgID: %v", err)
	}

	err = _verifyClientOrgMatchesPeerOrg(clientOrgID)
	if err != nil {
		return "", err
	}

	return _buildClientOrgName(clientOrgID), nil
}

// TransferAsset checks transfer conditions and then transfers asset state to buyer.
// TransferAsset can only be called by current owner of the asset
func (s *SmartContract) TransferAsset(ctx contractapi.TransactionContextInterface, assetID string, buyerOrgID string) error {
	clientOrgID, err := _getClientOrgID(ctx, false)
	if err != nil {
		return fmt.Errorf("failed to get verified OrgID: %v", err)
	}

	transMap, err := ctx.GetStub().GetTransient() //get private data
	if err != nil {
		return fmt.Errorf("error getting transient data: %v", err)
	}

	privatePropertiesJSON, key := transMap["asset_properties"] //get the description of asset_properties
	if !key {
		return fmt.Errorf("asset_properties key not found in the transient map")
	}

	priceJSON, key := transMap["asset_price"] //get price
	if !key {
		return fmt.Errorf("asset_price key not found in the transient map")
	}

	var agreement Agreement                     //make variable based on agreement struct
	err = json.Unmarshal(priceJSON, &agreement) //string to datastruct pointer to the agreement variable memory address
	if err != nil {
		return fmt.Errorf("failed to unmarshal price JSON: %v", err)
	}

	asset, err := s.ReadAsset(ctx, assetID) //read data
	if err != nil {
		return fmt.Errorf("failed to get asset: %v", err)
	}

	err = _SetApproval(ctx, asset, privatePropertiesJSON, clientOrgID, buyerOrgID, priceJSON) //approve
	if err != nil {
		return fmt.Errorf("failed transfer verification: %v", err)
	}

	err = _SetTransferAssetState(ctx, asset, privatePropertiesJSON, clientOrgID, buyerOrgID, agreement.Price) //set state tp transfer
	if err != nil {
		return fmt.Errorf("failed asset transfer: %v", err)
	}

	return nil
}

func main() {
	//NewChaincode function will error if contracts are invalid e.g. public functions take in illegal types.
	//A system contract is added to the chaincode which provides functionality for getting the metadata of the chaincode.
	chaincode, err := contractapi.NewChaincode(new(SmartContract))
	if err := chaincode.Start(); err != nil {
		log.Panicf("Error starting asset chaincode: %v", err)
	}
	if err != nil {
		log.Panicf("Error create transfer asset chaincode: %v", err)
	}
}
