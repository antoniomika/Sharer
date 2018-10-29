package main

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
)

func shorten(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	switch r.Method {
	case "GET":
		shortenGet(ctx, w, r)
		return
	case "POST":
		shortenPost(ctx, w, r)
		return
	case "DELETE":
		shortenDelete(ctx, w, r)
		return
	case "PUT":
	}
}

func shortenGet(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	var links []*Link
	keys, err := datastore.NewQuery("Link").GetAll(ctx, &links)
	if err != nil {
		returnErr(w, r, err, 0)
		return
	}

	res := make(map[string]interface{})

	res["status"] = true
	res["keys"] = keys
	res["links"] = links

	returnJSON(w, r, res, 0)

	return
}

func shortenPost(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	token := RandStringBytesMaskImprSrc(6)
	query := r.URL.Query()

	url := r.URL.Scheme + "://" + r.URL.Host + "/s/" + token

	expireClicks := query.Get("clicks")
	if expireClicks == "" {
		expireClicks = "0"
	}

	expireClicksInt, err := strconv.Atoi(expireClicks)
	if err != nil {
		log.Errorf(ctx, "failed to get convert int: %v", err)
	}

	expireTime := query.Get("time")

	duration, err := time.ParseDuration(expireTime)
	if err != nil {
		log.Errorf(ctx, "failed to parse duration: %v", err)
	}

	var expireTimeTime time.Time
	if duration != 0 {
		expireTimeTime = time.Now().Add(duration)
	}

	link := new(Link)

	link.Token = token
	link.URL = r.URL.Query()["url"][0]
	link.Clicks = 0
	link.Clickers = make([]string, 0)
	link.ShortURL = url
	link.CreateTime = time.Now()
	link.ExpireClicks = expireClicksInt
	link.ExpireTime = expireTimeTime

	key := datastore.NewKey(ctx, "Link", token, 0, nil)

	if _, err := datastore.Put(ctx, key, link); err != nil {
		returnErr(w, r, err, 0)
		return
	}

	if query.Get("s") != "" {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(url))
	} else {
		res := make(map[string]interface{})

		res["status"] = true
		res["token"] = token
		res["url"] = url
		res["link"] = link

		returnJSON(w, r, res, 0)
	}

	return
}

func shortenDelete(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	link := new(Link)

	key := datastore.NewKey(ctx, "Link", r.URL.Query()["token"][0], 0, nil)

	if err := datastore.Get(ctx, key, link); err != nil {
		if err == datastore.ErrNoSuchEntity {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		returnErr(w, r, err, 0)
		return
	}

	if err := datastore.Delete(ctx, key); err != nil {
		returnErr(w, r, err, 0)
		return
	}

	res := make(map[string]interface{})

	res["status"] = true

	returnJSON(w, r, res, 0)

	return
}
