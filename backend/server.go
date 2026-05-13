package main

import (
	"database/sql"
	"embed"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	_ "github.com/jackc/pgx/v5/stdlib" // postgres driver for database/sql
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/pion/webrtc/v4"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // sqlite driver for database/sql

	"roundtable/backend/db"
)

var version = "0.1.0"

//go:embed db/migrations/*.sql
var embedMigrations embed.FS

func openDB(dsn string) (*sql.DB, string, error) {
	if strings.HasPrefix(dsn, "sqlite:") || strings.HasPrefix(dsn, "file:") {
		sqlDB, err := sql.Open("sqlite", strings.TrimPrefix(dsn, "sqlite:"))
		return sqlDB, "sqlite3", err
	}
	sqlDB, err := sql.Open("pgx", dsn)
	return sqlDB, "postgres", err
}

func runMigrations(sqlDB *sql.DB, dialect string) error {
	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect(dialect); err != nil {
		return err
	}
	return goose.Up(sqlDB, "db/migrations")
}

func main() {
	serverName := os.Getenv("SERVER_NAME")
	if serverName == "" {
		serverName = "Roundtable"
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://roundtable:roundtable@localhost:5432/roundtable?sslmode=disable"
	}

	sqlDB, dialect, err := openDB(dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer sqlDB.Close()

	if err := runMigrations(sqlDB, dialect); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	queries := db.New(sqlDB)

	rooms := NewRoomRegistry(queries)

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

	e.GET("/info", func(c *echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"name":    serverName,
			"version": version,
		})
	})

	e.GET("/rooms", func(c *echo.Context) error {
		list, err := rooms.List(c.Request().Context())
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, list)
	})

	e.POST("/rooms", func(c *echo.Context) error {
		var req struct {
			Name string `json:"name"`
		}
		if err := c.Bind(&req); err != nil || req.Name == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "name required"})
		}
		room, err := rooms.Create(c.Request().Context(), req.Name)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusCreated, map[string]string{"id": room.id, "name": room.name})
	})

	e.DELETE("/rooms/:id", func(c *echo.Context) error {
		if err := rooms.Delete(c.Request().Context(), c.Param("id")); err != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "room not found"})
		}
		return c.NoContent(http.StatusNoContent)
	})

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	e.GET("/events", func(c *echo.Context) error {
		ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
		if err != nil {
			return err
		}
		defer ws.Close()

		// Subscribe before snapshotting initial state so no event is missed.
		ch := rooms.bus.subscribe()
		defer rooms.bus.unsubscribe(ch)

		// Send the current member list for every room that has active peers.
		rooms.forEachRoom(func(room *Room) {
			members := room.Members()
			event := RoomEvent{Type: "members", RoomID: room.id, Members: members}
			_ = ws.WriteJSON(event)
		})

		// Detect client disconnect via a background reader.
		disconnected := make(chan struct{})
		go func() {
			defer close(disconnected)
			for {
				if _, _, err := ws.ReadMessage(); err != nil {
					return
				}
			}
		}()

		// Forward events until the client disconnects or the bus closes.
		for {
			select {
			case <-disconnected:
				return nil
			case event, ok := <-ch:
				if !ok {
					return nil
				}
				if err := ws.WriteJSON(event); err != nil {
					return nil
				}
			}
		}
	})

	e.GET("/ws", func(c *echo.Context) error {
		roomID := c.QueryParam("room")
		room, ok := rooms.Get(c.Request().Context(), roomID)
		if !ok {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "room not found"})
		}

		ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
		if err != nil {
			return err
		}
		defer ws.Close()
		log.Printf("Client connected: %s", c.Request().RemoteAddr)

		peer := &Peer{
			id:          uuid.New().String(),
			displayName: c.QueryParam("peer_name"),
			ws:          ws,
			answerCh:    make(chan webrtc.SessionDescription, 1),
		}
		defer room.RemovePeer(peer.id)

		for {
			_, rawMsg, err := ws.ReadMessage()
			if err != nil {
				log.Printf("Connection closed: %v", err)
				return nil
			}

			var typeProbe struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(rawMsg, &typeProbe); err != nil {
				log.Printf("Failed to parse message type: %v", err)
				continue
			}

			switch typeProbe.Type {
			case "offer", "answer":
				var sdp webrtc.SessionDescription
				if err := json.Unmarshal(rawMsg, &sdp); err != nil {
					log.Printf("Failed to decode SDP: %v", err)
					continue
				}
				switch sdp.Type {
				case webrtc.SDPTypeOffer:
					answer, err := handleWebRTCOffer(sdp, room, peer)
					if err != nil {
						log.Printf("Error processing offer: %v", err)
						ws.WriteJSON(map[string]string{"error": "Failed to process offer"})
						continue
					}
					ws.WriteJSON(answer)
				case webrtc.SDPTypeAnswer:
					peer.answerCh <- sdp
				}
			case "speaking":
				var msg struct {
					IsSpeaking bool `json:"is_speaking"`
				}
				if err := json.Unmarshal(rawMsg, &msg); err != nil {
					log.Printf("Failed to decode speaking message: %v", err)
					continue
				}
				room.SetSpeaking(peer.id, msg.IsSpeaking)
			default:
				log.Printf("Unknown message type: %q", typeProbe.Type)
			}
		}
	})

	e.Logger.Info("Starting WebRTC server on :1323")
	if err := e.Start(":1323"); err != nil {
		e.Logger.Error("failed to start server", "error", err)
	}
}
