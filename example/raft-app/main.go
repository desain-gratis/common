package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	raftchat "github.com/desain-gratis/common/example/raft-app/src/app/raft-chat"
	raftchat_http "github.com/desain-gratis/common/example/raft-app/src/app/raft-chat/integration"
	notifier_api "github.com/desain-gratis/common/lib/notifier/api"
	notifier_impl "github.com/desain-gratis/common/lib/notifier/impl"
	raft_replica "github.com/desain-gratis/common/lib/raft/replica"
	raft_runner "github.com/desain-gratis/common/lib/raft/runner"
	"github.com/julienschmidt/httprouter"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger()
}

func main() {
	appCtx, appCancel := context.WithCancelCause(context.Background())

	var c, address string
	flag.StringVar(&c, "c", "config.json", "config path")
	flag.StringVar(&address, "address", "0.0.0.0:9090", "api bind address")
	flag.Parse()

	initConfig(appCtx, c)

	// TODO: merge raft_replica & raft_runner to become one. raft.Init(), raft.ForEachReplica, raft.Run, raft.GetReplicaConfig(ctx), raft.GetClient()
	err := raft_replica.Init()
	if err != nil {
		log.Panic().Msgf("panic init replica: %v", err)
	}

	router := httprouter.New()

	raft_runner.ForEachReplica[raftchat.Config]("happy", func(ctx context.Context) error {
		// init topic
		chatTopic := notifier_impl.NewStandardTopic()

		// init raft app
		chatApp := raftchat.New(chatTopic)

		// run raft app
		err := raft_runner.Run(ctx, chatApp)
		if err != nil {
			return err
		}

		// integrate the chat app with outside world / "delivery"
		// in this case, a chat application that we've built.
		chatIntegration := raftchat_http.New(ctx)

		// spawn topic for each replica instance
		topicAPI := notifier_api.NewTopicAPI(chatTopic, parseTable)

		raftCtx, _ := raft_runner.GetRaftContext(ctx)
		router.GET("/happy/"+raftCtx.ID, topicAPI.Metrics)
		router.POST("/happy/"+raftCtx.ID, topicAPI.Publish)
		router.GET("/happy/"+raftCtx.ID+"/tail", topicAPI.Tail)
		router.GET("/happy/"+raftCtx.ID+"/ws", chatIntegration.Websocket)

		return nil
	})

	// router.PanicHandler = func(w http.ResponseWriter, r *http.Request, i interface{}) {
	// 	w.WriteHeader(http.StatusInternalServerError)
	// 	w.Write([]byte("oh no"))
	// }

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
			header.Set("Access-Control-Allow-Headers", "Content-Type")
			router.ServeHTTP(w, r)
		})
	}

	// provides a way to stop a long running connnection cleanly
	// ender := make(chan struct{})
	wsWg := &sync.WaitGroup{}
	server := http.Server{
		Addr:        address,
		Handler:     withCors(router),
		ReadTimeout: 2 * time.Second,

		// important: do not set WriteTimeout if we enable long running connection like this example
		// WriteTimeout: 15 * time.Second,

		BaseContext: func(l net.Listener) context.Context {
			// inject with application context.
			ctx := context.WithValue(appCtx, "ws-wg", wsWg)
			return ctx
		},
	}

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

func parseTable(v any) any {
	switch t := v.(type) {
	case []byte:
		return string(t)
	case raftchat.Event:
		d, _ := json.Marshal(v)
		return string(d)
	}
	return v
}
