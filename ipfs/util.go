package ipfs

import (
	"io/ioutil"
	"os"

	crypto "github.com/libp2p/go-libp2p-crypto"
)

func GenKeyIfNotExists(privateKeyFile string) (crypto.PrivKey, error) {
	_, err := os.Stat(privateKeyFile)
	if err != nil {
		private, pub, err := crypto.GenerateKeyPair(crypto.RSA, 2048)
		pub2 := private.GetPublic()
		if !pub.Equals(pub2) {
			panic("What?!")
		}
		b, err := crypto.MarshalPrivateKey(private)
		if err != nil {
			return nil, err
		}
		err = ioutil.WriteFile(privateKeyFile, b, 0400)
		if err != nil {
			return nil, err
		}
	}
	bytes, err := ioutil.ReadFile(privateKeyFile)
	if err != nil {
		return nil, err
	}
	pk, err := crypto.UnmarshalPrivateKey(bytes)
	if err != nil {
		return nil, err
	}
	return pk, nil
}
