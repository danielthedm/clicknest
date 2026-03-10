package server

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/danielthedm/clicknest/internal/auth"
)

const sessionDuration = 30 * 24 * time.Hour

func (s *Server) setupRequiredHandler(w http.ResponseWriter, r *http.Request) {
	n, err := s.meta.CountUsers(r.Context())
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"required": n == 0})
}

func (s *Server) setupHandler(w http.ResponseWriter, r *http.Request) {
	n, err := s.meta.CountUsers(r.Context())
	if err != nil || n > 0 {
		http.Error(w, `{"error":"setup already complete"}`, http.StatusForbidden)
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || len(req.Password) < 8 || len(req.Password) > 1024 {
		http.Error(w, `{"error":"email and password (min 8 chars) required"}`, http.StatusBadRequest)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	user, err := s.meta.CreateUser(r.Context(), req.Email, string(hash))
	if err != nil {
		http.Error(w, `{"error":"failed to create user"}`, http.StatusInternalServerError)
		return
	}

	// Link new user to all existing projects as owner.
	projects, _ := s.meta.ListProjects(r.Context())
	for _, p := range projects {
		s.meta.AddProjectMember(r.Context(), user.ID, p.ID, "owner")
	}

	// Issue session with first project (if any).
	var firstProjectID string
	if len(projects) > 0 {
		firstProjectID = projects[0].ID
	}
	if err := s.issueSession(w, r, user.ID, firstProjectID); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) loginHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	user, err := s.meta.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		// Run bcrypt anyway to prevent timing attacks.
		bcrypt.CompareHashAndPassword([]byte("$2a$10$dummy.dummy.dummy.dummy.dummy.dummy.dummy.dummy.dummyu"), []byte(req.Password))
		http.Error(w, `{"error":"invalid email or password"}`, http.StatusUnauthorized)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		http.Error(w, `{"error":"invalid email or password"}`, http.StatusUnauthorized)
		return
	}

	// Get user's first project for the session.
	var firstProjectID string
	userProjects, _ := s.meta.ListUserProjects(r.Context(), user.ID)
	if len(userProjects) > 0 {
		firstProjectID = userProjects[0].ID
	}

	if err := s.issueSession(w, r, user.ID, firstProjectID); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) logoutHandler(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(auth.SessionCookieName); err == nil {
		s.meta.DeleteUserSession(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) meHandler(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	activeProject := auth.ProjectFromContext(r.Context())

	projects, _ := s.meta.ListUserProjects(r.Context(), userID)
	// Fallback for users with no memberships yet.
	if len(projects) == 0 {
		projects, _ = s.meta.ListProjects(r.Context())
	}

	type projectInfo struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	pList := make([]projectInfo, len(projects))
	for i, p := range projects {
		pList[i] = projectInfo{ID: p.ID, Name: p.Name}
	}

	resp := struct {
		UserID        string        `json:"user_id"`
		ActiveProject *projectInfo  `json:"active_project,omitempty"`
		Projects      []projectInfo `json:"projects"`
	}{
		UserID:   userID,
		Projects: pList,
	}
	if activeProject != nil {
		resp.ActiveProject = &projectInfo{ID: activeProject.ID, Name: activeProject.Name}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) issueSession(w http.ResponseWriter, r *http.Request, userID, projectID string) error {
	expires := time.Now().UTC().Add(sessionDuration)
	token, err := s.meta.CreateUserSession(r.Context(), userID, expires, projectID)
	if err != nil {
		return err
	}
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
	return nil
}

// --- Multi-project management handlers ---

func (s *Server) switchProjectHandler(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	var req struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ProjectID == "" {
		http.Error(w, `{"error":"project_id required"}`, http.StatusBadRequest)
		return
	}

	// Verify user has access to this project.
	_, err := s.meta.GetUserProjectRole(r.Context(), userID, req.ProjectID)
	if err != nil {
		http.Error(w, `{"error":"not a member of this project"}`, http.StatusForbidden)
		return
	}

	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	if err := s.meta.SwitchSessionProject(r.Context(), cookie.Value, req.ProjectID); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) listProjectsHandler(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	projects, err := s.meta.ListUserProjects(r.Context(), userID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"projects": projects})
}

func (s *Server) createProjectHandler(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, `{"error":"name required"}`, http.StatusBadRequest)
		return
	}

	id, err := generateProjectID()
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	project, err := s.meta.CreateProject(r.Context(), id, req.Name)
	if err != nil {
		http.Error(w, `{"error":"failed to create project"}`, http.StatusInternalServerError)
		return
	}

	// Auto-add creator as owner.
	if err := s.meta.AddProjectMember(r.Context(), userID, project.ID, "owner"); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(project)
}

func (s *Server) listMembersHandler(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	userID := auth.UserIDFromContext(r.Context())

	// Verify caller is a member.
	_, err := s.meta.GetUserProjectRole(r.Context(), userID, projectID)
	if err != nil {
		http.Error(w, `{"error":"not a member of this project"}`, http.StatusForbidden)
		return
	}

	members, err := s.meta.ListProjectMembers(r.Context(), projectID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"members": members})
}

func (s *Server) addMemberHandler(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	callerID := auth.UserIDFromContext(r.Context())

	// Only owners can add members.
	role, err := s.meta.GetUserProjectRole(r.Context(), callerID, projectID)
	if err != nil || role != "owner" {
		http.Error(w, `{"error":"owner access required"}`, http.StatusForbidden)
		return
	}

	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		http.Error(w, `{"error":"email required"}`, http.StatusBadRequest)
		return
	}
	if req.Role == "" {
		req.Role = "member"
	}
	if req.Role != "owner" && req.Role != "member" {
		http.Error(w, `{"error":"role must be owner or member"}`, http.StatusBadRequest)
		return
	}

	user, err := s.meta.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	if err := s.meta.AddProjectMember(r.Context(), user.ID, projectID, req.Role); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) removeMemberHandler(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	targetUserID := r.PathValue("userID")
	callerID := auth.UserIDFromContext(r.Context())

	// Only owners can remove members.
	role, err := s.meta.GetUserProjectRole(r.Context(), callerID, projectID)
	if err != nil || role != "owner" {
		http.Error(w, `{"error":"owner access required"}`, http.StatusForbidden)
		return
	}

	if err := s.meta.RemoveProjectMember(r.Context(), targetUserID, projectID); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func generateProjectID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "proj_" + hex.EncodeToString(b), nil
}
