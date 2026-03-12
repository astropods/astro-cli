package envelope

// NewTestEncryptor creates an Encryptor from a raw AES-256 key for testing.
// The key is used directly (no KMS call). EncryptedDataKey is set to the raw key
// so NewTestDecryptor can reconstruct the cipher.
func NewTestEncryptor(key []byte) (*Encryptor, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	enc := make([]byte, len(key))
	copy(enc, key)
	return &Encryptor{
		gcm:              gcm,
		EncryptedDataKey: enc,
		KMSKeyARN:        "arn:aws:kms:test:000:key/test",
	}, nil
}

// NewTestDecryptor creates a Decryptor from a raw AES-256 key for testing.
// The encryptedDataKey parameter is ignored; the raw key is used directly.
func NewTestDecryptor(key, _ []byte) (*Decryptor, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	return &Decryptor{gcm: gcm}, nil
}
