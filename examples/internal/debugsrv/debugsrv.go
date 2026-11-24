// Package debugsrv 是各个示例共用的调试服务接线。
//
// 放在 examples/internal/ 下：Go 的 internal 规则保证它只对 examples/ 目录树
// 里的包可见（仓库其它地方 import 不到），所以它可以随意引用 debug 包而不
// 违反「生产代码不许依赖 debug」的架构护栏（core/arch_test.go）。
//
// 每个示例的接线都一样三步：
//
//  1. 加一个 -debug-port 旗标（空 = 不起服务）；
//  2. Run 之前调 Serve 起服务；
//  3. 服务失败就把错误打出来并退出——「以为开了其实没开」必须让人看见。
//
// 没给端口时也会看环境变量 GHROMEX_DEBUG_PORT，于是同一套脚本既能改旗标
// 也能改环境变量，不用为每个示例记两套开关。
package debugsrv

import (
	"flag"
	"fmt"
	"os"

	"github.com/go-comm/ghromex/browser"
	"github.com/go-comm/ghromex/debug"
)

// Flag 是各示例统一的 -debug-port 旗标定义。
//
// 用法：main 里先注册旗标，Parse 之后把指针交给 Serve。
//
//	debugPort := debugsrv.Flag()
//	flag.Parse()
//	...
//	defer debugsrv.Serve(win, *debugPort)
func Flag() *string {
	return flag.String("debug-port", "", "调试服务端口（如 9223；0=随机，仅绑回环）。也可用环境变量 GHROMEX_DEBUG_PORT")
}

// Serve 按需起调试服务，返回一个「收尾」函数。
//
// 端口为空时看环境变量 GHROMEX_DEBUG_PORT；两者都没有就什么都不做，返回的
// 函数是空操作（调用方照样 defer 即可，不必分支）。
//
// 端口被占、地址不是回环这类错误直接写到 stderr 并退出进程：调试开关失灵
// 却继续跑，脚本会以为「服务开着但没反应」，比直接报错难查得多。
func Serve(win *browser.Window, port string) func() {
	if port == "" {
		// 环境变量未设置时 ServeFromEnv 返回 (nil, nil)：不起服务。
		srv, err := debug.ServeFromEnv(win)
		if err != nil {
			fmt.Fprintln(os.Stderr, "调试服务启动失败:", err)
			os.Exit(1)
		}
		if srv == nil {
			return func() {}
		}
		return serveDone(srv)
	}
	srv, err := debug.Serve(win, debug.Options{Addr: port})
	if err != nil {
		fmt.Fprintln(os.Stderr, "调试服务启动失败:", err)
		os.Exit(1)
	}
	return serveDone(srv)
}

// serveDone 打印服务地址并返回收尾函数。
func serveDone(srv *debug.Server) func() {
	fmt.Println("调试服务:", srv.URL("/debug/"))
	return func() { srv.Close() }
}
