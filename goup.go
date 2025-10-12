// Goup
package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	goDownloadURL = "https://go.dev/dl/"
	goDownloadAPI = "https://go.dev/dl/?mode=json&include=all"
	colorRed      = "\033[0;31m"
	colorGreen    = "\033[0;32m"
	colorYellow   = "\033[1;33m"
	colorBlue     = "\033[0;34m"
	colorReset    = "\033[0m"
)

type Release struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
	Files   []struct {
		Filename string `json:"filename"`
		OS       string `json:"os"`
		Arch     string `json:"arch"`
		Kind     string `json:"kind"`
		SHA256   string `json:"sha256"`
		Size     int64  `json:"size"`
	} `json:"files"`
}

type goup struct {
	currentVersion  string
	installVersion  string
	latestVersion   string
	architecture    string
	operatingSystem string
	installDir      string
	profileFiles    []string
	goPaths         []string
}

func printInfo(msg string) {
	fmt.Printf("%s[INFO]%s %s\n", colorBlue, colorReset, msg)
}

func printSuccess(msg string) {
	fmt.Printf("%s[PASS]%s %s\n", colorGreen, colorReset, msg)
}

func printWarning(msg string) {
	fmt.Printf("%s[WARN]%s %s\n", colorYellow, colorReset, msg)
}

func printError(msg string) {
	fmt.Printf("%s[FAIL]%s %s\n", colorRed, colorReset, msg)
}

func (g *goup) run() error {
	// Get current version
	current, err := g.getCurrentVersion()
	if err != nil {
		printWarning(fmt.Sprintf("Could not determine current Go version: %v", err))
		current = "none"
	}
	g.currentVersion = current

	if current == "none" {
		printInfo("Go is not currently installed")
	} else {
		printInfo(fmt.Sprintf("Current Go version: %s", current))
	}

	// Get latest version
	printInfo("Fetching latest Go version...")
	latest, err := g.getLatestVersion()
	if err != nil {
		return fmt.Errorf("failed to fetch latest version: %w", err)
	}
	g.latestVersion = latest
	printInfo(fmt.Sprintf("Latest Go version: %s", latest))

	if g.installVersion == "" {
		g.installVersion = latest
	} else if !g.validVersion() {
		return fmt.Errorf("invalid version: %s", g.installVersion)
	}

	// Compare versions
	if !g.needsUpdate() {
		printSuccess(fmt.Sprintf("Go is already up to date (version %s)", current))
		return nil
	}

	// Perform update
	if current != "none" {
		printInfo(fmt.Sprintf("Updating Go from %s to %s...", current, g.installVersion))
		if err := g.backupCurrentInstallation(); err != nil {
			printWarning(fmt.Sprintf("Failed to backup current installation: %v", err))
		}
	} else {
		printInfo(fmt.Sprintf("Installing Go %s...", g.installVersion))
	}

	if err := g.downloadAndInstall(); err != nil {
		return fmt.Errorf("failed to download and install: %w", err)
	}

	if err := g.updatePath(); err != nil {
		printWarning(fmt.Sprintf("Failed to update PATH: %v", err))
	}

	if err := g.verifyInstallation(); err != nil {
		return fmt.Errorf("installation verification failed: %w", err)
	}

	printSuccess(fmt.Sprintf("Go has been successfully updated to version %s", g.installVersion))
	g.printPostInstallInstructions()

	return nil
}

func (g *goup) getCurrentVersion() (string, error) {
	var lastErr error
	for _, goPath := range g.goPaths {
		cmd := exec.Command(goPath, "version")
		output, err := cmd.Output()
		if err != nil {
			lastErr = err
			continue
		}

		// Parse "go version go1.xx.x linux/amd64"
		parts := strings.Fields(string(output))
		if len(parts) >= 3 && strings.HasPrefix(parts[2], "go") {
			version := strings.TrimPrefix(parts[2], "go")
			// Also store the path for later use
			if goPath != "go" {
				printInfo(fmt.Sprintf("Found Go at: %s", goPath))
			}
			return version, nil
		}
	}

	return "", fmt.Errorf("go not found in common locations, last error: %v", lastErr)
}

func (g *goup) getLatestVersion() (string, error) {
	resp, err := http.Get(goDownloadAPI)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return "", err
	}

	// Find the latest stable release for current OS
	for _, release := range releases {
		if release.Stable && strings.HasPrefix(release.Version, "go") {
			// Check if this release has files for our OS/arch
			for _, file := range release.Files {
				if file.OS == g.operatingSystem && file.Arch == g.architecture && file.Kind == "archive" {
					return strings.TrimPrefix(release.Version, "go"), nil
				}
			}
		}
	}

	return "", fmt.Errorf("no stable release found for %s/%s", g.operatingSystem, g.architecture)
}

func (g *goup) needsUpdate() bool {
	if g.currentVersion == "none" {
		return true
	}

	current := parseVersion(g.currentVersion)
	install := parseVersion(g.installVersion)

	return compareVersions(current, install) < 0
}

func (g *goup) validVersion() bool {
	install := parseVersion(g.installVersion)
	if len(install) != 3 {
		return false
	}
	latest := parseVersion(g.latestVersion)

	return compareVersions(install, latest) <= 0
}

func (g *goup) backupCurrentInstallation() error {
	goDir := filepath.Join(g.installDir, "go")
	if _, err := os.Stat(goDir); os.IsNotExist(err) {
		return nil // Nothing to backup
	}

	timestamp := time.Now().Format("20060102_150405")
	backupDir := filepath.Join(g.installDir, fmt.Sprintf("go.backup.%s", timestamp))

	printInfo("Backing up current Go installation...")
	if err := os.Rename(goDir, backupDir); err != nil {
		return err
	}

	printSuccess(fmt.Sprintf("Backup created at: %s", backupDir))
	return nil
}

func (g *goup) downloadAndInstall() error {
	filename := fmt.Sprintf("go%s.%s-%s.tar.gz", g.installVersion, g.operatingSystem, g.architecture)
	downloadURL := fmt.Sprintf("%s%s", goDownloadURL, filename)
	tempFile := filepath.Join(os.TempDir(), filename)

	printInfo(fmt.Sprintf("Downloading Go %s for %s/%s...", g.installVersion, g.operatingSystem, g.architecture))
	if err := g.downloadFile(downloadURL, tempFile); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(tempFile)

	printInfo(fmt.Sprintf("Installing Go %s...", g.installVersion))
	if err := g.extractTarGz(tempFile, g.installDir); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Set proper permissions (important for Unix systems)
	goPath := filepath.Join(g.installDir, "go")
	if err := g.setPermissions(goPath); err != nil {
		printWarning(fmt.Sprintf("Failed to set permissions: %v", err))
	}

	printSuccess(fmt.Sprintf("Go %s installed successfully!", g.installVersion))
	return nil
}

func (g *goup) downloadFile(url, filepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	return err
}

func (g *goup) extractTarGz(src, dest string) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		path := filepath.Join(dest, header.Name)

		// Security check - ensure path is within destination
		if !strings.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}

			outFile, err := os.Create(path)
			if err != nil {
				return err
			}

			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}

			outFile.Close()

			if err := os.Chmod(path, os.FileMode(header.Mode)); err != nil {
				return err
			}
		}
	}

	return nil
}

func (g *goup) updatePath() error {
	goBinPath := filepath.Join(g.installDir, "go", "bin")

	for _, profilePath := range g.profileFiles {
		if err := g.addToProfile(profilePath, goBinPath); err != nil {
			printWarning(fmt.Sprintf("Failed to update %s: %v", profilePath, err))
			continue
		}
		printSuccess(fmt.Sprintf("Go added to system PATH in %s", profilePath))
		return nil // Success with at least one file
	}

	return fmt.Errorf("failed to update any profile files")
}

func (g *goup) addToProfile(profilePath, goBinPath string) error {
	// Check if file exists and is writable
	if _, err := os.Stat(profilePath); os.IsNotExist(err) {
		// Try to create the file
		file, err := os.Create(profilePath)
		if err != nil {
			return err
		}
		file.Close()
	}

	// Check if already in profile
	content, err := os.ReadFile(profilePath)
	if err == nil && strings.Contains(string(content), goBinPath) {
		return nil // Already added
	}

	printInfo(fmt.Sprintf("Adding Go to system PATH in %s...", profilePath))

	file, err := os.OpenFile(profilePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	pathEntry := fmt.Sprintf("\n# Go programming language\nexport PATH=$PATH:%s\n", goBinPath)
	_, err = file.WriteString(pathEntry)
	return err
}

func (g *goup) verifyInstallation() error {
	goBinPath := filepath.Join(g.installDir, "go", "bin", "go")

	cmd := exec.Command(goBinPath, "version")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("go command failed: %w", err)
	}

	parts := strings.Fields(string(output))
	if len(parts) >= 3 && strings.HasPrefix(parts[2], "go") {
		installedVersion := strings.TrimPrefix(parts[2], "go")
		if installedVersion == g.installVersion {
			printSuccess(fmt.Sprintf("Installation verified: Go %s", installedVersion))
			return nil
		}
		return fmt.Errorf("version mismatch: expected %s, got %s", g.installVersion, installedVersion)
	}

	return fmt.Errorf("unexpected go version output: %s", string(output))
}

func (g *goup) setOSDefaults() {
	switch g.operatingSystem {
	case "linux":
		g.installDir = "/usr/local"
		g.profileFiles = []string{
			"/etc/profile",
			"/etc/bash.bashrc",
			"/etc/zsh/zshrc",
		}
	case "darwin":
		g.installDir = "/usr/local"
		g.profileFiles = []string{
			"/etc/profile",
			"/etc/bashrc",
			"/etc/zshrc",
		}
	case "freebsd", "openbsd", "netbsd":
		g.installDir = "/usr/local"
		g.profileFiles = []string{
			"/etc/profile",
			"/etc/bash.bashrc",
		}
	default:
		// Generic Unix fallback
		g.installDir = "/usr/local"
		g.profileFiles = []string{
			"/etc/profile",
		}
	}

	// Get common Go installation paths for Unix systems
	g.getCommonGoPaths()
}

func (g *goup) getCommonGoPaths() {
	g.goPaths = append(g.goPaths, "go") // Try PATH first

	// Standard system locations
	systemPaths := []string{
		filepath.Join(g.installDir, "go", "bin", "go"),
		"/usr/bin/go",
		"/opt/go/bin/go",
	}

	// User locations
	homeDir := os.Getenv("HOME")
	if homeDir != "" {
		userPaths := []string{
			filepath.Join(homeDir, "go", "bin", "go"),
			filepath.Join(homeDir, ".local", "share", "go", "bin", "go"),
			filepath.Join(homeDir, ".go", "bin", "go"),
		}

		// macOS specific paths
		if g.operatingSystem == "darwin" {
			userPaths = append(userPaths,
				filepath.Join(homeDir, "Developer", "go", "bin", "go"),
				"/usr/local/Cellar/go/*/bin/go", // Homebrew (would need glob expansion)
			)
		}

		g.goPaths = append(g.goPaths, userPaths...)
	}

	g.goPaths = append(g.goPaths, systemPaths...)

	// Check GOROOT if set
	if goroot := os.Getenv("GOROOT"); goroot != "" {
		g.goPaths = append(g.goPaths, filepath.Join(goroot, "bin", "go"))
	}
}

func (g *goup) setPermissions(path string) error {
	return filepath.Walk(path, func(file string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return os.Chmod(file, 0755)
		}
		// Executable files in bin directory and pkg/tool directory
		if strings.Contains(file, "/bin/") || strings.Contains(file, "/pkg/tool/") {
			return os.Chmod(file, 0755)
		}
		return os.Chmod(file, 0644)
	})
}

func (g *goup) printPostInstallInstructions() {
	printInfo("Installation complete!")

	switch g.operatingSystem {
	case "darwin":
		printInfo("Please restart your terminal or run one of:")
		for _, profile := range g.profileFiles {
			if _, err := os.Stat(profile); err == nil {
				printInfo(fmt.Sprintf("  source %s", profile))
			}
		}
		printInfo("Or add the following to your shell profile:")
		printInfo(fmt.Sprintf("  export PATH=$PATH:%s/go/bin", g.installDir))
	case "linux":
		printInfo("Please restart your terminal or run:")
		printInfo("  source /etc/profile")
		printInfo("Or add the following to your shell profile:")
		printInfo(fmt.Sprintf("  export PATH=$PATH:%s/go/bin", g.installDir))
	default:
		printInfo("Please add the following to your shell profile:")
		printInfo(fmt.Sprintf("  export PATH=$PATH:%s/go/bin", g.installDir))
		printInfo("Then restart your terminal or source the profile file.")
	}
}

func isRoot() bool {
	return os.Geteuid() == 0
}

func parseVersion(version string) []int {
	parts := strings.Split(version, ".")
	result := make([]int, len(parts))
	for i, part := range parts {
		if num, err := strconv.Atoi(part); err == nil {
			result[i] = num
		}
	}
	return result
}

func compareVersions(v1, v2 []int) int {
	maxLen := max(len(v2), len(v1))

	for i := range maxLen {
		val1, val2 := 0, 0
		if i < len(v1) {
			val1 = v1[i]
		}
		if i < len(v2) {
			val2 = v2[i]
		}

		if val1 < val2 {
			return -1
		} else if val1 > val2 {
			return 1
		}
	}
	return 0
}

var version = "v0.1.0-alpha.1"

func main() {
	specVersion := flag.String("i", "", "Update to a specific version (SemVer: MAJOR.MINOR[.PATCH])")
	goupVersion := flag.Bool("v", false, "Show goup version")
	flag.Parse()

	if *goupVersion {
		fmt.Printf("goup %s\n", version)
		os.Exit(0)
	}

	updater := &goup{
		architecture:    runtime.GOARCH,
		operatingSystem: runtime.GOOS,
		installVersion:  *specVersion,
	}

	// Set OS-specific defaults
	updater.setOSDefaults()

	printInfo("goup - Go Version Updater")
	printInfo("-------------------------")

	if !isRoot() {
		printError("This program must be run as root (use sudo)")
		os.Exit(1)
	}

	// Debug: Show current PATH and OS info
	printInfo(fmt.Sprintf("Operating System: %s", updater.operatingSystem))
	printInfo(fmt.Sprintf("Architecture: %s", updater.architecture))
	printInfo(fmt.Sprintf("Install Directory: %s", updater.installDir))

	if err := updater.run(); err != nil {
		printError(fmt.Sprintf("Update failed: %v", err))
		os.Exit(1)
	}
}
