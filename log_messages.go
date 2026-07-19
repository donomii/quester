package main

import "log"

func fatalError(err error) { log.Fatal(err) }

func logListening(addr, prefix string) { log.Printf("quester listening on http://%s%s", addr, prefix) }

func logMutationFailed(err error) { log.Printf("mutation failed: %v", err) }

func logRenderFailed(templateName string, err error) {
	log.Printf("render %s failed: %v", templateName, err)
}
