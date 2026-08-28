package gatekeeper

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

const sameOriginRequestDeniedMessage = "cross-origin request rejected"

// RequireSameOriginFormRequests rejects browser form submissions that present
// an explicit Origin or Referer for a different origin than the current host.
// Requests without either header are allowed for compatibility.
func RequireSameOriginFormRequests() func(http.Handler) http.Handler {
	return RequireSameOriginFormRequestsForProxies(nil)
}

// RequireSameOriginFormRequestsForProxies is RequireSameOriginFormRequests
// with a trusted-proxy list for forwarded proto/host headers.
func RequireSameOriginFormRequestsForProxies(trust *ProxyTrust) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Browsers set this forbidden request header from the actual request URL,
			// so it remains accurate when a reverse proxy terminates HTTPS.
			if strings.EqualFold(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")), "same-origin") {
				next.ServeHTTP(w, r)
				return
			}
			if errOrigin := validateSameOriginHeader(r, r.Header.Get("Origin"), trust); errOrigin != nil {
				http.Error(w, sameOriginRequestDeniedMessage, http.StatusForbidden)
				return
			}
			if errReferer := validateSameOriginHeader(r, r.Header.Get("Referer"), trust); errReferer != nil {
				http.Error(w, sameOriginRequestDeniedMessage, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// IsSameOriginRequest reports whether a request's Origin matches its effective
// request origin, honoring forwarded scheme and host only for trusted proxies.
func IsSameOriginRequest(r *http.Request, trust *ProxyTrust) bool {
	if r == nil {
		return false
	}
	return validateSameOriginHeader(r, r.Header.Get("Origin"), trust) == nil
}

func validateSameOriginHeader(r *http.Request, rawHeader string, trust *ProxyTrust) error {
	rawHeader = strings.TrimSpace(rawHeader)
	if rawHeader == "" {
		return nil
	}

	headerURL, errParse := url.Parse(rawHeader)
	if errParse != nil {
		return errors.New("invalid origin header")
	}
	if headerURL.Scheme == "" || headerURL.Host == "" {
		return errors.New("invalid origin header")
	}

	requestScheme := requestSchemeForSameOrigin(r, trust)
	requestHost := requestHostForSameOrigin(r, trust)
	if !strings.EqualFold(headerURL.Host, requestHost) {
		return errors.New("origin does not match request host")
	}
	if !strings.EqualFold(headerURL.Scheme, requestScheme) {
		directRequestHost := ""
		if r != nil {
			directRequestHost = strings.TrimSpace(r.Host)
		}
		// Limit the untrusted forwarded-proto fallback to HTTPS termination where
		// the browser Origin already matches the request Host.
		forwardedHTTPSOnSameHost := requestScheme == "http" &&
			strings.EqualFold(headerURL.Scheme, "https") &&
			strings.EqualFold(headerURL.Host, directRequestHost) &&
			strings.EqualFold(firstForwardedProto(r), "https")
		if !forwardedHTTPSOnSameHost {
			return errors.New("origin does not match request scheme")
		}
	}

	return nil
}

func requestSchemeForSameOrigin(r *http.Request, trust *ProxyTrust) string {
	if trust.RequestIsHTTPS(r) {
		return "https"
	}
	return "http"
}

func requestHostForSameOrigin(r *http.Request, trust *ProxyTrust) string {
	if r == nil {
		return ""
	}

	requestHost := trust.ForwardedHost(r)
	if requestHost != "" {
		return requestHost
	}

	requestHost = strings.TrimSpace(r.Host)
	if requestHost != "" {
		return requestHost
	}

	return strings.TrimSpace(r.URL.Host)
}

// RequestOrigin returns the externally visible request origin, honoring
// forwarded scheme and host only for configured trusted proxies.
func RequestOrigin(r *http.Request, trust *ProxyTrust) string {
	return (&url.URL{
		Scheme: requestSchemeForSameOrigin(r, trust),
		Host:   requestHostForSameOrigin(r, trust),
	}).String()
}

func requestForwardedHostForSameOrigin(r *http.Request) string {
	forwardedHeader := strings.TrimSpace(r.Header.Get("Forwarded"))
	if forwardedHeader != "" {
		for forwardedValue := range strings.SplitSeq(forwardedHeader, ",") {
			for forwardedParam := range strings.SplitSeq(forwardedValue, ";") {
				paramName, paramValue, ok := strings.Cut(strings.TrimSpace(forwardedParam), "=")
				if !ok {
					continue
				}
				if strings.EqualFold(strings.TrimSpace(paramName), "host") {
					return trimForwardedHeaderValue(paramValue)
				}
			}
		}
	}

	xForwardedHost := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if xForwardedHost != "" {
		if hostValue, _, ok := strings.Cut(xForwardedHost, ","); ok {
			return strings.TrimSpace(hostValue)
		}
		return xForwardedHost
	}

	return ""
}

func trimForwardedHeaderValue(value string) string {
	trimmedValue := strings.TrimSpace(value)
	trimmedValue = strings.Trim(trimmedValue, `"`)
	return trimmedValue
}
