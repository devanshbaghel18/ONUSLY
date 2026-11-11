//go:build ignore

package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

const (
	fixturePort  = "8765"
	extensionID  = "jifigaejbinmdhnljobkodcgeaplojmg"
	version      = "1.0.0"
)

func main() {
	crxPath := filepath.Join("frontend", "public", "downloads", "onusly.crx")
	if _, err := os.Stat(crxPath); os.IsNotExist(err) {
		// Try relative to repo root
		crxPath = filepath.Join("..", "..", "..", "frontend", "public", "downloads", "onusly.crx")
	}

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","fixture":"extension_distribution"}`)
	})

	http.HandleFunc("/downloads/updates.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		xmlContent := fmt.Sprintf(`<?xml version='1.0' encoding='UTF-8'?>
<gupdate xmlns='http://www.google.com/update2/response' protocol='2.0'>
  <app appid='%s'>
    <updatecheck codebase='http://127.0.0.1:%s/downloads/onusly.crx' version='%s' />
  </app>
</gupdate>`, extensionID, fixturePort, version)
		w.Write([]byte(xmlContent))
		log.Printf("[Fixture Server] Served /downloads/updates.xml to %s", r.RemoteAddr)
	})

	http.HandleFunc("/downloads/onusly.crx", func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile(crxPath)
		if err != nil {
			http.Error(w, fmt.Sprintf("CRX artifact not found at %s", crxPath), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/x-chrome-extension")
		w.Header().Set("Content-Disposition", "attachment; filename=\"onusly.crx\"")
		w.Write(data)
		log.Printf("[Fixture Server] Served /downloads/onusly.crx (%d bytes) to %s", len(data), r.RemoteAddr)
	})

	log.Printf("[Fixture Server] Starting Local Extension Distribution Fixture on http://127.0.0.1:%s...", fixturePort)
	log.Printf("[Fixture Server] Serving updates.xml with codebase http://127.0.0.1:%s/downloads/onusly.crx", fixturePort)
	if err := http.ListenAndServe("127.0.0.1:"+fixturePort, nil); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}


