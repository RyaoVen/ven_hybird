package main

import (
	"os"
	"time"
	"ven_hybird/router"

	"github.com/gofiber/fiber/v2"
)

var ControllerConfig router.ControllerConfig = router.ControllerConfig{
	FiberConfig: fiber.Config{
		AppName:               "VenHybird",
		ReadTimeout:           10 * time.Second,
		WriteTimeout:          10 * time.Second,
		IdleTimeout:           120 * time.Second,
		DisableStartupMessage: os.Getenv("APP_ENV") == "production",
	},
}
