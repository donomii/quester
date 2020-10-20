package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	//"github.com/gin-gonic/autotls"

	"github.com/gin-gonic/gin"
)

var safe bool = false

type Task struct {
	Name      string
	Text      string
	TimeStamp time.Time
	Checked   bool
	SubTasks  []*Task
}

func LoadJson(id string) *Task {
	var out *Task
	res, err := ioutil.ReadFile(fmt.Sprintf("quester/%v.json", id))
	err = json.Unmarshal(res, &out)
	if err != nil {
		log.Println("Could not load quests", err)
		//panic(err)
	}
	if out == nil {
		t := Task{Name: "Quester", Text: "Quest style task tracking"}
		out = &t
	}
	return out
}

func SaveJson(id string, tasks *Task) {
	payload, err := json.Marshal(tasks)
	if err != nil {
		panic("Could not marshall quests")
	}
	ioutil.WriteFile(fmt.Sprintf("quester/%v.json", id), payload, 0600)
}

func makeAuthed(handlerFunc func(*gin.Context, string, string)) func(c *gin.Context) {
	return func(c *gin.Context) {
		id := c.Request.Header.Get("authentigate-id")
		baseUrl := c.Request.Header.Get("authentigate-base-url")
		if id == "" {
			id = "personalusermode"
		}
		log.Printf("Got real user id: '%v'", id)
		handlerFunc(c, id, baseUrl)
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
   ` + taskDisplay(id, "nodes", false) + `
 
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
  <!-- 4 include the jQuery library -->
  <script src="https://cdnjs.cloudflare.com/ajax/libs/jquery/1.12.1/jquery.min.js"></script>
  <!-- 5 include the minified jstree source -->
  <script src="https://cdnjs.cloudflare.com/ajax/libs/jstree/3.2.1/jstree.min.js"></script>
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
   ` + taskDisplay(id, q, true) + `
</body>
</html>
`))
}

func addWaypoint(c *gin.Context, id string, token string) {
	title := c.PostForm("title")
	content := c.PostForm("content")
	quest := c.PostForm("q")
	path := quest + "/" + title

	topNode := LoadJson(id)
	t := FindTask(quest, topNode)
	existing := FindTask(path, topNode)
	if existing == nil {
		log.Println("Adding waypoint", path)
		newTask := Task{Name: title, Text: content}
		t.SubTasks = append(t.SubTasks, &newTask)
		SaveJson(id, topNode)
	} else {
		log.Println("Waypoint exists, not adding", path)
	}

	summary(c, id, token)
}

func FindTask(path string, task *Task) *Task {

	paths := strings.Split(path, "/")
	if paths[0] == "" {
		return task
	}
	if paths[0] == "nodes" {
		return FindTask(strings.Join(paths[1:], "/"), task)
	}
	for _, t := range task.SubTasks {
		log.Println("Comparing", t.Name, "to '", paths[0], "'")
		if t.Name == paths[0] {
			return FindTask(strings.Join(paths[1:], "/"), t)
		}

	}
	return nil
}
func isTaskChecked(task *Task) string {
	var out string
	if task.Checked {
		out = `checked="checked"`
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

func loadTasks(id, path string, task *Task, detailed bool) string {
	out := ""
	log.Println("Loading tasks for", path)
	//if task == nil Do string to task
	if task == nil {
		task = FindTask(path, LoadJson(id))
	}
	if task == nil {
		return ""
	}
	if len(task.SubTasks) > 0 {
		fmt.Println(path, "is a container task")
		out = out + fmt.Sprintf("<li><input type=\"checkbox\" "+isTaskChecked(task)+" onclick=\"$.get('toggle?path=%s')\"><a href=\"detailed?q=%s\">", path, path) + task.Name + "</a><ul>"
		tasks := task.SubTasks

		for _, f := range tasks {
			log.Println("Loading task", f.Name)
			out = out + loadTasks(id, path+"/"+f.Name, f, detailed)
		}
		out = out + "</ul></li>"
	} else {
		fmt.Println(path, "is leaf task")
		var contents = task.Text

		if detailed {
			out = out + "<li><input type=\"checkbox\"  " + isTaskChecked(task) + " onclick=\"$.get('toggle?path=" + path + "')\">" + task.Name + " <a href=\"detailed?q=" + path + "\">+</a><p style=\"margin-left: 10em\">" + string(contents) + "</p>" + "</li>"
		} else {
			out = out + "<li><input type=\"checkbox\"  " + isTaskChecked(task) + " onclick=\"$.get('toggle?path=" + path + "')\">" + task.Name + " <a href=\"detailed?q=" + path + "\">+</a></li>"
		}
	}
	return out
}

func taskDisplay(id, path string, detailed bool) string {
	return loadTasks(id, path, nil, detailed) + `<form action="addWaypoint"  ><input type="hidden" id="q" name="q" value="` + path + `"><input id="title" name="title" type="text"><input id="content" name="content" type="text"><input type="submit" formmethod="post" value="Add"></form>`
}

func toggle(c *gin.Context, id string, token string) {
	upath := c.Query("path")
	path := `metadata/` + string(upath) + `.checked`
	fmt.Println("Toggling", path)

	topNode := LoadJson(id)
	t := FindTask(upath, topNode)
	t.Checked = !t.Checked
	SaveJson(id, topNode)

}

func main() {
	os.Mkdir("quester", 0700)
	router := gin.Default()
	serveQuester(router, "/quester/")
	router.Run("127.0.0.1:93")
}

func serveQuester(router *gin.Engine, prefix string) {

	router.GET(prefix+"summary", makeAuthed(summary))
	router.GET(prefix+"detailed", makeAuthed(detailed))
	router.POST(prefix+"addWaypoint", makeAuthed(addWaypoint))

	router.GET(prefix+"toggle", makeAuthed(toggle))
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
