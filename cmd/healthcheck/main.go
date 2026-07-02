package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	resp, err := http.Get("http://localhost:8081/swagger/index.html")
	if err != nil || resp.StatusCode >= 500 {
		fmt.Fprintln(os.Stderr, "unhealthy")
		os.Exit(1)
	}
	fmt.Println("healthy")
}
