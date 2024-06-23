package encryption

import (
	"reflect"
	"testing"
)

func Test_encrypt_decrypt(t *testing.T) {

	key := []byte{12, 12, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31}

	wantResult := []byte("Hello world, I will try to create a very long message. Whether it can do encrypt hehe")
	encrypted, iv, err := Encrypt(key, wantResult)
	t.Log("LEN", len(encrypted))
	if err != nil {
		t.Errorf("ERR: %v", err)
	}
	gotResult, err := Decrypt(key, iv, encrypted)
	if err != nil {
		t.Errorf("ERR: %v", err)
	}

	if !reflect.DeepEqual(gotResult, wantResult) {
		t.Errorf("encrypt() decrypt() gotResult = %v, want %v", gotResult, wantResult)
	}
}
