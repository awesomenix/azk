package helpers

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"strings"

	"github.com/Masterminds/semver"
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

func GetKubernetesVersion(version string) (string, error) {
	if version == "stable" || version == "latest" {
		resp, err := http.Get("https://storage.googleapis.com/kubernetes-release/release/" + version + ".txt")
		if err != nil {
			return version, err
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return version, err
		}

		return strings.TrimSpace(string(body[1:])), nil
	}
	return version, nil
}

func GetStableUpgradeKubernetesVersion(version string) (string, error) {
	if version == "stable" || version == "latest" {
		return GetKubernetesVersion(version)
	}
	ver, err := semver.NewVersion(version)
	if err != nil {
		return version, err
	}
	queryVersion := fmt.Sprintf("%d.%d", ver.Major(), ver.Minor())
	resp, err := http.Get("https://storage.googleapis.com/kubernetes-release/release/stable-" + queryVersion + ".txt")
	if err != nil {
		return version, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return version, err
	}

	return strings.TrimSpace(string(body[1:])), nil
}

func GetLatestUpgradeKubernetesVersion(version string) (string, error) {
	if version == "stable" || version == "latest" {
		return GetKubernetesVersion(version)
	}
	ver, err := semver.NewVersion(version)
	if err != nil {
		return version, err
	}
	queryVersion := fmt.Sprintf("%d.%d", ver.Major(), ver.Minor())
	resp, err := http.Get("https://storage.googleapis.com/kubernetes-release/release/latest-" + queryVersion + ".txt")
	if err != nil {
		return version, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return version, err
	}

	return strings.TrimSpace(string(body[1:])), nil
}
