package wtc

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func statusForgeCacheAge() time.Duration {
	seconds, err := strconv.Atoi(os.Getenv("WTC_FORGE_CACHE_AGE"))
	if err != nil || seconds < 1 {
		seconds = 90
	}
	return time.Duration(seconds) * time.Second
}

func statusForgeCacheDir() string {
	return filepath.Join(os.TempDir(), "wtc-status-"+strconv.Itoa(os.Getuid()))
}

func statusForgeCachePath(forge, slug, number string) string {
	key := sha256.Sum256([]byte(forge + "\x00" + slug + "\x00" + number))
	return filepath.Join(statusForgeCacheDir(), "pr-"+hex.EncodeToString(key[:])+".json")
}

func statusSecureCacheDir(path string) bool {
	if err := os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
		return false
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	if info.Mode().Perm() != 0700 && os.Chmod(path, 0700) != nil {
		return false
	}
	return true
}

func statusReadForgeCache(forge, slug, number string) ([]byte, bool) {
	path := statusForgeCachePath(forge, slug, number)
	if !statusSecureCacheDir(filepath.Dir(path)) {
		return nil, false
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || time.Since(info.ModTime()) >= statusForgeCacheAge() {
		return nil, false
	}
	data, err := os.ReadFile(path)
	return data, err == nil && len(data) != 0
}

func statusWriteForgeCache(forge, slug, number string, data []byte) {
	if len(data) == 0 {
		return
	}
	path := statusForgeCachePath(forge, slug, number)
	if !statusSecureCacheDir(filepath.Dir(path)) {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".pr-")
	if err != nil {
		return
	}
	defer os.Remove(tmp.Name())
	if tmp.Chmod(0600) != nil {
		tmp.Close()
		return
	}
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return
	}
	if tmp.Close() != nil {
		return
	}
	_ = os.Rename(tmp.Name(), path)
}
