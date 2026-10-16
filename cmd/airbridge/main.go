package main

import (
	"airbridge/internal/web"
	"log"
	"net/http"
)

func main() {
	log.Println("Airbridge: http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", web.Handler()))
}
