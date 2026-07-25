package supervisor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/node/rcon"
	"github.com/ClintonCollins/Xylona/internal/node/restinput"
)

func executeRemoteInput(ctx context.Context, inputMethod InputMethod, input string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	switch inputMethod.Type {
	case InputTypeRCON:
		credentials := inputMethod.RCONCredentials
		if credentials == nil {
			return "", ErrRemoteInputConfiguration
		}
		protocol, errProtocol := rconProtocol(credentials.Protocol)
		if errProtocol != nil {
			return "", errProtocol
		}
		client := rcon.Client{
			Address:  net.JoinHostPort(credentials.Host, strconv.Itoa(credentials.Port)),
			Password: credentials.Password,
			Protocol: protocol,
		}
		response, errExecute := client.Execute(ctx, input)
		if errExecute != nil {
			return "", fmt.Errorf("execute RCON input: %w", errExecute)
		}
		return response, nil
	case InputTypeREST:
		credentials := inputMethod.RESTCredentials
		if credentials == nil {
			return "", ErrRemoteInputConfiguration
		}
		var response string
		var errExecute error
		switch credentials.Kind {
		case RESTInputKindSatisfactory:
			response, errExecute = restinput.ExecuteSatisfactory(
				ctx,
				credentials.Host,
				credentials.Port,
				credentials.Password,
				input,
			)
		case RESTInputKindPalworld:
			response, errExecute = restinput.ExecutePalworld(
				ctx,
				credentials.Host,
				credentials.Port,
				credentials.Password,
				input,
			)
		default:
			return "", ErrRemoteInputConfiguration
		}
		if errExecute != nil {
			if errors.Is(errExecute, restinput.ErrPalworldCommandRejected) {
				return "", &ConsoleInputRejectedError{Detail: errExecute.Error()}
			}
			return "", fmt.Errorf("execute REST input: %w", errExecute)
		}
		return response, nil
	default:
		return "", errors.New("console input is not a remote transport")
	}
}

func rconProtocol(protocol RCONProtocol) (rcon.Protocol, error) {
	switch protocol {
	case RCONProtocolSource:
		return rcon.ProtocolSource, nil
	case RCONProtocolMinecraft:
		return rcon.ProtocolMinecraft, nil
	case RCONProtocolRustWeb:
		return rcon.ProtocolRustWeb, nil
	default:
		return 0, ErrRemoteInputConfiguration
	}
}

func configureSatisfactoryAdminPasswordAfterStartup(
	ctx context.Context,
	commandID string,
	gameServerName string,
	credentials RESTCredentials,
) {
	const retryInterval = 3 * time.Second

	for {
		errConfigure := restinput.ConfigureSatisfactoryAdminPassword(
			ctx,
			credentials.Host,
			credentials.Port,
			gameServerName,
			credentials.Password,
			credentials.PreviousPasswords,
		)
		if errConfigure == nil {
			log.Info().Str("command_id", commandID).Msg("Configured Satisfactory admin interface password")
			return
		}

		timer := time.NewTimer(retryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			log.Debug().Err(errConfigure).Str("command_id", commandID).
				Msg("Stopped retrying Satisfactory admin interface setup")
			return
		case <-timer.C:
		}
	}
}
