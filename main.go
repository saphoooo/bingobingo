package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/", bingo).Methods("POST")
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
			return redis.Dial("tcp", "redis-master:6379")
		},
	}
	fmt.Println(r.Body)
	var myForm bingoNumber
	myForm.Name = r.PostFormValue("name")
	formValue, err := strconv.Atoi(r.PostFormValue("number"))
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
		fmt.Fprint(w, err.Error())
		return
	}
	myForm.Number = formValue
	userDailyQuota, err := checkUserDailyQuota(pool, myForm.Name)
	if err != nil {
		log.Error().
			Str("hostname", r.Host).
			Str("method", r.Method).
			Str("proto", r.Proto).
			Str("remote_ip", r.RemoteAddr).
			Str("path", r.RequestURI).
			Str("user-agent", r.UserAgent()).
			Str("username", myForm.Name).
			Int("number", myForm.Number).
			Int("status", http.StatusInternalServerError).
			Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err.Error())
		return
	}
	if userDailyQuota {
		message := "hey " + myForm.Name + ", you already tried your luck today"
		log.Info().
			Str("hostname", r.Host).
			Str("method", r.Method).
			Str("proto", r.Proto).
			Str("remote_ip", r.RemoteAddr).
			Str("path", r.RequestURI).
			Str("user-agent", r.UserAgent()).
			Str("username", myForm.Name).
			Int("number", myForm.Number).
			Int("status", http.StatusOK).
			Msg(message)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, message)
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
			Str("username", myForm.Name).
			Int("number", myForm.Number).
			Int("status", http.StatusInternalServerError).
			Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err.Error())
		return
	}
	if bingoNumberOfTheDay == myForm.Number {
		message := "hooray " + myForm.Name + ", great job you guess the correct number!"
		log.Info().
			Str("hostname", r.Host).
			Str("method", r.Method).
			Str("proto", r.Proto).
			Str("remote_ip", r.RemoteAddr).
			Str("path", r.RequestURI).
			Str("user-agent", r.UserAgent()).
			Str("username", myForm.Name).
			Int("number", myForm.Number).
			Int("status", http.StatusOK).
			Msg(message)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, message)
		return
	}

	message := "sorry " + myForm.Name + ", you didn't guess the right number this time, but try again tomorrow!"
	log.Info().
		Str("hostname", r.Host).
		Str("method", r.Method).
		Str("proto", r.Proto).
		Str("remote_ip", r.RemoteAddr).
		Str("path", r.RequestURI).
		Str("user-agent", r.UserAgent()).
		Str("username", myForm.Name).
		Int("number", myForm.Number).
		Int("status", http.StatusOK).
		Msg(message)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, message)

}

func checkUserDailyQuota(pool *redis.Pool, name string) (bool, error) {
	conn := pool.Get()
	_, err := conn.Do("AUTH", os.Getenv("REDIS_PASSWORD"))
	if err != nil {
		log.Error().Msg("error during authentication")
		return false, err
	}
	defer conn.Close()

	exists, err := redis.Int(conn.Do("EXISTS", name))
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
		_, err = conn.Do("EXPIRE", name, 86400)
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
	_, err := conn.Do("AUTH", os.Getenv("REDIS_PASSWORD"))
	if err != nil {
		log.Error().Msg("error during authentication")
		return 0, err
	}
	defer conn.Close()

	exists, err := redis.Int(conn.Do("EXISTS", "bingoNumberOfTheDay"))
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
	number, err := redis.Int(conn.Do("GET", "bingoNumberOfTheDay"))
	if err != nil {
		log.Error().Msg("error getting bingoNumberOfTheDay")
		return 0, err
	}
	return number, nil
}

type bingoNumber struct {
	Name   string `json:"name"`
	Number int    `json:"number"`
}
