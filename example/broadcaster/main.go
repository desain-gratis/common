package main

import (
	"context"
	"errors"
	"flag"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	logapi_impl_replicated "github.com/desain-gratis/common/delivery/log-api/impl/replicated"
	"github.com/desain-gratis/common/utility/smregistry"
	"github.com/julienschmidt/httprouter"
	"github.com/lni/dragonboat/v4"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger()
}

func main() {
	ctx := context.Background()

	var c, dc, address string
	flag.StringVar(&c, "c", "config.json", "config path")
	flag.StringVar(&dc, "dc", "dragonboat-config.json", "config path")
	flag.StringVar(&address, "address", "0.0.0.0:9090", "api bind address")
	flag.Parse()

	initConfig(ctx, c)

	dregistry, err := smregistry.NewDragonboat(ctx, dc)
	if err != nil {
		log.Panic().Msgf("UHUY CONFIG %v", err)
	}

	router := httprouter.New()

	// register state machine of type "log"
	smregistry.Register(dregistry, "log", logapi_impl_replicated.CreateSM)
	smregistry.RegisterEtc(dregistry, "log", func(dhost *dragonboat.NodeHost, instance smregistry.SMConfig2) {
		// Create API handler for the state machine
		brokerAPI := broker{
			shardID:   instance.ShardID,
			replicaID: instance.ReplicaID,
			dhost:     dhost,
		}

		router.GET("/"+instance.Name, brokerAPI.GetTopic)
		router.POST("/"+instance.Name, brokerAPI.Publish)
		router.GET("/"+instance.Name+"/tail", brokerAPI.Tail)
	})

	dregistry.Start(context.Background())

	router.HandleOPTIONS = true
	router.PanicHandler = func(w http.ResponseWriter, r *http.Request, i interface{}) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("oh no"))
	}

	// provides a way for long running connnection to stop cleanly
	ctx, stop := context.WithCancelCause(context.Background())
	server := http.Server{
		Addr:    address,
		Handler: router,

		// Important: do not set this if we enable long running connection like this example
		// ReadTimeout:  15 * time.Second,
		// WriteTimeout: 15 * time.Second,

		BaseContext: func(l net.Listener) context.Context { return ctx },
	}

	idleConnsClosed := make(chan struct{})
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint
		log.Info().Msgf("SIGINT RECEIVED")

		stop(errors.New("karena server closed"))

		// We received an interrupt signal, shut down.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		log.Info().Msgf("Shutting down HTTP server..")
		if err := server.Shutdown(ctx); err != nil {
			// Error from closing listeners, or context timeout:
			log.Err(err).Msgf("HTTP server Shutdown")
		}
		log.Info().Msgf("Stopped serving new connections.")
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
