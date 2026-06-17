package main

import (
	"embed"
	"log"

	"github.com/SAVE-Labs/roundtable/desktop/services"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	log.Println("starting: creating services")
	roomsSvc := services.NewRoomsService()
	audioSvc := services.NewAudioService()
	sessionSvc := services.NewSessionService()
	configSvc := services.NewConfigService()
	eventsSvc := services.NewEventsService()
	log.Println("starting: services created")

	app := application.New(application.Options{
		Name:        "Roundtable",
		Description: "Voice channels for your team",
		Services: []application.Service{
			application.NewService(roomsSvc),
			application.NewService(audioSvc),
			application.NewService(sessionSvc),
			application.NewService(configSvc),
			application.NewService(eventsSvc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
	})

	log.Println("starting: app created, creating window")
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Roundtable",
		Width:            1200,
		Height:           750,
		BackgroundColour: application.NewRGBA(18, 18, 18, 255),
		URL:              "/",
	})

	log.Println("starting: window created, calling app.Run()")
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
	log.Println("app exited cleanly")
}
