package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/pion/webrtc/v4"
	"golang.org/x/net/websocket"
)

func main() {
	e := echo.New()
	e.Use(middleware.RequestLogger())

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete},
	}))

	e.GET("/", func(c *echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "ok",
			"service": "Roundtable WebRTC Echo Server",
		})
	})

	e.GET("/ws", func(c *echo.Context) error {
		websocket.Handler(func(ws *websocket.Conn) {
			defer ws.Close()
			log.Printf("Client connected: %s", ws.Request().RemoteAddr)

			// Handle incoming WebRTC
			for {
				var msgBytes []byte
				if err := websocket.Message.Receive(ws, &msgBytes); err != nil {
					log.Printf("Connection closed: %v", err)
					return
				}

				var offer webrtc.SessionDescription
				if err := json.Unmarshal(msgBytes, &offer); err != nil {
					websocket.JSON.Send(ws, map[string]string{"error": "Invalid offer"})
					continue
				}

				answer, err := handleWebRTCOffer(offer)
				if err != nil {
					log.Printf("Error processing offer: %v", err)
					websocket.JSON.Send(ws, map[string]string{"error": "Failed to process offer"})
					continue
				}

				websocket.JSON.Send(ws, answer)
			}
		}).ServeHTTP(c.Response(), c.Request())
		return nil
	})

	e.Logger.Info("Starting WebRTC echo server on :1323")
	if err := e.Start(":1323"); err != nil {
		e.Logger.Error("failed to start server", "error", err)
	}
}
