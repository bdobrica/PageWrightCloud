package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/storage/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockBackend is a mock implementation of storage.Backend
type MockBackend struct {
	mock.Mock
}

func (m *MockBackend) StoreArtifact(siteID, buildID string, reader io.Reader) error {
	args := m.Called(siteID, buildID, reader)
	return args.Error(0)
}

func (m *MockBackend) FetchArtifact(siteID, buildID string) (io.ReadCloser, error) {
	args := m.Called(siteID, buildID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

func (m *MockBackend) WriteLogEntry(siteID string, entry *storage.LogEntry) error {
	args := m.Called(siteID, entry)
	return args.Error(0)
}

func (m *MockBackend) ListVersions(siteID string) ([]*storage.Version, error) {
	args := m.Called(siteID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Version), args.Error(1)
}

func TestHealthCheck(t *testing.T) {
	mockBackend := new(MockBackend)
	handler := NewHandler(mockBackend)
	router := handler.SetupRoutes()

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "healthy", response["status"])
	assert.NotEmpty(t, response["time"])
}

func TestStoreArtifact(t *testing.T) {
	mockBackend := new(MockBackend)
	handler := NewHandler(mockBackend)
	router := handler.SetupRoutes()

	content := []byte("test artifact")
	mockBackend.On("StoreArtifact", "test-site", "build-123", mock.Anything).Return(nil)

	req := httptest.NewRequest("PUT", "/sites/test-site/artifacts/build-123", bytes.NewReader(content))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "test-site", response["site_id"])
	assert.Equal(t, "build-123", response["build_id"])

	mockBackend.AssertExpectations(t)
}

func TestStoreArtifactError(t *testing.T) {
	mockBackend := new(MockBackend)
	handler := NewHandler(mockBackend)
	router := handler.SetupRoutes()

	content := []byte("test artifact")
	mockBackend.On("StoreArtifact", "test-site", "build-123", mock.Anything).Return(assert.AnError)

	req := httptest.NewRequest("PUT", "/sites/test-site/artifacts/build-123", bytes.NewReader(content))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockBackend.AssertExpectations(t)
}

func TestFetchArtifact(t *testing.T) {
	mockBackend := new(MockBackend)
	handler := NewHandler(mockBackend)
	router := handler.SetupRoutes()

	content := []byte("test artifact content")
	mockReader := io.NopCloser(bytes.NewReader(content))

	mockBackend.On("FetchArtifact", "test-site", "build-123").Return(mockReader, nil)

	req := httptest.NewRequest("GET", "/sites/test-site/artifacts/build-123", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/gzip", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Header().Get("Content-Disposition"), "test-site-build-123.tar.gz")
	assert.Equal(t, content, w.Body.Bytes())

	mockBackend.AssertExpectations(t)
}

func TestFetchArtifactNotFound(t *testing.T) {
	mockBackend := new(MockBackend)
	handler := NewHandler(mockBackend)
	router := handler.SetupRoutes()

	mockBackend.On("FetchArtifact", "test-site", "build-123").Return(nil, assert.AnError)

	req := httptest.NewRequest("GET", "/sites/test-site/artifacts/build-123", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	mockBackend.AssertExpectations(t)
}

func TestWriteLog(t *testing.T) {
	mockBackend := new(MockBackend)
	handler := NewHandler(mockBackend)
	router := handler.SetupRoutes()

	logReq := LogRequest{
		BuildID: "build-123",
		Action:  "build",
		Status:  "success",
		Metadata: map[string]string{
			"key": "value",
		},
	}

	mockBackend.On("WriteLogEntry", "test-site", mock.AnythingOfType("*storage.LogEntry")).Return(nil)

	body, _ := json.Marshal(logReq)
	req := httptest.NewRequest("POST", "/sites/test-site/logs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "test-site", response["site_id"])
	assert.Equal(t, "build-123", response["build_id"])

	mockBackend.AssertExpectations(t)
}

func TestWriteLogMissingFields(t *testing.T) {
	mockBackend := new(MockBackend)
	handler := NewHandler(mockBackend)
	router := handler.SetupRoutes()

	logReq := LogRequest{
		BuildID: "build-123",
		// Missing Action and Status
	}

	body, _ := json.Marshal(logReq)
	req := httptest.NewRequest("POST", "/sites/test-site/logs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListVersions(t *testing.T) {
	mockBackend := new(MockBackend)
	handler := NewHandler(mockBackend)
	router := handler.SetupRoutes()

	versions := []*storage.Version{
		{
			BuildID:   "build-3",
			Timestamp: time.Now().UTC(),
			Action:    "deploy",
			Status:    "success",
		},
		{
			BuildID:   "build-2",
			Timestamp: time.Now().UTC().Add(-1 * time.Hour),
			Action:    "build",
			Status:    "success",
		},
	}

	mockBackend.On("ListVersions", "test-site").Return(versions, nil)

	req := httptest.NewRequest("GET", "/sites/test-site/versions", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "test-site", response["site_id"])
	assert.Equal(t, float64(2), response["count"])

	mockBackend.AssertExpectations(t)
}

func TestListVersionsEmpty(t *testing.T) {
	mockBackend := new(MockBackend)
	handler := NewHandler(mockBackend)
	router := handler.SetupRoutes()

	mockBackend.On("ListVersions", "test-site").Return([]*storage.Version{}, nil)

	req := httptest.NewRequest("GET", "/sites/test-site/versions", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, float64(0), response["count"])

	mockBackend.AssertExpectations(t)
}

func TestListVersionsError(t *testing.T) {
	mockBackend := new(MockBackend)
	handler := NewHandler(mockBackend)
	router := handler.SetupRoutes()

	mockBackend.On("ListVersions", "test-site").Return(nil, assert.AnError)

	req := httptest.NewRequest("GET", "/sites/test-site/versions", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockBackend.AssertExpectations(t)
}
