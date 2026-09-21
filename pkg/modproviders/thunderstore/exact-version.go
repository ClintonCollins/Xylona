package thunderstore

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ExactPackage describes one package version and its dependencies.
type ExactPackage struct {
	ID           string
	Version      string
	Dependencies []string
}

// ExactVersion fetches metadata for the requested version without consulting latest metadata.
func (p *Provider) ExactVersion(ctx context.Context, sourceIDValue string, version string) (ExactPackage, error) {
	reference, errReference := parseSourceID(sourceIDValue)
	if errReference != nil {
		return ExactPackage{}, fmt.Errorf("thunderstore exact version: %w", errReference)
	}
	version = strings.TrimSpace(version)
	if !isSafeIdentifier(version) {
		return ExactPackage{}, fmt.Errorf("thunderstore exact version: invalid version %q", version)
	}
	endpoint := fmt.Sprintf("%s/api/experimental/package/%s/%s/%s/", p.baseURL,
		url.PathEscape(reference.namespace), url.PathEscape(reference.name), url.PathEscape(version))
	var response struct {
		Namespace    string   `json:"namespace"`
		Name         string   `json:"name"`
		Version      string   `json:"version_number"`
		Dependencies []string `json:"dependencies"`
	}
	errFetch := p.getJSON(ctx, endpoint, &response)
	if errFetch != nil {
		return ExactPackage{}, fmt.Errorf("thunderstore exact version: %w", errFetch)
	}
	if response.Namespace != reference.namespace || response.Name != reference.name || response.Version != version {
		return ExactPackage{}, fmt.Errorf("thunderstore exact version: response does not match requested package %q version %q", sourceIDValue, version)
	}
	if response.Dependencies == nil {
		return ExactPackage{}, errors.New("thunderstore exact version: missing dependency metadata")
	}
	return ExactPackage{
		ID: sourceID(response.Namespace, response.Name), Version: response.Version,
		Dependencies: response.Dependencies,
	}, nil
}
