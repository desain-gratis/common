package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	notifierapi "github.com/desain-gratis/common/delivery/notifier-api"
	"github.com/julienschmidt/httprouter"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger()
}

func main() {
	ctx := context.Background()

	var c, dc string
	flag.StringVar(&c, "c", "config.json", "config path")
	flag.StringVar(&dc, "dc", "dragonboat-config.json", "config path")
	flag.Parse()

	initConfig(ctx, c)

	dcc, err := initDragonboatConfig(ctx, dc)
	if err != nil {
		log.Panic().Msgf("UHUY CONFIG %v", err)
	}

	dhost, fsms := enableBroadcaster(dcc)

	router := httprouter.New()

	address := "0.0.0.0:9090"

	router.GET("/", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		// print list of topic
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "success")
	})

	router.GET("/:topic", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		// print list of topic
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "success %v", p.ByName("topic"))
		// print topic metadta
	})

	router.POST("/:topic", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		// post message to topic
		payload, _ := io.ReadAll(r.Body)

		// topic to shard mapping
		mapping, ok := fsms[p.ByName("topic")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, "topic not found: %v", p.ByName("topic"))
			return
		}

		sess := dhost.GetNoOPSession(mapping.shardID)

		ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()

		v, err := dhost.SyncPropose(ctx, sess, payload)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "error cuk: %v", err)
			return
		}

		w.WriteHeader(http.StatusAccepted)
		fmt.Fprintf(w, "success: %v (value: %v)", string(v.Data), v.Value)
	})

	router.GET("/:topic/tail", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		// tail topic log

		// topic to shard mapping
		mapping, ok := fsms[p.ByName("topic")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, "topic not found: %v", p.ByName("topic"))
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// todo: retry
		v, err := dhost.SyncRead(ctx, mapping.shardID, "haii")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "error cuk: %v", err)
			return
		}

		defer func() {
			go func() {
				sess := dhost.GetNoOPSession(mapping.shardID)
				dhost.SyncPropose(context.Background(), sess, []byte("cleanup listener with this id"))
			}()
		}()

		notifier, ok := v.(notifierapi.Notifier)
		if !ok {
			http.Error(w, "notifier noT?", http.StatusInternalServerError)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		log.Info().Msgf("LISTENING...")

		ctxx := context.WithValue(r.Context(), "mhm", "aha")

		w.WriteHeader(http.StatusAccepted)

		for msg := range notifier.Listen(ctxx) {
			data, err := json.Marshal(msg)
			if err != nil {
				log.Err(err).Msgf("marshal feel %v", msg)
				continue
			}

			_, err = fmt.Fprintf(w, "%v\n", string(data))
			if err != nil {
				log.Err(err).Msgf("cannot write uhuy")
				return
			}

			flusher.Flush() // can use ticker to flush every x millis...
		}

		log.Info().Msgf("KELUARKAH? %v", r.Context().Err())
		fmt.Fprintf(w, "Bye")
	})

	// provides a way for long running connnection to stop cleanly
	ctx, stop := context.WithCancelCause(context.Background())
	server := http.Server{
		Addr:    address,
		Handler: router,

		// I think this caused context to be cancelled (triggered somehow after the message are sent)
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
