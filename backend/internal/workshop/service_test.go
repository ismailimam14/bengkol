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
	photos         map[uuid.UUID]*domain.WorkshopPhoto
	employees      map[uuid.UUID][]domain.WorkshopEmployee
}

func newMockWorkshopRepo() *mockWorkshopRepo {
	return &mockWorkshopRepo{
		workshops:      make(map[uuid.UUID]*domain.Workshop),
		operatingHours: make(map[uuid.UUID][]domain.OperatingHour),
		photos:         make(map[uuid.UUID]*domain.WorkshopPhoto),
		employees:      make(map[uuid.UUID][]domain.WorkshopEmployee),
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

func (m *mockWorkshopRepo) GetByOwnerID(ctx context.Context, userID uuid.UUID) ([]domain.Workshop, error) {
	var list []domain.Workshop
	seen := make(map[uuid.UUID]bool)
	for _, w := range m.workshops {
		if w.OwnerID == userID {
			list = append(list, *w)
			seen[w.ID] = true
			continue
		}
		if emps, ok := m.employees[w.ID]; ok {
			for _, emp := range emps {
				if emp.UserID != nil && *emp.UserID == userID && emp.Status == domain.EmployeeStatusActive {
					if !seen[w.ID] {
						list = append(list, *w)
						seen[w.ID] = true
					}
					break
				}
			}
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

func (m *mockWorkshopRepo) SavePhoto(ctx context.Context, photo *domain.WorkshopPhoto) error {
	m.photos[photo.ID] = photo
	return nil
}

func (m *mockWorkshopRepo) GetPhotoByID(ctx context.Context, id uuid.UUID) (*domain.WorkshopPhoto, error) {
	p, ok := m.photos[id]
	if !ok {
		return nil, workshop.ErrPhotoNotFound
	}
	copied := *p
	return &copied, nil
}

func (m *mockWorkshopRepo) GetPhotosByWorkshopID(ctx context.Context, workshopID uuid.UUID) ([]domain.WorkshopPhoto, error) {
	var list []domain.WorkshopPhoto
	for _, p := range m.photos {
		if p.WorkshopID == workshopID {
			list = append(list, *p)
		}
	}
	return list, nil
}

func (m *mockWorkshopRepo) DeletePhotosByWorkshopID(ctx context.Context, workshopID uuid.UUID) error {
	for id, p := range m.photos {
		if p.WorkshopID == workshopID {
			delete(m.photos, id)
		}
	}
	return nil
}

func (m *mockWorkshopRepo) CreateEmployees(ctx context.Context, workshopID uuid.UUID, employees []domain.WorkshopEmployee) error {
	m.employees[workshopID] = append(m.employees[workshopID], employees...)
	return nil
}

func (m *mockWorkshopRepo) GetEmployees(ctx context.Context, workshopID uuid.UUID) ([]domain.WorkshopEmployee, error) {
	emps, ok := m.employees[workshopID]
	if !ok {
		return []domain.WorkshopEmployee{}, nil
	}
	return emps, nil
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
		Photos: []string{
			"https://example.com/photos/1.jpg",
			"https://example.com/photos/2.jpg",
			"https://example.com/photos/3.jpg",
		},
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

	if len(ws.Photos) != 3 {
		t.Errorf("expected 3 photos, got %d", len(ws.Photos))
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
	if _, ok := valErrors["photos"]; !ok {
		t.Errorf("expected validation error on photos")
	}
}

func TestWorkshopService_Create_MinThreePhotos(t *testing.T) {
	svc, _ := setupWorkshopService()
	ownerID := uuid.New()

	baseReq := workshop.CreateWorkshopRequest{
		Name:      "Bengkel Foto Test",
		Address:   "Jl. Foto No. 1",
		Phone:     "0812345678",
		Latitude:  -6.2,
		Longitude: 106.8,
	}

	// 1. Nil photos -> should fail
	reqNil := baseReq
	reqNil.Photos = nil
	_, valErrors, err := svc.CreateWorkshop(context.Background(), ownerID, reqNil)
	if !errors.Is(err, workshop.ErrValidationFailed) {
		t.Fatalf("expected ErrValidationFailed for nil photos")
	}
	if valErrors["photos"] != "at least 3 photos are required" {
		t.Errorf("expected 'at least 3 photos are required', got %s", valErrors["photos"])
	}

	// 2. 2 photos -> should fail
	reqTwo := baseReq
	reqTwo.Photos = []string{"https://example.com/1.jpg", "https://example.com/2.jpg"}
	_, valErrors, err = svc.CreateWorkshop(context.Background(), ownerID, reqTwo)
	if !errors.Is(err, workshop.ErrValidationFailed) {
		t.Fatalf("expected ErrValidationFailed for 2 photos")
	}
	if valErrors["photos"] != "at least 3 photos are required" {
		t.Errorf("expected 'at least 3 photos are required', got %s", valErrors["photos"])
	}

	// 3. 3 photos with empty/whitespace string -> should fail (only 2 valid)
	reqWhitespace := baseReq
	reqWhitespace.Photos = []string{"https://example.com/1.jpg", "   ", "https://example.com/2.jpg"}
	_, valErrors, err = svc.CreateWorkshop(context.Background(), ownerID, reqWhitespace)
	if !errors.Is(err, workshop.ErrValidationFailed) {
		t.Fatalf("expected ErrValidationFailed for photos with whitespace")
	}
	if valErrors["photos"] != "at least 3 photos are required" {
		t.Errorf("expected 'at least 3 photos are required', got %s", valErrors["photos"])
	}

	// 4. Exactly 3 valid photos -> success
	reqThree := baseReq
	reqThree.Photos = []string{
		"https://example.com/1.jpg",
		"https://example.com/2.jpg",
		"https://example.com/3.jpg",
	}
	ws, valErrors, err := svc.CreateWorkshop(context.Background(), ownerID, reqThree)
	if err != nil {
		t.Fatalf("unexpected error for 3 valid photos: %v", err)
	}
	if len(ws.Photos) != 3 {
		t.Errorf("expected 3 photos, got %d", len(ws.Photos))
	}

	// 5. 4 valid photos -> success
	reqFour := baseReq
	reqFour.Photos = []string{
		"https://example.com/1.jpg",
		"https://example.com/2.jpg",
		"https://example.com/3.jpg",
		"https://example.com/4.jpg",
	}
	ws4, valErrors, err := svc.CreateWorkshop(context.Background(), ownerID, reqFour)
	if err != nil {
		t.Fatalf("unexpected error for 4 valid photos: %v", err)
	}
	if len(ws4.Photos) != 4 {
		t.Errorf("expected 4 photos, got %d", len(ws4.Photos))
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

func TestWorkshopService_Update_Photos(t *testing.T) {
	svc, repo := setupWorkshopService()

	ownerID := uuid.New()
	wsID := uuid.New()

	repo.workshops[wsID] = &domain.Workshop{
		ID:        wsID,
		OwnerID:   ownerID,
		Name:      "Workshop Photo Test",
		Address:   "Jl. Tes",
		Phone:     "081111111",
		Photos: []string{
			"https://example.com/1.jpg",
			"https://example.com/2.jpg",
			"https://example.com/3.jpg",
		},
		Status:    domain.WorkshopStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// 1. Updating with fewer than 3 photos fails validation
	badPhotos := []string{"https://example.com/new1.jpg", "https://example.com/new2.jpg"}
	_, valErrors, err := svc.UpdateWorkshop(context.Background(), wsID, ownerID, domain.RoleOwner, workshop.UpdateWorkshopRequest{
		Photos: &badPhotos,
	})
	if !errors.Is(err, workshop.ErrValidationFailed) {
		t.Fatalf("expected ErrValidationFailed for < 3 photos update")
	}
	if valErrors["photos"] != "at least 3 photos are required" {
		t.Errorf("expected 'at least 3 photos are required', got %s", valErrors["photos"])
	}

	// 2. Updating with 3 valid photos succeeds
	goodPhotos := []string{
		"https://example.com/new1.jpg",
		"https://example.com/new2.jpg",
		"https://example.com/new3.jpg",
	}
	updated, _, err := svc.UpdateWorkshop(context.Background(), wsID, ownerID, domain.RoleOwner, workshop.UpdateWorkshopRequest{
		Photos: &goodPhotos,
	})
	if err != nil {
		t.Fatalf("unexpected error updating photos: %v", err)
	}
	if len(updated.Photos) != 3 || updated.Photos[0] != "https://example.com/new1.jpg" {
		t.Errorf("expected updated photos, got %+v", updated.Photos)
	}
}

func TestCreateWorkshop_WithInitialEmployees(t *testing.T) {
	svc, _ := setupWorkshopService()
	ownerID := uuid.New()

	req := workshop.CreateWorkshopRequest{
		Name:    "Workshop With Employees",
		Address: "Jl. Sudirman No. 100",
		Phone:   "081299998888",
		Photos: []string{
			"https://example.com/p1.jpg",
			"https://example.com/p2.jpg",
			"https://example.com/p3.jpg",
		},
		Employees: []workshop.CreateEmployeeInput{
			{
				Name:           "Budi Montir",
				Phone:          "0811111111",
				Role:           "MECHANIC",
				Specialization: "Engine",
			},
			{
				Name:  "Siti Kasir",
				Phone: "0822222222",
				Role:  "ADMIN_CASHIER",
			},
			{
				Name:  "Agus Gudang",
				Phone: "0833333333",
				Role:  "ADMIN_INVENTORY",
			},
			{
				Name:  "Rina Admin",
				Phone: "0844444444",
				Role:  "ADMIN_BOTH",
			},
			{
				Name:  "Doni Manager",
				Phone: "0855555555",
				Role:  "MANAGER",
			},
		},
	}

	ws, valErrors, err := svc.CreateWorkshop(context.Background(), ownerID, req)
	if err != nil {
		t.Fatalf("unexpected error creating workshop with employees: %v (valErrors: %v)", err, valErrors)
	}

	if len(ws.Employees) != 5 {
		t.Fatalf("expected 5 employees, got %d", len(ws.Employees))
	}

	// Verify generated initial passwords and permissions calculated
	for _, emp := range ws.Employees {
		if emp.InitialPassword == "" {
			t.Errorf("expected initial password to be generated for %s, got empty", emp.Name)
		}
		if emp.Permissions == nil {
			t.Errorf("expected permissions to be populated for %s", emp.Name)
			continue
		}
		switch emp.Role {
		case domain.EmployeeRoleMechanic:
			if !emp.Permissions.CanAccessRepairJobs || emp.Permissions.CanAccessCashier || emp.Permissions.CanManageInventory {
				t.Errorf("unexpected permissions for mechanic: %+v", emp.Permissions)
			}
		case domain.EmployeeRoleAdminCashier:
			if !emp.Permissions.CanAccessCashier || emp.Permissions.CanManageInventory || emp.Permissions.CanAccessRepairJobs {
				t.Errorf("unexpected permissions for cashier: %+v", emp.Permissions)
			}
		case domain.EmployeeRoleAdminInventory:
			if !emp.Permissions.CanManageInventory || emp.Permissions.CanAccessCashier || emp.Permissions.CanAccessRepairJobs {
				t.Errorf("unexpected permissions for inventory admin: %+v", emp.Permissions)
			}
		case domain.EmployeeRoleAdminBoth:
			if !emp.Permissions.CanAccessCashier || !emp.Permissions.CanManageInventory || emp.Permissions.CanManageEmployees {
				t.Errorf("unexpected permissions for admin both: %+v", emp.Permissions)
			}
		case domain.EmployeeRoleManager:
			if !emp.Permissions.CanManageEmployees || !emp.Permissions.CanManageWorkshopOperations {
				t.Errorf("unexpected permissions for manager: %+v", emp.Permissions)
			}
		}
	}

	// Also verify GetWorkshopByID loads employees
	loaded, err := svc.GetWorkshopByID(context.Background(), ws.ID)
	if err != nil {
		t.Fatalf("unexpected error getting workshop: %v", err)
	}
	if len(loaded.Employees) != 5 {
		t.Errorf("expected loaded workshop to have 5 employees, got %d", len(loaded.Employees))
	}
}

func TestService_GetMyWorkshops_OwnerAndEmployee(t *testing.T) {
	repo := newMockWorkshopRepo()
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)
	svc := workshop.NewService(repo, log)

	ownerID := uuid.New()
	activeEmpUserID := uuid.New()
	inactiveEmpUserID := uuid.New()
	unrelatedUserID := uuid.New()

	wsID := uuid.New()
	ws := &domain.Workshop{
		ID:       wsID,
		OwnerID:  ownerID,
		Name:     "Bengkel Jaya Motor",
		Address:  "Jl. Merdeka No. 10",
		Status:   domain.WorkshopStatusActive,
		Photos:   []string{},
	}
	repo.workshops[wsID] = ws

	activeEmp := domain.WorkshopEmployee{
		ID:         uuid.New(),
		WorkshopID: wsID,
		UserID:     &activeEmpUserID,
		Name:       "Staff Inventory",
		Role:       domain.EmployeeRoleAdminInventory,
		Status:     domain.EmployeeStatusActive,
	}
	inactiveEmp := domain.WorkshopEmployee{
		ID:         uuid.New(),
		WorkshopID: wsID,
		UserID:     &inactiveEmpUserID,
		Name:       "Mantan Staff",
		Role:       domain.EmployeeRoleMechanic,
		Status:     domain.EmployeeStatusInactive,
	}
	repo.employees[wsID] = []domain.WorkshopEmployee{activeEmp, inactiveEmp}

	ctx := context.Background()

	// 1. Owner gets workshop
	ownerList, err := svc.GetMyWorkshops(ctx, ownerID)
	if err != nil {
		t.Fatalf("unexpected error for owner: %v", err)
	}
	if len(ownerList) != 1 || ownerList[0].ID != wsID {
		t.Fatalf("expected 1 workshop for owner, got %d", len(ownerList))
	}

	// 2. Active employee gets workshop
	empList, err := svc.GetMyWorkshops(ctx, activeEmpUserID)
	if err != nil {
		t.Fatalf("unexpected error for active employee: %v", err)
	}
	if len(empList) != 1 || empList[0].ID != wsID {
		t.Fatalf("expected 1 workshop for active employee, got %d", len(empList))
	}

	// 3. Inactive employee does not get workshop
	inactiveList, err := svc.GetMyWorkshops(ctx, inactiveEmpUserID)
	if err != nil {
		t.Fatalf("unexpected error for inactive employee: %v", err)
	}
	if len(inactiveList) != 0 {
		t.Fatalf("expected 0 workshops for inactive employee, got %d", len(inactiveList))
	}

	// 4. Unrelated user gets 0 workshops
	unrelatedList, err := svc.GetMyWorkshops(ctx, unrelatedUserID)
	if err != nil {
		t.Fatalf("unexpected error for unrelated user: %v", err)
	}
	if len(unrelatedList) != 0 {
		t.Fatalf("expected 0 workshops for unrelated user, got %d", len(unrelatedList))
	}
}


