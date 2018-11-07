package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"cloud.google.com/go/storage"
	"github.com/gin-gonic/gin"
	"google.golang.org/api/iterator"
	"google.golang.org/appengine"
	"google.golang.org/appengine/blobstore"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/file"
	"google.golang.org/appengine/log"
	"gopkg.in/h2non/filetype.v1"
)

func upload(c *gin.Context) {
	ctx := appengine.NewContext(c.Request)

	switch c.Request.Method {
	case "GET":
		uploadGet(ctx, c)
		return
	case "PUT":
		fallthrough
	case "POST":
		uploadPost(ctx, c)
		return
	case "DELETE":
		uploadDelete(ctx, c)
		return
	}
}

func uploadGet(ctx context.Context, c *gin.Context) {
	var uploads []*Upload
	keys, err := datastore.NewQuery("Upload").GetAll(ctx, &uploads)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	res := make(map[string]interface{})

	res["status"] = true
	res["keys"] = keys
	res["uploads"] = uploads

	returnJSON(c, res, 0)

	return
}

func uploadPost(ctx context.Context, c *gin.Context) {
	token := RandStringBytesMaskImprSrc(6)

	bucket, err := file.DefaultBucketName(ctx)
	if err != nil {
		log.Errorf(ctx, "failed to get default GCS bucket name: %v", err)
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		log.Errorf(ctx, "failed to create client: %v", err)
		return
	}
	defer client.Close()

	bucketHandle := client.Bucket(bucket)

	filename := ""

	var uploadFile io.Reader
	if c.Request.Method == "POST" {
		uploadedFile, err := c.FormFile("uploadfile")
		uploadFile, err = uploadedFile.Open()
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		filename = uploadedFile.Filename
	} else {
		uploadFile = c.Request.Body
		filename = c.Param("filename")

		if filename == "" {
			filename = c.Request.URL.Path[1:]
		}
	}

	storedFile := filename

	it := bucketHandle.Objects(ctx, &storage.Query{
		Prefix: filename,
	})

	exists := 0
	for {
		_, err := it.Next()
		if err == iterator.Done {
			break
		} else {
			exists++
		}
	}

	if exists > 0 {
		storedFile += fmt.Sprintf(".%d", exists)
	}

	wrt := bucketHandle.Object(storedFile).NewWriter(ctx)

	_, err = io.Copy(wrt, uploadFile)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	err = wrt.Close()
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	blobKey, err := blobstore.BlobKeyForFile(ctx, "/gs/"+bucket+"/"+storedFile)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	url := c.GetHeader("X-Forwarded-Proto") + "://" + c.Request.Host + "/u/" + token + "/" + filename

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

	uploaded := new(Upload)

	uploaded.Key = blobKey
	uploaded.Clicks = 0
	uploaded.Clickers = make([]string, 0)
	uploaded.Token = token
	uploaded.Filename = storedFile
	uploaded.ShortURL = url
	uploaded.CreateTime = time.Now()
	uploaded.ExpireClicks = expireClicksInt
	uploaded.ExpireTime = expireTimeTime

	uploadedType, err := filetype.MatchReader(uploadFile)
	if err != nil {
		log.Errorf(ctx, "failed to get content-type: %v", err)
	}

	uploaded.ContentType = uploadedType

	key := datastore.NewKey(ctx, "Upload", token, 0, nil)

	if _, err := datastore.Put(ctx, key, uploaded); err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
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
		res["upload"] = uploaded
		res["bucket"] = bucket

		returnJSON(c, res, 0)
	}

	return
}

func uploadDelete(ctx context.Context, c *gin.Context) {
	upload := new(Upload)

	key := datastore.NewKey(ctx, "Upload", c.QueryArray("token")[0], 0, nil)

	if err := datastore.Get(ctx, key, upload); err != nil {
		if err == datastore.ErrNoSuchEntity {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		returnErr(c, err, 0)
		return
	}

	bucket, err := file.DefaultBucketName(ctx)
	if err != nil {
		returnErr(c, err, 0)
		return
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		returnErr(c, err, 0)
		return
	}
	defer client.Close()

	bucketHandle := client.Bucket(bucket)

	if err := bucketHandle.Object(upload.Filename).Delete(ctx); err != nil {
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
