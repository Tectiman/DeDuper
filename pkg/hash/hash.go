package hash

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"

	"github.com/zeebo/blake3"
	"golang.org/x/crypto/sha3"
)

const (
	// 默认缓冲区大小 64KB
	defaultBufferSize = 64 * 1024
)

// Hasher 定义哈希计算接口
// 支持流式计算
// name: 算法名，如 blake3、sha256、sha3-256
// 返回哈希值（hex字符串）和错误
type Hasher interface {
	Name() string
	Sum(r io.Reader) (string, error)
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
	buf := make([]byte, defaultBufferSize)

	for {
		n, err := r.Read(buf)
		if n > 0 {
			hasher.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
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
	buf := make([]byte, defaultBufferSize)

	for {
		n, err := r.Read(buf)
		if n > 0 {
			hasher.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
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
	buf := make([]byte, defaultBufferSize)

	for {
		n, err := r.Read(buf)
		if n > 0 {
			hasher.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
