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
	_ "embed"

	//"github.com/gin-gonic/autotls"

	"github.com/gin-gonic/gin"
)

//embed the style.css file into a variable

//go:embed style.css
var styleCss string


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
	//SetIds(out)
	//SaveJson(id, out)
	
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

//Function returns the strings "odd" or "even" depending on the value of i
func oddEven(i int) string {
	if i%2 == 0 {
		return "even"
	} else {
		return "odd"
	}
}

func summary(c *gin.Context, id string, token string) {

	c.Writer.Write([]byte(`
<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>Unfinished Business</title>
  <style>`+ styleCss + `</style>
  </head>

<body class="dark"><div id="topbar"><nav><div class="nav-item left"><a href="/"><img src="/favicon.png" alt="">unfinished business</a></div><div class="settings"><div class="icon-container"><a href="/about">[about]</a></div><div class="icon-container"><a href="/preferences">[preferences]</a></div></div></nav><div class="top-links"><a href="/r/Popular">Popular</a><a href="/r/All">All</a><a href="/saved">Saved</a><a href="/r/AskReddit">AskReddit</a><a href="/r/pics">pics</a><a href="/r/news">news</a><a href="/r/worldnews">worldnews</a><a href="/r/funny">funny</a><a href="/r/tifu">tifu</a><a href="/r/videos">videos</a><a href="/r/gaming">gaming</a><a href="/r/aww">aww</a><a href="/r/todayilearned">todayilearned</a><a href="/r/gifs">gifs</a><a href="/r/Art">Art</a><a href="/r/explainlikeimfive">explainlikeimfive</a><a href="/r/movies">movies</a><a href="/r/Jokes">Jokes</a><a href="/r/TwoXChromosomes">TwoXChromosomes</a><a href="/r/mildlyinteresting">mildlyinteresting</a><a href="/r/LifeProTips">LifeProTips</a><a href="/r/askscience">askscience</a><a href="/r/IAmA">IAmA</a><a href="/r/dataisbeautiful">dataisbeautiful</a><a href="/r/books">books</a><a href="/r/science">science</a><a href="/r/Showerthoughts">Showerthoughts</a><a href="/r/gadgets">gadgets</a><a href="/r/Futurology">Futurology</a><a href="/r/nottheonion">nottheonion</a><a href="/r/history">history</a><a href="/r/sports">sports</a><a href="/r/OldSchoolCool">OldSchoolCool</a><a href="/r/GetMotivated">GetMotivated</a><a href="/r/DIY">DIY</a><a href="/r/photoshopbattles">photoshopbattles</a><a href="/r/nosleep">nosleep</a><a href="/r/Music">Music</a><a href="/r/space">space</a><a href="/r/food">food</a><a href="/r/UpliftingNews">UpliftingNews</a><a href="/r/EarthPorn">EarthPorn</a><a href="/r/Documentaries">Documentaries</a><a href="/r/InternetIsBeautiful">InternetIsBeautiful</a><a href="/r/WritingPrompts">WritingPrompts</a><a href="/r/creepy">creepy</a><a href="/r/philosophy">philosophy</a><a href="/r/announcements">announcements</a><a href="/r/listentothis">listentothis</a><a href="/r/blog">blog</a><a href="/subreddits" id="sr-more-link">more »</a></div></div><header><a class="main" href="/"><h1>unfinished business</h1></a><div class="bottom"><ul class="tabmenu"><li class="active"><a href="/">hot</a></li><li><a href="/new">new</a></li><li><a href="/rising">rising</a></li><li><a href="downloadAll">Backup</a>

</li><li><a href="restoreAllPage">Restore</a></li></ul></div></header><div id="intro">
<h1>Welcome to Unfinished Business</h1>
<h2>the online hierarchical task manager.</h2>
</div>


<div class="sr" id="links">




   ` + summaryView(id, str2md5("nodes"), nil) + `
   </div>
  <!-- 4 include the jQuery library -->
  <script src="https://cdnjs.cloudflare.com/ajax/libs/jquery/1.12.1/jquery.min.js"></script>
</style>
</div>
</body>
</html>
`))
}

func summaryView(id, path string, t *Task) string{
	if t == nil {
		t = FindTask(path, LoadJson(id))
	}
	out := ""
	if t == nil {
		return ""
	}
	for _, task := range t.SubTasks {
		subPath := path + "/" + task.Id
		out = out + buildItem("","", subPath, task.Name, task.Id, task.Checked)
	}

	out=out + `<form action="addWaypoint" method="post" >
	<input type="hidden" id="q" name="q" value="` + path + 
	`"><input id="title" name="title" type="text">
	<input id="content" name="content" type="text"
	><input type="submit"  value="Add"></form>` + 
	`<form action="deleteWaypoint" method="post"  >
	<input type="hidden" id="q" name="q" value="` + 
	path + `"><input type="submit" value="Delete"></form>` + 
	`<form action="editWaypoint" method="post">
	<input type="hidden" id="q" name="q" value="` + path + 
	`"><input id="title" name="title" type="text" value="` + t.Name + 
	`"><input id="content" name="content" type="text" value="` + t.Text + 
	`"><input type="submit" value="Update">`
	return out

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
  <style>`+ styleCss + `</style>
</head>
<body>
   ` + detailedTaskDisplay(id, q,  -1) + `
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

func buildItem(picurl, thumburl, path, description, extradescription string, checked bool) string {
	template := `


	<div class="link">
		<div class="upvotes">
			<div class="arrow"></div>
			<span>84k</span>
			<div class="arrow down"></div>
		</div>
		<div class="image">
			<a href="/r/nextfuckinglevel/comments/x9qj8j/ooh_ooh_here_she_comes/"><img src="PICURL" alt="">
				<span class="duration">00:23</span>
			</a>
		</div>
		<div class="entry">
			<div class="title">
				<a href="detailed?q=PATH">
					<h2><input type="checkbox" CHECKED onclick="$.get('toggle?path=PATH')">DESCRIPTION</h2>
				</a>
				<span>SHORTEXTRADESCRIPTION</span>
			</div>
			<div class="meta">
				<p class="submitted">submitted
					<span title="Fri, 09 Sep 2022 09:06:55 GMT">12 hours ago by</span>
					<a href="/u/the-highlife-artia">the-highlife-artia</a>
				</p>
				<p class="to">
					to
					<a href="/r/nextfuckinglevel">nextfuckinglevel</a>
				</p>
				<div class="links">
					<a class="comments" href="detailed?q=PATH">5933 comments</a><a href="">save</a>
				</div>
			</div>
		</div>
	</div>



	
	  `
	  //Replace all variables in the template
	  out := strings.ReplaceAll(template, "PICURL", picurl)
	  out = strings.ReplaceAll(out, "SHORTEXTRADESCRIPTION", extradescription)
	  out = strings.ReplaceAll(out, "THUMBURL", thumburl)
	  out = strings.ReplaceAll(out, "PATH", path)
	  out = strings.ReplaceAll(out, "DESCRIPTION", description)
	  
	  if checked {
	  out = strings.ReplaceAll(out, "CHECKED", `checked="checked"`)
	  } else {
		out = strings.ReplaceAll(out, "CHECKED", ``)
	  }
	  return out
}



func detailedTasks(id, path string, task *Task,  depth, alternator int) string {
	
	out := ""
	log.Println("Loading tasks for", path)
	//if task == nil Do string to task
	if task == nil {
		task = FindTask(path, LoadJson(id))
	}
	if task == nil {
		return ""
	}
	var contents = task.Text
	if !task.Deleted {
		
		if len(task.SubTasks) > 0 {
			//fmt.Println(path, "is a container task")
			out=out+` 
		<div class="comment `+oddEven(alternator)+`-depth" id="io7v1nv">
			<details open>
			  <summary>
				<p class="author"><a href="/u/" class="">A User</a></p>
				<p class="ups">2k points</p>
				<p class="created" title="Tue, 13 Sep 2022 04:28:20 GMT">13 hours ago</p>
				<p class="stickied"></p>
			  </summary>
			<div class="meta">
			  <p class="author"><a href="/u/" class="">a user</a></p>
			  <p></p>
			  <p class="ups">2k points</p>
			  <p class="created" title="Tue, 13 Sep 2022 04:28:20 GMT">
				 <a href="/r/AskReddit/comments#c">13 hours ago</a>
			  </p>
			  <p class="stickied"></p>
			</div>
			<div class="body"><div class="md"><p>` + task.Name  + string(contents) + `</p>
		`
			out = out + buildItem("","", path, task.Name, task.Id, task.Checked)
			
			/*
			fmt.Sprintf("<li><input type=\"checkbox\" "+isTaskChecked(task)+" onclick=\"$.get('toggle?path=%s')\"><a href=\"detailed?q=%s\">", path, path) + task.Name + "</a><ul>"
			*/
			tasks := task.SubTasks

			if depth != 0 {
			for i, f := range tasks {
				//log.Println("Loading task", f.Name)
				out = out + detailedTasks(f.Id, path+"/"+f.Id, f, depth-1, alternator+i+1)
			}

			out=out+`</details></div>`
			
		}
			//out = out + "</ul></li>"
		} else {
			//fmt.Println(path, "is leaf task")
			var contents = task.Text

			
				out=out+` 
			<div class="comment `+oddEven(alternator)+`-depth" id="io7v1nv">
				<details open>
				  <summary>
					<p class="author"><a href="/u/" class="">A User</a></p>
					<p class="ups">2k points</p>
					<p class="created" title="Tue, 13 Sep 2022 04:28:20 GMT">13 hours ago</p>
					<p class="stickied"></p>
				  </summary>
				<div class="meta">
				  <p class="author"><a href="/u/" class="">a user</a></p>
				  <p></p>
				  <p class="ups">2k points</p>
				  <p class="created" title="Tue, 13 Sep 2022 04:28:20 GMT">
					 <a href="/r/AskReddit/comments#c">13 hours ago</a>
				  </p>
				  <p class="stickied"></p>
				</div>
				<div class="body"><div class="md"><p>` + task.Name  + string(contents) + `</p>
  			</details></div>
			
				  `
				  /*
				out = out + "<li><input type=\"checkbox\"  " + isTaskChecked(task) + " onclick=\"$.get('toggle?path=" + path + "')\">" + task.Name + " <a href=\"detailed?q=" + path + "\">+</a><p style=\"margin-left: 10em\">" + string(contents) + "</p>" + "</li>"
				*/
			
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

func detailedTaskDisplay(id, path string,  depth int) string {
	task := FindTask(path, LoadJson(id))
	if task == nil {
		panic("Task not found " + path)
	}
	return detailedTasks(id, path, task, depth,0) + `<form action="addWaypoint" method="post" ><input type="hidden" id="q" name="q" value="` + path + `"><input id="title" name="title" type="text"><input id="content" name="content" type="text"><input type="submit"  value="Add"></form>` + `<form action="deleteWaypoint" method="post"  ><input type="hidden" id="q" name="q" value="` + path + `"><input type="submit" value="Delete"></form>` + `<form action="editWaypoint" method="post"  ><input type="hidden" id="q" name="q" value="` + path + `"><input id="title" name="title" type="text" value="` + task.Name + `"><input id="content" name="content" type="text" value="` + task.Text + `"><input type="submit" value="Update"></form>`

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
