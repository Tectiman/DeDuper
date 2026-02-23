package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FileInfo 文件信息结构
type FileInfo struct {
	Path string // 文件完整路径
	Size int64  // 文件大小
}

// Scanner 文件扫描器
type Scanner struct {
	workers int            // 并发工作协程数
	quiet   bool           // 静默模式
	files   []FileInfo     // 扫描结果
	mu      sync.Mutex     // 保护files切片的互斥锁
	wg      sync.WaitGroup // 等待所有goroutine完成
}

// NewScanner 创建新的扫描器
func NewScanner(workers int, quiet bool) *Scanner {
	if workers <= 0 {
		workers = 1
	}
	return &Scanner{
		workers: workers,
		quiet:   quiet,
		files:   make([]FileInfo, 0),
	}
}

// Scan 递归扫描目录
func (s *Scanner) Scan(root string) ([]FileInfo, error) {
	// 清理路径
	root = filepath.Clean(root)

	// 检查路径是否存在
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, fmt.Errorf("路径不存在: %s", root)
	}

	// 检查是否为目录
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("无法访问路径: %s, 错误: %v", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("路径不是目录: %s", root)
	}

	if !s.quiet {
		fmt.Printf("[INFO] 开始扫描: %s\n", root)
	}

	// 使用filepath.Walk进行递归扫描
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if !s.quiet {
				fmt.Printf("[WARN] 无法访问: %s, 错误: %v\n", path, err)
			}
			return nil // 继续扫描其他文件
		}

		// 跳过目录本身
		if info.IsDir() {
			return nil
		}

		// 跳过符号链接
		if info.Mode()&os.ModeSymlink != 0 {
			if !s.quiet {
				fmt.Printf("[INFO] 跳过符号链接: %s\n", path)
			}
			return nil
		}

		// 跳过空文件
		if info.Size() == 0 {
			if !s.quiet {
				fmt.Printf("[INFO] 跳过空文件: %s\n", path)
			}
			return nil
		}

		// 添加文件信息
		s.mu.Lock()
		s.files = append(s.files, FileInfo{
			Path: path,
			Size: info.Size(),
		})
		s.mu.Unlock()

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("扫描过程中发生错误: %v", err)
	}

	if !s.quiet {
		fmt.Printf("[INFO] 扫描完成，发现 %d 个文件\n", len(s.files))
	}

	return s.files, nil
}

// GetFiles 获取扫描结果
func (s *Scanner) GetFiles() []FileInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 返回副本以避免外部修改
	result := make([]FileInfo, len(s.files))
	copy(result, s.files)
	return result
}
