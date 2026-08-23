// Command keyproof 是多方密钥轮换覆盖证明服务。
// 支持 --addr、--db、--smoke-test 三个标志；smoke-test 使用
// 独立临时库执行端到端自检后以 0 退出码结束（Docker 双架构验证契约）。
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"task167-keyproof/internal/httpapi"
	"task167-keyproof/internal/service"
	"task167-keyproof/internal/smoke"
	"task167-keyproof/internal/store"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP 监听地址")
	dbPath := flag.String("db", "keyproof.db", "SQLite 数据库路径")
	smokeTest := flag.Bool("smoke-test", false, "运行离线自检后退出")
	flag.Parse()

	if *smokeTest {
		if err := smoke.Run(""); err != nil {
			fmt.Fprintf(os.Stderr, "smoke-test 失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("smoke-test 通过")
		return
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	defer st.Close()

	app := service.New(st)
	srv := httpapi.New(app)

	log.Printf("keyproof 服务启动: addr=%s db=%s", *addr, *dbPath)
	server := &http.Server{
		Addr:              *addr,
		Handler:           withRecovery(srv.Handler()),
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP 服务退出: %v", err)
	}
}

// withRecovery 兜底 panic 恢复，保证单个请求异常不拖垮服务。
func withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic recovered: %v", rec)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
