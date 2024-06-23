package secretmanager

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"hash"
	"io"
	"io/ioutil"
	"os"
)

// GetFromPEM utility function to obtain RSA key from file
func GetFromPEM(filePath string) (key *rsa.PrivateKey, err error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}

	return getFromPEM(f)
}

func getFromPEM(reader io.Reader) (key *rsa.PrivateKey, err error) {
	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	// https://stackoverflow.com/questions/44230634/how-to-read-an-rsa-key-from-file/44231740
	der, _ := pem.Decode(data)

	return x509.ParsePKCS1PrivateKey(der.Bytes)
}

// TODO UNIT TEST IF READER IS SMALL, LARGE, EMPTY, INVALID, ETC.
// TODO PROVIDE OPENSSL COMMAND TO GENERATE THE PRIVATE KEY FROM OUTSIDE GOLANG

func Encrypt(pk *rsa.PublicKey, msg []byte) (result []byte, err error) {
	return nil, nil
}

// https://dev.to/0xbf/golang-derive-fingerprint-from-ssl-cert-file-2o14
func Fingerprint(key []byte, hashAlgo hash.Hash) string {
	return ""
}
