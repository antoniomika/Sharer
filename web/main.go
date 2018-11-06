package main

import (
	"net/http"

	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"google.golang.org/appengine"
	"google.golang.org/appengine/blobstore"
	"google.golang.org/appengine/datastore"
)

var (
	firebaseConfig = FirebaseConfig{
		APIKey:            os.Getenv("FIREBASE_APIKEY"),
		AuthDomain:        os.Getenv("FIREBASE_AUTHDOMAIN"),
		DatabaseURL:       os.Getenv("FIREBASE_DATABASEURL"),
		ProjectID:         os.Getenv("FIREBASE_PROJECTID"),
		StorageBucket:     os.Getenv("FIREBASE_STORAGEBUCKET"),
		MessagingSenderID: os.Getenv("FIREBASE_MESSAGINGSENDERID"),
		EditorURL:         os.Getenv("EDITOR_HOSTNAME"),
		IPAddress:         "",
	}
)

func main() {
	r := gin.Default()
	r.LoadHTMLGlob("templates/*")
	r.Use(cleanupMiddleware)

	r.GET("/", handleIndex)
	r.GET("/e", handleEdit)
	r.GET("/admin", handleAdmin)

	r.GET("/s/:id", loadData)
	r.GET("/u/:id", loadData)
	r.GET("/u/:id/:filename", loadData)

	apiGroup := r.Group("/api", authMiddleware)
	{
		apiGroup.Any("/shorten", shorten)
		apiGroup.Any("/upload", upload)
		apiGroup.Any("/upload/:filename", upload)
	}

	r.Run(":" + os.Getenv("PORT"))
}

func handleIndex(c *gin.Context) {
	if c.Request.Host == os.Getenv("EDITOR_HOSTNAME") {
		handleEdit(c)
		return
	}

	c.Redirect(http.StatusFound, os.Getenv("REDIRECT_MAIN"))
}

func handleEdit(c *gin.Context) {
	firebaseConfig.IPAddress = c.Request.RemoteAddr
	c.HTML(http.StatusOK, "edit.html", firebaseConfig)
}

func handleAdmin(c *gin.Context) {
	c.HTML(http.StatusOK, "admin.html", firebaseConfig)
}

func loadData(c *gin.Context) {
	ctx := appengine.NewContext(c.Request)
	kind := ""
	var ent interface{}

	if strings.HasPrefix(c.Request.URL.Path, "/s/") {
		kind = "Link"
		ent = new(Link)
	} else if strings.HasPrefix(c.Request.URL.Path, "/u/") {
		kind = "Upload"
		ent = new(Upload)
	}

	id := ""

	stringArr := strings.Split(c.Param("id"), ".")

	id = stringArr[0]

	key := datastore.NewKey(ctx, kind, id, 0, nil)

	if err := datastore.Get(ctx, key, ent); err != nil {
		if err == datastore.ErrNoSuchEntity {
			c.Redirect(http.StatusFound, "/")
			return
		}
		returnErr(c, err, 0)
		return
	}

	link, ok := ent.(*Link)

	if ok {
		newLink := link
		newLink.Clicks++
		newLink.Clickers = append(newLink.Clickers, c.Request.RemoteAddr)

		if _, err := datastore.Put(ctx, key, newLink); err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
		}

		c.Redirect(http.StatusFound, link.URL)
		return
	}

	upload, _ := ent.(*Upload)
	uploadKey := upload.Key

	newUpload := upload
	newUpload.Clicks++
	newUpload.Clickers = append(newUpload.Clickers, c.Request.RemoteAddr)

	if _, err := datastore.Put(ctx, key, newUpload); err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
	}

	blobstore.Send(c.Writer, uploadKey)
	return
}
