package rpc

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"connectrpc.com/connect"
	gonanoid "github.com/matoous/go-nanoid/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/internal/controller/actions"
	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

var statusPageIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{3,64}$`)

var errPublicStatusPageUnavailable = errors.New("public game server status page unavailable")

// GetOrCreateGameServerStatusPageSettings returns the owner's page settings,
// creating the disabled page on first use.
func (xs *XylonaService) GetOrCreateGameServerStatusPageSettings(
	_ context.Context,
	request *connect.Request[xylona.GetOrCreateGameServerStatusPageSettingsRequest],
) (*connect.Response[xylona.GetOrCreateGameServerStatusPageSettingsResponse], error) {
	owner, errOwner := xs.statusPageOwner(request.Header(), request.Msg.GetOwnerId())
	if errOwner != nil {
		return nil, errOwner
	}

	page, errPage := xs.db.GetGameServerStatusPageByUserID(owner.ID)
	if errors.Is(errPage, sql.ErrNoRows) {
		page, errPage = xs.createGameServerStatusPage(owner)
	}
	if errPage != nil {
		return nil, internalErr()
	}

	settings, errSettings := xs.gameServerStatusPageSettings(owner, page)
	if errSettings != nil {
		return nil, internalErr()
	}
	return connect.NewResponse(&xylona.GetOrCreateGameServerStatusPageSettingsResponse{Settings: settings}), nil
}

// UpdateGameServerStatusPageSettings atomically updates the owner's page and
// the public address override for every owned server.
func (xs *XylonaService) UpdateGameServerStatusPageSettings(
	_ context.Context,
	request *connect.Request[xylona.UpdateGameServerStatusPageSettingsRequest],
) (*connect.Response[xylona.UpdateGameServerStatusPageSettingsResponse], error) {
	owner, errOwner := xs.statusPageOwner(request.Header(), request.Msg.GetOwnerId())
	if errOwner != nil {
		return nil, errOwner
	}
	title, errTitle := normalizeStatusPageTitle(request.Msg.GetTitle())
	if errTitle != nil {
		return nil, invalidArg("title: " + errTitle.Error())
	}
	identifier := request.Msg.GetPublicIdentifier()
	errIdentifier := validateStatusPageIdentifier(identifier)
	if errIdentifier != nil {
		return nil, invalidArg("public_identifier: " + errIdentifier.Error())
	}

	servers, errServers := xs.db.GetGameServersByUser(owner.ID)
	if errServers != nil {
		return nil, internalErr()
	}
	owned := make(map[string]struct{}, len(servers))
	for _, server := range servers {
		owned[server.ID] = struct{}{}
	}
	addresses := make(map[string]string, len(servers))
	for _, value := range request.Msg.GetConnectionAddresses() {
		if value == nil {
			return nil, invalidArg("connection_addresses: entries are required")
		}
		serverID := strings.TrimSpace(value.GetGameServerId())
		_, isOwned := owned[serverID]
		if !isOwned {
			return nil, invalidArg("connection_addresses: game server is not owned by the selected user")
		}
		_, duplicate := addresses[serverID]
		if duplicate {
			return nil, invalidArg("connection_addresses: duplicate game server")
		}
		address, errAddress := normalizePublicConnectionAddress(value.GetPublicConnectionAddress())
		if errAddress != nil {
			return nil, invalidArg("connection_addresses: " + errAddress.Error())
		}
		addresses[serverID] = address
	}
	if len(addresses) != len(servers) {
		return nil, invalidArg("connection_addresses: a complete server set is required")
	}

	page, errUpdate := xs.db.UpdateGameServerStatusPage(db.GameServerStatusPageUpdate{
		UserID:              owner.ID,
		PublicIdentifier:    identifier,
		Title:               title,
		Enabled:             request.Msg.GetEnabled(),
		ConnectionAddresses: addresses,
	})
	if errors.Is(errUpdate, db.ErrGameServerStatusPageIdentifierConflict) {
		return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("public identifier is unavailable"))
	}
	if errors.Is(errUpdate, sql.ErrNoRows) {
		return nil, notFoundErr()
	}
	if errUpdate != nil {
		return nil, internalErr()
	}

	settings, errSettings := xs.gameServerStatusPageSettings(owner, page)
	if errSettings != nil {
		return nil, internalErr()
	}
	return connect.NewResponse(&xylona.UpdateGameServerStatusPageSettingsResponse{Settings: settings}), nil
}

// GetPublicGameServerStatusPage returns the dedicated public projection for
// an enabled page. It reads cached query and process state only.
func (xs *XylonaService) GetPublicGameServerStatusPage(
	_ context.Context,
	request *connect.Request[xylona.GetPublicGameServerStatusPageRequest],
) (*connect.Response[xylona.GetPublicGameServerStatusPageResponse], error) {
	identifier := request.Msg.GetPublicIdentifier()
	page, errPage := xs.publicGameServerStatusPage(identifier, nil)
	if errors.Is(errPage, errPublicStatusPageUnavailable) {
		return nil, statusPageNotFound()
	}
	if errPage != nil {
		return nil, internalErr()
	}
	response := connect.NewResponse(&xylona.GetPublicGameServerStatusPageResponse{
		Page: page,
	})
	response.Header().Set("Cache-Control", "public, max-age=5, stale-while-revalidate=10")
	response.Header().Set("Referrer-Policy", "no-referrer")
	response.Header().Set("X-Robots-Tag", "noindex, nofollow")
	return response, nil
}

func (xs *XylonaService) publicGameServerStatusPage(identifier string, statuses map[string]xylona.Status) (*xylona.PublicGameServerStatusPage, error) {
	errIdentifier := validateStatusPageIdentifier(identifier)
	if errIdentifier != nil {
		return nil, errPublicStatusPageUnavailable
	}
	page, errPage := xs.db.GetEnabledGameServerStatusPageByIdentifier(identifier)
	if errors.Is(errPage, sql.ErrNoRows) {
		return nil, errPublicStatusPageUnavailable
	}
	if errPage != nil {
		return nil, fmt.Errorf("get enabled game server status page: %w", errPage)
	}
	servers, errServers := xs.db.GetGameServersByUser(page.UserID)
	if errServers != nil {
		return nil, fmt.Errorf("get game servers for public status page: %w", errServers)
	}

	queries := &xylona.AllServersQueryInfo{}
	telemetry := func(string) actions.GameServerQueryTelemetrySnapshot {
		return actions.GameServerQueryTelemetrySnapshot{Status: actions.GameServerQueryTelemetryStatusNotYetQueried}
	}
	status := func(server *models.GameServer) xylona.Status {
		value, ok := statuses[server.ID]
		if ok {
			return value
		}
		return xylona.Status_UNKNOWN
	}
	if xs.actionsInst != nil {
		queries = xs.actionsInst.GetServerQueries()
		telemetry = xs.actionsInst.GetGameServerQueryTelemetry
		status = func(server *models.GameServer) xylona.Status {
			value, ok := statuses[server.ID]
			if ok {
				return value
			}
			return xs.actionsInst.GetCachedGameServerStatus(server.ID)
		}
	}
	return projectPublicGameServerStatusPage(page, servers, queries, telemetry, status, xs.versionState), nil
}

func (xs *XylonaService) statusPageOwner(header http.Header, requestedOwnerID string) (*models.User, error) {
	actingUser, errUser := xs.getUserFromHeader(header)
	if errUser != nil {
		return nil, unauthenticated()
	}
	ownerID := strings.TrimSpace(requestedOwnerID)
	if ownerID == "" {
		ownerID = actingUser.ID
	}
	if ownerID != actingUser.ID && !actingUser.SuperUser {
		return nil, permissionDenied("only the owner or a superuser can manage this status page")
	}
	owner, errOwner := xs.db.GetUserByID(ownerID)
	if errOwner != nil {
		return nil, dbLookup(errOwner)
	}
	return owner, nil
}

func (xs *XylonaService) createGameServerStatusPage(owner *models.User) (*db.GameServerStatusPage, error) {
	identifierGenerator := xs.statusPageIdentifier
	if identifierGenerator == nil {
		identifierGenerator = newStatusPageIdentifier
	}
	for range 10 {
		identifier, errIdentifier := identifierGenerator()
		if errIdentifier != nil {
			return nil, fmt.Errorf("generate game server status page identifier: %w", errIdentifier)
		}
		page, errCreate := xs.db.CreateGameServerStatusPage(owner.ID, owner.UserName, identifier)
		if errors.Is(errCreate, db.ErrGameServerStatusPageIdentifierConflict) {
			continue
		}
		if errCreate != nil {
			return nil, fmt.Errorf("create game server status page: %w", errCreate)
		}
		return page, nil
	}
	return nil, errors.New("could not allocate a game server status page identifier")
}

func newStatusPageIdentifier() (string, error) {
	identifier, errIdentifier := gonanoid.New()
	if errIdentifier != nil {
		return "", fmt.Errorf("generate status page identifier: %w", errIdentifier)
	}
	return identifier, nil
}

func (xs *XylonaService) gameServerStatusPageSettings(owner *models.User, page *db.GameServerStatusPage) (*xylona.GameServerStatusPageSettings, error) {
	servers, errServers := xs.db.GetGameServersByUser(owner.ID)
	if errServers != nil {
		return nil, fmt.Errorf("get game servers for status page settings: %w", errServers)
	}
	slices.SortFunc(servers, compareGameServersByName)
	settingsServers := make([]*xylona.GameServerStatusPageSettingsServer, 0, len(servers))
	for _, server := range servers {
		configured := configuredGameServerAddress(server)
		effective := configured
		var publicAddress *string
		if server.PublicConnectionAddress.IsValue() {
			value := server.PublicConnectionAddress.MustGet()
			publicAddress = &value
			effective = value
		}
		settingsServers = append(settingsServers, &xylona.GameServerStatusPageSettingsServer{
			Id:                          server.ID,
			Name:                        server.Name,
			ConfiguredConnectionAddress: configured,
			PublicConnectionAddress:     publicAddress,
			EffectiveConnectionAddress:  effective,
		})
	}
	return &xylona.GameServerStatusPageSettings{
		OwnerId:          owner.ID,
		OwnerName:        owner.UserName,
		Title:            page.Title,
		PublicIdentifier: page.PublicIdentifier,
		Enabled:          page.Enabled,
		PublicPath:       "/status/" + page.PublicIdentifier,
		Servers:          settingsServers,
	}, nil
}

func projectPublicGameServerStatusPage(
	page *db.GameServerStatusPage,
	servers []*models.GameServer,
	queries *xylona.AllServersQueryInfo,
	telemetryFor func(string) actions.GameServerQueryTelemetrySnapshot,
	statusFor func(*models.GameServer) xylona.Status,
	versionState *versiontracker.VersionStateMap,
) *xylona.PublicGameServerStatusPage {
	servers = slices.Clone(servers)
	slices.SortFunc(servers, compareGameServersByName)
	publicServers := make([]*xylona.PublicGameServerStatus, 0, len(servers))
	for _, server := range servers {
		telemetry := telemetryFor(server.ID)
		version := strings.TrimSpace(server.Version)
		if versionState != nil {
			state, ok := versionState.GetWithOK(server.ID)
			if ok {
				version = cmp.Or(strings.TrimSpace(state.InstalledVersionLabel), strings.TrimSpace(state.InstalledVersion), version)
			}
		}
		publicServer := &xylona.PublicGameServerStatus{
			Id:                server.ID,
			Name:              server.Name,
			Status:            statusFor(server),
			ConnectionAddress: effectiveGameServerAddress(server),
			MaxPlayerCount:    uint32FromInt64(server.MaxPlayers),
			RosterState:       xylona.GameServerStatusPageRosterState_GAME_SERVER_STATUS_PAGE_ROSTER_STATE_UNAVAILABLE,
			Version:           version,
		}
		if server.R.Game != nil {
			publicServer.GameName = server.R.Game.Name
		}
		if !telemetry.LastSuccessAt.IsZero() {
			publicServer.ObservedAt = timestamppb.New(telemetry.LastSuccessAt)
		}
		switch telemetry.Status {
		case actions.GameServerQueryTelemetryStatusSuccess:
			if telemetry.PlayerCountValid {
				count := telemetry.PlayerCount
				publicServer.CurrentPlayerCount = &count
			}
			if telemetry.PlayerCapacityValid {
				publicServer.MaxPlayerCount = telemetry.PlayerCapacity
			}
			applyPublicRoster(publicServer, queryForServer(queries, server.ID), telemetry.QueryType)
		case actions.GameServerQueryTelemetryStatusUnsupported:
			publicServer.RosterState = xylona.GameServerStatusPageRosterState_GAME_SERVER_STATUS_PAGE_ROSTER_STATE_UNSUPPORTED
		}
		publicServers = append(publicServers, publicServer)
	}
	return &xylona.PublicGameServerStatusPage{
		Title:       page.Title,
		Servers:     publicServers,
		GeneratedAt: timestamppb.Now(),
	}
}

func applyPublicRoster(publicServer *xylona.PublicGameServerStatus, query *xylona.ServerQuery, queryType xylona.ServerQuery_Type) {
	if query == nil {
		return
	}
	switch queryType {
	case xylona.ServerQuery_Minecraft:
		info := query.GetMinecraft()
		if info == nil || !info.GetResponded() || !info.GetPlayerListSupported() {
			publicServer.RosterState = xylona.GameServerStatusPageRosterState_GAME_SERVER_STATUS_PAGE_ROSTER_STATE_UNSUPPORTED
			return
		}
		publicServer.PlayerNames = slices.Clone(info.GetPlayerList())
	case xylona.ServerQuery_Source:
		info := query.GetSource()
		if info == nil || !info.GetResponded() || !info.GetPlayerListSupported() {
			return
		}
		publicServer.PlayerNames = slices.Clone(info.GetPlayerList())
	case xylona.ServerQuery_Palworld:
		info := query.GetPalworld()
		if info == nil || !info.GetResponded() {
			return
		}
		publicServer.PlayerNames = slices.Clone(info.GetPlayerList())
	default:
		publicServer.RosterState = xylona.GameServerStatusPageRosterState_GAME_SERVER_STATUS_PAGE_ROSTER_STATE_UNSUPPORTED
		return
	}
	publicServer.RosterState = xylona.GameServerStatusPageRosterState_GAME_SERVER_STATUS_PAGE_ROSTER_STATE_AVAILABLE
}

func queryForServer(queries *xylona.AllServersQueryInfo, serverID string) *xylona.ServerQuery {
	if queries == nil {
		return nil
	}
	return queries.GetServers()[serverID]
}

func compareGameServersByName(left *models.GameServer, right *models.GameServer) int {
	nameComparison := strings.Compare(strings.ToLower(left.Name), strings.ToLower(right.Name))
	if nameComparison != 0 {
		return nameComparison
	}
	return strings.Compare(left.ID, right.ID)
}

func configuredGameServerAddress(server *models.GameServer) string {
	return net.JoinHostPort(server.IP, strconv.FormatInt(server.Port, 10))
}

func effectiveGameServerAddress(server *models.GameServer) string {
	if server.PublicConnectionAddress.IsValue() {
		return server.PublicConnectionAddress.MustGet()
	}
	return configuredGameServerAddress(server)
}

func uint32FromInt64(value int64) uint32 {
	if value <= 0 {
		return 0
	}
	if value > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(value)
}

func normalizeStatusPageTitle(value string) (string, error) {
	normalized := strings.TrimSpace(value)
	length := utf8.RuneCountInString(normalized)
	if length < 1 || length > 80 {
		return "", errors.New("must contain 1 to 80 characters")
	}
	return normalized, nil
}

func validateStatusPageIdentifier(value string) error {
	if !statusPageIdentifierPattern.MatchString(value) {
		return errors.New("must be 3 to 64 case-sensitive letters, numbers, underscores, or hyphens")
	}
	return nil
}

func normalizePublicConnectionAddress(value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", nil
	}
	if len(normalized) > 255 {
		return "", errors.New("must contain at most 255 bytes")
	}
	if strings.ContainsAny(normalized, `/\\@`) {
		return "", errors.New("must be a host and port without a scheme, path, or credentials")
	}
	for _, char := range normalized {
		if unicode.IsSpace(char) || unicode.IsControl(char) {
			return "", errors.New("must not contain whitespace or control characters")
		}
	}
	host, portText, errSplit := net.SplitHostPort(normalized)
	if errSplit != nil || host == "" {
		return "", errors.New("must be a valid host:port address")
	}
	port, errPort := strconv.Atoi(portText)
	if errPort != nil || port < 1 || port > 65535 {
		return "", errors.New("port must be between 1 and 65535")
	}
	return normalized, nil
}

func statusPageNotFound() error {
	errNotFound := connect.NewError(connect.CodeNotFound, errors.New("status page unavailable"))
	errNotFound.Meta().Set("Cache-Control", "no-store")
	errNotFound.Meta().Set("Referrer-Policy", "no-referrer")
	errNotFound.Meta().Set("X-Robots-Tag", "noindex, nofollow")
	return errNotFound
}
