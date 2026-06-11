package main

import (
	"flag"
	"log"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	addr := envOrDefault("QUESTER_ADDR", "127.0.0.1:93")
	dataDir := envOrDefault("QUESTER_DATA_DIR", ".quester-data")
	prefix := envOrDefault("QUESTER_PREFIX", "/quester/")

	flag.StringVar(&addr, "addr", addr, "address to listen on")
	flag.StringVar(&dataDir, "data-dir", dataDir, "directory for task JSON files")
	flag.StringVar(&prefix, "prefix", prefix, "URL prefix to serve from")
	flag.Parse()

	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	store, err := NewStore(dataDir)
	if err != nil {
		log.Fatal(err)
	}

	app := NewApp(store, prefix)
	router := gin.Default()
	if err := router.SetTrustedProxies(nil); err != nil {
		log.Fatal(err)
	}
	app.Register(router)

	log.Printf("quester listening on http://%s%s", addr, app.prefix)
	if err := router.Run(addr); err != nil {
		log.Fatal(err)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
