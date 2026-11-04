//go:build ignore

// 单独运行：go run hello.go
// build tag 让 go build . 跳过本文件，避免与 test.go 的 main 重复定义。
package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}
