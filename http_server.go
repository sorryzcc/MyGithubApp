package main

import (
	"fmt"
	"net/http"
	"time"
)

func main() {
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		// 格式字符串必须是常量，否则 go vet 会报 non-constant format string
		fmt.Fprintf(w, "hello, now is %s\n", time.Now().String())
	})
	if err := http.ListenAndServe(":8088", nil); err != nil {
		fmt.Printf("listen error: %s\n", err.Error())
	}
}
