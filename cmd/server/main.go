// @title Emplacc API
// @version 1.0
// @description API для Emplacc.
// (host не задаём умышленно, чтобы Swagger использовал текущий origin)
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

package main

import (
	"log"
	"time"

	"emplacc-api/internal/app"
)

func main() {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		panic(err)
	}
	time.Local = loc

	application, err := app.Bootstrap()
	if err != nil {
		log.Fatalf("bootstrap failed: %v", err)
	}
	defer application.Close()

	log.Println("Server started on :8081")
	application.Echo.Logger.Fatal(application.Echo.Start("0.0.0.0:8081"))
}
