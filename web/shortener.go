package main

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
)

func shorten(c *gin.Context) {
	ctx := appengine.NewContext(c.Request)

	switch c.Request.Method {
	case "GET":
		shortenGet(ctx, c)
		return
	case "POST":
		shortenPost(ctx, c)
		return
	case "DELETE":
		shortenDelete(ctx, c)
		return
	case "PUT":
	}
}

func shortenGet(ctx context.Context, c *gin.Context) {
	var links []*Link
	keys, err := datastore.NewQuery("Link").GetAll(ctx, &links)
	if err != nil {
		returnErr(c, err, 0)
		return
	}

	res := make(map[string]interface{})

	res["status"] = true
	res["keys"] = keys
	res["links"] = links

	returnJSON(c, res, 0)

	return
}

func shortenPost(ctx context.Context, c *gin.Context) {
	token := RandStringBytesMaskImprSrc(6)

	url := c.GetHeader("X-Forwarded-Proto") + "://" + c.Request.Host + "/s/" + token

	expireClicks := c.Query("clicks")
	if expireClicks == "" {
		expireClicks = "0"
	}

	expireClicksInt, err := strconv.Atoi(expireClicks)
	if err != nil {
		log.Errorf(ctx, "failed to get convert int: %v", err)
	}

	expireTime := c.Query("time")

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
	link.URL = c.QueryArray("url")[0]
	link.Clicks = 0
	link.Clickers = make([]string, 0)
	link.ShortURL = url
	link.CreateTime = time.Now()
	link.ExpireClicks = expireClicksInt
	link.ExpireTime = expireTimeTime

	key := datastore.NewKey(ctx, "Link", token, 0, nil)

	if _, err := datastore.Put(ctx, key, link); err != nil {
		returnErr(c, err, 0)
		return
	}

	if c.Query("s") != "" {
		c.Header("Content-Type", "text/plain")
		c.Writer.WriteHeader(http.StatusOK)
		c.Writer.Write([]byte(url))
	} else {
		res := make(map[string]interface{})

		res["status"] = true
		res["token"] = token
		res["url"] = url
		res["link"] = link

		returnJSON(c, res, 0)
	}

	return
}

func shortenDelete(ctx context.Context, c *gin.Context) {
	link := new(Link)

	key := datastore.NewKey(ctx, "Link", c.QueryArray("token")[0], 0, nil)

	if err := datastore.Get(ctx, key, link); err != nil {
		if err == datastore.ErrNoSuchEntity {
			c.Redirect(http.StatusFound, "/")
			return
		}

		returnErr(c, err, 0)
		return
	}

	if err := datastore.Delete(ctx, key); err != nil {
		returnErr(c, err, 0)
		return
	}

	res := make(map[string]interface{})

	res["status"] = true

	returnJSON(c, res, 0)

	return
}
