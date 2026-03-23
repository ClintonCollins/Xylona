package supervisor

import (
	"errors"
	"syscall"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func checkErrorAccessDenied(err error, command *Command) {
	if !errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
		log.Error().Err(err).Msg("Error attempting to stop server.")
		oldStatus := command.Status()
		command.sendJobStatusNotification(oldStatus, xylona.Status_OFFLINE)
		command.sendJobNotification(err.Error())
	}
}
