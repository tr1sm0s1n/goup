package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var (
	tmpDir      = os.TempDir()
	profilePath = tmpDir + "/profile"
)

func TestMain(m *testing.M) {
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		panic(err)
	}

	if _, err := os.Create(profilePath); err != nil {
		panic(err)
	}

	code := m.Run()
	os.Exit(code)
}

// This is a test-exclusive method to substitute `setOSDefaults`.
func (g *goup) testOSDefaults() {
	installDir, _ := os.MkdirTemp(tmpDir, "*")
	g.installDir = filepath.Join(installDir, "/local")
	g.profileFiles = append(g.profileFiles, profilePath)
	g.goPaths = append(g.goPaths, g.installDir)
}

func TestInvalidInstall(t *testing.T) {
	updater := &goup{
		ctx:             context.Background(),
		architecture:    runtime.GOARCH,
		operatingSystem: runtime.GOOS,
		installVersion:  "xyz",
	}

	updater.testOSDefaults()

	err := updater.run()
	if !strings.Contains(err.Error(), "invalid version") {
		t.Fatalf("Unexpected error during invalid installation: %v", err)
	}
}

func TestSpecificInstall(t *testing.T) {
	updater := &goup{
		ctx:             context.Background(),
		architecture:    runtime.GOARCH,
		operatingSystem: runtime.GOOS,
		installVersion:  "1.24.0",
	}

	updater.testOSDefaults()

	if err := updater.run(); err != nil {
		t.Fatalf("Failed to test specific installation: %v", err)
	}
}

func TestLatestInstall(t *testing.T) {
	updater := &goup{
		ctx:             context.Background(),
		architecture:    runtime.GOARCH,
		operatingSystem: runtime.GOOS,
	}

	updater.testOSDefaults()

	if err := updater.run(); err != nil {
		t.Fatalf("Failed to test latest installation: %v", err)
	}
}

func TestContextTimeout(t *testing.T) {
	var err error
	done := make(chan struct{})
	ticker := time.NewTicker(3 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	updater := &goup{
		ctx:             ctx,
		architecture:    runtime.GOARCH,
		operatingSystem: runtime.GOOS,
	}

	updater.testOSDefaults()

	go func() {
		err = updater.run()
		close(done)
	}()

	select {
	case <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Unexpected error during context timeout: %v", err)
		}
	case <-ticker.C:
		t.Fatal("Context persisted after timeout")
	}
}
