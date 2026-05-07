package api

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

type Template struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Image       string            `json:"image"`
	Ports       []string          `json:"ports"`
	Volumes     []string          `json:"volumes"`
	Env         map[string]string `json:"env"`
	Network     string            `json:"network"`
	RestartPolicy string          `json:"restart_policy"`
	CreatedBy   string            `json:"created_by"`
	CreatedAt   time.Time         `json:"created_at"`
	Public      bool              `json:"public"`
	UsageCount  int               `json:"usage_count"`
	Tags        []string          `json:"tags"`
}

type TemplateStore struct {
	templates map[string]*Template
	mu        sync.RWMutex
	filePath  string
}

func NewTemplateStore(filePath string) *TemplateStore {
	store := &TemplateStore{
		templates: make(map[string]*Template),
		filePath:  filePath,
	}
	store.load()
	return store
}

func (s *TemplateStore) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}
	if err := json.Unmarshal(data, &s.templates); err != nil {
		return
	}
}

func (s *TemplateStore) save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := json.MarshalIndent(s.templates, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
}

func (s *TemplateStore) Create(template *Template) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	template.ID = time.Now().Format("20060102150405") + "-" + template.CreatedBy
	template.CreatedAt = time.Now()
	template.UsageCount = 0
	s.templates[template.ID] = template
	return s.save()
}

func (s *TemplateStore) Get(id string) (*Template, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	template, exists := s.templates[id]
	return template, exists
}

func (s *TemplateStore) List(userID string, publicOnly bool) []*Template {
	s.mu.RLock()
	defer s.mu.RUnlock()
	templates := make([]*Template, 0)
	for _, t := range s.templates {
		if publicOnly && !t.Public {
			continue
		}
		if !publicOnly && !t.Public && t.CreatedBy != userID {
			continue
		}
		templates = append(templates, t)
	}
	return templates
}

func (s *TemplateStore) Delete(id, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	template, exists := s.templates[id]
	if !exists {
		return nil
	}
	if template.CreatedBy != userID {
		return nil
	}
	delete(s.templates, id)
	return s.save()
}

func (s *TemplateStore) IncrementUsage(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if template, exists := s.templates[id]; exists {
		template.UsageCount++
		s.save()
	}
}
