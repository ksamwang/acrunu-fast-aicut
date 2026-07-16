package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

func TestUserManagementRequiresAdminAndUsesDatabaseStyleSessions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userService := services.NewUserService(config.Config{})
	server := New(Options{
		Config:      config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
		UserService: userService,
	})

	adminSession := loginSession(t, server, "admin", "admin123")
	listRequest := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	listRequest.Header.Set("Authorization", "Bearer "+adminSession)
	listRecorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(listRecorder, listRequest)
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("list users: expected 200, got %d body=%s", listRecorder.Code, listRecorder.Body.String())
	}

	createBody := bytes.NewBufferString(`{"username":"writer","display_name":"文案用户","password":"writer123","role":"user"}`)
	createRequest := httptest.NewRequest(http.MethodPost, "/api/admin/users", createBody)
	createRequest.Header.Set("Authorization", "Bearer "+adminSession)
	createRequest.Header.Set("Content-Type", "application/json")
	createRecorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(createRecorder, createRequest)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create user: expected 201, got %d body=%s", createRecorder.Code, createRecorder.Body.String())
	}
	var created struct {
		Data services.User `json:"data"`
	}
	if err := json.Unmarshal(createRecorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created user: %v", err)
	}
	if created.Data.ID == "" {
		t.Fatal("created user did not include an id")
	}

	userSession := loginSession(t, server, "writer", "writer123")
	forbiddenRequest := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	forbiddenRequest.Header.Set("Authorization", "Bearer "+userSession)
	forbiddenRecorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(forbiddenRecorder, forbiddenRequest)
	if forbiddenRecorder.Code != http.StatusForbidden {
		t.Fatalf("user management access: expected 403, got %d body=%s", forbiddenRecorder.Code, forbiddenRecorder.Body.String())
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/admin/users/"+created.Data.ID, nil)
	deleteRequest.Header.Set("Authorization", "Bearer "+adminSession)
	deleteRecorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(deleteRecorder, deleteRequest)
	if deleteRecorder.Code != http.StatusOK {
		t.Fatalf("delete user: expected 200, got %d body=%s", deleteRecorder.Code, deleteRecorder.Body.String())
	}

	deletedSessionRequest := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	deletedSessionRequest.Header.Set("Authorization", "Bearer "+userSession)
	deletedSessionRecorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(deletedSessionRecorder, deletedSessionRequest)
	if deletedSessionRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("deleted user session: expected 401, got %d body=%s", deletedSessionRecorder.Code, deletedSessionRecorder.Body.String())
	}
}

func loginSession(t *testing.T, server *Server, username string, password string) string {
	t.Helper()
	body := bytes.NewBufferString(`{"username":"` + username + `","password":"` + password + `"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.Engine().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("login %s: expected 200, got %d body=%s", username, recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if response.Data.Token == "" {
		t.Fatal("login response did not include a session token")
	}
	return response.Data.Token
}
