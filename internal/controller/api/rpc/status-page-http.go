package rpc

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ClintonCollins/Xylona/internal/controller/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/internal/eventbus"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

const (
	publicStatusPageEventPath         = "/api/public/status-pages/{identifier}/events"
	publicStatusPagePath              = "/status/{identifier}"
	publicStatusPagePollInterval      = 5 * time.Second
	publicStatusPageStatusOverrideTTL = 2 * publicStatusPagePollInterval
)

var (
	statusPageTitleElement       = regexp.MustCompile(`(?is)<title>.*?</title>`)
	statusPageDescriptionElement = regexp.MustCompile(`(?is)<meta\s+name=["']?description["']?\s+content=["'][^"']*["']\s*/?>`)
)

// GameServerStatusPageHTTPHandler serves status-page HTML, discovery files,
// and the live event stream.
type GameServerStatusPageHTTPHandler struct {
	service  *XylonaService
	frontend fs.FS
	trust    *gatekeeper.ProxyTrust
	mu       sync.Mutex
	total    int
	clientIP map[string]int
}

// NewGameServerStatusPageHTTPHandler prepares the dedicated public handlers.
func NewGameServerStatusPageHTTPHandler(frontend fs.FS, service *XylonaService, trust *gatekeeper.ProxyTrust) *GameServerStatusPageHTTPHandler {
	return &GameServerStatusPageHTTPHandler{
		service:  service,
		frontend: frontend,
		trust:    trust,
		clientIP: make(map[string]int),
	}
}

// RegisterGameServerStatusPageRoutes registers public routes before the SPA fallback.
func RegisterGameServerStatusPageRoutes(router chi.Router, handler *GameServerStatusPageHTTPHandler) {
	router.Get(publicStatusPagePath, handler.StatusPage)
	router.Get(publicStatusPageEventPath, handler.Events)
	router.Get("/robots.txt", handler.Robots)
	router.Get("/sitemap.xml", handler.Sitemap)
}

// StatusPage serves the SPA shell with page-specific, escaped discovery metadata.
func (h *GameServerStatusPageHTTPHandler) StatusPage(w http.ResponseWriter, r *http.Request) {
	index, errRead := fs.ReadFile(h.frontend, "index.html")
	if errRead != nil {
		writeStatusPageError(w)
		return
	}
	identifier := chi.URLParam(r, "identifier")
	page, errPage := h.service.db.GetEnabledGameServerStatusPageByIdentifier(identifier)
	errIdentifier := validateStatusPageIdentifier(identifier)
	if errIdentifier != nil || errors.Is(errPage, sql.ErrNoRows) {
		writeUnavailableStatusPage(w, index)
		return
	}
	if errPage != nil {
		writeStatusPageError(w)
		return
	}

	origin := gatekeeper.RequestOrigin(r, h.trust)
	canonical := origin + "/status/" + identifier
	title := page.Title + " · Xylona status"
	description := "Live game server status for " + page.Title + "."
	body := statusPageTitleElement.ReplaceAll(index, []byte("<title>"+html.EscapeString(title)+"</title>"))
	body = statusPageDescriptionElement.ReplaceAll(body, []byte(`<meta name="description" content="`+html.EscapeString(description)+`">`))
	metadata := `<link rel="canonical" href="` + html.EscapeString(canonical) + `">` +
		`<meta property="og:type" content="website">` +
		`<meta property="og:title" content="` + html.EscapeString(title) + `">` +
		`<meta property="og:description" content="` + html.EscapeString(description) + `">` +
		`<meta property="og:url" content="` + html.EscapeString(canonical) + `">` +
		`<meta name="robots" content="index, follow">`
	body = bytes.Replace(body, []byte("</head>"), []byte(metadata+"</head>"), 1)
	digest := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(digest[:]) + `"`
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("ETag", etag)
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Vary", "Host, X-Forwarded-Host, X-Forwarded-Proto")
	w.Header().Set("X-Robots-Tag", "index, follow")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, errWrite := w.Write(body)
	if errWrite != nil {
		return
	}
}

// Events streams complete public snapshots from cached state.
func (h *GameServerStatusPageHTTPHandler) Events(w http.ResponseWriter, r *http.Request) {
	identifier := chi.URLParam(r, "identifier")
	clientIP := h.trust.ClientIP(r)
	if !h.acquire(clientIP) {
		http.Error(w, "too many status page streams", http.StatusTooManyRequests)
		return
	}
	defer h.release(clientIP)

	page, errPage := h.service.publicGameServerStatusPage(identifier, nil)
	if errors.Is(errPage, errPublicStatusPageUnavailable) {
		writeUnavailableStatusEvent(w)
		return
	}
	if errPage != nil {
		writeStatusPageError(w)
		return
	}

	w.Header().Set("Cache-Control", "no-store, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	_, errRetry := fmt.Fprint(w, "retry: 3000\n")
	if errRetry != nil {
		return
	}
	last, errWrite := writeStatusPageSnapshot(w, page)
	if errWrite != nil {
		return
	}
	errFlush := flushResponse(w)
	if errFlush != nil {
		return
	}

	bus := eventbus.Get()
	statusEvents := bus.SubscribeReliable(eventbus.TopicGameServerStatusChanged)
	defer bus.Unsubscribe(eventbus.TopicGameServerStatusChanged, statusEvents)
	statusOverrides := make(map[string]xylona.Status)
	statusOverrideExpiresAt := make(map[string]time.Time)
	cachedStatus := func(string) xylona.Status { return xylona.Status_UNKNOWN }
	if h.service.actionsInst != nil {
		cachedStatus = h.service.actionsInst.GetCachedGameServerStatus
	}
	poll := time.NewTicker(publicStatusPagePollInterval)
	heartbeat := time.NewTicker(15 * time.Second)
	defer poll.Stop()
	defer heartbeat.Stop()

	refresh := func() bool {
		discardResolvedStatusOverrides(statusOverrides, statusOverrideExpiresAt, time.Now(), cachedStatus)
		next, errSnapshot := h.service.publicGameServerStatusPage(identifier, statusOverrides)
		if errSnapshot != nil {
			return false
		}
		fingerprint, errFingerprint := statusPageSnapshotFingerprint(next)
		if errFingerprint != nil || bytes.Equal(fingerprint, last) {
			return true
		}
		last, errWrite = writeStatusPageSnapshot(w, next)
		if errWrite != nil {
			return false
		}
		errFlush = flushResponse(w)
		return errFlush == nil
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case data, ok := <-statusEvents:
			if !ok {
				return
			}
			statusEvent, valid := data.(eventbus.StatusChangedEvent)
			if !valid {
				continue
			}
			statusValue, known := xylona.Status_value[statusEvent.NewStatus]
			if !known {
				continue
			}
			status := xylona.Status(statusValue)
			statusOverrides[statusEvent.ServerID] = status
			statusOverrideExpiresAt[statusEvent.ServerID] = time.Now().Add(publicStatusPageStatusOverrideTTL)
			if !refresh() {
				return
			}
		case <-poll.C:
			if !refresh() {
				return
			}
		case <-heartbeat.C:
			_, errHeartbeat := fmt.Fprint(w, ": heartbeat\n\n")
			if errHeartbeat != nil {
				return
			}
			errFlush = flushResponse(w)
			if errFlush != nil {
				return
			}
		}
	}
}

// Robots advertises public status pages and the sitemap.
func (h *GameServerStatusPageHTTPHandler) Robots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Vary", "Host, X-Forwarded-Host, X-Forwarded-Proto")
	_, errWrite := fmt.Fprintf(w, "User-agent: *\nDisallow: /\nDisallow: /api/\nDisallow: /shared/\nAllow: /status/\nSitemap: %s/sitemap.xml\n", gatekeeper.RequestOrigin(r, h.trust))
	if errWrite != nil {
		return
	}
}

// Sitemap lists enabled current identifiers only.
func (h *GameServerStatusPageHTTPHandler) Sitemap(w http.ResponseWriter, r *http.Request) {
	pages, errPages := h.service.db.ListEnabledGameServerStatusPages()
	if errPages != nil {
		http.Error(w, "could not build sitemap", http.StatusInternalServerError)
		return
	}
	type sitemapURL struct {
		Location string `xml:"loc"`
	}
	type sitemap struct {
		XMLName xml.Name     `xml:"urlset"`
		XMLNS   string       `xml:"xmlns,attr"`
		URLs    []sitemapURL `xml:"url"`
	}
	origin := gatekeeper.RequestOrigin(r, h.trust)
	result := sitemap{XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9", URLs: make([]sitemapURL, 0, len(pages))}
	for _, page := range pages {
		result.URLs = append(result.URLs, sitemapURL{Location: origin + "/status/" + page.PublicIdentifier})
	}
	body, errMarshal := xml.Marshal(result)
	if errMarshal != nil {
		http.Error(w, "could not build sitemap", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Vary", "Host, X-Forwarded-Host, X-Forwarded-Proto")
	_, errWrite := w.Write(append([]byte(xml.Header), body...))
	if errWrite != nil {
		return
	}
}

func (h *GameServerStatusPageHTTPHandler) acquire(clientIP string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.total >= 512 || h.clientIP[clientIP] >= 10 {
		return false
	}
	h.total++
	h.clientIP[clientIP]++
	return true
}

func (h *GameServerStatusPageHTTPHandler) release(clientIP string) {
	h.mu.Lock()
	h.total--
	h.clientIP[clientIP]--
	if h.clientIP[clientIP] == 0 {
		delete(h.clientIP, clientIP)
	}
	h.mu.Unlock()
}

func writeStatusPageSnapshot(w http.ResponseWriter, page *xylona.PublicGameServerStatusPage) ([]byte, error) {
	body, errMarshal := protojson.Marshal(page)
	if errMarshal != nil {
		return nil, errMarshal
	}
	_, errWrite := fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", body)
	if errWrite != nil {
		return nil, errWrite
	}
	return statusPageSnapshotFingerprint(page)
}

func statusPageSnapshotFingerprint(page *xylona.PublicGameServerStatusPage) ([]byte, error) {
	return protojson.Marshal(&xylona.PublicGameServerStatusPage{Title: page.GetTitle(), Servers: page.GetServers()})
}

func discardResolvedStatusOverrides(
	statuses map[string]xylona.Status,
	expiresAt map[string]time.Time,
	now time.Time,
	cachedStatus func(string) xylona.Status,
) {
	for serverID, status := range statuses {
		if cachedStatus(serverID) == status || !now.Before(expiresAt[serverID]) {
			delete(statuses, serverID)
			delete(expiresAt, serverID)
		}
	}
}

func flushResponse(w http.ResponseWriter) error {
	return http.NewResponseController(w).Flush()
}

func writeUnavailableStatusPage(w http.ResponseWriter, index []byte) {
	body := statusPageTitleElement.ReplaceAll(index, []byte("<title>Status page unavailable</title>"))
	body = statusPageDescriptionElement.ReplaceAll(body, []byte(`<meta name="description" content="This status page is not available.">`))
	body = bytes.Replace(body, []byte("</head>"), []byte(`<meta name="robots" content="noindex, nofollow"></head>`), 1)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Vary", "Host, X-Forwarded-Host, X-Forwarded-Proto")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.WriteHeader(http.StatusNotFound)
	_, errWrite := w.Write(body)
	if errWrite != nil {
		return
	}
}

func writeUnavailableStatusEvent(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	http.Error(w, "status page unavailable", http.StatusNotFound)
}

func writeStatusPageError(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	http.Error(w, "status page unavailable", http.StatusInternalServerError)
}
