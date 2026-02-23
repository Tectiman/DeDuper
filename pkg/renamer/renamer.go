package renamer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// RenameResult 重命名结果
type RenameResult struct {
	OldPath string // 原路径
	NewPath string // 新路径
	Error   error  // 错误信息
}

// Renamer 文件重命名器
type Renamer struct {
	dryRun  bool           // 预览模式
	quiet   bool           // 静默模式
	results []RenameResult // 重命名结果
	mu      sync.Mutex     // 保护results的互斥锁
}

// NewRenamer 创建新的重命名器
func NewRenamer(dryRun, quiet bool) *Renamer {
	return &Renamer{
		dryRun:  dryRun,
		quiet:   quiet,
		results: make([]RenameResult, 0),
	}
}

// GenerateDepName 生成dep_前缀的新文件名
func (r *Renamer) GenerateDepName(originalPath string) string {
	dir := filepath.Dir(originalPath)
	filename := filepath.Base(originalPath)
	ext := filepath.Ext(filename)
	name := strings.TrimSuffix(filename, ext)

	newName := fmt.Sprintf("dep_%s%s", name, ext)
	newPath := filepath.Join(dir, newName)

	// 如果目标文件已存在，添加数字后缀
	counter := 1
	for {
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			break
		}
		newName = fmt.Sprintf("dep_%s(%d)%s", name, counter, ext)
		newPath = filepath.Join(dir, newName)
		counter++
	}

	return newPath
}

// RenameFile 重命名单个文件
func (r *Renamer) RenameFile(oldPath string) RenameResult {
	// 验证源文件存在
	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		return RenameResult{
			OldPath: oldPath,
			Error:   fmt.Errorf("源文件不存在: %s", oldPath),
		}
	}

	// 生成新文件名
	newPath := r.GenerateDepName(oldPath)

	// 预览模式下只记录，不实际重命名
	if r.dryRun {
		if !r.quiet {
			fmt.Printf("[DRY-RUN] 将重命名: %s -> %s\n", oldPath, newPath)
		}
		return RenameResult{
			OldPath: oldPath,
			NewPath: newPath,
		}
	}

	// 执行重命名
	err := os.Rename(oldPath, newPath)
	if err != nil {
		return RenameResult{
			OldPath: oldPath,
			NewPath: newPath,
			Error:   fmt.Errorf("重命名失败: %v", err),
		}
	}

	// 记录成功结果
	result := RenameResult{
		OldPath: oldPath,
		NewPath: newPath,
	}

	r.mu.Lock()
	r.results = append(r.results, result)
	r.mu.Unlock()

	if !r.quiet {
		fmt.Printf("[RENAME] %s -> %s\n", oldPath, newPath)
	}

	return result
}

// RenameFiles 批量重命名文件
func (r *Renamer) RenameFiles(paths []string) []RenameResult {
	var results []RenameResult

	for _, path := range paths {
		result := r.RenameFile(path)
		results = append(results, result)
	}

	return results
}

// GetResults 获取重命名结果
func (r *Renamer) GetResults() []RenameResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 返回副本
	result := make([]RenameResult, len(r.results))
	copy(result, r.results)
	return result
}

// GetSuccessCount 获取成功重命名的数量
func (r *Renamer) GetSuccessCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	count := 0
	for _, result := range r.results {
		if result.Error == nil {
			count++
		}
	}
	return count
}

// GetErrorCount 获取重命名失败的数量
func (r *Renamer) GetErrorCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	count := 0
	for _, result := range r.results {
		if result.Error != nil {
			count++
		}
	}
	return count
}
