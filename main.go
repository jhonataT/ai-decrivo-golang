package main

import (
	"context"
	"embed"
	"log"
	"net/http"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/jhonataT/ai-decrivo-golang/internal/gitrepo"
	"github.com/jhonataT/ai-decrivo-golang/internal/jsonstore"
	"github.com/jhonataT/ai-decrivo-golang/internal/mcpserver"
	"github.com/jhonataT/ai-decrivo-golang/internal/review"
	"github.com/jhonataT/ai-decrivo-golang/internal/ui"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	dir, err := jsonstore.DefaultDir("reviews")
	if err != nil {
		log.Fatal(err)
	}
	db, err := jsonstore.Reviews(dir)
	if err != nil {
		log.Fatal(err)
	}
	svc := review.NewService(gitrepo.Source{}, db)
	// Um arquivo corrompido não impede o app de abrir: só é registrado no log.
	if err := svc.Load(); err != nil {
		log.Println("histórico:", err)
	}
	log.Println("revisões em", dir)

	var appCtx context.Context

	svc.OnChange(func(reviewID string) {
		if appCtx != nil {
			runtime.EventsEmit(appCtx, "review:changed", reviewID)
		}
	})

	onFinish := func(reviewID string) {
		if appCtx == nil {
			return
		}
		runtime.WindowUnminimise(appCtx)
		runtime.WindowShow(appCtx)
		runtime.EventsEmit(appCtx, "review:ready", reviewID)
	}

	go func() {
		mux := http.NewServeMux()
		mux.Handle("/mcp", mcpserver.New(svc, onFinish))
		srv := &http.Server{Addr: "127.0.0.1:7337", Handler: mux}
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Println("mcp:", err)
		}
	}()

	err = wails.Run(&options.App{
		Title: "Decrivo", Width: 1440, Height: 900,
		OnStartup:   func(ctx context.Context) { appCtx = ctx },
		AssetServer: &assetserver.Options{Assets: assets, Handler: ui.NewHandler(svc)},
	})
	if err != nil {
		log.Fatal(err)
	}
}
