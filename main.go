package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	addr := envOrDefault("QUESTER_ADDR", "127.0.0.1:93")
	dataDir := envOrDefault("QUESTER_DATA_DIR", ".quester-data")
	prefix := envOrDefault("QUESTER_PREFIX", "/quester/")
	trustedProxySpec := envOrDefault("QUESTER_TRUSTED_PROXIES", "")
	migrateFrom := ""
	testMode := false

	flag.StringVar(&addr, "addr", addr, "address to listen on")
	flag.StringVar(&dataDir, "data-dir", dataDir, "directory for task JSON files")
	flag.StringVar(&prefix, "prefix", prefix, "URL prefix to serve from")
	flag.StringVar(&trustedProxySpec, "trusted-proxies", trustedProxySpec, "comma-separated proxy IP addresses or CIDR blocks allowed to authenticate users; required for non-loopback addresses")
	flag.StringVar(&migrateFrom, "migrate-from", migrateFrom, "legacy directory containing *.json task files to validate and migrate into -data-dir, then exit; the destination must not exist")
	flag.BoolVar(&testMode, "test", false, "run an in-process model and template check, then exit")
	flag.Parse()
	if testMode && migrateFrom != "" {
		fatalError(fmt.Errorf("-test and -migrate-from cannot be used together"))
	}
	if migrateFrom != "" {
		count, err := migrateLegacyData(migrateFrom, dataDir)
		if err != nil {
			fatalError(err)
		}
		logMigrationComplete(count, migrateFrom, dataDir)
		return
	}
	if testMode {
		if err := runSelfTest(); err != nil {
			fatalError(err)
		}
		fmt.Println("quester self-test passed")
		return
	}

	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	store, err := NewStore(dataDir)
	if err != nil {
		fatalError(err)
	}

	trustedProxies, err := parseTrustedProxies(trustedProxySpec)
	if err != nil {
		fatalError(err)
	}
	if !isLoopbackAddr(addr) && len(trustedProxies) == 0 {
		fatalError(fmt.Errorf("address %q is not loopback; configure -trusted-proxies so only an authenticating reverse proxy can reach Quester", addr))
	}

	app := NewApp(store, prefix)
	app.trustedProxies = trustedProxies
	router := gin.Default()
	if err := router.SetTrustedProxies(nil); err != nil {
		fatalError(err)
	}
	app.Register(router)

	logListening(addr, app.prefix)
	if err := router.Run(addr); err != nil {
		fatalError(err)
	}
}

func runSelfTest() error {
	root := defaultRoot()
	root.Forums = append(root.Forums, &Forum{Id: "trips", Name: "Trips"})
	post := newTask("Trip", "Plan it", defaultForumID, defaultUserID, true)
	reply := newTask("", "Agent response", defaultForumID, "agent", false)
	post.SubTasks = append(post.SubTasks, reply)
	root.SubTasks = append(root.SubTasks, post)
	root = normalizeTree(root)
	if FindTask(reply.Id, root) != reply {
		return fmt.Errorf("self-test stable lookup: expected node %q to resolve", reply.Id)
	}
	if err := moveTask(root, reply.Id, "", "trips", "Promoted response"); err != nil {
		return fmt.Errorf("self-test promote node %q: %w", reply.Id, err)
	}
	if findParent(root, reply) != root || reply.ForumId != "trips" || !reply.Track {
		return fmt.Errorf("self-test promoted node %q: expected top-level tracked node in trips, received forum %q tracked %t", reply.Id, reply.ForumId, reply.Track)
	}
	legacy, err := normalizeLegacyData([]byte(`{"Name":"Legacy","SubTasks":[{"Id":"old","Name":"Old task"}]}`), "self-test.json")
	if err != nil {
		return fmt.Errorf("self-test legacy migration: %w", err)
	}
	var migrated Task
	if err := json.Unmarshal(legacy, &migrated); err != nil {
		return fmt.Errorf("self-test migrated data: %w", err)
	}
	if migrated.Schema != currentSchema || len(migrated.SubTasks) != 1 || !migrated.SubTasks[0].Track {
		return fmt.Errorf("self-test migrated data: expected schema %d with one tracked task, received schema %d and %d tasks", currentSchema, migrated.Schema, len(migrated.SubTasks))
	}
	if newTemplates() == nil {
		return fmt.Errorf("self-test templates: expected parsed templates")
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
