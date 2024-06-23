package main

import (
	"crypto/rand"
	"os"
)

func main() {
	key := make([]byte, 32)

	_, err := rand.Read(key)
	if err != nil {
		// handle error here
	}
	os.Stdout.Write(key)
}
