package main

import (
	"database/sql"
	"embed"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib" // postgres driver for database/sql
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/pion/webrtc/v4"
	"github.com/pressly/goose/v3"
	"golang.org/x/net/websocket"
	_ "modernc.org/sqlite" // sqlite driver for database/sql

	"roundtable/backend/db"
)

const version = "0.1.0"

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

	e.GET("/ws", func(c *echo.Context) error {
		roomID := c.QueryParam("room")
		room, ok := rooms.Get(c.Request().Context(), roomID)
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
