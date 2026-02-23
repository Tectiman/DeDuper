package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"dedup/pkg/cache"
)

var (
	dedupFlag   = flag.Bool("dedup", false, "单文件夹去重")
	compareFlag = flag.Bool("compare", false, "双文件夹比较")
	hashAlg     = flag.String("hash", "blake3", "哈希算法 blake3|sha256|sha3")
	workers     = flag.Int("workers", 0, "并发数，默认CPU核数")
	dryRun      = flag.Bool("dry-run", false, "仅预览不执行")
	quiet       = flag.Bool("quiet", false, "静默模式")
	cacheOpt    = flag.String("cache", "default", "缓存选项: default|none|<自定义路径>")
	clearCache  = flag.Bool("clear-cache", false, "清除缓存")
	helpFlag    = flag.Bool("help", false, "显示帮助")
)

func Execute() {
	flag.Parse()
	args := flag.Args()

	if *helpFlag || (len(os.Args) == 1) {
		printHelp()
		os.Exit(0)
	}

	// 处理清除缓存命令
	if *clearCache {
		handleClearCache()
		os.Exit(0)
	}

	// 验证哈希算法参数
	validAlgorithms := map[string]bool{
		"blake3": true,
		"sha256": true,
		"sha3":   true,
	}

	if !validAlgorithms[*hashAlg] {
		fmt.Printf("[ERROR] 不支持的哈希算法: %s\n", *hashAlg)
		fmt.Println("支持的算法: blake3, sha256, sha3")
		os.Exit(1)
	}

	// 初始化缓存系统
	var cacheManager *cache.CacheManager
	var err error
	cacheManager, err = cache.NewCacheManager(*cacheOpt, *quiet)
	if err != nil {
		if !*quiet {
			fmt.Printf("[WARN] 缓存初始化失败: %v\n", err)
		}
		// 使用构造函数创建禁用的缓存管理器
		cacheManager = cache.NewDisabledCacheManager(*quiet)
	} else {
		defer cacheManager.Close()
	}

	if *dedupFlag {
		if len(args) < 1 {
			fmt.Println("[ERROR] 缺少文件夹参数")
			os.Exit(1)
		}
		if !*quiet {
			fmt.Printf("[INFO] 执行单文件夹去重: %s\n", args[0])
			if *cacheOpt != "none" {
				fmt.Printf("[INFO] 缓存模式: %s\n", getCacheModeDescription(*cacheOpt))
			}
		}

		err := runDedup(args[0], *hashAlg, *workers, *dryRun, *quiet, cacheManager)
		if err != nil {
			if !*quiet {
				fmt.Printf("[ERROR] %v\n", err)
			}
			os.Exit(1)
		}
		os.Exit(0)
	}

	if *compareFlag {
		if len(args) < 2 {
			fmt.Println("[ERROR] 缺少文件夹参数")
			os.Exit(1)
		}
		if !*quiet {
			fmt.Printf("[INFO] 执行双文件夹比较: %s %s\n", args[0], args[1])
			if *cacheOpt != "none" {
				fmt.Printf("[INFO] 缓存模式: %s\n", getCacheModeDescription(*cacheOpt))
			}
		}

		err := runCompare(args[0], args[1], *hashAlg, *workers, *dryRun, *quiet, cacheManager)
		if err != nil {
			if !*quiet {
				fmt.Printf("[ERROR] %v\n", err)
			}
			os.Exit(1)
		}
		os.Exit(0)
	}

	fmt.Println("[ERROR] 未知命令，使用 --help 查看用法")
	os.Exit(1)
}

func printHelp() {
	fmt.Println(`dedup 文件去重工具

用法:
  dedup --dedup <folder> [选项]
  dedup --compare <folder1> <folder2> [选项]
  dedup --clear-cache
  dedup --help

选项:
  --dedup         单文件夹去重模式
  --compare       双文件夹比较模式
  --hash          哈希算法 (blake3|sha256|sha3) [默认: blake3]
  --workers       并发数 (默认: CPU核心数)
  --dry-run       预览模式，仅显示操作
  --quiet         静默模式，仅输出错误
  --cache         缓存选项:
                  default - 使用默认缓存文件
                  none - 禁用缓存
                  <路径> - 使用指定缓存文件
  --clear-cache   清除所有缓存
  --help          显示帮助

示例:
  # 基本去重（启用缓存）
  dedup --dedup ./downloads
  
  # 禁用缓存
  dedup --dedup ./photos --cache none
  
  # 使用自定义缓存文件
  dedup --dedup ./videos --cache /tmp/mycache.db
  
  # 清除缓存
  dedup --clear-cache
`)
}

func getCacheModeDescription(cacheOpt string) string {
	switch cacheOpt {
	case "default":
		return "默认缓存 (" + getDefaultCachePath() + ")"
	case "none":
		return "禁用缓存"
	default:
		return "自定义缓存 (" + cacheOpt + ")"
	}
}

func getDefaultCachePath() string {
	// 根据操作系统确定默认缓存路径
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

func handleClearCache() {
	cachePath := getDefaultCachePath()

	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		fmt.Println("[INFO] 缓存文件不存在")
		return
	}

	// 简单的文件删除方式
	if err := os.Remove(cachePath); err != nil {
		fmt.Printf("[ERROR] 清除缓存失败: %v\n", err)
		os.Exit(1)
	}

	// 删除空的目录
	cacheDir := filepath.Dir(cachePath)
	if entries, _ := os.ReadDir(cacheDir); len(entries) == 0 {
		os.Remove(cacheDir)
	}

	fmt.Printf("[INFO] 缓存已清除: %s\n", cachePath)
}
