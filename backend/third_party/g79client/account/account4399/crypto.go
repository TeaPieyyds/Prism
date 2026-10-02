package account4399

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

const aesKey = "lzYW5qaXVqa"

// evpBytesToKey implements OpenSSL EVP_BytesToKey (MD5 variant)
func evpBytesToKey(data, salt []byte, keyLen, ivLen int) ([]byte, []byte) {
	var derived []byte
	lastBlock := []byte{}
	for len(derived) < keyLen+ivLen {
		h := md5.New()
		h.Write(lastBlock)
		h.Write(data)
		h.Write(salt)
		lastBlock = h.Sum(nil)
		derived = append(derived, lastBlock...)
	}
	return derived[:keyLen], derived[keyLen : keyLen+ivLen]
}

// AESEncrypt implements OpenSSL-compatible AES-256-CBC with Salted__ header
func AESEncrypt(plainText, passphrase string) (string, error) {
	salt, err := randomBytes(8)
	if err != nil {
		return "", err
	}

	key, iv := evpBytesToKey([]byte(passphrase), salt, 32, 16)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	padded := pkcs7Pad([]byte(plainText), aes.BlockSize)
	cipherText := make([]byte, len(padded))

	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(cipherText, padded)

	result := append([]byte("Salted__"), salt...)
	result = append(result, cipherText...)

	return base64.StdEncoding.EncodeToString(result), nil
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return b, err
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - len(data)%blockSize
	pad := byte(padLen)
	result := make([]byte, len(data)+padLen)
	copy(result, data)
	for i := len(data); i < len(result); i++ {
		result[i] = pad
	}
	return result
}

// Md5Hex returns the MD5 hex digest of the input
func Md5Hex(data string) string {
	h := md5.Sum([]byte(data))
	return hex.EncodeToString(h[:])
}

// DecodeSaltedAES decrypts OpenSSL Salted__ AES-256-CBC
func DecodeSaltedAES(cipherB64, passphrase string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(cipherB64)
	if err != nil {
		return "", err
	}
	if len(raw) < 16 || string(raw[:8]) != "Salted__" {
		return "", nil // not encrypted or unexpected format
	}
	salt := raw[8:16]
	cipherText := raw[16:]

	key, iv := evpBytesToKey([]byte(passphrase), salt, 32, 16)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	plain := make([]byte, len(cipherText))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(plain, cipherText)

	// Remove PKCS7 padding
	padLen := int(plain[len(plain)-1])
	if padLen > 0 && padLen <= aes.BlockSize {
		plain = plain[:len(plain)-padLen]
	}

	return strings.TrimRight(string(plain), "\x00"), nil
}
