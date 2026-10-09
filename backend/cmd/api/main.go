package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/bengkol/backend/config"
	"github.com/bengkol/backend/database"
	"github.com/bengkol/backend/internal/auth"
	"github.com/bengkol/backend/internal/booking"
	"github.com/bengkol/backend/internal/handler"
	"github.com/bengkol/backend/internal/history"
	"github.com/bengkol/backend/internal/notification"
	"github.com/bengkol/backend/internal/queue"
	"github.com/bengkol/backend/internal/review"
	"github.com/bengkol/backend/internal/service"
	"github.com/bengkol/backend/internal/sparepart"
	"github.com/bengkol/backend/internal/vehicle"
	"github.com/bengkol/backend/internal/websocket"
	"github.com/bengkol/backend/internal/workshop"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/security"
)

func main() {
	// 1. Load Configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize Structured Logger
	log := logger.New(cfg.AppEnv, cfg.LogLevel)
	log.Info("starting Bengkol API service",
		"app_env", cfg.AppEnv,
		"port", cfg.AppPort,
	)

	// 3. Connect to Database
	db, err := database.NewPostgres(cfg.Database, log)
	if err != nil {
		log.Error("failed to connect to postgres database", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Error("error closing database connection", "error", err)
		}
	}()

	// 4. Run Database Migrations
	migrator := database.NewMigrator(db.DB, log)
	migrationCtx, migrationCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer migrationCancel()

	if err := migrator.Up(migrationCtx); err != nil {
		log.Error("failed to run database migrations", "error", err)
		os.Exit(1)
	}
	log.Info("database migrations verified and up to date")

	// 5. Initialize Security / JWT
	jwtMgr := security.NewJWTManager(
		cfg.JWT.AccessSecret,
		cfg.JWT.RefreshSecret,
		cfg.JWT.AccessExpiryMinutes,
		cfg.JWT.RefreshExpiryDays,
	)

	// 6. Initialize WebSocket Hub & Real-time Dispatcher
	hubCtx, hubCancel := context.WithCancel(context.Background())
	defer hubCancel()

	wsHub := websocket.NewHub(log)
	go wsHub.Run(hubCtx)
	log.Info("websocket hub initialized and running")

	// 7. Initialize Push Notification Infrastructure & Dispatcher
	deviceRepo := notification.NewRepository(db.DB)
	fcmDispatcher := notification.NewFCMDispatcher(deviceRepo, log)
	deviceService := notification.NewService(deviceRepo, log)

	// 8. Initialize Repositories and Domain Services
	authRepo := auth.NewRepository(db.DB)
	authService := auth.NewService(authRepo, jwtMgr, log)

	workshopRepo := workshop.NewRepository(db.DB)
	workshopService := workshop.NewService(workshopRepo, log)

	serviceRepo := service.NewRepository(db.DB)
	serviceService := service.NewService(serviceRepo, log)

	sparePartRepo := sparepart.NewRepository(db.DB)
	sparePartService := sparepart.NewService(sparePartRepo, log)

	vehicleRepo := vehicle.NewRepository(db.DB)
	vehicleService := vehicle.NewService(vehicleRepo, log)

	bookingRepo := booking.NewRepository(db.DB)
	bookingService := booking.NewService(bookingRepo, log)

	queueRepo := queue.NewRepository(db.DB)
	queueService := queue.NewService(queueRepo, log, wsHub, fcmDispatcher)

	historyRepo := history.NewRepository(db.DB)
	historyService := history.NewService(historyRepo, log)

	reviewRepo := review.NewRepository(db.DB)
	reviewService := review.NewService(reviewRepo, log)

	// 9. Initialize Handlers
	healthHandler := handler.NewHealthHandler(db)
	docsHandler := handler.NewDocsHandler()
	authHandler := auth.NewHandler(authService, log)
	workshopHandler := workshop.NewHandler(workshopService, log)
	workshopHandler.SetUploadDir(filepath.Join(cfg.UploadDir, "workshops"))
	serviceHandler := service.NewHandler(serviceService, log)
	sparePartHandler := sparepart.NewHandler(sparePartService, log)
	vehicleHandler := vehicle.NewHandler(vehicleService, log)
	bookingHandler := booking.NewHandler(bookingService, log)
	queueHandler := queue.NewHandler(queueService, log)
	historyHandler := history.NewHandler(historyService, log)
	reviewHandler := review.NewHandler(reviewService, log)
	wsHandler := websocket.NewHandler(wsHub, jwtMgr, log)
	deviceHandler := notification.NewHandler(deviceService, log)

	// 10. Build HTTP Router
	router := handler.NewRouter(handler.RouterConfig{
		Config:           cfg,
		Logger:           log,
		HealthHandler:    healthHandler,
		DocsHandler:      docsHandler,
		AuthHandler:      authHandler,
		WorkshopHandler:  workshopHandler,
		ServiceHandler:   serviceHandler,
		SparePartHandler: sparePartHandler,
		VehicleHandler:   vehicleHandler,
		BookingHandler:   bookingHandler,
		QueueHandler:     queueHandler,
		HistoryHandler:   historyHandler,
		ReviewHandler:    reviewHandler,
		DeviceHandler:    deviceHandler,
		WSHandler:        wsHandler,
		JWTManager:       jwtMgr,
	})

	// 9. Setup HTTP Server
	server := &http.Server{
		Addr:         ":" + cfg.AppPort,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 10. Graceful Shutdown Channel
	serverErrors := make(chan error, 1)
	go func() {
		log.Info("http server listening", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		log.Error("http server error", "error", err)
		os.Exit(1)

	case sig := <-shutdown:
		log.Info("shutdown signal received", "signal", sig.String())

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Error("graceful server shutdown failed", "error", err)
			_ = server.Close()
		}
		log.Info("server gracefully stopped")
	}
}
