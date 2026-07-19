package main

import "log"

func fatalError(err error) { log.Fatal(err) }

func logListening(addr, prefix string) { log.Printf("quester listening on http://%s%s", addr, prefix) }

func logMigrationComplete(count int, sourceDir, destinationDir string) {
	log.Printf("migrated %d legacy task files from %s to %s", count, sourceDir, destinationDir)
}

func logMutationFailed(err error) { log.Printf("mutation failed: %v", err) }

func logBackupFailed(err error) { log.Printf("backup failed: %v", err) }

func logBackupSkippedBlob(ref string, err error) { log.Printf("backup skipped blob %s: %v", ref, err) }

func logCleanupFailed(err error) { log.Printf("cleanup failed: %v", err) }

func logRenderFailed(templateName string, err error) {
	log.Printf("render %s failed: %v", templateName, err)
}
