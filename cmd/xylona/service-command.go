package main

import (
	"fmt"

	"github.com/ClintonCollins/Xylona/internal/appservice"
)

const (
	controllerServiceName        = "Xylona"
	controllerServiceUnitName    = "xylona.service"
	controllerServiceDisplayName = "Xylona"
	controllerServiceDescription = "Xylona game server control panel"
)

func controllerServiceDefinition(arguments ...string) appservice.Definition {
	return appservice.Definition{
		Name:        controllerServiceName,
		UnitName:    controllerServiceUnitName,
		DisplayName: controllerServiceDisplayName,
		Description: controllerServiceDescription,
		Arguments:   append([]string(nil), arguments...),
	}
}

func writeServiceOutput(format string, values ...any) error {
	_, errWrite := fmt.Fprintf(rootCLIStdout, format+"\n", values...)
	if errWrite != nil {
		return fmt.Errorf("write service command output: %w", errWrite)
	}
	return nil
}
