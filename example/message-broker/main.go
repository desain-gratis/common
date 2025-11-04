package main

import (
	"context"
	"flag"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	chatlogwriter "github.com/desain-gratis/common/example/message-broker/src/log-api/impl/chat-log-writer"
	"github.com/desain-gratis/common/example/message-broker/src/log-api/impl/statemachine"
	notifier_api "github.com/desain-gratis/common/lib/notifier/api"
	notifier_impl "github.com/desain-gratis/common/lib/notifier/impl"
	"github.com/desain-gratis/common/utility/replica"
	"github.com/julienschmidt/httprouter"
	"github.com/lni/dragonboat/v4/client"
	"github.com/lni/goutils/random"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger()
}

func main() {
	ctx := context.Background()
	appCtx, appCancel := context.WithCancel(ctx)

	var c, address string
	flag.StringVar(&c, "c", "config.json", "config path")
	flag.StringVar(&address, "address", "0.0.0.0:9090", "api bind address")
	flag.Parse()

	initConfig(ctx, c)

	err := replica.Init()
	if err != nil {
		log.Panic().Msgf("panic init replica: %v", err)
	}

	router := httprouter.New()

	replica.ForEachType("happy", func(config replica.Config[chatlogwriter.LogConfig]) error {
		topic := notifier_impl.NewTopic()
		happy := chatlogwriter.NewHappy(topic, config.ShardID, config.ReplicaID)
		sm := statemachine.NewWithHappy(config.AppConfig.ClickhouseAddr, happy)

		err := config.StartOnDiskReplica(sm)
		if err != nil {
			return err
		}

		// Create HTTP API handler to interact with the replica
		sess := client.NewNoOPSession(config.ShardID, random.NewLockedRand())

		brokerAPI := broker{
			dhost:     config.Host,
			shardID:   config.ShardID,
			replicaID: config.ReplicaID,
			sess:      sess,
		}

		topicAPI := notifier_api.NewTopicAPI(topic, nil)

		router.GET("/happy/"+config.ID, topicAPI.Metrics)
		router.POST("/happy/"+config.ID, topicAPI.Publish)
		router.GET("/happy/"+config.ID+"/tail", topicAPI.Subscribe)
		router.GET("/happy/"+config.ID+"/ws", brokerAPI.Websocket)

		// For realtime part:
		// todo: brokerAPI.WebSocket(topic) Tail(topic)
		// in other words, a default API (jsonl stream / websocket) for "notifier.Topic".
		// able to filter by:
		// 1. table_name / event_name
		// 2. by other custom key value filter fn, (type assertion on Event's data)
		// 3. or other capabilities... map/reduce / functional programming / DAGs/ UDFs etc..
		// 4. can have default API for parsing simple DAGs to combine more than 1 real time topic
		//. and any other custom implementations...s

		// For the non realtime/snapshot / key-value part:
		// and then later, can have key value storage or any derivatives of the event
		// there is default for desain.gratis.. but user can create custom filter / DAGs / UDFslater
		// router.GET("/happy/"+config.ID+"/table/active-users", ...)
		// router.GET("/happy/"+config.ID+"/table/chat?room_id=...", ...)
		// router.GET("/happy/"+config.ID+"/table/purchase?id=...", ...)

		// lets implement that.. and desain.gratis will be unstoppable

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
			ctx := context.WithValue(ctx, "app-ctx", appCtx)
			ctx = context.WithValue(ctx, "ws-wg", wsWg)
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

		log.Info().Msgf("Shutting down HTTP server..")
		if err := server.Shutdown(ctx); err != nil {
			// Error from closing listeners, or context timeout:
			log.Err(err).Msgf("HTTP server Shutdown")
		}

		// websocket ws (todo: better naming)
		appCancel()
		log.Info().Msgf("Waiting for websocket connection to close..")
		wsWg.Wait()

		close(idleConnsClosed)
	}()

	log.Info().Msgf("Serving at %v..\n", address)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		// Error starting or closing listener:
		log.Fatal().Msgf("HTTP server ListendAndServe: %v", err)
	}

	<-idleConnsClosed
	log.Info().Msgf("Bye bye")
}
