package kobo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

// UpgradeCheckResult represents an update check result from the Kobo API.
type UpgradeCheckResult struct {
	Data           interface{}
	ReleaseNoteURL string
	UpgradeType    UpgradeType
	UpgradeURL     string
}

var verRe = regexp.MustCompile(`[0-9]+\.[0-9]+(\.[0-9]+)?`)

// ParseVersion tries to extract the version from the UpgradeURL. It returns 0.0.0 if none is present.
func (u UpgradeCheckResult) ParseVersion() string {
	m := verRe.FindString(u.UpgradeURL)
	if !u.UpgradeType.IsUpdate() || m == "" {
		return "0.0.0"
	}
	return m
}

// UpgradeType represents an upgrade type.
type UpgradeType int

// Upgrade types.
const (
	UpgradeTypeNone      UpgradeType = 0 // No upgrade available.
	UpgradeTypeAvailable UpgradeType = 1 // Optional update, but never seen this before.
	UpgradeTypeRequired  UpgradeType = 2 // Automatic update.
)

func (u UpgradeType) String() string {
	switch u {
	case UpgradeTypeNone:
		return "None"
	case UpgradeTypeAvailable:
		return "Available"
	case UpgradeTypeRequired:
		return "Required"
	default:
		return "Unknown (" + strconv.Itoa(int(u)) + ")"
	}
}

// IsUpdate checks if an UpdateType signifies an available update.
func (u UpgradeType) IsUpdate() bool {
	return u != UpgradeTypeNone
}

// CheckUpgrade queries the Kobo API for an update.
func CheckUpgrade(device, affiliate, curVersion, serial string) (*UpgradeCheckResult, error) {
	resp, err := (&http.Client{Timeout: time.Second * 10}).Get(fmt.Sprintf("https://api.kobobooks.com/1.0/UpgradeCheck/Device/%s/%s/%s/%s", device, affiliate, curVersion, serial))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("response status %d", resp.StatusCode)
	}

	var res UpgradeCheckResult
	err = json.NewDecoder(resp.Body).Decode(&res)

	return &res, err
}
