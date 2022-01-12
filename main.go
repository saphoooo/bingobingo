package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/rs/zerolog/log"
	redigotrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/gomodule/redigo"
	muxtrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/gorilla/mux"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

func main() {
	tracer.Start(
		/*
			tracer.WithEnv("prod"),
			tracer.WithService("bingo"),
			tracer.WithServiceVersion("v1.0"),
		*/
		tracer.WithGlobalTag("env", "prod"),
		tracer.WithGlobalTag("service", "bingo"),
		tracer.WithGlobalTag("version", "v1.0"),
	)
	defer tracer.Stop()
	r := muxtrace.NewRouter(muxtrace.WithServiceName("bingo"))
	r.HandleFunc("/api/try", bingo).Methods("POST")
	log.Print("Start listening on :8000...")
	err := http.ListenAndServe(":8000", r)
	if err != nil {
		log.Panic().Msg(err.Error())
	}
}

func bingo(w http.ResponseWriter, r *http.Request) {
	pool := &redis.Pool{
		MaxIdle:     10,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			return redigotrace.Dial("tcp", "redis-master:6379",
				redigotrace.WithServiceName("redis"),
			)
		},
	}
	if span, ok := tracer.SpanFromContext(r.Context()); ok {
		span.SetTag("http.url", r.URL.Path)
	}
	body, err := ioutil.ReadAll(r.Body)

	// to be removed
	fmt.Println(string(body))

	if err != nil {
		log.Error().
			Str("hostname", r.Host).
			Str("method", r.Method).
			Str("proto", r.Proto).
			Str("remote_ip", r.RemoteAddr).
			Str("path", r.RequestURI).
			Str("user-agent", r.UserAgent()).
			Int("status", http.StatusInternalServerError).
			Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "{\"message\": \"Oops something wrong happened...\"")
		return
	}
	var try bingoTry
	err = json.Unmarshal(body, &try)
	if err != nil {
		log.Error().
			Str("hostname", r.Host).
			Str("method", r.Method).
			Str("proto", r.Proto).
			Str("remote_ip", r.RemoteAddr).
			Str("path", r.RequestURI).
			Str("user-agent", r.UserAgent()).
			Int("status", http.StatusInternalServerError).
			Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "{\"message\": \"Oops something wrong happened...\"")
		return
	}
	userDailyQuota, err := checkUserDailyQuota(pool, try.Name)
	if err != nil {
		log.Error().
			Str("hostname", r.Host).
			Str("method", r.Method).
			Str("proto", r.Proto).
			Str("remote_ip", r.RemoteAddr).
			Str("path", r.RequestURI).
			Str("user-agent", r.UserAgent()).
			Str("username", try.Name).
			Str("number", try.Number).
			Int("status", http.StatusInternalServerError).
			Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "{\"message\": \"Oops something wrong happened...\"")
		return
	}
	if userDailyQuota {
		message := "hey " + try.Name + ", you already tried your luck today"
		log.Info().
			Str("hostname", r.Host).
			Str("method", r.Method).
			Str("proto", r.Proto).
			Str("remote_ip", r.RemoteAddr).
			Str("path", r.RequestURI).
			Str("user-agent", r.UserAgent()).
			Str("username", try.Name).
			Str("number", try.Number).
			Int("status", http.StatusOK).
			Msg(message)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "{\"message\": "+message+"}")
		return
	}

	bingoNumberOfTheDay, err := getBingoNumberOfTheDay(pool)
	if err != nil {
		log.Error().
			Str("hostname", r.Host).
			Str("method", r.Method).
			Str("proto", r.Proto).
			Str("remote_ip", r.RemoteAddr).
			Str("path", r.RequestURI).
			Str("user-agent", r.UserAgent()).
			Str("username", try.Name).
			Str("number", try.Number).
			Int("status", http.StatusInternalServerError).
			Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "{\"message\": \"Oops something wrong happened...\"")
		return
	}
	tryNumber, err := strconv.Atoi(try.Number)
	if err != nil {
		log.Error().
			Str("hostname", r.Host).
			Str("method", r.Method).
			Str("proto", r.Proto).
			Str("remote_ip", r.RemoteAddr).
			Str("path", r.RequestURI).
			Str("user-agent", r.UserAgent()).
			Str("username", try.Name).
			Str("number", try.Number).
			Int("status", http.StatusInternalServerError).
			Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "{\"message\": \"Oops something wrong happened...\"")
		return
	}

	if bingoNumberOfTheDay == tryNumber {
		message := "hooray " + try.Name + ", great job you guess the correct number!"
		log.Info().
			Str("hostname", r.Host).
			Str("method", r.Method).
			Str("proto", r.Proto).
			Str("remote_ip", r.RemoteAddr).
			Str("path", r.RequestURI).
			Str("user-agent", r.UserAgent()).
			Str("username", try.Name).
			Str("number", try.Number).
			Int("status", http.StatusOK).
			Msg(message)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "{\"message\": "+message+"}")
		return
	}

	message := "sorry " + try.Name + ", you didn't guess the right number this time, but try again tomorrow!"
	log.Info().
		Str("hostname", r.Host).
		Str("method", r.Method).
		Str("proto", r.Proto).
		Str("remote_ip", r.RemoteAddr).
		Str("path", r.RequestURI).
		Str("user-agent", r.UserAgent()).
		Str("username", try.Name).
		Str("number", try.Number).
		Int("status", http.StatusOK).
		Msg(message)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "{\"message\": "+message+"}")

}

func checkUserDailyQuota(pool *redis.Pool, name string) (bool, error) {
	conn := pool.Get()
	defer conn.Close()

	root, ctx := tracer.StartSpanFromContext(context.Background(), "parent.request",
		tracer.ServiceName("bingo"),
		tracer.ResourceName("redis"),
	)
	defer root.Finish()

	_, err := conn.Do("AUTH", os.Getenv("REDIS_PASSWORD"), ctx)
	if err != nil {
		log.Error().Msg("error during authentication")
		return false, err
	}

	exists, err := redis.Int(conn.Do("EXISTS", name, ctx))
	if err != nil {
		log.Error().Msg("error getting name")
		return false, err
	}
	if exists == 0 { // the key does not exist
		log.Info().Msg("user " + name + " plays for the first time today")
		// create a new key entry
		_, err = conn.Do("SET", name, "1")
		if err != nil {
			log.Error().Msg("error setting name")
			return false, err
		}
		// set expiry of 1 day
		_, err = conn.Do("EXPIRE", name, 86400, ctx)
		if err != nil {
			log.Error().Msg("error setting expiry")
			return false, err
		}
		return false, nil
	}
	log.Info().Msg("user " + name + " has already played today")
	return true, nil
}

func getBingoNumberOfTheDay(pool *redis.Pool) (int, error) {
	conn := pool.Get()
	defer conn.Close()

	root, ctx := tracer.StartSpanFromContext(context.Background(), "parent.request",
		tracer.ServiceName("bingo"),
		tracer.ResourceName("redis"),
	)
	defer root.Finish()

	_, err := conn.Do("AUTH", os.Getenv("REDIS_PASSWORD"), ctx)
	if err != nil {
		log.Error().Msg("error during authentication")
		return 0, err
	}

	exists, err := redis.Int(conn.Do("EXISTS", "bingoNumberOfTheDay", ctx))
	if err != nil {
		log.Error().Msg("error verifying bingoNumberOfTheDay")
		return 0, err
	} else if exists == 0 { // <- the key does not exist
		// call the bingoNumberOfTheDayGenerator
		resp, err := http.Get("http://bingo-generator:8000/trigger")
		if err != nil {
			return 0, err
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return 0, err
		}
		returnValue, err := strconv.Atoi(string(body))
		if err != nil {
			return 0, err
		}
		return returnValue, nil
	}
	number, err := redis.Int(conn.Do("GET", "bingoNumberOfTheDay", ctx))
	if err != nil {
		log.Error().Msg("error getting bingoNumberOfTheDay")
		return 0, err
	}
	return number, nil
}

type bingoTry struct {
	Name   string `json:"name"`
	Number string `json:"number"`
}
