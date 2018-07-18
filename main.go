package main

import (
	"net/http"
	"html/template"

	"github.com/gorilla/mux"
	"google.golang.org/appengine"
	"google.golang.org/appengine/blobstore"
	"google.golang.org/appengine/datastore"
	"os"
	"strings"
)

var (
	templ = template.Must(template.ParseFiles("templates/edit.html"))
	firebaseConfig = FirebaseConfig{
		ApiKey: os.Getenv("FIREBASE_APIKEY"),
		AuthDomain: os.Getenv("FIREBASE_AUTHDOMAIN"),
		DatabaseURL: os.Getenv("FIREBASE_DATABASEURL"),
		ProjectId: os.Getenv("FIREBASE_PROJECTID"),
		StorageBucket: os.Getenv("FIREBASE_STORAGEBUCKET"),
		MessagingSenderId: os.Getenv("FIREBASE_MESSAGINGSENDERID"),
	}
)

func main() {
	router := mux.NewRouter()
	router.Use(cleanupMiddleware)

	apiRouter := router.PathPrefix("/api").Subrouter()

	apiRouter.Use(authMiddleware)

	router.HandleFunc("/", handleIndex).Methods("GET")
	router.HandleFunc("/e", handleEdit).Methods("GET")

	router.HandleFunc("/s/{id}", loadData).Methods("GET")
	router.HandleFunc("/u/{id}", loadData).Methods("GET")
	router.HandleFunc("/u/{id}/{filename}", loadData).Methods("GET")

	apiRouter.HandleFunc("/shorten", shorten).Methods("GET", "POST", "PUT", "DELETE")
	apiRouter.HandleFunc("/upload", upload).Methods("GET", "POST", "PUT", "DELETE")
	apiRouter.HandleFunc("/upload/{filename}", upload).Methods("GET", "POST", "PUT", "DELETE")

	http.Handle("/", router)
	appengine.Main()
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, os.Getenv("REDIRECT_MAIN"), http.StatusFound)
}

func handleEdit(w http.ResponseWriter, r *http.Request) {
	templ.Execute(w, firebaseConfig)
}

func loadData(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ctx := appengine.NewContext(r)
	kind := ""
	var ent interface{}

	if strings.HasPrefix(r.URL.Path, "/s/") {
		kind = "Link"
		ent = new(Link)
	} else if strings.HasPrefix(r.URL.Path, "/u/") {
		kind = "Upload"
		ent = new(Upload)
	}

	id := ""

	stringArr := strings.Split(vars["id"], ".")

	id = stringArr[0]

	key := datastore.NewKey(ctx, kind, id, 0, nil)

	if err := datastore.Get(ctx, key, ent); err != nil {
		if err == datastore.ErrNoSuchEntity {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		} else {
			returnErr(w, r, err, 0)
			return
		}
	} else {
		link, ok := ent.(*Link)

		if ok {
			newLink := link
			newLink.Clicks++
			newLink.Clickers = append(newLink.Clickers, r.RemoteAddr)

			if _, err := datastore.Put(ctx, key, newLink); err != nil {
				http.Error(w, err.Error(), 500)
			}

			http.Redirect(w, r, link.Url, http.StatusFound)
			return
		} else {
			upload, _ := ent.(*Upload)
			uploadKey := upload.Key

			newUpload := upload
			newUpload.Clicks++
			newUpload.Clickers = append(newUpload.Clickers, r.RemoteAddr)

			if _, err := datastore.Put(ctx, key, newUpload); err != nil {
				http.Error(w, err.Error(), 500)
			}

			blobstore.Send(w, uploadKey)
			return
		}
	}
}
