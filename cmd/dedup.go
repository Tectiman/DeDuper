package cmd

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"sync"
	"time"

	"dedup/pkg/cache"
	"dedup/pkg/hash"
	"dedup/pkg/renamer"
	"dedup/pkg/scanner"
)

// runDedup 执行单文件夹去重（集成缓存）
func runDedup(folder string, hashAlg string, workers int, dryRun bool, quiet bool, cacheMgr *cache.CacheManager) error {
	// 设置默认并发数
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	if !quiet {
		fmt.Printf("[INFO] 使用 %d 个工作协程\n", workers)
		fmt.Printf("[INFO] 哈希算法: %s\n", hashAlg)
	}

	// 创建哈希计算器
	hasher, err := hash.NewHasher(hashAlg)
	if err != nil {
		return fmt.Errorf("[ERROR] 哈希算法初始化失败: %v", err)
	}

	// 创建文件扫描器
	fileScanner := scanner.NewScanner(workers, quiet)

	// 扫描文件
	files, err := fileScanner.Scan(folder)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		if !quiet {
			fmt.Println("[INFO] 未找到任何文件")
		}
		return nil
	}

	// 按文件大小分组，提高效率
	sizeGroups := make(map[int64][]scanner.FileInfo)
	for _, file := range files {
		sizeGroups[file.Size] = append(sizeGroups[file.Size], file)
	}

	// 创建重命名器
	fileRenamer := renamer.NewRenamer(dryRun, quiet)

	// 收集需要计算哈希的文件
	var filesToHash []scanner.FileInfo
	var cachedResults = make(map[string]string) // filepath -> hash

	// 先尝试从缓存获取哈希值
	for _, group := range sizeGroups {
		if len(group) > 1 { // 只处理可能重复的文件
			for _, file := range group {
				if hashValue, found := cacheMgr.GetCachedHash(file.Path); found {
					cachedResults[file.Path] = hashValue
					if !quiet {
						fmt.Printf("[CACHE] %s\n", file.Path)
					}
				} else {
					filesToHash = append(filesToHash, file)
				}
			}
		}
	}

	if !quiet {
		fmt.Printf("[INFO] 缓存命中: %d 个文件\n", len(cachedResults))
		fmt.Printf("[INFO] 需要计算: %d 个文件\n", len(filesToHash))
	}

	// 计算未缓存文件的哈希值
	var newHashes map[string]string
	var hashErrors []error

	if len(filesToHash) > 0 {
		newHashes, hashErrors = calculateHashesWithCache(filesToHash, hasher, workers, quiet, cacheMgr)

		// 输出错误信息
		for _, err := range hashErrors {
			if !quiet {
				fmt.Printf("[ERROR] 哈希计算错误: %v\n", err)
			}
		}
	}

	// 合并缓存和新计算的结果
	allHashes := make(map[string][]string) // hash -> []filepath

	// 添加缓存结果
	for filepath, hashValue := range cachedResults {
		allHashes[hashValue] = append(allHashes[hashValue], filepath)
	}

	// 添加新计算结果
	for filepath, hashValue := range newHashes {
		allHashes[hashValue] = append(allHashes[hashValue], filepath)
	}

	// 识别重复文件
	var duplicates []string
	for _, group := range allHashes {
		if len(group) > 1 {
			sort.Strings(group)
			duplicates = append(duplicates, group[1:]...) // 保留第一个，其余标记为重复
		}
	}

	if !quiet {
		fmt.Printf("[INFO] 发现 %d 组重复文件，共 %d 个重复项\n", len(allHashes)-len(hashErrors), len(duplicates))
	}

	if len(duplicates) == 0 {
		if !quiet {
			fmt.Println("[INFO] 未发现重复文件")
		}
		return nil
	}

	// 执行重命名
	_ = fileRenamer.RenameFiles(duplicates)

	// 统计结果
	successCount := fileRenamer.GetSuccessCount()
	errorCount := fileRenamer.GetErrorCount()

	if !quiet || errorCount > 0 {
		fmt.Printf("[INFO] 完成！成功重命名 %d 个文件", successCount)
		if errorCount > 0 {
			fmt.Printf("，%d 个失败", errorCount)
		}
		fmt.Println()
	}

	// 检查是否有错误
	if errorCount > 0 {
		return fmt.Errorf("部分文件重命名失败")
	}

	return nil
}

// calculateHashesWithCache 计算哈希值并缓存结果
func calculateHashesWithCache(files []scanner.FileInfo, hasher hash.Hasher, workers int, quiet bool, cacheMgr *cache.CacheManager) (map[string]string, []error) {
	type hashJob struct {
		filePath string
		fileSize int64
		index    int
	}

	type hashResult struct {
		filePath string
		hash     string
		err      error
		fileSize int64
		modified time.Time
		index    int
	}

	jobs := make(chan hashJob, len(files))
	results := make(chan hashResult, len(files))

	// 启动工作协程
	var wg sync.WaitGroup
	for i := 0; i < min(workers, len(files)); i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for job := range jobs {
				f, err := os.Open(job.filePath)
				if err != nil {
					results <- hashResult{
						filePath: job.filePath,
						err:      fmt.Errorf("无法打开文件: %v", err),
						index:    job.index,
					}
					continue
				}

				fileInfo, _ := f.Stat()
				modifiedTime := fileInfo.ModTime()

				hashValue, err := hasher.Sum(f)
				f.Close()

				if err != nil {
					results <- hashResult{
						filePath: job.filePath,
						err:      fmt.Errorf("哈希计算失败: %v", err),
						index:    job.index,
					}
					continue
				}

				results <- hashResult{
					filePath: job.filePath,
					hash:     hashValue,
					fileSize: job.fileSize,
					modified: modifiedTime,
					index:    job.index,
				}
			}
		}(i)
	}

	// 发送任务
	go func() {
		for i, file := range files {
			jobs <- hashJob{
				filePath: file.Path,
				fileSize: file.Size,
				index:    i,
			}
		}
		close(jobs)
	}()

	// 收集结果
	go func() {
		wg.Wait()
		close(results)
	}()

	// 处理结果
	hashMap := make(map[string]string)
	var errors []error
	resultsSlice := make([]hashResult, len(files))

	received := 0
	for result := range results {
		resultsSlice[result.index] = result
		received++
		if received == len(files) {
			break
		}
	}

	// 处理结果并缓存
	for _, result := range resultsSlice {
		if result.err != nil {
			errors = append(errors, result.err)
			if !quiet {
				fmt.Printf("[ERROR] %s: %v\n", result.filePath, result.err)
			}
		} else {
			hashMap[result.filePath] = result.hash
			// 缓存结果
			cacheMgr.CacheHash(result.filePath, result.hash, result.fileSize, result.modified)
		}
	}

	return hashMap, errors
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
