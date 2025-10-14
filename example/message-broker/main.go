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

	logapi "github.com/desain-gratis/common/example/message-broker/src/log-api"
	logapi_impl "github.com/desain-gratis/common/example/message-broker/src/log-api/impl"
	logapi_impl_replicated "github.com/desain-gratis/common/example/message-broker/src/log-api/impl/replicated"
	"github.com/desain-gratis/common/utility/replica"
	"github.com/julienschmidt/httprouter"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger()
}

func main() {
	ctx := context.Background()

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

	replica.ForEachType("message-broker", func(config replica.Config[logapi_impl_replicated.LogConfig]) error {
		// Initialize

		// Start replica based on config
		smf := logapi_impl_replicated.CreateSM(config.AppConfig)
		err := config.StartReplica(smf)
		if err != nil {
			return err
		}

		// Create HTTP API handler to interact with the replica
		brokerAPI := broker{
			dhost:     config.Host,
			shardID:   config.ShardID,
			replicaID: config.ReplicaID,
		}

		router.GET("/log/"+config.ID, brokerAPI.GetTopic)
		router.POST("/log/"+config.ID, brokerAPI.Publish)
		router.GET("/log/"+config.ID+"/tail", brokerAPI.Tail)
		router.GET("/log/"+config.ID+"/ws", brokerAPI.Websocket)

		return nil
	})

	replica.ForEachType("happy", func(config replica.Config[logapi_impl_replicated.LogConfig]) error {
		topic := logapi_impl.NewTopic(func(key string) logapi.Subscription {
			return logapi_impl.NewSubscription(key, true, 0, config.AppConfig.ExitMessage, time.Duration(config.AppConfig.ListenTimeoutS)*time.Second)
		})

		happy := logapi_impl_replicated.NewHappy(topic, config.AppConfig.ClickhouseAddr)

		err := config.StartOnDiskReplica(happy)
		if err != nil {
			return err
		}

		// Create HTTP API handler to interact with the replica
		brokerAPI := broker{
			dhost:     config.Host,
			shardID:   config.ShardID,
			replicaID: config.ReplicaID,
		}

		router.GET("/happy/"+config.ID, brokerAPI.GetTopic)
		router.POST("/happy/"+config.ID, brokerAPI.Publish)
		router.GET("/happy/"+config.ID+"/tail", brokerAPI.Tail)
		router.GET("/happy/"+config.ID+"/ws", brokerAPI.Websocket)
		// TODO: brokerAPI.Websocket(wsController)

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
	// wg := &sync.WaitGroup{}
	ctx, _ = context.WithCancel(context.Background())
	server := http.Server{
		Addr:        address,
		Handler:     withCors(router),
		ReadTimeout: 2 * time.Second,

		// important: do not set WriteTimeout if we enable long running connection like this example
		// WriteTimeout: 15 * time.Second,

		BaseContext: func(l net.Listener) context.Context {
			// inject with application context.
			return context.WithValue(ctx, "app-ctx", ctx)
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

		// close hijacked websocket connection

		// close(ender)
		// log.Info().Msgf("Waiting for websocket close..")
		// appStop()

		// TODO: wsController.Shutdown(ctx)
		// wg.Wait()
		// log.Info().Msgf("CLOSED ALL WS")

		// stop() // move stop here (so we can still send to ws in code above)

		// We received an interrupt signal, shutdown sequence (stop listen, wait existing to finish), wait 30 second max.

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

type Stopper struct {
	wg *sync.WaitGroup
}
