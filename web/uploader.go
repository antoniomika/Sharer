package main

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"time"

	"cloud.google.com/go/storage"
	"github.com/gorilla/mux"
	"google.golang.org/appengine"
	"google.golang.org/appengine/blobstore"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/file"
	"google.golang.org/appengine/log"
	"gopkg.in/h2non/filetype.v1"
)

func upload(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	switch r.Method {
	case "GET":
		uploadGet(ctx, w, r)
		return
	case "PUT":
		fallthrough
	case "POST":
		uploadPost(ctx, w, r)
		return
	case "DELETE":
		uploadDelete(ctx, w, r)
		return
	}
}

func uploadGet(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	var uploads []*Upload
	keys, err := datastore.NewQuery("Upload").GetAll(ctx, &uploads)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	res := make(map[string]interface{})

	res["status"] = true
	res["keys"] = keys
	res["uploads"] = uploads

	returnJSON(w, r, res, 0)

	return
}

func uploadPost(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	query := r.URL.Query()
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
	if r.Method == "POST" {
		r.ParseMultipartForm(32 << 20)
		uploadedFile, handler, err := r.FormFile("uploadfile")
		uploadFile = uploadedFile
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer uploadedFile.Close()

		filename = handler.Filename
	} else {
		uploadFile = r.Body
		filename = vars["filename"]
	}

	wrt := bucketHandle.Object(filename).NewWriter(ctx)

	_, err = io.Copy(wrt, uploadFile)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	err = wrt.Close()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	blobKey, err := blobstore.BlobKeyForFile(ctx, "/gs/"+bucket+"/"+filename)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	url := r.URL.Scheme + "://" + r.URL.Host + "/u/" + token + "/" + filename

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

	uploaded := new(Upload)

	uploaded.Key = blobKey
	uploaded.Clicks = 0
	uploaded.Clickers = make([]string, 0)
	uploaded.Token = token
	uploaded.Filename = filename
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
		http.Error(w, err.Error(), 500)
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
		res["upload"] = uploaded
		res["bucket"] = bucket

		returnJSON(w, r, res, 0)
	}

	return
}

func uploadDelete(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	upload := new(Upload)

	key := datastore.NewKey(ctx, "Upload", r.URL.Query()["token"][0], 0, nil)

	if err := datastore.Get(ctx, key, upload); err != nil {
		if err == datastore.ErrNoSuchEntity {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		returnErr(w, r, err, 0)
		return
	}

	bucket, err := file.DefaultBucketName(ctx)
	if err != nil {
		returnErr(w, r, err, 0)
		return
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		returnErr(w, r, err, 0)
		return
	}
	defer client.Close()

	bucketHandle := client.Bucket(bucket)

	if err := bucketHandle.Object(upload.Filename).Delete(ctx); err != nil {
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
