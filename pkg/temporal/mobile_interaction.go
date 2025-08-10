package temporal

import (
	"fmt"
	"log/slog"
)

type MobileInteractionActivities struct {
	Logger              *slog.Logger
	FuncDetectLoginLoop func(int) error
}

// Activities

func NewMobileInteractionActivities(logger *slog.Logger) MobileInteractionActivities {
	return MobileInteractionActivities{
		Logger: logger,
		FuncDetectLoginLoop: func(i int) error {
			fmt.Println("I:", i)
			return nil
		},
	}
}

func (mia MobileInteractionActivities) DetectLoginLoop() {
	mia.Logger.Info("INSIDE: DetectLoginLoop")
	err := mia.FuncDetectLoginLoop(1)
	if err != nil {
		panic(err)
	}
}

// Workflows ..
