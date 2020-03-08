package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/donomii/goof"
)

func summary(w http.ResponseWriter, req *http.Request) {

	fmt.Fprintf(w, `
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
   `+nodeDisplay("nodes", false)+`
 
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
`)
}

func detailed(w http.ResponseWriter, req *http.Request) {
	q, _ := req.URL.Query()["q"]
	fmt.Fprintf(w, `
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
   `+nodeDisplay(q[0], true)+`
</body>
</html>
`)
}

func addWaypoint(w http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	title := req.FormValue("title")
	content := req.FormValue("content")
	quest := req.FormValue("q")
	path := quest + "/" + title
	fmt.Println(path)
	ioutil.WriteFile(path, []byte(content), 0644)
}

func addQuest(w http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	title := req.FormValue("title")
	//content := req.FormValue("content")
	quest := req.FormValue("q")
	path := quest + "/" + title
	fmt.Println(path)
	os.Mkdir(path, 0700)
}

func headers(w http.ResponseWriter, req *http.Request) {

	for name, headers := range req.Header {
		for _, h := range headers {
			fmt.Fprintf(w, "%v: %v\n", name, h)
		}
	}
}

// ReadDir reads the directory named by dirname and returns

// a list of directory entries sorted by filename.

func ReadDir(dirname string) ([]os.FileInfo, error) {

	f, err := os.Open(dirname)

	if err != nil {

		return nil, err

	}

	list, err := f.Readdir(-1)

	f.Close()

	if err != nil {

		return nil, err

	}

	return list, nil

}

func loadNodes(path string, detailed bool) string {
	out := ""
	if goof.IsDir(path) {
		fmt.Println(path, "is directory")
		out = out + fmt.Sprintf("<li><a href=\"detailed?q=%s\">", path) + filepath.Base(path) + "<ul>"

		files, err := ReadDir(path)
		if err != nil {
			log.Fatal(err)
		}

		for _, f := range files {
			out = out + loadNodes(path+"/"+f.Name(), detailed)
		}
		out = out + "</ul></a></li>"
	} else {
		fmt.Println(path, "is file")
		contents, _ := ioutil.ReadFile(path)
		if detailed {
			out = out + "<li>" + filepath.Base(path) + "<p style=\"margin-left: 10em\">" + string(contents) + "</p>" + "</li>"
		} else {
			out = out + "<li>" + filepath.Base(path) + "</li>"
		}
	}
	return out
}
func nodeDisplay(path string, detailed bool) string {
	return loadNodes(path, detailed) + `<form action="/addQuest"><input type="hidden" id="q" name="q" value="` + path + `"><input id="title" name="title" type="text"><input type="submit" value="Add Quest"></form>` + `<form action="/addWaypoint"><input type="hidden" id="q" name="q" value="` + path + `"><input id="title" name="title" type="text"><input id="content" name="content" type="text"><input type="submit" value="Add"></form>`
}

func main() {
	os.Mkdir("nodes", 0700)
	//Nocache is probably useless done this way, the server has already procesed the headers
	http.HandleFunc("/summary", NoCache(summary))
	http.HandleFunc("/detailed", NoCache(detailed))
	http.HandleFunc("/addWaypoint", NoCache(addWaypoint))
	http.HandleFunc("/addQuest", NoCache(addQuest))
	http.HandleFunc("/headers", NoCache(headers))

	http.ListenAndServe(":8090", nil)
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
