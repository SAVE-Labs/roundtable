package main

import (
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/pion/webrtc/v4"
	"golang.org/x/net/websocket"
)

func main() {
	rooms := NewRoomRegistry()

	e := echo.New()
	e.Use(middleware.RequestLogger())

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete},
	}))

	e.GET("/", func(c *echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "ok",
			"service": "Roundtable WebRTC Server",
		})
	})

	e.POST("/rooms", func(c *echo.Context) error {
		var req struct {
			Name string `json:"name"`
		}
		if err := c.Bind(&req); err != nil || req.Name == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "name required"})
		}
		room := rooms.Create(req.Name)
		return c.JSON(http.StatusCreated, map[string]string{"id": room.id, "name": room.name})
	})

	e.GET("/ws", func(c *echo.Context) error {
		roomID := c.QueryParam("room")
		room, ok := rooms.Get(roomID)
		if !ok {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "room not found"})
		}

		websocket.Handler(func(ws *websocket.Conn) {
			defer ws.Close()
			log.Printf("Client connected: %s", ws.Request().RemoteAddr)

			peer := &Peer{
				id:       uuid.New().String(),
				ws:       ws,
				answerCh: make(chan webrtc.SessionDescription, 1),
			}
			defer room.RemovePeer(peer.id)

			for {
				var sdp webrtc.SessionDescription
				if err := websocket.JSON.Receive(ws, &sdp); err != nil {
					log.Printf("Connection closed: %v", err)
					return
				}

				switch sdp.Type {
				case webrtc.SDPTypeOffer:
					answer, err := handleWebRTCOffer(sdp, room, peer)
					if err != nil {
						log.Printf("Error processing offer: %v", err)
						websocket.JSON.Send(ws, map[string]string{"error": "Failed to process offer"})
						continue
					}
					websocket.JSON.Send(ws, answer)
				case webrtc.SDPTypeAnswer:
					peer.answerCh <- sdp
				}
			}
		}).ServeHTTP(c.Response(), c.Request())
		return nil
	})

	e.Logger.Info("Starting WebRTC server on :1323")
	if err := e.Start(":1323"); err != nil {
		e.Logger.Error("failed to start server", "error", err)
	}
}
