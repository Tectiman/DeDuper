package cmd

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"dedup/pkg/cache"
	"dedup/pkg/hash"
	"dedup/pkg/renamer"
	"dedup/pkg/scanner"
)

// runCompare 执行双文件夹比较（集成缓存）
func runCompare(folder1, folder2, hashAlg string, workers int, dryRun bool, quiet bool, cacheMgr *cache.CacheManager) error {
	// 设置默认并发数
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	if !quiet {
		fmt.Printf("[INFO] 使用 %d 个工作协程\n", workers)
		fmt.Printf("[INFO] 哈希算法: %s\n", hashAlg)
		fmt.Printf("[INFO] 建立基准库: %s\n", folder1)
		fmt.Printf("[INFO] 查找重复项: %s\n", folder2)
	}

	// 创建哈希计算器
	hasher, err := hash.NewHasher(hashAlg)
	if err != nil {
		return fmt.Errorf("[ERROR] 哈希算法初始化失败: %v", err)
	}

	// 扫描第一个文件夹建立哈希库
	scanner1 := scanner.NewScanner(workers, quiet)
	files1, err := scanner1.Scan(folder1)
	if err != nil {
		return err
	}

	if len(files1) == 0 {
		if !quiet {
			fmt.Printf("[INFO] 基准文件夹 %s 中未找到文件\n", folder1)
		}
		return nil
	}

	// 计算第一个文件夹的哈希值（使用缓存）
	baseHashes, hashErrors1 := calculateHashesWithCacheSimple(files1, hasher, workers, quiet, cacheMgr)

	for _, err := range hashErrors1 {
		if !quiet {
			fmt.Printf("[ERROR] 哈希计算错误: %v\n", err)
		}
	}

	if !quiet {
		fmt.Printf("[INFO] 基准库建立完成，包含 %d 个唯一文件\n", len(baseHashes))
	}

	// 扫描第二个文件夹
	scanner2 := scanner.NewScanner(workers, quiet)
	files2, err := scanner2.Scan(folder2)
	if err != nil {
		return err
	}

	if len(files2) == 0 {
		if !quiet {
			fmt.Printf("[INFO] 比较文件夹 %s 中未找到文件\n", folder2)
		}
		return nil
	}

	// 计算第二个文件夹的哈希值（使用缓存）
	compareHashes, hashErrors2 := calculateHashesWithCacheSimple(files2, hasher, workers, quiet, cacheMgr)

	for _, err := range hashErrors2 {
		if !quiet {
			fmt.Printf("[ERROR] 哈希计算错误: %v\n", err)
		}
	}

	// 查找重复文件
	var duplicates []string
	for hashValue, filePaths := range compareHashes {
		if _, exists := baseHashes[hashValue]; exists {
			duplicates = append(duplicates, filePaths...)
		}
	}

	if !quiet {
		fmt.Printf("[INFO] 在 %s 中发现 %d 个与 %s 重复的文件\n", folder2, len(duplicates), folder1)
	}

	if len(duplicates) == 0 {
		if !quiet {
			fmt.Println("[INFO] 未发现重复文件")
		}
		return nil
	}

	// 创建重命名器并执行重命名
	fileRenamer := renamer.NewRenamer(dryRun, quiet)
	_ = fileRenamer.RenameFiles(duplicates)

	// 统计结果
	successCount := fileRenamer.GetSuccessCount()
	errorCount := fileRenamer.GetErrorCount()

	if !quiet || errorCount > 0 {
		fmt.Printf("[INFO] 完成！成功重命名 %d 个重复文件", successCount)
		if errorCount > 0 {
			fmt.Printf("，%d 个失败", errorCount)
		}
		fmt.Println()
	}

	if errorCount > 0 {
		return fmt.Errorf("部分文件重命名失败")
	}

	return nil
}

// calculateHashesWithCacheSimple 简化版本的带缓存哈希计算
func calculateHashesWithCacheSimple(files []scanner.FileInfo, hasher hash.Hasher, workers int, quiet bool, cacheMgr *cache.CacheManager) (map[string][]string, []error) {
	type hashJob struct {
		filePath string
		fileSize int64
	}

	type hashResult struct {
		filePath string
		hash     string
		err      error
		fileSize int64
		modified time.Time
	}

	jobs := make(chan hashJob, len(files))
	results := make(chan hashResult, len(files))

	// 启动工作协程
	var wg sync.WaitGroup
	for i := 0; i < min(workers, len(files)); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				// 尝试从缓存获取
				if hashValue, found := cacheMgr.GetCachedHash(job.filePath); found {
					results <- hashResult{filePath: job.filePath, hash: hashValue}
					continue
				}

				// 计算哈希
				f, err := os.Open(job.filePath)
				if err != nil {
					results <- hashResult{filePath: job.filePath, err: err}
					continue
				}

				fileInfo, _ := f.Stat()
				modifiedTime := fileInfo.ModTime()

				hashValue, err := hasher.Sum(f)
				f.Close()

				if err != nil {
					results <- hashResult{filePath: job.filePath, err: err}
					continue
				}

				// 缓存结果
				cacheMgr.CacheHash(job.filePath, hashValue, job.fileSize, modifiedTime)

				results <- hashResult{
					filePath: job.filePath,
					hash:     hashValue,
					fileSize: job.fileSize,
					modified: modifiedTime,
				}
			}
		}()
	}

	// 发送任务
	go func() {
		for _, file := range files {
			jobs <- hashJob{filePath: file.Path, fileSize: file.Size}
		}
		close(jobs)
	}()

	// 收集结果
	go func() {
		wg.Wait()
		close(results)
	}()

	// 处理结果
	hashGroups := make(map[string][]string)
	var errors []error

	for result := range results {
		if result.err != nil {
			errors = append(errors, result.err)
			if !quiet {
				fmt.Printf("[ERROR] %s: %v\n", result.filePath, result.err)
			}
		} else {
			hashGroups[result.hash] = append(hashGroups[result.hash], result.filePath)
		}
	}

	return hashGroups, errors
}
