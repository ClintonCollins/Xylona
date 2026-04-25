package supervisor

import (
	"errors"
	"syscall"

	"github.com/rs/zerolog/log"
)

func checkErrorAccessDenied(err error, command *Command) {
	if !errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
		log.Error().Err(err).Msg("Error attempting to stop server.")
		command.sendJobNotification(err.Error())
	}
}
