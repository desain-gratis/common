# https://www.scottbrady91.com/openssl/creating-elliptical-curve-keys-using-openssl


# align with:
#
# package main	
#
# import "github.com/golang-jwt/jwt"
#
# func main() {
# 	ecdsapk, err := jwt.ParseECPrivateKeyFromPEM(data)
#   k := jwt.SigningMethodES256
# }
#
#
# 	
# This one compatible with "github.com/golang-jwt/jwt"
#	ecdsapk, err := jwt.ParseECPrivateKeyFromPEM(pem) 
# it will return *ecdsa.PrivateKey type object
openssl ecparam -name prime256v1 -genkey -noout -out private-key.pem

# or better, do we have the  golang ed25519 key?
# crypto/ed25519
# *ed25519.PrivateKey type object. Do the jwt support this?
# openssl genpkey -algorithm ed25519 > private-key.pem


# then save it to keyctl 
# cat private-key.pem | keyctl padd user account-api-dev @u
# keyctl pipe %user:account-api-dev
# later when we generate / run program in local
# cat secret | ./app -c config.hcl 

# debug to get public key
# https://stackoverflow.com/questions/15686821/generate-ec-keypair-from-openssl-command-line
openssl ec -in private-key.pem -pubout -out public.pem