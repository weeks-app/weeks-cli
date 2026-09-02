package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileStoreCleansLockAfterSaveAndDelete(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	creds := &Credentials{AccessToken: "tok", BaseURL: "https://weeks.example"}

	if err := store.Save("default", creds); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "credentials.json.lock")); !os.IsNotExist(err) {
		t.Fatalf("lock remains after save, err = %v", err)
	}
	if got, err := store.Load("default", creds.BaseURL); err != nil || got.AccessToken != "tok" {
		t.Fatalf("Load = %#v, %v", got, err)
	}

	if err := store.Delete("default", creds.BaseURL); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "credentials.json.lock")); !os.IsNotExist(err) {
		t.Fatalf("lock remains after delete, err = %v", err)
	}
	if _, err := store.Load("default", creds.BaseURL); !IsNotFound(err) {
		t.Fatalf("Load after delete err = %v", err)
	}
}
