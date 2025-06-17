package common

import (
	"os"
	"reflect"
	"testing"
)

func TestLoadTransports_EmptyEnv(t *testing.T) {
	orig := os.Getenv("LANTERN_TRANSPORTS")
	defer os.Setenv("LANTERN_TRANSPORTS", orig)

	os.Unsetenv("LANTERN_TRANSPORTS")
	got := loadTransports()
	want := []string{}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Expected %v, got %v", want, got)
	}
}

func TestLoadTransports_SingleTransport(t *testing.T) {
	orig := os.Getenv("LANTERN_TRANSPORTS")
	defer os.Setenv("LANTERN_TRANSPORTS", orig)

	os.Setenv("LANTERN_TRANSPORTS", "obfs4")
	got := loadTransports()
	want := []string{"obfs4"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Expected %v, got %v", want, got)
	}
}

func TestLoadTransports_MultipleTransports(t *testing.T) {
	orig := os.Getenv("LANTERN_TRANSPORTS")
	defer os.Setenv("LANTERN_TRANSPORTS", orig)

	os.Setenv("LANTERN_TRANSPORTS", "obfs4, meek,  shadowsocks ")
	got := loadTransports()
	want := []string{"obfs4", "meek", "shadowsocks"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Expected %v, got %v", want, got)
	}
}

func TestLoadTransports_WhitespaceOnly(t *testing.T) {
	orig := os.Getenv("LANTERN_TRANSPORTS")
	defer os.Setenv("LANTERN_TRANSPORTS", orig)

	os.Setenv("LANTERN_TRANSPORTS", "   ")
	got := loadTransports()
	want := []string{""}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Expected %v, got %v", want, got)
	}
}
