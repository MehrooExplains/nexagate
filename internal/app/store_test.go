package app

import (
	"path/filepath"
	"testing"
)

func TestStoreLifecycle(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "users.json"))
	user, err := store.Add("alice", 30)
	if err != nil {
		t.Fatal(err)
	}
	if user.UUID == "" || user.Password == "" || !user.Enabled {
		t.Fatalf("incomplete generated user: %#v", user)
	}
	if _, err := store.Add("alice", 30); err == nil {
		t.Fatal("duplicate username was accepted")
	}
	if err := store.Toggle(user.ID); err != nil {
		t.Fatal(err)
	}
	db, err := store.Load()
	if err != nil || len(db.Users) != 1 || db.Users[0].Enabled {
		t.Fatalf("toggle failed: %#v, %v", db, err)
	}
	if err := store.Delete(user.ID); err != nil {
		t.Fatal(err)
	}
	db, err = store.Load()
	if err != nil || len(db.Users) != 0 {
		t.Fatalf("delete failed: %#v, %v", db, err)
	}
}
