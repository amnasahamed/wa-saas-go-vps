package main

import "golang.org/x/crypto/bcrypt"

// bcryptGenerate is a tiny helper so we don't have to import bcrypt
// across every handler file.
func bcryptGenerate(plain string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), 10)
	if err != nil {
		return "", err
	}
	return string(h), nil
}
