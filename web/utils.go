package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/dsoprea/goappenginesessioncascade"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/file"
	"google.golang.org/appengine/log"
	"gopkg.in/h2non/filetype.v1/types"
)

var (
	sessionName   = os.Getenv("SESSION_NAME")
	sessionSecret = []byte(os.Getenv("SESSION_SECRET"))
	sessionStore  = cascadestore.NewCascadeStore(cascadestore.DistributedBackends, sessionSecret)
)

// User is the base user
type User struct {
	Email    string
	Password string
}

// FirebaseConfig is the firebase configuration
type FirebaseConfig struct {
	APIKey            string
	AuthDomain        string
	DatabaseURL       string
	ProjectID         string
	StorageBucket     string
	MessagingSenderID string
	EditorURL         string
	IPAddress         string
}

// Link is the link location
type Link struct {
	URL          string
	Token        string
	Clicks       int
	Clickers     []string
	ShortURL     string
	CreateTime   time.Time
	ExpireTime   time.Time
	ExpireClicks int
}

// Upload is the data model for an upload
type Upload struct {
	Key          appengine.BlobKey
	Filename     string
	Token        string
	Clicks       int
	Clickers     []string
	ShortURL     string
	ContentType  types.Type
	CreateTime   time.Time
	ExpireTime   time.Time
	ExpireClicks int
}

func returnErr(w http.ResponseWriter, r *http.Request, err error, code int) {
	if code == 0 {
		code = http.StatusInternalServerError
	}

	http.Error(w, err.Error(), code)
	return
}

func returnJSON(w http.ResponseWriter, r *http.Request, data interface{}, status int) {
	if status == 0 {
		status = http.StatusOK
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "    ")

	encoder.Encode(data)
	return
}

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := appengine.NewContext(r)

		session, err := sessionStore.Get(r, sessionName)
		if err != nil {
			returnErr(w, r, err, 0)
			return
		}

		if vRaw, found := session.Values["loggedin"]; found == true {
			v := vRaw.(bool)

			if v {
				next.ServeHTTP(w, r)
				return
			}
		}

		token := r.Header.Get("X-Authorization")

		if len(token) == 0 {
			token = r.URL.Query().Get("authorization")
		}

		if len(token) > 0 {
			user := new(User)
			key := datastore.NewKey(ctx, "User", token, 0, nil)

			if err := datastore.Get(ctx, key, user); err != nil {
				res := make(map[string]interface{})

				res["status"] = false

				returnJSON(w, r, res, http.StatusUnauthorized)

				return
			}
		} else {
			user := new(User)
			key := datastore.NewKey(ctx, "User", "admin", 0, nil)

			if err := datastore.Get(ctx, key, user); err != nil {
				if err == datastore.ErrNoSuchEntity {
					token, err := bcrypt.GenerateFromPassword([]byte(os.Getenv("ADMIN_PASS")), 14)

					if err != nil {
						returnErr(w, r, err, 0)
						return
					}

					user.Email = os.Getenv("ADMIN_EMAIL")
					user.Password = string(token)

					if _, err := datastore.Put(ctx, key, user); err != nil {
						returnErr(w, r, err, 0)
						return
					}

					hash := sha256.New()
					hash.Write(token)

					newtoken := hex.EncodeToString(hash.Sum(nil))

					key := datastore.NewKey(ctx, "User", string(newtoken), 0, nil)

					if _, err := datastore.Put(ctx, key, user); err != nil {
						returnErr(w, r, err, 0)
						return
					}
				} else {
					returnErr(w, r, err, 0)
					return
				}
			} else {
				res := make(map[string]interface{})

				res["status"] = false

				returnJSON(w, r, res, http.StatusUnauthorized)

				return
			}
		}

		if !strings.Contains(strings.ToLower(r.UserAgent()), "wget") && !strings.Contains(strings.ToLower(r.UserAgent()), "curl") {
			session.Values["loggedin"] = true
			if err := session.Save(r, w); err != nil {
				returnErr(w, r, err, 0)
				return
			}

		}

		next.ServeHTTP(w, r)
	})
}

// RandStringBytesMaskImprSrc creates a random string of length n
func RandStringBytesMaskImprSrc(n int) string {
	const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const (
		letterIdxBits = 6                    // 6 bits to represent a letter index
		letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
		letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
	)
	var src = rand.NewSource(time.Now().UnixNano())

	b := make([]byte, n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			b[i] = letterBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return string(b)
}

func cleanupMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := appengine.NewContext(r)

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

		var links []*Link
		linkKeys, err := datastore.NewQuery("Link").GetAll(ctx, &links)
		if err != nil {
			log.Errorf(ctx, "failed to get links %v", err)
			return
		}

		for index, link := range links {
			if (!link.ExpireTime.IsZero() && time.Now().Unix() >= link.ExpireTime.Unix()) || (link.ExpireClicks != 0 && link.Clicks >= link.ExpireClicks) {
				if err := datastore.Delete(ctx, linkKeys[index]); err != nil {
					log.Errorf(ctx, "failed to delete link %v", err)
					return
				}
			}
		}

		var uploads []*Upload
		uploadKeys, err := datastore.NewQuery("Upload").GetAll(ctx, &uploads)
		if err != nil {
			log.Errorf(ctx, "failed to get uploads %v", err)
			return
		}

		for index, upload := range uploads {
			if (!upload.ExpireTime.IsZero() && time.Now().Unix() >= upload.ExpireTime.Unix()) || (upload.ExpireClicks != 0 && upload.Clicks >= upload.ExpireClicks) {
				if err := bucketHandle.Object(upload.Filename).Delete(ctx); err != nil {
					log.Errorf(ctx, "failed to delete upload file %v", err)
					return
				}

				if err := datastore.Delete(ctx, uploadKeys[index]); err != nil {
					log.Errorf(ctx, "failed to delete upload %v", err)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
