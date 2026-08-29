package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	mycontentapi "github.com/desain-gratis/common/delivery/mycontent-api"
	mycontent_base "github.com/desain-gratis/common/delivery/mycontent-api/mycontent/base"
	content_badgerraft "github.com/desain-gratis/common/delivery/mycontent-api/storage/content/badger-raft"
	"github.com/desain-gratis/common/example/etcdraft/entity"
	runneretcd "github.com/desain-gratis/common/lib/raft/runner-etcd"
	"github.com/dgraph-io/badger/v4"
	"github.com/julienschmidt/httprouter"
	"github.com/rs/zerolog/log"
)

func main() {
	appCtx, appCancel := context.WithCancelCause(context.Background())

	var configPath string
	var address string
	flag.StringVar(&configPath, "c", "raft.yaml", "config path")
	flag.StringVar(&address, "p", ":9000", "serve address (default :9000)")
	flag.Parse()

	// test in memory first
	opts := badger.DefaultOptions("").WithInMemory(true)
	db, err := badger.Open(opts)
	if err != nil {
		log.Fatal().Msgf("UHUY", err)
	}

	// A raft application that provides distributed mycontent storage
	badgerStorageApp := content_badgerraft.New(
		db,
		content_badgerraft.TableConfig{Name: "user_profile", RefSize: 0, Versioned: true},
	)

	// Run the raft engine for this app
	raftCtx, in, _ := runneretcd.RunWithConfig(
		configPath,
		"user-profile",
		badgerStorageApp,
	)

	// lets pass the version parameter via context in the client. 
	// and allow delivery to accept this parameter, and pass it via context
	//  maybe in the mycontent interface we define it (the context setter & getter)
	// and it will be ready to be used in the repo (just like the error)

	userProfileRepo, err := badgerStorageApp.GetContentRepository(raftCtx, "user_profile")
	if err != nil {
		return
	}

	userProfileHandler := mycontentapi.New(
		mycontent_base.New[*entity.UserProfile](userProfileRepo),
		"www.mantap.com"+"/user",
		nil,
	)

	router := httprouter.New()

	// global cors handlign
	router.HandleOPTIONS = true
	router.GlobalOPTIONS = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	withCors := func(router http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := w.Header()
			header.Set("Access-Control-Allow-Methods", header.Get("Allow"))
			header.Set("Access-Control-Allow-Origin", "*")
			// header.Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			header.Set("Access-Control-Allow-Headers", "*")
			router.ServeHTTP(w, r)
		})
	}

	// router.OPTIONS("/user", Empty)
	router.GET("/user", userProfileHandler.Get)
	router.POST("/user", userProfileHandler.Post)
	router.DELETE("/user", userProfileHandler.Delete)

	// inject with application context.
	wsWg := &sync.WaitGroup{}

	server := http.Server{
		Addr:        address,
		Handler:     withCors(router),
		ReadTimeout: 2 * time.Second,

		BaseContext: func(l net.Listener) context.Context {
			// inject with application context.
			ctx := context.WithValue(appCtx, "ws-wg", wsWg)
			return ctx
		},
	}

	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := scanner.Text()
			in <- line
		}
	}()

	idleConnsClosed := make(chan struct{})
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		log.Info().Msgf("WAITING FOR SIGINT")
		<-sigint
		log.Info().Msgf("SIGINT RECEIVED")

		// close HTTP connection
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		appCancel(errors.New("server is shutting down"))

		log.Info().Msgf("Shutting down HTTP server..")
		if err := server.Shutdown(ctx); err != nil {
			// Error from closing listeners, or context timeout:
			log.Err(err).Msgf("HTTP server Shutdown")
		}

		log.Info().Msgf("Waiting for websocket connection to close..")
		wsWg.Wait()

		close(idleConnsClosed)
	}()

	// TODO: maybe can use this for more graceful handling
	// server.RegisterOnShutdown()

	log.Info().Msgf("Serving at %v..\n", address)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		// Error starting or closing listener:
		log.Fatal().Msgf("HTTP server ListendAndServe: %v", err)
	}

	<-idleConnsClosed
	log.Info().Msgf("Bye bye")
}
