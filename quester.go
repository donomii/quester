package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	//"github.com/gin-gonic/autotls"

	"github.com/boltdb/bolt"
	"github.com/donomii/goof"
	"github.com/gin-gonic/gin"
)

var safe bool = false
var bdb *uniStore

func sessionTokenToId(sessionToken string) string {
	id := "-1"
	return string(id)
}

func makeAuthed(handlerFunc func(*gin.Context, string, string)) func(c *gin.Context) {
	return func(c *gin.Context) {
		sessionToken := c.Query("id")
		log.Printf("Got token: '%v'", sessionToken)
		//id := sessionTokenToId(sessionToken)
		//log.Printf("Got real user id: '%v'", id)
		handlerFunc(c, "-1", sessionToken)
	}

}

func summary(c *gin.Context, id string, token string) {

	c.Writer.Write([]byte(`
<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>jsTree test</title>
  <!-- 2 load the theme CSS file --><link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/jstree/3.2.1/themes/default/style.min.css" />
<script>
window.addEventListener( "pageshow", function ( event ) {
  var historyTraversal = event.persisted || 
                         ( typeof window.performance != "undefined" && 
                              window.performance.navigation.type === 2 );
  if ( historyTraversal ) {
    // Handle page restore.
    window.location.reload();
  }
});
</script>
</head>
<body>
   ` + nodeDisplay("nodes", false) + `
 
  <!-- 4 include the jQuery library -->
  <script src="https://cdnjs.cloudflare.com/ajax/libs/jquery/1.12.1/jquery.min.js"></script>
  <!-- 5 include the minified jstree source -->
  <script src="https://cdnjs.cloudflare.com/ajax/libs/jstree/3.2.1/jstree.min.js"></script>
  <script>
  $(function () {
    // 6 create an instance when the DOM is ready
    $('#jstree').jstree();
    // 7 bind to events triggered on the tree
    $('#jstree').on("changed.jstree", function (e, data) {
      console.log(data.selected);
    });
	
    // 8 interact with the tree - either way is OK
    $('button').on('click', function () {
      $('#jstree').jstree(true).select_node('child_node_1');
      $('#jstree').jstree('select_node', 'child_node_1');
      $.jstree.reference('#jstree').select_node('child_node_1');
    });
  });
  </script>
</body>
</html>
`))
}

func detailed(c *gin.Context, id string, token string) {
	q := c.Query("q")
	c.Writer.Write([]byte(`
<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>jsTree test</title>
  <!-- 2 load the theme CSS file --><link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/jstree/3.2.1/themes/default/style.min.css" />
<script>
window.addEventListener( "pageshow", function ( event ) {
  var historyTraversal = event.persisted || 
                         ( typeof window.performance != "undefined" && 
                              window.performance.navigation.type === 2 );
  if ( historyTraversal ) {
    // Handle page restore.
    window.location.reload();
  }
});
</script>
</head>
<body>
   ` + nodeDisplay(q, true) + `
</body>
</html>
`))
}

func addWaypoint(c *gin.Context, id string, token string) {
	title := c.PostForm("title")
	content := c.PostForm("content")
	quest := c.PostForm("q")
	path := quest + "/" + title
	fmt.Println("Adding waypoint", path)
	if safe {
		bdb.Put([]byte("quests"), []byte(path), []byte(content))
	} else {
		ioutil.WriteFile(path, []byte(content), 0644)
	}
	summary(c, id, token)
}

func addQuest(c *gin.Context, id string, token string) {

	title := c.PostForm("title")
	//content := req.FormValue("content")
	quest := c.PostForm("q")
	path := quest + "/" + title
	fmt.Println(path)
	if safe {
		bdb.Put([]byte("quests"), []byte(path), []byte(""))
		bdb.Put([]byte("directories"), []byte(path), []byte("directory"))
	} else {
		os.Mkdir(path, 0700)
	}
	summary(c, token, id)
}

// ReadDir reads the directory named by dirname and returns

// a list of directory entries sorted by filename.

func ReadDir(dirname string) ([][]byte, error) {
	var out [][]byte
	if safe {
		files := bdb.List([]byte("quests"))
		for _, v := range files {
			if bytes.HasPrefix(v, []byte(dirname)) {
				out = append(out, bytes.Replace(v, []byte(dirname+"/"), []byte(""), 1))
			}
		}
	} else {

		f, err := os.Open(dirname)

		if err != nil {

			return out, err

		}

		list, err := f.Readdir(-1)

		f.Close()

		if err != nil {

			return out, err

		}

		for _, v := range list {
			out = append(out, []byte(v.Name()))
		}
	}
	return out, nil
}

func isPathChecked(path string) string {
	var out string
	metapath := "metadata/" + path + ".checked"
	fmt.Println("Checking", metapath)
	if safe {
		if bdb.Exists([]byte("quests"), []byte(metapath)) {
			out = `checked="checked"`
		}
	} else {
		if goof.Exists(metapath) {
			out = `checked="checked"`
		}
	}
	return out
}

func forceTrailingSlash(path string) string {
	if strings.HasSuffix(path, "/") {
		return path
	} else {
		return path + "/"
	}
}

func myIsDir(path string) bool {
	if safe {
		return bdb.Exists([]byte("directories"), []byte(path))
	} else {
		return goof.IsDir(path)
	}
	return false
}

func loadNodes(path string, detailed bool) string {
	out := ""
	if myIsDir(path) {
		fmt.Println(path, "is directory")
		out = out + fmt.Sprintf("<li><input type=\"checkbox\" "+isPathChecked(path)+" onclick=\"$.get('toggle?path=%s')\"><a href=\"detailed?q=%s\">", path, path) + filepath.Base(path) + "</a><ul>"
		files, err := ReadDir(path)
		if err != nil {
			log.Fatal(err)
		}

		for _, f := range files {
			out = out + loadNodes(path+"/"+string(f), detailed)
		}
		out = out + "</ul></li>"
	} else {
		fmt.Println(path, "is file")
		var contents []byte
		if safe {
			contents, _ = bdb.Get([]byte("quests"), []byte(path))
		} else {
			contents, _ = ioutil.ReadFile(path)
		}
		if detailed {
			out = out + "<li><input type=\"checkbox\"  " + isPathChecked(path) + " onclick=\"$.get('toggle?path=" + path + "')\">" + filepath.Base(path) + "<p style=\"margin-left: 10em\">" + string(contents) + "</p>" + "</li>"
		} else {
			out = out + "<li><input type=\"checkbox\"  " + isPathChecked(path) + " onclick=\"$.get('toggle?path=" + path + "')\">" + filepath.Base(path) + "</li>"
		}
	}
	return out
}
func nodeDisplay(path string, detailed bool) string {
	return loadNodes(path, detailed) + `<form action="addQuest"><input type="hidden" id="q" name="q" value="` + path + `"><input id="title" name="title" type="text"><input type="submit" value="Add Quest"></form>` + `<form action="addWaypoint"><input type="hidden" id="q" name="q" value="` + path + `"><input id="title" name="title" type="text"><input id="content" name="content" type="text"><input type="submit" value="Add"></form>`
}

func toggle(c *gin.Context, id string, token string) {
	paths := c.Query("path")
	path := `metadata/` + string(paths[0]) + `.checked`
	fmt.Println(path)
	if safe {
		if bdb.Exists([]byte("quests"), []byte(path)) {
			bdb.Delete([]byte("quests"), []byte(path))
		} else {
			bdb.Put([]byte("quests"), []byte(path), []byte(""))
		}
	} else {
		if goof.Exists(path) {
			os.Remove(path)
		} else {
			os.MkdirAll(path, 0700)
		}
	}
}

func main() {
	if safe {
		bdb, _ = newUniStore("quests.db")
		bdb.Put([]byte("quests"), []byte("1"), []byte("1"))
		bdb.Put([]byte("directories"), []byte("nodes"), []byte("directory"))
	} else {
		os.Mkdir("nodes", 0700)
	}
	router := gin.Default()
	serveQuester(router, "/quester/")
	//log.Fatal(autotls.Run(router, "localhost", "localhost"))
	router.Run()
}

func serveQuester(router *gin.Engine, prefix string) {

	//Nocache is probably useless done this way, the server has already procesed the headers
	router.GET(prefix+"summary", makeAuthed(summary))
	router.GET(prefix+"detailed", makeAuthed(detailed))
	router.POST(prefix+"addWaypoint", makeAuthed(addWaypoint))
	router.POST(prefix+"addQuest", makeAuthed(addQuest))
	router.POST(prefix+"toggle", makeAuthed(toggle))
}

//Force nocache
var epoch = time.Unix(0, 0).Format(time.RFC1123)

var noCacheHeaders = map[string]string{
	"Expires":         epoch,
	"Cache-Control":   "no-cache, private, max-age=0",
	"Pragma":          "no-cache",
	"X-Accel-Expires": "0",
}

var etagHeaders = []string{
	"ETag",
	"If-Modified-Since",
	"If-Match",
	"If-None-Match",
	"If-Range",
	"If-Unmodified-Since",
}

func NoCache(f func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	fn := func(w http.ResponseWriter, r *http.Request) {
		// Delete any ETag headers that may have been set
		for _, v := range etagHeaders {
			if r.Header.Get(v) != "" {
				r.Header.Del(v)
			}
		}

		// Set our NoCache headers
		for k, v := range noCacheHeaders {
			w.Header().Set(k, v)
		}

		f(w, r)
	}

	return fn
}

type uniStore struct {
	db *bolt.DB
}

func newUniStore(filename string) (s *uniStore, err error) {
	s = &uniStore{}
	s.db, err = bolt.Open(filename, 0600, &bolt.Options{Timeout: 1 * time.Second})
	return
}

func (s *uniStore) Exists(bucket, key []byte) bool {

	var v []byte
	v = nil
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return nil
		}
		v = b.Get([]byte(key))
		return nil
	})
	if v == nil {
		return false
	} else {
		return true
	}
	return false
}
func (s *uniStore) Put(bucket, key []byte, val []byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			panic(err)
		}
		b = tx.Bucket([]byte(bucket))
		if err = b.Put([]byte(key), val); err != nil {
			log.Printf("%v", err)
			panic(err)
		}
		//log.Printf("Wrote %v:%v to %v", key, string(val), bucket)
		return nil
	})
}

func (s *uniStore) Delete(bucket, key []byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			panic(err)
		}
		b = tx.Bucket([]byte(bucket))
		if err = b.Delete([]byte(key)); err != nil {
			log.Printf("%v", err)
			panic(err)
		}
		//log.Printf("Wrote %v:%v to %v", key, string(val), bucket)
		return nil
	})
}

func (s *uniStore) Get(bucket, key []byte) (data []byte, err error) {
	err = errors.New("Id '" + string(key) + "' not found!")
	s.db.View(func(tx *bolt.Tx) error {
		bb := tx.Bucket([]byte(bucket))
		r := bb.Get([]byte(key))
		if r != nil && len(r) > 10 {
			data = make([]byte, len(r))
			copy(data, r)
			err = nil
		}
		return nil
	})
	return
}

func (s *uniStore) List(bucket []byte) [][]byte {
	var out [][]byte
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		// Iterate over items in sorted key order.
		if err := b.ForEach(func(k, v []byte) error {
			out = append(out, k)
			return nil
		}); err != nil {
			return err
		}
		return nil
	})
	return out
}
