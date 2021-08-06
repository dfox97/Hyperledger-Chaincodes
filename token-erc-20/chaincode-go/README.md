
https://github.com/hyperledger/fabric-samples/tree/main/token-erc-20

cd fabric-samples/test-network
#Run test network
./network.sh up createChannel -ca

##Deploy contract##
./network.sh deployCC -ccn token_erc20 -ccp ../token-erc-20/chaincode-go/ -ccl go

#register identity
export PATH=${PWD}/../bin:${PWD}:$PATH
export FABRIC_CFG_PATH=$PWD/../config/
export FABRIC_CA_CLIENT_HOME=${PWD}/organizations/peerOrganizations/org1.example.com/

fabric-ca-client register --caname ca-org1 --id.name minter --id.secret minterpw --id.type client --tls.certfiles "${PWD}/organizations/fabric-ca/org1/tls-cert.pem"

fabric-ca-client enroll -u https://minter:minterpw@localhost:7054 --caname ca-org1 -M "${PWD}/organizations/peerOrganizations/org1.example.com/users/minter@org1.example.com/msp" --tls.certfiles "${PWD}/organizations/fabric-ca/org1/tls-cert.pem"

cp "${PWD}/organizations/peerOrganizations/org1.example.com/msp/config.yaml" "${PWD}/organizations/peerOrganizations/org1.example.com/users/minter@org1.example.com/msp/config.yaml"

#ORG2 Terminal 2

cd fabric-samples/test-network
export PATH=${PWD}/../bin:${PWD}:$PATH
export FABRIC_CA_CLIENT_HOME=${PWD}/organizations/peerOrganizations/org2.example.com/
fabric-ca-client register --caname ca-org2 --id.name recipient --id.secret recipientpw --id.type client --tls.certfiles "${PWD}/organizations/fabric-ca/org2/tls-cert.pem"

fabric-ca-client enroll -u https://recipient:recipientpw@localhost:8054 --caname ca-org2 -M "${PWD}/organizations/peerOrganizations/org2.example.com/users/recipient@org2.example.com/msp" --tls.certfiles "${PWD}/organizations/fabric-ca/org2/tls-cert.pem"

cp "${PWD}/organizations/peerOrganizations/org2.example.com/msp/config.yaml" "${PWD}/organizations/peerOrganizations/org2.example.com/users/recipient@org2.example.com/msp/config.yaml"

#Mint new tokens
##ORG1
export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID="Org1MSP"
export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org1.example.com/users/minter@org1.example.com/msp
export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
export CORE_PEER_ADDRESS=localhost:7051
export TARGET_TLS_OPTIONS=(-o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --tls --cafile "${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem" --peerAddresses localhost:7051 --tlsRootCertFiles "${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt" --peerAddresses localhost:9051 --tlsRootCertFiles "${PWD}/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt")


peer chaincode invoke "${TARGET_TLS_OPTIONS[@]}" -C mychannel -n token_erc20 -c '{"function":"Mint","Args":["5000"]}'


peer chaincode query -C mychannel -n token_erc20 -c '{"function":"ClientAccountBalance","Args":[]}'

#Transfer 

##ORG2
export FABRIC_CFG_PATH=$PWD/../config/
export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID="Org2MSP"
export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org2.example.com/users/recipient@org2.example.com/msp
export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt
export CORE_PEER_ADDRESS=localhost:9051

peer chaincode query -C mychannel -n token_erc20 -c '{"function":"ClientAccountID","Args":[]}'


echo eDUwOTo6Q049cmVjaXBpZW50LE9VPWNsaWVudCxPPUh5cGVybGVkZ2VyLFNUPU5vcnRoIENhcm9saW5hLEM9VVM6OkNOPWNhLm9yZzIuZXhhbXBsZS5jb20sTz1vcmcyLmV4YW1wbGUuY29tLEw9SHVyc2xleSxTVD1IYW1wc2hpcmUsQz1VSw== | base64 --decode

x509::CN=recipient,OU=client,O=Hyperledger,ST=North Carolina,C=US::CN=ca.org2.example.com,O=org2.example.com,L=Hursley,ST=Hampshire,C=UK


export RECIPIENT="eDUwOTo6Q049cmVjaXBpZW50LE9VPWNsaWVudCxPPUh5cGVybGVkZ2VyLFNUPU5vcnRoIENhcm9saW5hLEM9VVM6OkNOPWNhLm9yZzIuZXhhbXBsZS5jb20sTz1vcmcyLmV4YW1wbGUuY29tLEw9SHVyc2xleSxTVD1IYW1wc2hpcmUsQz1VSw=="
peer chaincode invoke "${TARGET_TLS_OPTIONS[@]}" -C mychannel -n token_erc20 -c '{"function":"Transfer","Args":[ "'"$RECIPIENT"'","100"]}'


##org1
peer chaincode query -C mychannel -n token_erc20 -c '{"function":"ClientAccountBalance","Args":[]}'

##org2
peer chaincode query -C mychannel -n token_erc20 -c '{"function":"ClientAccountBalance","Args":[]}'