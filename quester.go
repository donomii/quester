package main

import (
	"crypto/md5"
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
	Id string
	Name      string
	Text      string
	TimeStamp time.Time
	Checked   bool
	Deleted   bool
	SubTasks  []*Task
}

func LoadRawJson(id string) []byte {
	res, _ := ioutil.ReadFile(fmt.Sprintf("quester/%v.json", id))
	return res
}

	//Walk through the task tree, visiting every node, and setting the id
	//to the md5 hash of the Name
func SetIds(t *Task) {
	t.Id = str2md5(t.Name+t.Text)
	
	for _, subTask := range t.SubTasks {
		SetIds(subTask)
	}
}

func LoadJson(id string) *Task {
	var out *Task
	res := LoadRawJson(id)
	err := json.Unmarshal(res, &out)
	if err != nil {
		log.Println("Could not load quests", err)
		//panic(err)
	}
	if out == nil {
		t := Task{Name: "Quester", Text: "Quest style task tracking"}
		out = &t
	}
	SetIds(out)
	SaveJson(id, out)
	
	return out
}

func SaveRawJson(id string, data []byte) {
	//FIXME create directory, possibly at startup
	ioutil.WriteFile(fmt.Sprintf("quester/%v.json", id), data, 0600)
}

func SaveJson(id string, tasks *Task) {
	payload, err := json.Marshal(tasks)
	if err != nil {
		panic("Could not marshall quests")
	}
	SaveRawJson(id, payload)
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

func downloadAll(c *gin.Context, id string, token string) {
	c.Header("Content-Type","application/json")
	c.Header("Content-Disposition","attachment; filename=\"tasks.json\"")
	tasks := LoadJson(id)
	j, _ := json.MarshalIndent(tasks, "", "\t")
	c.Writer.Write(j);
}

func summary(c *gin.Context, id string, token string) {

	c.Writer.Write([]byte(`
<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>Unfinished Business</title>
</head>
<body>
<a href="downloadAll">Download all tasks</a>
<a href="restoreAllPage">Restore from backup</a>
   ` + taskDisplay(id, str2md5("nodes"), false) + `
 
  <!-- 4 include the jQuery library -->
  <script src="https://cdnjs.cloudflare.com/ajax/libs/jquery/1.12.1/jquery.min.js"></script>

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
  <title>Unfinished Business</title>
  <!-- 4 include the jQuery library -->
  <script src="https://cdnjs.cloudflare.com/ajax/libs/jquery/1.12.1/jquery.min.js"></script>
  
</head>
<body>
   ` + taskDisplay(id, q, true) + `
</body>
</html>
`))
}

func restoreAll(c *gin.Context, id string, token string) {
	content, err := c.FormFile("content")
	if err != nil {
		panic(err)
	}
	openedFile, _ := content.Open()
	file, _ := ioutil.ReadAll(openedFile)
	SaveRawJson(id, []byte(file))
}

func addWaypoint(c *gin.Context, id string, token string) {
	title := c.PostForm("title")
	content := c.PostForm("content")
	quest := c.PostForm("q")
	path := quest + "/" + fmt.Sprintf("%x", md5.Sum([]byte(title+content)))

	topNode := LoadJson(id)
	t := FindTask(quest, topNode)
	
		log.Println("Adding waypoint", path)
		newTask := Task{Id: fmt.Sprintf("%x", md5.Sum([]byte(title+content))),Name: title, Text: content, TimeStamp: time.Now()}
		t.SubTasks = append(t.SubTasks, &newTask)
		SaveJson(id, topNode)

	summary(c, id, token)
}

func deleteWaypoint(c *gin.Context, id string, token string) {
	quest := c.PostForm("q")

	topNode := LoadJson(id)
	t := FindTask(quest, topNode)

	if t == nil {
		log.Println("Task does not exist, cannot be deleted:", quest)
	} else {
		log.Println("Deleting waypoint", quest)
		t.Deleted = true
		SaveJson(id, topNode)
	}

	summary(c, id, token)
}

func editWaypoint(c *gin.Context, id string, token string) {
	quest := c.PostForm("q")
	title := c.PostForm("title")
	content := c.PostForm("content")

	topNode := LoadJson(id)
	t := FindTask(quest, topNode)

	if t == nil {
		log.Println("Task does not exist, cannot be updated:", quest)
	} else {
		log.Println("updating waypoint", quest)
		t.Text = content
		t.Name = title
		SaveJson(id, topNode)
	}

	summary(c, id, token)
}

func str2md5(str string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(str)))
}

func FindTask(path string, task *Task) *Task {

	paths := strings.Split(path, "/")
	if paths[0] == "" {
		return task
	}
	if paths[0] == str2md5("nodes") {
		return FindTask(strings.Join(paths[1:], "/"), task)
	}
	for _, t := range task.SubTasks {
		//log.Println("Comparing", t.Name, "to '", paths[0], "'")
		if t.Id == paths[0] {
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
	//log.Println("Loading tasks for", path)
	//if task == nil Do string to task
	if task == nil {
		task = FindTask(path, LoadJson(id))
	}
	if task == nil {
		return ""
	}
	if !task.Deleted {
		if len(task.SubTasks) > 0 {
			fmt.Println(path, "is a container task")
			out = out + fmt.Sprintf("<li><input type=\"checkbox\" "+isTaskChecked(task)+" onclick=\"$.get('toggle?path=%s')\"><a href=\"detailed?q=%s\">", path, path) + task.Name + "</a><ul>"
			tasks := task.SubTasks

			for _, f := range tasks {
				//log.Println("Loading task", f.Name)
				out = out + loadTasks(id, path+"/"+f.Id, f, detailed)
			}
			out = out + "</ul></li>"
		} else {
			//fmt.Println(path, "is leaf task")
			var contents = task.Text

			if detailed {
				out = out + "<li><input type=\"checkbox\"  " + isTaskChecked(task) + " onclick=\"$.get('toggle?path=" + path + "')\">" + task.Name + " <a href=\"detailed?q=" + path + "\">+</a><p style=\"margin-left: 10em\">" + string(contents) + "</p>" + "</li>"
			} else {
				out = out + "<li><input type=\"checkbox\"  " + isTaskChecked(task) + " onclick=\"$.get('toggle?path=" + path + "')\">" + task.Name + " <a href=\"detailed?q=" + path + "\">+</a></li>"
			}
		}
	}
	return out
}

func restoreAllDisplay(c *gin.Context, id string, token string) {
	c.Writer.Write([]byte(`<!DOCTYPE html>
<html>
<head></head>
<body>
	Select a backup file to restore from.  Your current tasks will be wiped, and replaced with this file.<P><P><form action="restoreAll" method="post" enctype="multipart/form-data"><input type="file" id="content" name="content"><input type="submit" formmethod="post" value="Restore"></form>
	</body>
	</html>`))
}

func taskDisplay(id, path string, detailed bool) string {
	task := FindTask(path, LoadJson(id))
	if task == nil {
		panic("Task not found " + path)
	}
	return loadTasks(id, path, nil, detailed) + `<form action="addWaypoint" method="post" ><input type="hidden" id="q" name="q" value="` + path + `"><input id="title" name="title" type="text"><input id="content" name="content" type="text"><input type="submit"  value="Add"></form>` + `<form action="deleteWaypoint" method="post"  ><input type="hidden" id="q" name="q" value="` + path + `"><input type="submit" value="Delete"></form>` + `<form action="editWaypoint" method="post"  ><input type="hidden" id="q" name="q" value="` + path + `"><input id="title" name="title" type="text" value="` + task.Name + `"><input id="content" name="content" type="text" value="` + task.Text + `"><input type="submit" value="Update"></form>`

}

func toggle(c *gin.Context, id string, token string) {
	upath := c.Query("path")
	log.Println("user", id, "toggling", upath)

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
	router.GET(prefix+"downloadAll", makeAuthed(downloadAll))
	router.POST(prefix+"restoreAll", makeAuthed(restoreAll))
	router.GET(prefix+"restoreAllPage", makeAuthed(restoreAllDisplay))
	router.GET(prefix+"detailed", makeAuthed(detailed))
	router.POST(prefix+"addWaypoint", makeAuthed(addWaypoint))
	router.POST(prefix+"deleteWaypoint", makeAuthed(deleteWaypoint))

	router.POST(prefix+"editWaypoint", makeAuthed(editWaypoint))

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
