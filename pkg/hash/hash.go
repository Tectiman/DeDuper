package hash

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"sync"

	"github.com/zeebo/blake3"
	"golang.org/x/crypto/sha3"
)

const (
	// 增加默认缓冲区大小至 1MB 以优化大文件 I/O 性能
	defaultBufferSize = 1 * 1024 * 1024
)

// bufferPool 用于复用缓冲区，减少内存分配
var bufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, defaultBufferSize)
		return &buf
	},
}

// Hasher 定义哈希计算接口
// 支持流式计算
// name: 算法名，如 blake3、sha256、sha3-256
// 返回哈希值（hex字符串）和错误
type Hasher interface {
	Name() string
	Sum(r io.Reader) (string, error)
	PartialSum(r io.Reader, size int64) (string, error)
}

// NewHasher 创建指定算法的哈希计算器
func NewHasher(algorithm string) (Hasher, error) {
	switch algorithm {
	case "blake3":
		return &Blake3Hasher{}, nil
	case "sha256":
		return &SHA256Hasher{}, nil
	case "sha3":
		return &SHA3Hasher{}, nil
	default:
		return nil, errors.New("unsupported hash algorithm: " + algorithm)
	}
}

// Blake3Hasher BLAKE3算法实现
type Blake3Hasher struct{}

func (h *Blake3Hasher) Name() string {
	return "blake3"
}

func (h *Blake3Hasher) Sum(r io.Reader) (string, error) {
	hasher := blake3.New()
	bufPtr := bufferPool.Get().(*[]byte)
	defer bufferPool.Put(bufPtr)

	_, err := io.CopyBuffer(hasher, r, *bufPtr)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (h *Blake3Hasher) PartialSum(r io.Reader, size int64) (string, error) {
	hasher := blake3.New()
	bufSize := minInt64(size, int64(defaultBufferSize))
	bufPtr := bufferPool.Get().(*[]byte)
	defer bufferPool.Put(bufPtr)

	buf := (*bufPtr)[:bufSize]
	_, err := io.CopyBuffer(hasher, io.LimitReader(r, size), buf)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// SHA256Hasher SHA256算法实现
type SHA256Hasher struct{}

func (h *SHA256Hasher) Name() string {
	return "sha256"
}

func (h *SHA256Hasher) Sum(r io.Reader) (string, error) {
	hasher := sha256.New()
	bufPtr := bufferPool.Get().(*[]byte)
	defer bufferPool.Put(bufPtr)

	_, err := io.CopyBuffer(hasher, r, *bufPtr)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (h *SHA256Hasher) PartialSum(r io.Reader, size int64) (string, error) {
	hasher := sha256.New()
	bufSize := minInt64(size, int64(defaultBufferSize))
	bufPtr := bufferPool.Get().(*[]byte)
	defer bufferPool.Put(bufPtr)

	buf := (*bufPtr)[:bufSize]
	_, err := io.CopyBuffer(hasher, io.LimitReader(r, size), buf)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// SHA3Hasher SHA3-256算法实现
type SHA3Hasher struct{}

func (h *SHA3Hasher) Name() string {
	return "sha3"
}

func (h *SHA3Hasher) Sum(r io.Reader) (string, error) {
	hasher := sha3.New256()
	bufPtr := bufferPool.Get().(*[]byte)
	defer bufferPool.Put(bufPtr)

	_, err := io.CopyBuffer(hasher, r, *bufPtr)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (h *SHA3Hasher) PartialSum(r io.Reader, size int64) (string, error) {
	hasher := sha3.New256()
	bufSize := minInt64(size, int64(defaultBufferSize))
	bufPtr := bufferPool.Get().(*[]byte)
	defer bufferPool.Put(bufPtr)

	buf := (*bufPtr)[:bufSize]
	_, err := io.CopyBuffer(hasher, io.LimitReader(r, size), buf)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
