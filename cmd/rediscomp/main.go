package main

// src側redisのキーを列挙してdst側に存在するかチェックする

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis"
)

var (
	version = "to be replaced"
)

type redisUrls []string

func (r *redisUrls) String() string {
	return strings.Join(*r, ", ")
}

func (r *redisUrls) Set(v string) error {
	*r = append(*r, v)
	return nil
}

// checkHash チェック対象キーがHASH型
func checkHash(src redis.Cmdable, dst redis.Cmdable, key string, ch chan string) {
	scanAtOnce := int64(*fetchCount)

	var keysSrc []string
	var keysDst []string
	var err error
	var cursorSrc uint64
	var cursorDst uint64

	// hashのキー列挙が順不同だとしても同じアルゴリズムで同じキーが列挙されれば
	// 結果も同じになる、という期待の元比較している
	for {
		keysSrc, cursorSrc, err = src.HScan(key, cursorSrc, "", scanAtOnce).Result()
		if err != nil {
			ch <- fmt.Sprintf("HScan:src(k=%s): %s", key, err)
			return
		}

		keysDst, cursorDst, err = dst.HScan(key, cursorDst, "", scanAtOnce).Result()
		if err != nil {
			ch <- fmt.Sprintf("HScan:dst(k=%s): %s", key, err)
			return
		}

		if !reflect.DeepEqual(keysSrc, keysDst) {
			ch <- fmt.Sprintf("HScan:key(%s): Fields not match", key)
			return
		}

		valsSrc, err := src.HMGet(key, keysSrc...).Result()
		if err != nil {
			ch <- fmt.Sprintf("HMGet:src(k=%s): %s", key, err)
			return
		}

		valsDst, err := dst.HMGet(key, keysSrc...).Result()
		if err != nil {
			ch <- fmt.Sprintf("HMGet:dst(k=%s): %s", key, err)
			return
		}

		if !reflect.DeepEqual(valsSrc, valsDst) {
			ch <- fmt.Sprintf("HMGet:key(%s): Values not match", key)
			return
		}

		if cursorSrc == 0 {
			// srcが0でdstに残有
			if cursorDst != 0 {
				ch <- fmt.Sprintf("HScan:dst(k=%s): Too few keys than src", key)
				return
			}

			// srcもdstも同時に0になった
			return
		} else if cursorDst == 0 {
			// srcが残有でdstが0
			ch <- fmt.Sprintf("HScan:dst(k=%s): Too many keys than src", key)
			return
		}
	}
}

// checkHash チェック対象キーがString型
func checkString(src redis.Cmdable, dst redis.Cmdable, key string, ch chan string) {
	srcValue, err := src.Get(key).Result()
	if err != nil {
		ch <- fmt.Sprintf("Get:src(k=%s): %s", key, err)
		return
	}

	dstValue, err := dst.Get(key).Result()
	if err != nil {
		ch <- fmt.Sprintf("Get:dst(k=%s): %s", key, err)
		return
	}

	if srcValue != dstValue {
		ch <- fmt.Sprintf("Get:key(%s): src(v=%s) != dst(v=%s)", key, srcValue, dstValue)
		return
	}
}

var (
	fetchCount     = flag.Int("fetch-count", 10000, "Number of records to scan at once")
	readTimeoutSec = flag.Int("read-timeout-sec", 5, "Seconds of read timeout for redis")
	idleTimeoutSec = flag.Int("idle-timeout-sec", 100, "Seconds of pooling idle timeout for redis")
	poolSize       = flag.Int("pool-size", 30, "Size of pool for redis")
)

func main() {
	var srcRedisUrls redisUrls
	var dstRedisUrls redisUrls
	flag.Var(&srcRedisUrls, "src", "host:port of redis. More than once flags can be specified.")
	flag.Var(&dstRedisUrls, "dst", "host:port of redis. More than once flags can be specified.")
	scanPattern := flag.String("scan", "", "Match pattern for scan()")
	parallel := flag.Int("parallel", 8, "Number of workers")
	showVersion := flag.Bool("version", false, "Show version")
	reverse := flag.Bool("reverse", false, "Reverse src and dst")
	flag.Parse()

	if *showVersion {
		fmt.Printf("%s\n", version)
		return
	}

	if len(srcRedisUrls) == 0 {
		fmt.Fprintf(os.Stderr, "*** One or more --src must be specified\n")
		return
	}

	if len(dstRedisUrls) == 0 {
		fmt.Fprintf(os.Stderr, "*** One or more --dst must be specified\n")
		return
	}

	// srcとdstの指定を逆向きにする
	if *reverse {
		tmp := dstRedisUrls
		dstRedisUrls = srcRedisUrls
		srcRedisUrls = tmp
	}

	srcRedis := newRedisClient(&srcRedisUrls)
	dstRedis := newRedisClient(&dstRedisUrls)

	var errorCount int
	chError := make(chan string)
	wgErr := &sync.WaitGroup{}
	wgErr.Add(1)
	go func() {
		defer wgErr.Done()
		for e := range chError {
			errorCount = errorCount + 1
			fmt.Fprintf(os.Stdout, "%s\n", e)
		}
	}()

	chKey := make(chan string, *parallel)
	wgWorker := &sync.WaitGroup{}
	for i := 0; i < *parallel; i++ {
		wgWorker.Add(1)
		go func() {
			defer wgWorker.Done()

			for k := range chKey {
				redisType, err := srcRedis.Type(k).Result()
				if err != nil {
					chError <- fmt.Sprintf("Type:src(k=%s): %s", k, err)
					continue
				}

				switch redisType {
				case "string":
					checkString(srcRedis, dstRedis, k, chError)
				case "hash":
					checkHash(srcRedis, dstRedis, k, chError)
				default:
					chError <- fmt.Sprintf("Type:src(k=%s): type=%s is not supported", k, redisType)
					continue
				}
			}
		}()
	}

	wgScan := &sync.WaitGroup{}
	// scanはクラスタ横断で実行できないのでノード個別に
	for i, e := range srcRedisUrls {
		scanAtOnce := int64(*fetchCount)
		wgScan.Add(1)
		index := i
		redisUrl := e
		go func() {
			defer wgScan.Done()
			node := newRedisClient(&redisUrls{redisUrl})

			var n int
			var cursor uint64
			for {
				var keys []string
				var err error
				keys, cursor, err = node.Scan(cursor, *scanPattern, scanAtOnce).Result()
				if err != nil {
					panic(err)
				}

				for _, e := range keys {
					if n%100000 == 0 {
						fmt.Fprintf(os.Stderr, "[%d]%s: Scan %d keys\n", index, redisUrl, n)
					}
					chKey <- e
					n = n + 1
				}

				if cursor == 0 {
					// おわり
					break
				}
			}
			fmt.Fprintf(os.Stderr, "[%d]%s: Total %d keys\n", index, redisUrl, n)
		}()
	}

	wgScan.Wait()

	close(chKey)
	wgWorker.Wait()

	close(chError)
	wgErr.Wait()

	fmt.Fprintf(os.Stderr, "Error=%d\n", errorCount)

	if errorCount > 0 {
		os.Exit(2)
	}
}

func newRedisClient(r *redisUrls) redis.Cmdable {
	isClusterMode := len(*r) >= 2

	if isClusterMode {
		return redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:       *r,
			ReadTimeout: time.Duration(*readTimeoutSec) * time.Second,
			IdleTimeout: time.Duration(*idleTimeoutSec) * time.Second,
			PoolSize:    *poolSize,
		})
	} else {
		return redis.NewClient(&redis.Options{
			Addr:        (*r)[0],
			ReadTimeout: time.Duration(*readTimeoutSec) * time.Second,
			IdleTimeout: time.Duration(*idleTimeoutSec) * time.Second,
			PoolSize:    *poolSize,
		})
	}
}
