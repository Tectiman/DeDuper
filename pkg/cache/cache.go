package cache

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

// CacheManager 缓存管理器
type CacheManager struct {
	db        *bolt.DB
	cachePath string
	Enabled   bool // 注意：首字母大写以便外部访问
	Quiet     bool // 注意：首字母大写以便外部访问
	mutex     sync.RWMutex
}

// CacheEntry 缓存条目
type CacheEntry struct {
	FilePath string `json:"path"`
	Hash     string `json:"hash"`
	FileSize int64  `json:"size"`
	Modified int64  `json:"mtime"`  // Unix timestamp
	CachedAt int64  `json:"cached"` // Unix timestamp
}

// NewCacheManager 创建缓存管理器
func NewCacheManager(cacheOption string, quiet bool) (*CacheManager, error) {
	if cacheOption == "none" {
		return NewDisabledCacheManager(quiet), nil
	}

	var cachePath string
	if cacheOption == "default" {
		cachePath = getDefaultCachePath()
	} else {
		cachePath = cacheOption
	}

	// 确保缓存目录存在
	cacheDir := filepath.Dir(cachePath)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("创建缓存目录失败: %v", err)
	}

	// 打开或创建数据库
	db, err := bolt.Open(cachePath, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("打开缓存数据库失败: %v", err)
	}

	// 初始化buckets
	err = db.Update(func(tx *bolt.Tx) error {
		_, err1 := tx.CreateBucketIfNotExists([]byte("metadata"))
		_, err2 := tx.CreateBucketIfNotExists([]byte("indices"))
		_, err3 := tx.CreateBucketIfNotExists([]byte("entries"))
		if err1 != nil || err2 != nil || err3 != nil {
			return fmt.Errorf("初始化缓存buckets失败")
		}
		return nil
	})

	if err != nil {
		db.Close()
		return nil, err
	}

	cm := &CacheManager{
		db:        db,
		cachePath: cachePath,
		Enabled:   true,
		Quiet:     quiet,
	}

	if !quiet {
		fmt.Printf("[INFO] 缓存文件: %s\n", cachePath)
	}

	return cm, nil
}

// NewDisabledCacheManager 创建禁用的缓存管理器
func NewDisabledCacheManager(quiet bool) *CacheManager {
	return &CacheManager{
		Enabled: false,
		Quiet:   quiet,
	}
}

// GetCachedHash 获取缓存的哈希值
func (cm *CacheManager) GetCachedHash(filepath string) (string, bool) {
	if !cm.Enabled {
		return "", false
	}

	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	var hash string
	var found bool

	err := cm.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("indices"))
		if bucket == nil {
			return nil
		}

		value := bucket.Get([]byte(filepath))
		if value != nil {
			var entry CacheEntry
			buffer := bytes.NewBuffer(value)
			decoder := gob.NewDecoder(buffer)
			if decoder.Decode(&entry) == nil {
				// 验证文件是否仍然有效
				if cm.isFileStillValid(filepath, &entry) {
					hash = entry.Hash
					found = true
				}
			}
		}
		return nil
	})

	if err != nil && !cm.Quiet {
		fmt.Printf("[WARN] 读取缓存失败: %v\n", err)
	}

	return hash, found
}

// CacheHash 缓存哈希值
func (cm *CacheManager) CacheHash(filepath, hash string, fileSize int64, modifiedTime time.Time) error {
	if !cm.Enabled {
		return nil
	}

	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	entry := CacheEntry{
		FilePath: filepath,
		Hash:     hash,
		FileSize: fileSize,
		Modified: modifiedTime.Unix(),
		CachedAt: time.Now().Unix(),
	}

	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	err := encoder.Encode(entry)
	if err != nil {
		return err
	}
	entryBytes := buffer.Bytes()

	return cm.db.Update(func(tx *bolt.Tx) error {
		indices := tx.Bucket([]byte("indices"))
		entries := tx.Bucket([]byte("entries"))

		if indices != nil {
			indices.Put([]byte(filepath), entryBytes)
		}
		if entries != nil {
			entries.Put([]byte(hash), entryBytes)
		}
		return nil
	})
}

// isFileStillValid 检查文件是否仍然有效
func (cm *CacheManager) isFileStillValid(filepath string, entry *CacheEntry) bool {
	fileInfo, err := os.Stat(filepath)
	if err != nil {
		return false
	}

	return fileInfo.Size() == entry.FileSize &&
		fileInfo.ModTime().Unix() == entry.Modified
}

// ClearCache 清除所有缓存
func (cm *CacheManager) ClearCache() error {
	if !cm.Enabled {
		return nil
	}

	return cm.db.Update(func(tx *bolt.Tx) error {
		// 删除所有buckets并重新创建
		tx.DeleteBucket([]byte("metadata"))
		tx.DeleteBucket([]byte("indices"))
		tx.DeleteBucket([]byte("entries"))

		_, err1 := tx.CreateBucketIfNotExists([]byte("metadata"))
		_, err2 := tx.CreateBucketIfNotExists([]byte("indices"))
		_, err3 := tx.CreateBucketIfNotExists([]byte("entries"))

		return fmt.Errorf("%v %v %v", err1, err2, err3)
	})
}

// Close 关闭缓存
func (cm *CacheManager) Close() error {
	if cm.db != nil {
		return cm.db.Close()
	}
	return nil
}

// getDefaultCachePath 获取默认缓存路径
func getDefaultCachePath() string {
	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
		return filepath.Join(appData, "dedup", "cache.db")
	case "darwin": // macOS
		home := os.Getenv("HOME")
		if home == "" {
			home = "/tmp"
		}
		return filepath.Join(home, "Library", "Application Support", "dedup", "cache.db")
	case "linux", "freebsd", "openbsd", "netbsd", "dragonfly":
		home := os.Getenv("HOME")
		if home == "" {
			home = "/tmp"
		}
		return filepath.Join(home, ".cache", "dedup", "cache.db")
	default:
		// 未知操作系统，使用/tmp 作为后备
		home := os.Getenv("HOME")
		if home == "" {
			home = "/tmp"
		}
		return filepath.Join(home, ".cache", "dedup", "cache.db")
	}
}
