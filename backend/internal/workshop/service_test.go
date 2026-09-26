package workshop_test

import (
	"bytes"
	"context"
	"errors"
	"math"
	"sort"
	"testing"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/workshop"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/google/uuid"
)

// MockWorkshopRepository implements workshop.Repository for in-memory unit testing
type mockWorkshopRepo struct {
	workshops      map[uuid.UUID]*domain.Workshop
	operatingHours map[uuid.UUID][]domain.OperatingHour
}

func newMockWorkshopRepo() *mockWorkshopRepo {
	return &mockWorkshopRepo{
		workshops:      make(map[uuid.UUID]*domain.Workshop),
		operatingHours: make(map[uuid.UUID][]domain.OperatingHour),
	}
}

func (m *mockWorkshopRepo) Create(ctx context.Context, w *domain.Workshop) error {
	m.workshops[w.ID] = w
	return nil
}

func (m *mockWorkshopRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Workshop, error) {
	w, ok := m.workshops[id]
	if !ok {
		return nil, workshop.ErrWorkshopNotFound
	}
	// clone
	copied := *w
	return &copied, nil
}

func (m *mockWorkshopRepo) GetByOwnerID(ctx context.Context, ownerID uuid.UUID) ([]domain.Workshop, error) {
	var list []domain.Workshop
	for _, w := range m.workshops {
		if w.OwnerID == ownerID {
			list = append(list, *w)
		}
	}
	return list, nil
}

func (m *mockWorkshopRepo) List(ctx context.Context, pagination domain.PaginationParams, filter workshop.WorkshopFilter) ([]domain.Workshop, int64, error) {
	var list []domain.Workshop
	for _, w := range m.workshops {
		if filter.Status != nil && w.Status != *filter.Status {
			continue
		}
		list = append(list, *w)
	}

	total := int64(len(list))
	start := pagination.Offset()
	if start >= len(list) {
		return []domain.Workshop{}, total, nil
	}
	end := start + pagination.PageSize
	if end > len(list) {
		end = len(list)
	}

	return list[start:end], total, nil
}

// Haversine formula mock for PostGIS distance calculation in unit test
func haversineDistanceMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusMeters = 6371000
	dLat := (lat2 - lat1) * (math.Pi / 180.0)
	dLon := (lon2 - lon1) * (math.Pi / 180.0)

	lat1Rad := lat1 * (math.Pi / 180.0)
	lat2Rad := lat2 * (math.Pi / 180.0)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Sin(dLon/2)*math.Sin(dLon/2)*math.Cos(lat1Rad)*math.Cos(lat2Rad)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusMeters * c
}

func (m *mockWorkshopRepo) FindNearby(ctx context.Context, params workshop.NearbyParams) ([]domain.Workshop, error) {
	type itemWithDist struct {
		w    domain.Workshop
		dist float64
	}

	var matched []itemWithDist
	for _, w := range m.workshops {
		if w.Status != domain.WorkshopStatusActive {
			continue
		}
		dist := haversineDistanceMeters(params.Latitude, params.Longitude, w.Latitude, w.Longitude)
		if dist <= params.RadiusMeters {
			wCopy := *w
			wCopy.DistanceMeters = &dist
			matched = append(matched, itemWithDist{w: wCopy, dist: dist})
		}
	}

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].dist < matched[j].dist
	})

	var result []domain.Workshop
	for i, item := range matched {
		if i < params.Limit {
			result = append(result, item.w)
		}
	}
	return result, nil
}

func (m *mockWorkshopRepo) Update(ctx context.Context, w *domain.Workshop) error {
	if _, ok := m.workshops[w.ID]; !ok {
		return workshop.ErrWorkshopNotFound
	}
	m.workshops[w.ID] = w
	return nil
}

func (m *mockWorkshopRepo) GetOperatingHours(ctx context.Context, workshopID uuid.UUID) ([]domain.OperatingHour, error) {
	hours, ok := m.operatingHours[workshopID]
	if !ok {
		return []domain.OperatingHour{}, nil
	}
	return hours, nil
}

func (m *mockWorkshopRepo) UpsertOperatingHours(ctx context.Context, workshopID uuid.UUID, hours []domain.OperatingHour) error {
	m.operatingHours[workshopID] = hours
	return nil
}

func setupWorkshopService() (workshop.Service, *mockWorkshopRepo) {
	repo := newMockWorkshopRepo()
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)
	svc := workshop.NewService(repo, log)
	return svc, repo
}

func TestWorkshopService_Create_Success(t *testing.T) {
	svc, _ := setupWorkshopService()
	ownerID := uuid.New()

	req := workshop.CreateWorkshopRequest{
		Name:        "Bengkel Maju Jaya",
		Description: "Spesialis motor dan mobil",
		Address:     "Jl. Sudirman No. 10, Jakarta",
		Latitude:    -6.2088,
		Longitude:   106.8456,
		Phone:       "081234567890",
		OperatingHours: []workshop.OperatingHourInput{
			{DayOfWeek: 1, OpenTime: "08:00", CloseTime: "17:00", IsClosed: false},
			{DayOfWeek: 2, OpenTime: "08:00", CloseTime: "17:00", IsClosed: false},
			{DayOfWeek: 0, OpenTime: "", CloseTime: "", IsClosed: true}, // Sunday closed
		},
	}

	ws, valErrors, err := svc.CreateWorkshop(context.Background(), ownerID, req)
	if err != nil {
		t.Fatalf("unexpected error creating workshop: %v", err)
	}

	if valErrors != nil {
		t.Fatalf("expected no validation errors, got %+v", valErrors)
	}

	if ws.Name != "Bengkel Maju Jaya" {
		t.Errorf("expected name 'Bengkel Maju Jaya', got %s", ws.Name)
	}

	if ws.OwnerID != ownerID {
		t.Errorf("expected owner ID %s, got %s", ownerID, ws.OwnerID)
	}

	if len(ws.OperatingHours) != 3 {
		t.Errorf("expected 3 operating hours, got %d", len(ws.OperatingHours))
	}
}

func TestWorkshopService_Create_ValidationErrors(t *testing.T) {
	svc, _ := setupWorkshopService()
	ownerID := uuid.New()

	req := workshop.CreateWorkshopRequest{
		Name:      "",
		Address:   "",
		Phone:     "",
		Latitude:  120.0,  // Invalid latitude (> 90)
		Longitude: -200.0, // Invalid longitude (< -180)
	}

	_, valErrors, err := svc.CreateWorkshop(context.Background(), ownerID, req)
	if !errors.Is(err, workshop.ErrValidationFailed) {
		t.Fatalf("expected ErrValidationFailed, got %v", err)
	}

	if _, ok := valErrors["name"]; !ok {
		t.Errorf("expected validation error on name")
	}
	if _, ok := valErrors["latitude"]; !ok {
		t.Errorf("expected validation error on latitude")
	}
}

func TestWorkshopService_FindNearby_SpatialOrdering(t *testing.T) {
	svc, repo := setupWorkshopService()

	// Location A: 1km away from Jakarta center (-6.2000, 106.8456)
	idA := uuid.New()
	repo.workshops[idA] = &domain.Workshop{
		ID:        idA,
		Name:      "Bengkel Dekat (1km)",
		Latitude:  -6.2090,
		Longitude: 106.8456,
		Status:    domain.WorkshopStatusActive,
		CreatedAt: time.Now(),
	}

	// Location B: 4km away
	idB := uuid.New()
	repo.workshops[idB] = &domain.Workshop{
		ID:        idB,
		Name:      "Bengkel Menengah (4km)",
		Latitude:  -6.2360,
		Longitude: 106.8456,
		Status:    domain.WorkshopStatusActive,
		CreatedAt: time.Now(),
	}

	// Location C: 20km away (should be excluded when radius is 5km)
	idC := uuid.New()
	repo.workshops[idC] = &domain.Workshop{
		ID:        idC,
		Name:      "Bengkel Jauh (20km)",
		Latitude:  -6.3800,
		Longitude: 106.8456,
		Status:    domain.WorkshopStatusActive,
		CreatedAt: time.Now(),
	}

	params := workshop.NearbyParams{
		Latitude:     -6.2000,
		Longitude:    106.8456,
		RadiusMeters: 5000, // 5km
		Limit:        10,
	}

	nearby, err := svc.FindNearbyWorkshops(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error finding nearby workshops: %v", err)
	}

	if len(nearby) != 2 {
		t.Fatalf("expected 2 nearby workshops within 5km, got %d", len(nearby))
	}

	// Nearest should be first
	if nearby[0].Name != "Bengkel Dekat (1km)" {
		t.Errorf("expected first workshop to be nearest, got %s", nearby[0].Name)
	}

	if nearby[0].DistanceMeters == nil || *nearby[0].DistanceMeters > *nearby[1].DistanceMeters {
		t.Errorf("expected distance of first result to be smaller than second")
	}
}

func TestWorkshopService_Update_OwnershipEnforcement(t *testing.T) {
	svc, repo := setupWorkshopService()

	ownerA := uuid.New()
	ownerB := uuid.New()
	wsID := uuid.New()

	repo.workshops[wsID] = &domain.Workshop{
		ID:        wsID,
		OwnerID:   ownerA,
		Name:      "Owner A Workshop",
		Address:   "Alamat A",
		Phone:     "081111111",
		Status:    domain.WorkshopStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	newName := "Updated By Owner A"
	// 1. Owner A updating own workshop -> SUCCESS
	updated, _, err := svc.UpdateWorkshop(context.Background(), wsID, ownerA, domain.RoleOwner, workshop.UpdateWorkshopRequest{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("unexpected error on owner update: %v", err)
	}
	if updated.Name != newName {
		t.Errorf("expected updated name %s, got %s", newName, updated.Name)
	}

	// 2. Owner B attempting to update Owner A's workshop -> 403 FORBIDDEN
	hackerName := "Hacked Name"
	_, _, err = svc.UpdateWorkshop(context.Background(), wsID, ownerB, domain.RoleOwner, workshop.UpdateWorkshopRequest{
		Name: &hackerName,
	})
	if !errors.Is(err, workshop.ErrForbidden) {
		t.Fatalf("expected ErrForbidden when non-owner tries to update, got %v", err)
	}

	// 3. Admin updating Owner A's workshop -> SUCCESS (Admin override)
	adminID := uuid.New()
	adminName := "Admin Verified Name"
	updatedByAdmin, _, err := svc.UpdateWorkshop(context.Background(), wsID, adminID, domain.RoleAdmin, workshop.UpdateWorkshopRequest{
		Name: &adminName,
	})
	if err != nil {
		t.Fatalf("unexpected error on admin update: %v", err)
	}
	if updatedByAdmin.Name != adminName {
		t.Errorf("expected admin update to succeed")
	}
}
