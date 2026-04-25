// Package query provides Source and Minecraft server query helpers.
package query

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/dreamscached/minequery/v2"
	"github.com/rs/zerolog/log"
	"github.com/rumblefrog/go-a2s"

	"github.com/ClintonCollins/Xylona/pkg/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

var (
	// ErrMinecraftServerUnreachable indicates no supported query protocol succeeded.
	ErrMinecraftServerUnreachable = errors.New("minecraft server unreachable")
)

// type MinecraftInfo struct {
//	Host            string
//	Port            int
//	MOTD            string
//	GameType        string
//	Map             string
//	NumberOfPlayers int
//	MaxPlayers      int
//	PlayerList      []string
//	ProtocolVersion int
//	ServerVersion   string
// }

// type SourceInfo struct {
//	Protocol   uint8  `json:"Protocol"`
//	Name       string `json:"Name"`
//	Map        string `json:"Map"`
//	Game       string `json:"Game"`
//	AppID      uint16 `json:"AppID"`
//	SteamID    uint64 `json:"SteamID"`
//	GameID     uint64 `json:"GameID"`
//	Players    uint8  `json:"Players"`
//	MaxPlayers uint8  `json:"MaxPlayers"`
//	Bots       uint8  `json:"Bots"`
//	ServerOS   string `json:"ServerOS"`
//	Visibility bool   `json:"Visibility"`
//	VAC        bool   `json:"VAC"`
//	Version    string `json:"Version"`
// }

// Source queries a Source-engine server and returns its info payload.
func Source(host string, port int) (*xylona.SourceQueryInfo, error) {
	conn, errNewClient := a2s.NewClient(net.JoinHostPort(host, strconv.Itoa(port)), a2s.TimeoutOption(time.Second*1))

	if errNewClient != nil {
		return nil, fmt.Errorf("create source query client for %s:%d: %w", host, port, errNewClient)
	}

	defer func(client *a2s.Client) {
		errClose := client.Close()
		if errClose != nil {
			log.Error().Err(errClose).Msg("Failed to close client")
		}
	}(conn)

	info, errQuery := conn.QueryInfo()

	if errQuery != nil {
		return nil, fmt.Errorf("query source server %s:%d: %w", host, port, errQuery)
	}
	sourceInfo := &xylona.SourceQueryInfo{
		Protocol:   uint32(info.Protocol),
		Name:       info.Name,
		Map:        info.Map,
		Game:       info.Game,
		AppId:      uint32(info.ID),
		Players:    uint32(info.Players),
		MaxPlayers: uint32(info.MaxPlayers),
		Bots:       uint32(info.Bots),
		ServerOs:   info.ServerOS.String(),
		Visibility: info.Visibility,
		Vac:        info.VAC,
		Version:    info.Version,
	}
	if info.ExtendedServerInfo != nil {
		sourceInfo.SteamId = info.ExtendedServerInfo.SteamID
		sourceInfo.GameId = info.ExtendedServerInfo.GameID
	}

	return sourceInfo, nil
}

// Minecraft queries a Minecraft server using the supported ping protocols.
func Minecraft(host string, port int) (*xylona.MinecraftQueryInfo, error) {
	conn := minequery.NewPinger(minequery.WithTimeout(time.Second * 1))
	respQuery, errQuery := conn.QueryFull(host, port)
	if errQuery == nil {
		return &xylona.MinecraftQueryInfo{
			Motd:            respQuery.MOTD,
			GameType:        respQuery.GameType,
			Map:             respQuery.Map,
			NumberOfPlayers: helpers.ClampUint32FromInt(respQuery.OnlinePlayers),
			MaxPlayers:      helpers.ClampUint32FromInt(respQuery.MaxPlayers),
			PlayerList:      respQuery.SamplePlayers,
			ServerVersion:   respQuery.Version,
		}, nil
	}
	resp17, errPing17 := conn.Ping17(host, port)
	if errPing17 == nil {
		playerList := make([]string, len(resp17.SamplePlayers))
		for i, player := range resp17.SamplePlayers {
			playerList[i] = player.Nickname
		}
		return &xylona.MinecraftQueryInfo{
			Motd:            resp17.Description.String(),
			NumberOfPlayers: helpers.ClampUint32FromInt(resp17.OnlinePlayers),
			MaxPlayers:      helpers.ClampUint32FromInt(resp17.MaxPlayers),
			PlayerList:      playerList,
			ProtocolVersion: helpers.ClampUint32FromInt(resp17.ProtocolVersion),
			ServerVersion:   resp17.VersionName,
		}, nil
	}
	resp16, errPing16 := conn.Ping16(host, port)
	if errPing16 == nil {
		return &xylona.MinecraftQueryInfo{
			Motd:            resp16.MOTD,
			NumberOfPlayers: helpers.ClampUint32FromInt(resp16.OnlinePlayers),
			MaxPlayers:      helpers.ClampUint32FromInt(resp16.MaxPlayers),
			ProtocolVersion: helpers.ClampUint32FromInt(resp16.ProtocolVersion),
			ServerVersion:   resp16.ServerVersion,
		}, nil
	}
	respBeta, errPingBeta := conn.PingBeta18(host, port)
	if errPingBeta == nil {
		return &xylona.MinecraftQueryInfo{
			Motd:            respBeta.MOTD,
			NumberOfPlayers: helpers.ClampUint32FromInt(respBeta.OnlinePlayers),
			MaxPlayers:      helpers.ClampUint32FromInt(respBeta.MaxPlayers),
		}, nil
	}
	return nil, ErrMinecraftServerUnreachable
}
