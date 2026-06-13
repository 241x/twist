// twist 是一个通过 CDP 协议实时拦截和修改浏览器网络请求/响应的命令行工具。
package main

import (
	"os"

	"github.com/241x/twist/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(cmd.GetExitCode(err))
	}
}
