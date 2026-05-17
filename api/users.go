package api

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen"`
}

type AuditLog struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Action    string    `json:"action"`
	Resource  string    `json:"resource"`
	Success   bool      `json:"success"`
	Timestamp time.Time `json:"timestamp"`
	Details   string    `json:"details,omitempty"`
}

type UserStore struct {
	users    map[string]*User
	auditLog []AuditLog
	mu       sync.RWMutex
	filePath string
}

type UserData struct {
	Users    map[string]*User `json:"users"`
	AuditLog []AuditLog       `json:"audit_log"`
}

func NewUserStore(filePath string) *UserStore {
	store := &UserStore{
		users:    make(map[string]*User),
		auditLog: make([]AuditLog, 0),
		filePath: filePath,
	}
	store.load()
	return store
}

func (s *UserStore) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}
	var userData UserData
	if err := json.Unmarshal(data, &userData); err != nil {
		return
	}
	s.users = userData.Users
	s.auditLog = userData.AuditLog
}

// saveLocked writes data to disk. Caller MUST hold s.mu.
func (s *UserStore) saveLocked() error {
	userData := UserData{
		Users:    s.users,
		AuditLog: s.auditLog,
	}
	data, err := json.MarshalIndent(userData, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
}

func (s *UserStore) GetOrCreateUser(userID, username string) *User {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, exists := s.users[userID]
	if !exists {
		user = &User{
			ID:        userID,
			Username:  username,
			Role:      RoleViewer,
			CreatedAt: time.Now(),
			LastSeen:  time.Now(),
		}
		s.users[userID] = user
		s.saveLocked()
	} else {
		user.LastSeen = time.Now()
		s.saveLocked()
	}
	return user
}

func (s *UserStore) UpdateRole(userID string, role Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, exists := s.users[userID]
	if !exists {
		return nil
	}
	user.Role = role
	return s.saveLocked()
}

func (s *UserStore) GetUsers() []*User {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]*User, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, user)
	}
	return users
}

func (s *UserStore) LogAction(userID, username, action, resource string, success bool, details string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	log := AuditLog{
		ID:        time.Now().Format("20060102150405") + "-" + userID,
		UserID:    userID,
		Username:  username,
		Action:    action,
		Resource:  resource,
		Success:   success,
		Timestamp: time.Now(),
		Details:   details,
	}
	s.auditLog = append(s.auditLog, log)
	if len(s.auditLog) > 1000 {
		s.auditLog = s.auditLog[len(s.auditLog)-1000:]
	}
	s.saveLocked()
}

func (s *UserStore) GetAuditLog(limit int) []AuditLog {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.auditLog) {
		limit = len(s.auditLog)
	}
	logs := make([]AuditLog, limit)
	copy(logs, s.auditLog[len(s.auditLog)-limit:])
	for i, j := 0, len(logs)-1; i < j; i, j = i+1, j-1 {
		logs[i], logs[j] = logs[j], logs[i]
	}
	return logs
}

func (s *UserStore) CanPerformAction(userID, action string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, exists := s.users[userID]
	if !exists {
		return false
	}

	switch user.Role {
	case RoleAdmin:
		return true
	case RoleOperator:
		return action != "delete" && action != "manage_users"
	case RoleViewer:
		return action == "view" || action == "logs" || action == "stats"
	default:
		return false
	}
}
