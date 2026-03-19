package gsutils

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/tidwall/gjson"
)

type SteamAppBranch struct {
	Name        string `json:"name,omitempty"`
	BuildID     string `json:"buildid,omitempty"`
	Description string `json:"description,omitempty"`
	TimeUpdated string `json:"timeupdated,omitempty"`
}

type xylonaTransport struct{}

var (
	httpClient = &http.Client{
		Timeout:   time.Second * 15,
		Transport: xylonaTransport{},
	}
)

func (x xylonaTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Add("User-Agent", "Xylona/0.1 (https://github.com/ClintonCollins/Xylona)")
	return http.DefaultTransport.RoundTrip(req)
}

func SteamGetLatestVersionByAppID(appID string) (string, error) {
	branchMap, errGetBranchMap := SteamGetBranchesByAppID(appID)
	if errGetBranchMap != nil {
		log.Error().Err(errGetBranchMap).Msg("Failed to get branch map")
		return "", errGetBranchMap
	}

	public, exists := branchMap["public"]
	if !exists {
		log.Error().Msg("Failed to find public branch")
		return "", errors.New("failed to find public branch")
	}
	latestBranch := public
	for branch, branchInfo := range branchMap {
		log.Debug().Msgf("Branch: %s, BuildID: %s, Description: %s, TimeUpdated: %s", branch, branchInfo.BuildID, branchInfo.Description, branchInfo.TimeUpdated)
		if branch == "public" {
			continue
		}
		if branchInfo.BuildID == public.BuildID {
			latestBranch = branchInfo
			break
		}
	}

	latestVersion := latestBranch.Description
	if latestVersion == "" {
		latestVersion = latestBranch.Name
	}

	log.Debug().Msgf("Latest version: %s", latestVersion)
	return latestVersion, nil
}

func SteamGetBranchesByAppID(appID string) (map[string]SteamAppBranch, error) {
	resp, err := httpClient.Get("https://api.steamcmd.net/v1/info/" + appID) //nolint:noctx // TODO: refactor to use http.NewRequestWithContext
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, errReadBody := io.ReadAll(resp.Body)
	if errReadBody != nil {
		log.Error().Err(errReadBody).Msg("Failed to read body")
		return nil, errReadBody
	}

	gJsonMatcher := fmt.Sprintf("data.%s.depots.branches", appID)
	branches := gjson.GetBytes(body, gJsonMatcher)
	if !branches.Exists() {
		log.Error().Str("JSON matcher", gJsonMatcher).Msg("Failed to get branches. Branches not found.")
		return nil, fmt.Errorf("failed to get branches")
	}

	branchMap := make(map[string]SteamAppBranch)

	branches.ForEach(func(branch, branchInfo gjson.Result) bool {
		buildID := branchInfo.Get("buildid").String()
		description := branchInfo.Get("description").String()
		timeUpdated := branchInfo.Get("timeupdated").String()
		branchMap[branch.String()] = SteamAppBranch{
			Name:        branch.String(),
			BuildID:     buildID,
			Description: description,
			TimeUpdated: timeUpdated,
		}
		return true
	})

	return branchMap, nil
}
