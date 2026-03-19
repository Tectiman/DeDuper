package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"golang.org/x/sync/errgroup"
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
	mu      sync.Mutex     // 保护 files 切片的互斥锁
}

// NewScanner 创建新的扫描器
func NewScanner(workers int, quiet bool) *Scanner {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	return &Scanner{
		workers: workers,
		quiet:   quiet,
		files:   make([]FileInfo, 0),
	}
}

// Scan 并发递归扫描目录
func (s *Scanner) Scan(root string) ([]FileInfo, error) {
	// 清理路径
	root = filepath.Clean(root)

	// 检查路径是否存在
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, fmt.Errorf("路径不存在：%s", root)
	}

	// 检查是否为目录
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("无法访问路径：%s, 错误：%v", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("路径不是目录：%s", root)
	}

	if !s.quiet {
		fmt.Printf("[INFO] 开始扫描：%s (使用 %d 个协程)\n", root, s.workers)
	}

	// 使用并发目录遍历
	s.files = make([]FileInfo, 0)
	err = s.scanDirConcurrent(root)
	if err != nil {
		return nil, fmt.Errorf("扫描过程中发生错误：%v", err)
	}

	if !s.quiet {
		fmt.Printf("[INFO] 扫描完成，发现 %d 个文件\n", len(s.files))
	}

	return s.files, nil
}

// scanDirConcurrent 并发扫描目录
func (s *Scanner) scanDirConcurrent(root string) error {
	var eg errgroup.Group
	dirChan := make(chan string, s.workers)

	// 启动工作协程处理目录
	for i := 0; i < s.workers; i++ {
		eg.Go(func() error {
			for dir := range dirChan {
				if err := s.processDir(dir, dirChan); err != nil {
					return err
				}
			}
			return nil
		})
	}

	// 发送根目录
	dirChan <- root
	close(dirChan)

	return eg.Wait()
}

// processDir 处理单个目录
func (s *Scanner) processDir(dir string, dirChan chan<- string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !s.quiet {
			fmt.Printf("[WARN] 无法读取目录：%s, 错误：%v\n", dir, err)
		}
		return nil // 继续扫描其他目录
	}

	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())

		if entry.IsDir() {
			// 将子目录发送到队列进行并发处理
			select {
			case dirChan <- path:
			default:
				// 如果队列已满，直接处理
				if err := s.processDir(path, dirChan); err != nil {
					return err
				}
			}
			continue
		}

		// 处理文件
		fileInfo, err := entry.Info()
		if err != nil {
			if !s.quiet {
				fmt.Printf("[WARN] 获取文件信息失败：%s, 错误：%v\n", path, err)
			}
			continue
		}

		// 跳过符号链接
		if fileInfo.Mode()&os.ModeSymlink != 0 {
			if !s.quiet {
				fmt.Printf("[INFO] 跳过符号链接：%s\n", path)
			}
			continue
		}

		// 跳过空文件
		if fileInfo.Size() == 0 {
			if !s.quiet {
				fmt.Printf("[INFO] 跳过空文件：%s\n", path)
			}
			continue
		}

		// 添加文件信息
		s.mu.Lock()
		s.files = append(s.files, FileInfo{
			Path: path,
			Size: fileInfo.Size(),
		})
		s.mu.Unlock()
	}

	return nil
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
