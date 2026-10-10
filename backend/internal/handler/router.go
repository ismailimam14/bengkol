package handler

import (
	"net/http"

	"github.com/bengkol/backend/config"
	"github.com/bengkol/backend/internal/auth"
	"github.com/bengkol/backend/internal/booking"
	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/employee"
	"github.com/bengkol/backend/internal/history"
	"github.com/bengkol/backend/internal/middleware"
	"github.com/bengkol/backend/internal/notification"
	"github.com/bengkol/backend/internal/queue"
	"github.com/bengkol/backend/internal/review"
	"github.com/bengkol/backend/internal/service"
	"github.com/bengkol/backend/internal/sparepart"
	"github.com/bengkol/backend/internal/vehicle"
	"github.com/bengkol/backend/internal/websocket"
	"github.com/bengkol/backend/internal/workshop"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/response"
	"github.com/bengkol/backend/pkg/security"
	"github.com/go-chi/chi/v5"
)

// RouterConfig contains dependencies needed to configure the HTTP router.
type RouterConfig struct {
	Config           *config.Config
	Logger           *logger.Logger
	HealthHandler    *HealthHandler
	DocsHandler      *DocsHandler
	AuthHandler      *auth.Handler
	WorkshopHandler  *workshop.Handler
	EmployeeHandler  *employee.Handler
	ServiceHandler   *service.Handler
	SparePartHandler *sparepart.Handler
	VehicleHandler   *vehicle.Handler
	BookingHandler   *booking.Handler
	QueueHandler     *queue.Handler
	HistoryHandler   *history.Handler
	ReviewHandler    *review.Handler
	DeviceHandler    *notification.Handler
	WSHandler        *websocket.Handler
	JWTManager       *security.JWTManager
	StrictAppHeaders bool
}

// NewRouter constructs and configures the top-level Chi router.
func NewRouter(cfg RouterConfig) *chi.Mux {
	r := chi.NewRouter()

	// Global Middlewares
	r.Use(middleware.RequestID)
	r.Use(middleware.RequestLogger(cfg.Logger))
	r.Use(middleware.Recoverer(cfg.Logger))
	r.Use(middleware.CORS(cfg.Config.CORS))

	// Auth token extraction middleware (attaches claims if Bearer token present)
	if cfg.JWTManager != nil {
		r.Use(middleware.Authenticate(cfg.JWTManager, cfg.Logger))
	}

	// Custom 404 Not Found handler
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "The requested endpoint does not exist")
	})

	// Custom 405 Method Not Allowed handler
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		response.Error(w, http.StatusMethodNotAllowed, response.ErrCodeBadRequest, "HTTP method not allowed for this endpoint")
	})

	// Liveness and Readiness Probes
	r.Get("/health", cfg.HealthHandler.Health)
	r.Get("/ready", cfg.HealthHandler.Ready)

	// Interactive API Documentation (Swagger UI & ReDoc)
	if cfg.DocsHandler != nil {
		r.Get("/docs", cfg.DocsHandler.SwaggerUI)
		r.Get("/swagger", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/docs", http.StatusMovedPermanently)
		})
		r.Get("/redoc", cfg.DocsHandler.ReDoc)
		r.Get("/docs/openapi.yaml", cfg.DocsHandler.OpenAPI)
	}

	// API v1 Sub-router
	r.Route("/api/v1", func(v1 chi.Router) {
		v1.Use(middleware.ValidateAppHeaders(cfg.StrictAppHeaders))

		v1.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
			response.Success(w, http.StatusOK, map[string]string{
				"message": "Bengkol API v1 is operational",
				"version": "1.0.0",
			})
		})

		// Auth Routes (/api/v1/auth)
		if cfg.AuthHandler != nil {
			v1.Mount("/auth", cfg.AuthHandler.Routes())

			// Protected /me endpoint
			v1.Group(func(protected chi.Router) {
				protected.Use(middleware.RequireAuthenticated)
				protected.Get("/me", cfg.AuthHandler.GetMe)
				protected.Put("/me/password", cfg.AuthHandler.ChangePassword)
			})
		}

		// Workshop Routes (/api/v1/workshops)
		if cfg.WorkshopHandler != nil {
			// Public discovery endpoints
			v1.Get("/workshops", cfg.WorkshopHandler.List)
			v1.Get("/workshops/nearby", cfg.WorkshopHandler.FindNearby)
			v1.Get("/workshops/{id}", cfg.WorkshopHandler.GetByID)
			v1.Get("/workshops/{id}/operating-hours", cfg.WorkshopHandler.GetOperatingHours)
			v1.Get("/workshops/{id}/photos/{photoID}", cfg.WorkshopHandler.GetPhoto)
			v1.Get("/workshops/photos/{photoID}", cfg.WorkshopHandler.GetPhoto)

			// Protected workshop management endpoints
			v1.Group(func(protected chi.Router) {
				protected.Use(middleware.RequireAuthenticated)
				protected.Get("/me/workshops", cfg.WorkshopHandler.GetMyWorkshops)

				// Owner-only creation
				protected.With(middleware.RequireRoles(domain.RoleOwner)).Post("/workshops", cfg.WorkshopHandler.Create)

				// Owner / Admin modifications
				protected.With(middleware.RequireRoles(domain.RoleOwner, domain.RoleAdmin)).Patch("/workshops/{id}", cfg.WorkshopHandler.Update)
				protected.With(middleware.RequireRoles(domain.RoleOwner, domain.RoleAdmin)).Put("/workshops/{id}/operating-hours", cfg.WorkshopHandler.UpdateOperatingHours)
			})
		}

		// Workshop Employee Routes (/api/v1/workshops/{id}/employees, etc.)
		if cfg.EmployeeHandler != nil {
			v1.Group(func(protected chi.Router) {
				protected.Use(middleware.RequireAuthenticated)
				protected.Get("/workshops/{id}/employees", cfg.EmployeeHandler.List)
				protected.Post("/workshops/{id}/employees", cfg.EmployeeHandler.Create)
				protected.Get("/workshops/{id}/employees/me", cfg.EmployeeHandler.GetMe)
				protected.Get("/workshops/{id}/employees/{employeeId}", cfg.EmployeeHandler.GetByID)
				protected.Patch("/workshops/{id}/employees/{employeeId}", cfg.EmployeeHandler.Update)
				protected.Delete("/workshops/{id}/employees/{employeeId}", cfg.EmployeeHandler.Delete)
			})
		}

		// Services Routes (/api/v1/workshops/:id/services and /api/v1/services/:id)
		if cfg.ServiceHandler != nil {
			v1.Get("/workshops/{id}/services", cfg.ServiceHandler.ListByWorkshop)
			v1.Get("/services/{id}", cfg.ServiceHandler.GetByID)

			v1.Group(func(protected chi.Router) {
				protected.Use(middleware.RequireAuthenticated)
				protected.With(middleware.RequireRoles(domain.RoleOwner)).Post("/workshops/{id}/services", cfg.ServiceHandler.Create)
				protected.With(middleware.RequireRoles(domain.RoleOwner, domain.RoleAdmin)).Patch("/services/{id}", cfg.ServiceHandler.Update)
				protected.With(middleware.RequireRoles(domain.RoleOwner, domain.RoleAdmin)).Delete("/services/{id}", cfg.ServiceHandler.Delete)
			})
		}

		// Spare Parts Routes (/api/v1/workshops/:id/spare-parts and /api/v1/spare-parts/:id)
		if cfg.SparePartHandler != nil {
			v1.Get("/workshops/{id}/spare-parts", cfg.SparePartHandler.ListByWorkshop)
			v1.Get("/spare-parts/{id}", cfg.SparePartHandler.GetByID)

			v1.Group(func(protected chi.Router) {
				protected.Use(middleware.RequireAuthenticated)
				protected.Post("/workshops/{id}/spare-parts", cfg.SparePartHandler.Create)
				protected.Patch("/spare-parts/{id}", cfg.SparePartHandler.Update)
				protected.Delete("/spare-parts/{id}", cfg.SparePartHandler.Delete)
			})
		}

		// Vehicle Routes (/api/v1/vehicles, /api/v1/vehicles/:id, /api/v1/me/vehicles)
		if cfg.VehicleHandler != nil {
			v1.Group(func(protected chi.Router) {
				protected.Use(middleware.RequireAuthenticated)

				protected.Post("/vehicles", cfg.VehicleHandler.Create)
				protected.Get("/vehicles", cfg.VehicleHandler.ListMyVehicles)
				protected.Get("/me/vehicles", cfg.VehicleHandler.ListMyVehicles)
				protected.Get("/vehicles/{id}", cfg.VehicleHandler.GetByID)
				protected.Patch("/vehicles/{id}", cfg.VehicleHandler.Update)
				protected.Delete("/vehicles/{id}", cfg.VehicleHandler.Delete)
			})
		}

		// Booking Routes (/api/v1/workshops/:id/available-slots, /api/v1/bookings, /api/v1/me/bookings)
		if cfg.BookingHandler != nil {
			v1.Get("/workshops/{id}/available-slots", cfg.BookingHandler.GetAvailableSlots)

			v1.Group(func(protected chi.Router) {
				protected.Use(middleware.RequireAuthenticated)

				// Customer booking creation & list
				protected.Post("/bookings", cfg.BookingHandler.Create)
				protected.Get("/bookings/{id}", cfg.BookingHandler.GetByID)
				protected.Post("/bookings/{id}/cancel", cfg.BookingHandler.Cancel)
				protected.Get("/me/bookings", cfg.BookingHandler.ListMyBookings)

				// Owner / Admin workshop bookings
				protected.With(middleware.RequireRoles(domain.RoleOwner, domain.RoleAdmin)).Get("/workshops/{id}/bookings", cfg.BookingHandler.ListWorkshopBookings)
			})
		}

		// Queue Routes (/api/v1/queues, /api/v1/workshops/:id/queues, /api/v1/bookings/:id/check-in)
		if cfg.QueueHandler != nil {
			// Public queue summary board
			v1.Get("/workshops/{id}/queues/summary", cfg.QueueHandler.GetSummary)

			v1.Group(func(protected chi.Router) {
				protected.Use(middleware.RequireAuthenticated)

				// Customer / General queue actions
				protected.Post("/bookings/{id}/check-in", cfg.QueueHandler.CheckIn)
				protected.Get("/queues/{id}", cfg.QueueHandler.GetByID)
				protected.Get("/me/queue/active", cfg.QueueHandler.GetActiveCustomerQueue)
				protected.Post("/queues/{id}/cancel", cfg.QueueHandler.CancelQueue)

				// Workshop Owner / Admin queue operational controls
				protected.With(middleware.RequireRoles(domain.RoleOwner, domain.RoleAdmin)).Get("/workshops/{id}/queues", cfg.QueueHandler.ListByWorkshop)
				protected.With(middleware.RequireRoles(domain.RoleOwner, domain.RoleAdmin)).Post("/workshops/{id}/queues/call-next", cfg.QueueHandler.CallNext)
				protected.With(middleware.RequireRoles(domain.RoleOwner, domain.RoleAdmin)).Post("/queues/{id}/call", cfg.QueueHandler.CallQueue)
				protected.With(middleware.RequireRoles(domain.RoleOwner, domain.RoleAdmin)).Post("/queues/{id}/start", cfg.QueueHandler.StartService)
				protected.With(middleware.RequireRoles(domain.RoleOwner, domain.RoleAdmin)).Post("/queues/{id}/complete", cfg.QueueHandler.CompleteService)
				protected.With(middleware.RequireRoles(domain.RoleOwner, domain.RoleAdmin)).Post("/queues/{id}/no-show", cfg.QueueHandler.MarkNoShow)
			})
		}

		// Service History Routes (/api/v1/service-histories, /api/v1/workshops/:id/service-histories, /api/v1/me/service-histories)
		if cfg.HistoryHandler != nil {
			v1.Group(func(protected chi.Router) {
				protected.Use(middleware.RequireAuthenticated)

				// Customer / General view history
				protected.Get("/service-histories/{id}", cfg.HistoryHandler.GetByID)
				protected.Get("/bookings/{id}/service-history", cfg.HistoryHandler.GetByBookingID)
				protected.Get("/me/service-histories", cfg.HistoryHandler.ListCustomerHistories)

				// Workshop Owner / Admin records
				protected.With(middleware.RequireRoles(domain.RoleOwner, domain.RoleAdmin)).Post("/workshops/{id}/service-histories", cfg.HistoryHandler.Create)
				protected.With(middleware.RequireRoles(domain.RoleOwner, domain.RoleAdmin)).Get("/workshops/{id}/service-histories", cfg.HistoryHandler.ListWorkshopHistories)
			})
		}

		// Review Routes (/api/v1/reviews, /api/v1/workshops/:id/reviews, /api/v1/bookings/:id/review)
		if cfg.ReviewHandler != nil {
			// Public review discovery
			v1.Get("/workshops/{id}/reviews", cfg.ReviewHandler.ListByWorkshop)
			v1.Get("/reviews/{id}", cfg.ReviewHandler.GetByID)
			v1.Get("/bookings/{id}/review", cfg.ReviewHandler.GetByBookingID)

			// Protected customer review actions
			v1.Group(func(protected chi.Router) {
				protected.Use(middleware.RequireAuthenticated)

				protected.With(middleware.RequireRoles(domain.RoleCustomer)).Post("/reviews", cfg.ReviewHandler.Create)
				protected.Patch("/reviews/{id}", cfg.ReviewHandler.Update)
			})
		}

		// Real-time WebSocket Route (/api/v1/ws)
		if cfg.WSHandler != nil {
			v1.Get("/ws", cfg.WSHandler.ServeWS)
		}

		// Device Push Notification Routes (/api/v1/devices, /api/v1/me/devices)
		if cfg.DeviceHandler != nil {
			v1.Group(func(protected chi.Router) {
				protected.Use(middleware.RequireAuthenticated)
				protected.Post("/devices", cfg.DeviceHandler.RegisterDevice)
				protected.Delete("/devices/{token}", cfg.DeviceHandler.UnregisterDevice)
				protected.Get("/me/devices", cfg.DeviceHandler.GetMyDevices)
			})
		}
	})

	return r
}
