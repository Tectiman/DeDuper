package cmd

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

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
		fmt.Printf("[INFO] 哈希算法：%s\n", hashAlg)
		fmt.Printf("[INFO] 建立基准库：%s\n", folder1)
		fmt.Printf("[INFO] 查找重复项：%s\n", folder2)
	}

	// 创建哈希计算器
	hasher, err := hash.NewHasher(hashAlg)
	if err != nil {
		return fmt.Errorf("[ERROR] 哈希算法初始化失败：%v", err)
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

	// 建立第二个文件夹的文件列表
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

	// 1. 按文件大小进行初步分组与过滤
	sizeGroups1 := make(map[int64][]scanner.FileInfo)
	for _, file := range files1 {
		sizeGroups1[file.Size] = append(sizeGroups1[file.Size], file)
	}

	// 只保留在 folder1 中有相同大小文件的 folder2 文件
	var potentialDuplicates []scanner.FileInfo
	for _, file := range files2 {
		if _, exists := sizeGroups1[file.Size]; exists {
			potentialDuplicates = append(potentialDuplicates, file)
		}
	}

	if len(potentialDuplicates) == 0 {
		if !quiet {
			fmt.Println("[INFO] 未发现相同大小的文件，无需比较")
		}
		return nil
	}

	if !quiet {
		fmt.Printf("[INFO] 发现 %d 个可能重复的文件 (大小匹配)\n", len(potentialDuplicates))
	}

	// 提取 folder1 中也需要参与比较的文件
	var baseFilesToCompare []scanner.FileInfo
	for _, file2 := range potentialDuplicates {
		if group, exists := sizeGroups1[file2.Size]; exists {
			baseFilesToCompare = append(baseFilesToCompare, group...)
			// 从 sizeGroups1 移除，避免重复添加
			delete(sizeGroups1, file2.Size)
		}
	}

	// 使用 Partial Hash 快速过滤大文件
	const partialHashSize = 8192
	var baseFilesToFullHash []scanner.FileInfo
	var compareFilesToFullHash []scanner.FileInfo

	var hasLargeFiles bool
	for _, file := range baseFilesToCompare {
		if file.Size > partialHashSize {
			hasLargeFiles = true
			break
		}
	}

	if hasLargeFiles {
		if !quiet {
			fmt.Println("[INFO] 启用部分哈希 (Partial Hash) 快速过滤策略...")
		}
		basePartialHashes := calculatePartialHashes(baseFilesToCompare, hasher, workers, quiet, partialHashSize)
		comparePartialHashes := calculatePartialHashes(potentialDuplicates, hasher, workers, quiet, partialHashSize)

		// 筛选出 Partial Hash 也相同的项
		for hashValue, baseFiles := range basePartialHashes {
			if compareFiles, exists := comparePartialHashes[hashValue]; exists {
				baseFilesToFullHash = append(baseFilesToFullHash, baseFiles...)
				compareFilesToFullHash = append(compareFilesToFullHash, compareFiles...)
			}
		}

		if !quiet {
			fmt.Printf("[INFO] Partial Hash 过滤后，剩下 %d 个可能重复的文件对\n", len(compareFilesToFullHash))
		}
	} else {
		baseFilesToFullHash = baseFilesToCompare
		compareFilesToFullHash = potentialDuplicates
	}

	if len(compareFilesToFullHash) == 0 {
		if !quiet {
			fmt.Println("[INFO] 未发现重复文件")
		}
		return nil
	}

	// 计算可能重复文件的最终（完整）哈希值
	var hashErrors1 []error
	baseHashes, hashErrors1 := calculateHashesWithCacheSimple(baseFilesToFullHash, hasher, workers, quiet, cacheMgr)
	for _, err := range hashErrors1 {
		if !quiet {
			fmt.Printf("[ERROR] 基准文件哈希计算错误：%v\n", err)
		}
	}

	var hashErrors2 []error
	compareHashes, hashErrors2 := calculateHashesWithCacheSimple(compareFilesToFullHash, hasher, workers, quiet, cacheMgr)
	for _, err := range hashErrors2 {
		if !quiet {
			fmt.Printf("[ERROR] 目标文件哈希计算错误：%v\n", err)
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

	// 使用有界缓冲
	const maxBuffered = 100
	jobs := make(chan hashJob, maxBuffered)
	results := make(chan hashResult, maxBuffered)

	var eg errgroup.Group
	var resultsMu sync.Mutex
	hashGroups := make(map[string][]string)
	var errors []error

	// 结果收集协程
	eg.Go(func() error {
		for result := range results {
			resultsMu.Lock()
			if result.err != nil {
				errors = append(errors, result.err)
				if !quiet {
					fmt.Printf("[ERROR] %s: %v\n", result.filePath, result.err)
				}
			} else {
				hashGroups[result.hash] = append(hashGroups[result.hash], result.filePath)
				// 异步缓存
				if result.fileSize > 0 && !result.modified.IsZero() {
					cacheMgr.CacheHash(result.filePath, result.hash, result.fileSize, result.modified)
				}
			}
			resultsMu.Unlock()
		}
		return nil
	})

	// 工作协程
	for i := 0; i < min(workers, len(files)); i++ {
		eg.Go(func() error {
			defer func() {
				// 确保 channel 关闭时不会 panic
				recover()
			}()
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
			return nil
		})
	}

	// 发送任务
	go func() {
		for _, file := range files {
			jobs <- hashJob{filePath: file.Path, fileSize: file.Size}
		}
		close(jobs)
	}()

	// 等待完成
	if err := eg.Wait(); err != nil {
		return hashGroups, []error{err}
	}

	return hashGroups, errors
}

// calculatePartialHashes 只计算文件前 N 字节的哈希值，不使用缓存
func calculatePartialHashes(files []scanner.FileInfo, hasher hash.Hasher, workers int, quiet bool, size int64) map[string][]scanner.FileInfo {
	type hashJob struct {
		file scanner.FileInfo
	}

	type hashResult struct {
		file scanner.FileInfo
		hash string
		err  error
	}

	// 使用有界缓冲
	const maxBuffered = 100
	jobs := make(chan hashJob, maxBuffered)
	results := make(chan hashResult, maxBuffered)

	var wg sync.WaitGroup

	// 工作协程
	for i := 0; i < min(workers, len(files)); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				f, err := os.Open(job.file.Path)
				if err != nil {
					results <- hashResult{file: job.file, err: err}
					continue
				}

				hashValue, err := hasher.PartialSum(f, size)
				f.Close()

				if err != nil {
					results <- hashResult{file: job.file, err: err}
					continue
				}

				results <- hashResult{
					file: job.file,
					hash: hashValue,
				}
			}
		}()
	}

	go func() {
		for _, file := range files {
			jobs <- hashJob{file: file}
		}
		close(jobs)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	hashGroups := make(map[string][]scanner.FileInfo)
	for result := range results {
		if result.err != nil {
			if !quiet {
				fmt.Printf("[ERROR] Partial Hash 计算失败 %s: %v\n", result.file.Path, result.err)
			}
		} else {
			hashGroups[result.hash] = append(hashGroups[result.hash], result.file)
		}
	}

	return hashGroups
}
