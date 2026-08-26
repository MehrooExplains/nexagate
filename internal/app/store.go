package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"
)

var validUserName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,31}$`)

type Store struct {
	path string
	mu   sync.Mutex
}

func NewStore(path string) *Store { return &Store{path: path} }

func (s *Store) Load() (Database, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return loadDatabase(s.path)
}

func loadDatabase(path string) (Database, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Database{Users: []User{}}, nil
	}
	if err != nil {
		return Database{}, err
	}
	var db Database
	if err := json.Unmarshal(data, &db); err != nil {
		return Database{}, fmt.Errorf("decode user database: %w", err)
	}
	if db.Users == nil {
		db.Users = []User{}
	}
	sort.Slice(db.Users, func(i, j int) bool { return db.Users[i].CreatedAt.Before(db.Users[j].CreatedAt) })
	return db, nil
}

func (s *Store) Add(name string, days int) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validUserName.MatchString(name) {
		return User{}, errors.New("username must contain 1-32 letters, numbers, dot, dash or underscore")
	}
	db, err := loadDatabase(s.path)
	if err != nil {
		return User{}, err
	}
	for _, user := range db.Users {
		if user.Name == name {
			return User{}, errors.New("username already exists")
		}
	}
	id, err := randomToken(12)
	if err != nil {
		return User{}, err
	}
	password, err := randomToken(18)
	if err != nil {
		return User{}, err
	}
	uuid, err := randomUUID()
	if err != nil {
		return User{}, err
	}
	now := time.Now().UTC()
	user := User{ID: id, Name: name, Password: password, UUID: uuid, Enabled: true, CreatedAt: now}
	if days > 0 {
		user.ExpiresAt = now.Add(time.Duration(days) * 24 * time.Hour)
	}
	db.Users = append(db.Users, user)
	db.UpdatedAt = now
	return user, saveJSONAtomic(s.path, db, 0600)
}

func (s *Store) Toggle(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	db, err := loadDatabase(s.path)
	if err != nil {
		return err
	}
	found := false
	for i := range db.Users {
		if db.Users[i].ID == id {
			db.Users[i].Enabled = !db.Users[i].Enabled
			found = true
			break
		}
	}
	if !found {
		return errors.New("user not found")
	}
	db.UpdatedAt = time.Now().UTC()
	return saveJSONAtomic(s.path, db, 0600)
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	db, err := loadDatabase(s.path)
	if err != nil {
		return err
	}
	users := db.Users[:0]
	found := false
	for _, user := range db.Users {
		if user.ID == id {
			found = true
			continue
		}
		users = append(users, user)
	}
	if !found {
		return errors.New("user not found")
	}
	db.Users = users
	db.UpdatedAt = time.Now().UTC()
	return saveJSONAtomic(s.path, db, 0600)
}

func saveJSONAtomic(path string, value any, mode os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeAtomic(path, data, mode)
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".nexagate-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
