package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// handleSeed creates the first user and project in this instance.
// Authenticated by the INSTANCE_SECRET env var. Rejects if any user exists.
func (s *Server) handleSeed(w http.ResponseWriter, r *http.Request) {
	secret := os.Getenv("INSTANCE_SECRET")
	if secret == "" {
		http.Error(w, `{"error":"seed not configured"}`, http.StatusNotFound)
		return
	}

	auth := r.Header.Get("Authorization")
	if auth != "Bearer "+secret {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	ctx := r.Context()

	// Reject if any user already exists.
	userCount, _ := s.meta.CountUsers(ctx)
	if userCount > 0 {
		http.Error(w, `{"error":"instance already seeded"}`, http.StatusConflict)
		return
	}

	var req struct {
		Email        string `json:"email"`
		PasswordHash string `json:"password_hash"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	if req.Email == "" || req.PasswordHash == "" {
		http.Error(w, `{"error":"email and password_hash required"}`, http.StatusBadRequest)
		return
	}

	user, err := s.meta.CreateUser(ctx, req.Email, req.PasswordHash)
	if err != nil {
		log.Printf("seed: create user: %v", err)
		http.Error(w, `{"error":"failed to create user"}`, http.StatusInternalServerError)
		return
	}

	projectID := "proj_" + randomHexN(8)
	project, err := s.meta.CreateProject(ctx, projectID, "My Project")
	if err != nil {
		log.Printf("seed: create project: %v", err)
		http.Error(w, `{"error":"failed to create project"}`, http.StatusInternalServerError)
		return
	}

	if err := s.meta.AddProjectMember(ctx, user.ID, project.ID, "owner"); err != nil {
		log.Printf("seed: add project member: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"project_id": project.ID,
		"api_key":    project.APIKey,
	})
}

// handleTokenExchange receives a JWT from the control plane, validates it,
// finds or creates the local user by email, and issues a local session.
func (s *Server) handleTokenExchange(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	controlPlaneURL := s.config.ControlPlaneURL
	if controlPlaneURL == "" {
		http.Error(w, `{"error":"not a cloud instance"}`, http.StatusNotFound)
		return
	}

	// Validate the JWT by calling the control plane's verify endpoint.
	verifyURL := controlPlaneURL + "/api/v1/auth/verify"
	verifyReq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, verifyURL, nil)
	verifyReq.Header.Set("Authorization", "Bearer "+req.Token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(verifyReq)
	if err != nil || resp.StatusCode != http.StatusOK {
		http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
		return
	}
	defer resp.Body.Close()

	var claims struct {
		CustomerID string `json:"customer_id"`
		Email      string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&claims); err != nil {
		http.Error(w, `{"error":"invalid token response"}`, http.StatusUnauthorized)
		return
	}

	ctx := r.Context()

	// Find or create local user by email.
	user, err := s.meta.GetUserByEmail(ctx, claims.Email)
	if err != nil {
		// User doesn't exist locally yet — create with a random password hash.
		randomHash, _ := bcrypt.GenerateFromPassword([]byte(randomHexN(32)), bcrypt.DefaultCost)
		user, err = s.meta.CreateUser(ctx, claims.Email, string(randomHash))
		if err != nil {
			log.Printf("token-exchange: create user: %v", err)
			http.Error(w, `{"error":"failed to create user"}`, http.StatusInternalServerError)
			return
		}
	}

	// Find user's projects.
	projects, _ := s.meta.ListUserProjects(ctx, user.ID)
	projectID := ""
	if len(projects) > 0 {
		projectID = projects[0].ID
	}

	token, err := s.meta.CreateUserSession(ctx, user.ID, time.Now().Add(7*24*time.Hour), projectID)
	if err != nil {
		http.Error(w, `{"error":"failed to create session"}`, http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "clicknest_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   7 * 24 * 60 * 60,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"token": token,
		"user":  user,
	})
}

// handleBillingProxy forwards billing requests to the control plane.
func (s *Server) handleBillingProxy(w http.ResponseWriter, r *http.Request) {
	controlPlaneURL := s.config.ControlPlaneURL
	instanceSecret := s.config.InstanceSecret
	if controlPlaneURL == "" || instanceSecret == "" {
		http.Error(w, `{"error":"not a cloud instance"}`, http.StatusNotFound)
		return
	}

	// Forward the request path to the control plane.
	targetURL := controlPlaneURL + r.URL.Path

	var bodyReader io.Reader
	if r.Body != nil {
		bodyBytes, _ := io.ReadAll(r.Body)
		bodyReader = bytes.NewReader(bodyBytes)
	}

	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, bodyReader)
	if err != nil {
		http.Error(w, `{"error":"proxy error"}`, http.StatusInternalServerError)
		return
	}
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyReq.Header.Set("Authorization", "Bearer "+instanceSecret)
	// Forward the user's session info.
	if fwdAuth := r.Header.Get("Authorization"); fwdAuth != "" {
		proxyReq.Header.Set("X-Forwarded-Auth", fwdAuth)
	}

	proxyClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := proxyClient.Do(proxyReq)
	if err != nil {
		http.Error(w, `{"error":"control plane unreachable"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers and body.
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// reportUsageToControlPlane sends accumulated usage to the control plane.
func reportUsageToControlPlane(ctx context.Context, controlPlaneURL, instanceID, instanceSecret string, eventCount int64) error {
	url := controlPlaneURL + "/api/v1/usage/report"
	body, _ := json.Marshal(map[string]any{
		"instance_id": instanceID,
		"events":      eventCount,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("usage report: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+instanceSecret)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("usage report: request failed: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("usage report: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func randomHexN(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
