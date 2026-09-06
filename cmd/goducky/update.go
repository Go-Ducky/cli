package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	updateRepo        = "Go-Ducky/cli"
	updateCheckURL    = "https://api.github.com/repos/" + updateRepo + "/releases/latest"
	updateReleaseFmt  = "https://api.github.com/repos/" + updateRepo + "/releases/tags/%s"
	updateDownloadURL = "https://github.com/" + updateRepo + "/releases/download/%s/%s"
)

func updateCmd(args []string) error {
	ref := "latest"
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		ref = strings.TrimSpace(args[0])
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return err
	}

	release, tag, err := fetchRelease(ref)
	if err != nil {
		return err
	}
	if tag != "" && tag == buildVersionTag() {
		fmt.Println("You're already on the newest release (" + tag + ").")
		return nil
	}

	assetName := platformAsset()
	if err := updateFromRelease(exe, release, assetName, tag); err != nil {
		return err
	}
	fmt.Printf("Updated GoDucky to %s (%s). Restart GoDucky to use it.\n", tag, assetName)
	return nil
}

type releaseAssets struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Assets  []struct {
		Name        string `json:"name"`
		DownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func fetchRelease(ref string) (releaseAssets, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var url string
	if ref == "latest" {
		url = updateCheckURL
	} else {
		url = fmt.Sprintf(updateReleaseFmt, ref)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return releaseAssets{}, "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return releaseAssets{}, "", fmt.Errorf("could not reach GitHub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return releaseAssets{}, "", fmt.Errorf("release %q not found", ref)
	}
	if resp.StatusCode != http.StatusOK {
		return releaseAssets{}, "", fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}
	var rel releaseAssets
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return releaseAssets{}, "", err
	}
	return rel, rel.TagName, nil
}

func platformAsset() string {
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	return "goducky-" + runtime.GOOS + "-" + runtime.GOARCH + suffix
}

func buildVersionTag() string {
	v := strings.TrimSpace(version)
	for _, part := range strings.Split(v, "-") {
		if isHexSHA(part) {
			return "dev-" + part
		}
	}
	return "v" + v
}

func isHexSHA(s string) bool {
	if len(s) < 7 || len(s) > 40 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

func updateFromRelease(exe string, rel releaseAssets, assetName, tag string) error {
	var checksums map[string]string
	for _, a := range rel.Assets {
		if a.Name == "checksums.txt" {
			data, err := downloadBytes(a.DownloadURL, 2*1024*1024)
			if err == nil {
				checksums = parseChecksums(string(data))
			}
			break
		}
	}

	downloadURL := ""
	for _, a := range rel.Assets {
		if a.Name == assetName {
			downloadURL = a.DownloadURL
			break
		}
	}
	if downloadURL == "" {
		if runtime.GOOS == "darwin" {

			for _, a := range rel.Assets {
				if a.Name == "goducky-darwin-universal" {
					assetName = a.Name
					downloadURL = a.DownloadURL
				}
			}
		}
	}
	if downloadURL == "" {
		names := make([]string, 0, len(rel.Assets))
		for _, a := range rel.Assets {
			names = append(names, a.Name)
		}
		return fmt.Errorf("release %q has no asset for this platform (%s). Available: %s", tag, assetName, strings.Join(names, ", "))
	}

	fmt.Printf("Downloading %s...\n", assetName)
	body, err := downloadBytes(downloadURL, 512*1024*1024)
	if err != nil {
		return err
	}
	if want, ok := checksums[assetName]; ok {
		got := hex.EncodeToString(sum256(body))
		if !strings.EqualFold(want, got) {
			return fmt.Errorf("checksum mismatch for %s (got %s, want %s)", assetName, got, want)
		}
		fmt.Println("checksum verified")
	} else if len(checksums) > 0 {
		fmt.Println("warning: no checksum entry for " + assetName)
	} else {
		fmt.Println("warning: no checksums.txt in release, skipping verification")
	}

	tmp := filepath.Join(filepath.Dir(exe), ".goducky-update"+filepath.Ext(exe))
	if err := os.WriteFile(tmp, body, 0o755); err != nil {
		return err
	}
	if err := replaceExecutable(tmp, exe); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("could not replace executable: %w", err)
	}
	return nil
}

func downloadBytes(url string, max int) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed with HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(max)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > max {
		return nil, fmt.Errorf("download exceeded %d bytes", max)
	}
	return body, nil
}

func sum256(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}

func parseChecksums(s string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			out[fields[1]] = strings.ToLower(fields[0])
		}
	}
	return out
}

func replaceExecutable(newPath, exe string) error {
	if runtime.GOOS == "windows" {
		backup := exe + ".bak"
		os.Remove(backup)
		if err := os.Rename(exe, backup); err != nil {
			return err
		}
		if err := os.Rename(newPath, exe); err != nil {
			os.Rename(backup, exe)
			return err
		}
		os.Remove(backup)
		return nil
	}
	if err := os.Rename(newPath, exe); err != nil {
		return err
	}
	return os.Chmod(exe, 0o755)
}
