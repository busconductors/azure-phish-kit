package main

import (
	"embed"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
)

//go:embed dashboard.html
var tmplFS embed.FS

func main() {
	port := flag.String("port", "9091", "listen port")
	dataPath := flag.String("data", "../data/captures.jsonl", "path to captures.jsonl")
	flag.Parse()

	tmpl, err := template.ParseFS(tmplFS, "dashboard.html")
	if err != nil {
		log.Fatalf("FATAL: cannot parse template: %v", err)
	}

	cache := NewCache(*dataPath)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		data, err := cache.Get()
		if err != nil {
			log.Printf("[ERROR] cache get: %v", err)
			http.Error(w, fmt.Sprintf("cannot read data: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			log.Printf("[ERROR] template execute: %v", err)
		}
	})

	addr := ":" + *port
	log.Printf("Analytics dashboard listening on http://localhost%s (data: %s)", addr, *dataPath)
	log.Fatal(http.ListenAndServe(addr, mux))
}
