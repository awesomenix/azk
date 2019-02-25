package helpers

import (
	"math/rand"
)

var letterRunes = []rune("0123456789abcdef")

// generateRandomHexString is a convenience function for generating random strings of an arbitrary length.
func GenerateRandomHexString(length int) string {
	b := make([]rune, length)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func ContainsFinalizer(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func RemoveFinalizer(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}
