package helpers

import (
	"fmt"
	"testing"

	"github.com/Masterminds/semver"
)

func TestKubernetesVersions(t *testing.T) {
	if _, err := GetKubernetesVersion("stable"); err != nil {
		t.Fatalf("Failed to get stable kubernetes version: %v", err)
		return
	}

	if _, err := GetKubernetesVersion("latest"); err != nil {
		t.Fatalf("Failed to get latest kubernetes version: %v", err)
		return
	}

	ver, err := GetStableUpgradeKubernetesVersion("1.12.4")
	if err != nil {
		t.Fatalf("Failed to get stable kubernetes version: %v", err)
		return
	}

	up, err := semver.NewVersion(ver)
	if err != nil {
		t.Fatalf("Failed to get parse kubernetes version: %v", err)
		return
	}

	found := fmt.Sprintf("%d.%d", up.Major(), up.Minor())

	if found != "1.12" {
		t.Fatalf("Expected: 1.12, Found: %s", found)
		return
	}

	ver, err = GetLatestUpgradeKubernetesVersion("1.12.4")
	if err != nil {
		t.Fatalf("Failed to get stable kubernetes version: %v", err)
		return
	}

	up, err = semver.NewVersion(ver)
	if err != nil {
		t.Fatalf("Failed to get parse kubernetes version: %v", err)
		return
	}

	found = fmt.Sprintf("%d.%d", up.Major(), up.Minor())

	if found != "1.12" {
		t.Fatalf("Expected: 1.12, Found: %s", found)
		return
	}
}
