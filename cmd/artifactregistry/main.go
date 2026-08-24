package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"artifactregistry/internal/auth"
	"artifactregistry/internal/blob"
	"artifactregistry/internal/index"
	"artifactregistry/internal/manifest"
	"artifactregistry/internal/registry"
	"artifactregistry/internal/repo"
	"artifactregistry/internal/tag"
	"artifactregistry/internal/transfer"
)

func main() {
	addr := flag.String("addr", ":8377", "HTTP listen address")
	shards := flag.Int("shards", 8, "blob store shard count")
	browse := flag.String("browse", "web/browse.html", "path to the browse page")
	flag.Parse()

	repos := repo.NewMemoryStore()
	blobs := blob.NewMemoryStore(*shards)
	authStore := auth.NewMemoryStore()
	sessions := transfer.NewSessionManager(blobs, authStore, repos)
	manifests := manifest.NewMemoryStore(blobs, sessions)
	tags := tag.NewMemoryStore()
	indexStore := index.NewMemoryStore()
	reg := registry.New(repos, blobs, manifests, tags, indexStore, authStore, sessions)

	server := &http.Server{
		Addr:              *addr,
		Handler:           NewServer(reg, *browse),
		ReadHeaderTimeout: 10 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		log.Printf("artifactregistry listening on %s", *addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	log.Println("artifactregistry stopped")
}
