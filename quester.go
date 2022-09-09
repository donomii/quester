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
  <style>
  :root {
	--sm-font: 0.666rem;
	--lightgray: #f5f5f5;
	--whitebg: #ffffff;
	--linkcolor: #0645ad;
	--lightlinkcolor: #406bb3;
	--graytext: #6f6f6f;
  }
  * {
	padding: 0;
	margin: 0;
	font-family: sans-serif;
  }
  
  a {
	color: var(--linkcolor);
	text-decoration: none;
  }
  a:hover, a:focus {
	color: var(--linkcolor);
	text-decoration: underline;
  }
  .green {
	color: green;
  }
  .word-break {
	overflow-wrap: anywhere;
  }
  .top-links {
	width: 100%;
	float: left;
	overflow: hidden;
	position: relative;
  }
  .upvotes .arrow, .score .arrow {
	background: url(/css/sprite.png);
	background-position: -84px -1654px;
	background-repeat: no-repeat;
	margin: 2px 0 2px 0;
	width: 100%;
	height: 14px;
	display: block;
	width: 15px;
	margin-left: auto;
	margin-right: auto;
	outline: none;
  }
  .upvotes .arrow.down, .score .arrow.down {
	background-position: -42px -1654px;
	background-repeat: no-repeat;
  }
  #topbar {
	float: left;
	width: 100%;
  }
  nav {
	float: left;
	width: 100%;
	background: #170019;
  }
  nav .settings {
	float: right;
	padding-right: 20px;
	padding-top: 10px;
	font-size: 0.81rem;
  }
  nav .settings a {
	font-size: 0.81rem;
	margin-left: 18px;
  }
  nav .settings .icon-container {
	float: left;
  }
  nav a {
	color: white;
  }
  nav .nav-item.left {
	float: left;
	padding: 10px;
  }
  nav .nav-item.left a {
	font-size: 15px;
	font-weight: initial;
  }
  nav .nav-item.left img {
	width: 20px;
	vertical-align: bottom;
	margin: 0 7px 0 0;
  }
  nav .nav-item.left a:hover
  nav .nav-item.left a:focus {
	color: white;
  }
  nav a:hover,
  nav a:focus {
	color: white;
	text-decoration: underline;
  }
  .top-links a {
	padding-right: 6px;
	font-size: 0.76rem;
	color: black;
	background: #e8e8e8;
	color: #040404;
	text-transform: uppercase;
  }
  #intro {
	float: left;
	width: calc(100% - 40px);
	margin-bottom: 25px;
	margin-left: 20px;
  }
  .container {
	margin: 0 auto;
	min-height: 100vh;
	padding-top: 90px;
  }
  .container .content {
	max-width: 600px;
	width: 100%;
	margin: 0 auto;
	margin-top: 0;
	margin-top: 10px;
	padding: 10px 15px;
	border: 1px solid black;
  }
  .container .content h1 {
	padding-bottom: 20px;
	padding-top: 11px;
  }
  .container .content h2 {
	padding-top: 30px;
	padding-bottom: 10px;
  }
  .container .content p {
	line-height: 1.4;
	padding-bottom: 5px;
  }
  form legend {
	border-bottom: 1px solid #e3e3e3;
	margin-bottom: 10px;
	padding-bottom: 10px;
	margin-top: 40px;
	font-weight: bold;
  }
  form legend:first-child {
	margin-top: 0;
  }
  form .setting {
	margin: 10px 0;
	width: 100%;
  }
  .export-import-form input {
	margin: 10px 0px 10px 0px;
  }
  .bottom-prefs {
	  margin: 60px 0px 0px 0px;
  }
  .container .content small.notice {
	padding-top: 20px;
	padding-bottom: 5px;
	display: inline-block;
  }
  .container .content p.version {
	text-align: right;
	color: #4f4f4f;
  }
  .container .content ul {
	padding-left: 25px;
  }
  .content .bottom {
	text-align: center;
	padding-top: 40px;
  }
  header {
	float: left;
	width: 100%;
	padding-top: 15px;
	margin-bottom: 21px;
	margin-top: 2px;
	background: gainsboro;
  }
  header a {
	color: black;
	font-size: 0.8rem;
	float: left;
	vertical-align: bottom;
  }
  header a.main {
	margin-left: 12px;
  }
  header h3.username {
	margin: 0px 15px 0px 0px;
  }
  header .bottom {
	float: left;
	overflow: hidden;
	padding-top: 7px;
	padding-left: 19px;
  }
  header a.subreddit {
	text-transform: uppercase;
	font-size: 0.7rem;
	margin-right: 12px;
  }
  header a.subreddit h2 {
	overflow-wrap: anywhere;
  }
  header .tabmenu {
	float: left;
	overflow: hidden;
	list-style: none;
	padding: 0;
	margin: 0;
  }
  header .tabmenu li {
	float: left;
	width: auto;
	overflow: hidden;
	padding: 0;
	margin: 0;
  }
  header .tabmenu li a {
	padding: 2px 8px 2px 8px;
	background: #5a5a5a;
	color: white;
	margin-right: 8px;
  }
  header .tabmenu li.active a {
	background: black;
  }
  .view-more-links {
	float: left;
	width: 100%;
  }
  .view-more-links a {
	margin-left: 20px;
	padding: 1px 4px;
	background: #eee;
	border: 1px solid #ddd;
	border-radius: 3px;
	font-weight: bold;
  }
  .green {
	color: green !important;
  }
  .tag {
	display: inline-block;
	border: 1px solid;
	padding: 0 4px;
	margin: 2px 6px 0 0;
	border-radius: 3px;
	font-size: 0.68rem;
  }
  .tag.nsfw {
	border-color: #d10023;
	color: #d10023;
  }
  .nsfw-warning {
	text-align: center;
	float: left;
	width: 100%;
	margin: 40px 0;
  }
  .nsfw-warning span {
	font-size: 3rem;
	background: #ff575b;
	border-radius: 130px;
	display: inline-block;
	padding: 39px 20px 39px 20px;
	color: white;
  }
  .nsfw-warning h2 {
	margin: 20px 0;
  }
  .nsfw-warning a {
	margin: 20px;
	display: inline-block;
	background: #4f86b5;
	color: white;
	padding: 16px;
  }
  input[type="submit"],
  .btn {
	padding: 3px;
	margin-top: 7px;
	margin-right: 10px;
	border-radius: 0;
	border: 1px solid #a5a5a5;
	background: white;
	color: #464646;
	font-size: 13px;
  }
  input[type="submit"]:focus,
  input[type="submit"]:hover,
  .btn:focus,
  .btn:hover {
	background: #4c4c4c;
	color: white;
	cursor: pointer;
	text-decoration: none;
  }
  .reddit-error {
	text-align: center;
	float: left;
	width: 100%;
	padding: 37px 0px 20px 0px;
  }
  footer {
	padding: 10px;
	margin: 2.5% 0 0;
	background: #e1e1e1;
	float: left;
	width: calc(100% - 20px);
	text-align: center;
  }
  footer a {
	color: #646464;
	font-size: 0.85rem;
	text-decoration: underline;
  }
  /* SUBREDDIT LINKS */
  #links {
	float: left;
	width: 100%;
  }
  #links.sr {
	float: left;
	width: 75%;
	min-height: 100vh;
  }
  #links details {
	float: left;
	width: auto;
	cursor: pointer;
	margin-bottom: 20px;
	margin-left: 20px;
  }
  #links details ul li.active a {
	font-weight: bold;
  }
  #links details li.active a {
	font-weight: bold;
  }
  #links details ul {
	padding-left: 30px;
	margin-bottom: 20px;
  }
  #links .link {
	float: left;
	width: 100%;
	margin-bottom: 16px;
  }
  #links .link .upvotes {
	float: left;
	width: 60px;
	text-align: center;
	color: #c6c6c6;
	font-weight: bold;
	font-size: small;
  }
  #links .link .image .no-image, #user .entry .image .no-image {
	float: left;
	font-size: 0;
	margin-bottom: 2px;
	margin-right: 5px;
	margin-top: 0;
	margin-left: 0;
	overflow: hidden;
	width: 70px;
	background-image: url(/css/sprite.png);
	background-position: 0 -1267px;
	background-repeat: no-repeat;
	height: 50px;
  }
  #links .link .image {
	width: 80px;
	max-height: 80px;
	float: left;
	text-align: center;
	position: relative;
  }
  #links .link .image .duration {
	left: 0;
	background: gray;
	position: absolute;
	background-color: rgba(0,0,0,0.6);
	color: white;
	bottom: 0px;
	font-size: var(--sm-font);
	width: 100%;
	text-align: center;
  }
  #links .link .image img {
	width: auto;
	height: auto;
	max-width: 80px;
	max-height: 80px;
  }
  #links .link .entry {
	float: left;
	width: calc(100% - 148px);
	margin-left: 8px;
  }
  #links .link .entry .title span {
	color: #757575;
	font-size: x-small;
	display: inline-block;
	padding-left: 13px;
  }
  #links .link .entry .title a {
	font-size: 0.8rem;
	overflow-wrap: anywhere;
  }
  #links .link .entry .title a:hover {
	text-decoration: none;
  }
  #links .link .entry .title a h2 {
	display: initial;
	font-size: 16px;
	color: var(--linkcolor);
	font-size: medium;
	font-weight: normal;
  }
  #links .link .entry .title a:visited h2 {
	color: #6f6f6f;
  }
  #links .link .entry .meta {
	float: left;
	width: 100%;
	color: #757575;
	font-size: x-small;
	margin-top: 2px;
  }
  #links .link .entry .meta a {
	color: var(--lightlinkcolor);
	padding-left: 3px;
	padding-right: 3px;
  }
  #links .link .entry .meta .deleted {
	margin-left: 0 !important;
	padding-left: 5px;
	padding-right: 3px;
  }
  #links .link .entry .meta p {
	float: inherit;
	overflow-wrap: anywhere;
  }
  #links .link .entry .meta p.submitted span {
	margin-left: 4px;
  }
  #links .link .entry .meta .submitted a {
	text-decoration: none;
	padding-left: 5px;
  }
  #links .link .entry .meta .links {
	float: left;
	width: 100%;
	margin-top: 1px;
  }
  #links .link .entry .meta .links a {
	padding: 0;
	color: #888;
	font-weight: bold;
	margin: 0px 15px 0px 0px;
  }
  #links .link .entry .meta a:hover {
	text-decoration: underline;
  }
  #links.search .link .meta {
	font-size: small;
  }
  #links.search .link .meta a {
	margin-right: 6px;
	margin-left: 6px;
  }
  #links.search .link .meta a.comments {
	margin-left: 0;
  }
  #links .link .entry .meta .links .selftext a {
	color: var(--linkcolor);
	font-weight: initial;
  }
  #links .link .entry .selftext {
	unicode-bidi: isolate;
	background-color: #fafafa;
	border: 1px solid #369;
	border-radius: 7px;
	padding: 5px 10px;
	margin: 10px auto 5px 0;
	font-size: 0.84rem;
	max-width: 60em;
	word-wrap: break-word;
	float: left;
	width: calc(100% - 100px);
	margin-right: 11000px;
	overflow: hidden;
	cursor: initial;
  }
  #links .link .entry details {
	margin: 0 10px 0 0;
	font-size: 0.7rem;
  }
  #links .link .entry details[open] {
	width: 100%;
  }
  #links .link .entry details summary {
	font-size: 0.833rem;
	list-style-type: none;
	padding: 4px;
  }
  #links .link .entry  details > summary::-webkit-details-marker {
	display: none;
  }
  #links .link .entry  details summary:hover {
	text-decoration: underline;
	cursor: pointer;
  }
  #links .link .entry details .line {
	width: 16px;
	margin-top: 3px;
	background: #979797;
	border: 1px solid #b3b0b0;
  }
  #links .link .entry details .line:first-child {
	margin-top: 0;
  }
  #links .link .entry details.preview-container img {
	max-height: 600px !important;
  }
  /* COMMENTS */
  .comment {
	font-size: 0.83rem;
	clear: left;
  }
  .comment summary {
	float: left;
  }
  .comment .meta {
	float: left;
  }
  .comment .meta p {
	float: left;
	padding-right: 8px;
	color: var(--graytext);
  }
  .comment .meta p.author a {
	font-weight: bold;
	margin-left: 10px;
  }
  .comment .meta .created a {
	color: var(--graytext);
  }
  .comment .meta span.controversial {
	font-size: var(--sm-font);
	display:inline-block;
	vertical-align: baseline;
	position: relative;
	top: -0.4em;
  }
  .comment .body {
	float: left;
	width: 100%;
	padding-top: 20px;
	padding-bottom: 20px;
  }
  .comment details {
	float: left;
	width: 100%;
	padding-top: 15px;
  }
  .comment .comment:first-child {
	width: calc(100% - 30px);
  }
  .comment {
	padding-left: 30px;
	width: auto;
	overflow: hidden;
	background: var(--whitebg);
  }
  .commententry .comment {
	padding-left: 0;
  }
  .comment details summary {
	float: left;
	font-size: 0.833rem;
	list-style-type: none;
	color: #313131;
  }
  .comment details > summary::-webkit-details-marker {
	display: none;
  }
  .comment details > summary::before {
	content: '[+]';
	font-size: 0.9rem;
	padding-right: 10px;
  }
  .comment details[open] > summary::before {
	content: '[‒]';
	padding-right: 5px;
  }
  .comment details summary:hover {
	text-decoration: underline;
	cursor: pointer;
  }
  .comment details summary a,.comment details summary p {
	display: none;
  }
  .comment details:not([open]) summary a, .comment details:not([open]) summary p {
	display: initial;
	opacity: 0.5;
  }
  .comment details summary:hover {
	text-decoration: underline;
	cursor: pointer;
  }
  .comment details summary a,.comment details summary p {
	display: none;
  }
  .comment details:not([open]) summary a, .comment details:not([open]) summary p {
	display: initial;
	opacity: 0.5;
  }
  .comment .body blockquote {
	border-left: 2px solid #D6D5CF;
	display: block;
	background-color: #EEE;
	margin: 5px 5px !important;
	padding: 5px;
	color: #333;
	font-style: italic;
  }
  .comment .md {
	max-width: 60em;
  }
  
  .even-depth {
	background: var(--whitebg);
  }
  
  .odd-depth {
	background: var(--lightgray);
  }
  .comment .comment {
	border-left: 1px solid #dcdcdc;
	margin-top: 10px;
  }
  .infobar {
	background-color: #f6e69f;
	margin: 5px 305px 5px 11px;
	padding: 5px 10px;
	float: left;
	width: calc(100% - 50px);
  }
  .infobar.blue {
	background: #eff8ff;
	border: 1px solid #93abc2;
  }
  .infobar.explore {
	margin-bottom: 15px;
  }
  .explore#links .link {
	padding-left: 10%;
  }
  .explore#links .link .sub-button {
	float: left;
	margin: 7px 0;
	width: 90px;
  }
  .explore#links .link .content {
	float: left;
	width: calc(100% - 120px);
  }
  .explore#links .link .description {
	font-size: 0.86rem;
  }
  #sr-more-link {
	color: black;
	background-color: #f0f0f0;
	position: absolute;
	right: 0;
	top: 0;
	padding: 0 15px 0 15px;
	margin: 3px 0;
	font-weight: bold;
  }
  
  /* POST */
  #post {
	min-height: 100vh;
  }
  #post .info {
	float: left;
	width: 100%;
  }
  #post .info .links a {
	font-size: var(--sm-font);
  }
  #post .info .links a {
	float: initial;
  }
  #post header {
	padding-top: 0;
  }
  #post header div {
	padding: 20px 0 20px 20px;
	float: left;
  }
  #post header div a {
	color: #222;
	text-decoration: underline;
	font-size: 1rem;
  }
  #post .score, #user .upvotes {
	float: left;
	width: 60px;
	text-align: center;
	color: #c6c6c6;
	font-weight: bold;
	font-size: small;
  }
  #post .ratio {
	font-size: 0.6rem;
	display: block;
	padding: 4px 0px 5px 0px;
  }
  #post .title {
	float: left;
	width: calc(100% - 60px);
  }
  #post .title a {
	font-size: var(--sm-font);
	color: var(--linkcolor);
	float: left;
  }
  #post .title .domain {
	color: gray;
	font-size: 12px;
	margin-left: 10px;
  }
  #post .submitted {
	font-size: small;
	color: #686868;
  }
  #post .submitted a {
	float: initial;
  }
  #post .submitted a,
  #post .submitted span {
	margin-left: 5px;
  }
  #post .source-details {
	float: left;
	margin: 10px 0 10px 30px;
  }
  #post .source-details summary:hover {
	color: var(--linkcolor);
	text-decoration: underline;
  }
  #post .comments {
	float: left;
	width: 100%;
  }
  #post .comments-info {
	float: left;
	width: calc(100% - 30px);
	margin: 10px 0 10px 30px;
  }
  #post .comments-sort details {
	float: left;
	width: auto;
	cursor: pointer;
	margin-bottom: 10px;
  }
  #post .comments-sort details ul li.active a {
	font-weight: bold;
  }
  #post .comments-sort details li.active a {
	font-weight: bold;
  }
  #post .comments-sort details {
	font-size: 0.8rem;
  }
  #post .comments-sort details ul {
	margin-left: 20px;
  }
  #post .comment .meta p.stickied {
	color: green;
  }
  #post .comment .meta p.author a,
  #post .comment .meta p.author span {
	font-weight: initial;
	margin-left: 10px;
  }
  #post .comment .meta p.author a.submitter {
	font-weight: bold;
  }
  #post .comment .body {
	padding-top: 0;
	padding-bottom: 13px;
  }
  #post .usertext-body {
	unicode-bidi: isolate;
	font-size: small;
	background-color: #fafafa;
	border: 1px solid #369;
	border-radius: 7px;
	padding: 5px 10px;
	margin: 10px auto 5px 60px;
	font-size: 0.84rem;
	max-width: 60em;
	word-wrap: break-word;
	float: left;
	width: calc(100% - 100px);
  }
  #post .comment .load-more-comments {
	float: left;
	margin-bottom: 11px;
	margin-top: 11px;
	font-weight: bold;
	font-size: 11px;
  }
  #post .image {
	padding-left: 60px;
	max-width: 100%;
  }
  #post .image img, #post .video video {
	max-height: 700px;
	max-width: 100%;
  }
  #post .video {
	float: left;
	width: calc(100% - 60px);
	margin-left: 60px;
	max-width: 100%;
	max-height: 100%;
  }
  #post .video .title a {
	font-size: 1rem;
  }
  #post .youtube-info {
	font-size: 9px;
  }
  #post .crosspost {
	overflow: hidden;
	background: var(--whitebg);
	border: 1px solid #C6C6C6;
	border-radius: 4px;
	max-width: 600px;
	margin-left: 60px;
  }
  #post .crosspost .title {
	width: 100%;
	margin: 15px;
	margin-bottom: 0;
  }
  #post .crosspost .num_comments {
	float: left;
	width: 100%;
	font-size: small;
	margin-left: 15px;
	margin-bottom: 15px;
  }
  #post .crosspost .submitted,#post .crosspost .to {
	float: left;
	width: auto;
	margin-right: ;
	font-size: small;
	color: #686868;
  }
  #post .crosspost .submitted a,#post .crosspost .to a {
	margin-right: 6px;
	margin-left: 6px;
	font-size: small;
  }
  #post .gallery {
	float: left;
	margin: 10px auto auto 60px;
  }
  #post .gallery .item {
	float: left;
  }
  #post .gallery .item div {
	float: left;
	width: 100%;
	padding-bottom: 0;
  }
  #post .gallery .item a {
	float: left;
	overflow: hidden;
  }
  #post .gallery .item a.source-link {
	margin-top: -5px;
  }
  #post .gallery .item small {
	font-size: 10px;
  }
  #post .source-url {
	overflow-wrap: anywhere;
  }
  #post .usertext-body .poll {
	padding: 20px;
	border: 1px solid #369;
	margin: 10px 0px 20px 0px;
	float: left;
	width: calc(100% - 20%);
	position: relative;
  }
  #post .usertext-body .poll .option {
	float: left;
	width: 100%;
	position: relative;
	margin: 0px 0px 15px 0px;
  }
  #post .usertext-body .poll .option .vote_count {
	float: left;
	width: 20%;
	position: relative;
	z-index: 22;
	padding: 10px;
  }
  #post .usertext-body .poll .option .text {
	position: relative;
	width: 80%;
	z-index: 22;
	display: initial;
	padding: 10px 0px 0px 0px;
	line-height: initial;
	display: block;
  }
  #post .usertext-body .poll .option .background {
	position: absolute;
	background: #7171718c;
	height: 100%;
	float: left;
	width: 100%;
	position: absolute;
	z-index: 1;
  }
  #post .usertext-body .poll .votes {
	font-size: 1rem;
	padding: 10px 0px 10px 0px;
  }
  /* USER */
  #user .entries {
	float: left;
	width: 80%;
	min-height: 100vh;
  }
  #user .entries .commententry {
	padding-left: 5px;
	padding-top: 10px;
	padding-bottom: 15px;
	float: left;
	width: 100%;
  }
  #user .entries .commententry:first-child {
	padding-top: 0;
  }
  #user .info {
	float: right;
	width: 20%;
	text-align: center;
  }
  #user .info img {
	max-width: 50%;
	max-height: 190px;
  }
  #user .info h1 {
	font-size: 1.1rem;
	overflow-wrap: anywhere;
  }
  #user .entries .commententry .meta {
	float: left;
  }
  #user .entries .commententry .meta .title,
  #user .entries .commententry .meta .author,
  #user .entries .commententry .meta .subreddit,
  #user .entries .commententry .meta .flair {
	float: left;
  }
  #user .entries .commententry .meta a {
	margin-right: 5px;
	margin-left: 5px;
  }
  #user .entries .commententry .title a {
	margin-left: 0;
	font-size: 0.86rem;
  }
  #user .entries .commententry .meta .author,#user .entries .commententry .meta .subreddit {
	font-size: 11px;
	margin-top: 3px;
  }
  #user .entries .commententry .meta .author a {
	font-weight: bold;
  }
  #user .commententry details {
	padding-top: 2px;
  }
  #user .commententry details a.context,
  #user .commententry details a.comments {
	float: left;
  }
  #user .commententry .meta p.ups,#user .commententry .meta p.created {
	font-size: var(--sm-font);
	padding-right: 5px;
  }
  #user .entries .commententry .meta .created a {
	color: var(--graytext);
  }
  #user .entries .commententry.t3 .title .meta {
	float: left;
	width: 100%;
  }
  #user .entries .commententry.t3 .title a {
	margin-bottom: 3px;
  }
  #user .entries .commententry.t3 .upvotes {
	float: left;
	width: 60px;
  }
  #user .entries .commententry.t3 .image {
	float: left;
	width: 80px;
  }
  #user .entries .commententry.t3 .title {
	width: calc(100% - 200px);
	float: left;
  }
  #user .entries .commententry .commententry .meta .author {
	margin-top: 0;
  }
  #user .commententry .meta p {
	padding-right: 0;
  }
  #user .commententry .body {
	padding-top: 4px;
	padding-bottom: 0;
  }
  #user .info .user-stat span {
	font-weight: bold;
	font-size: 1.1rem;
  }
  #user .commententry details summary {
	font-size: var(--sm-font);
  }
  #user .commententry details summary p {
	margin-right: 5px;
	margin-left: 5px;
  }
  #user .commententry details summary a {
	margin-left: 5px;
  }
  #user .entries .commententry .image,#user .entries .commententry .upvotes,#user .entries .commententry .title,#user .entries .commententry .meta {
	float: left;
  }
  #user .entries .commententry .image {
	margin-left: 0;
	margin-right: 8px;
	position: relative;
  }
  #user .entries .link .image a span {
	bottom: 0;
	background: #0000005e;
	left: 0;
	width: 100%;
	text-align: center;
	color: white;
	font-size: var(--sm-font);
	margin-bottom: 4px;
  }
  #user .entries .link .image a span {
	bottom: 0;
	background: #0000005e;
	left: 0;
	width: 100%;
	text-align: center;
	color: white;
	font-size: var(--sm-font);
  }
  #user .entries .link .image img {
	max-width: 80px;
  }
  #user .entries .commententry .title a {
	float: left;
  }
  #user .entries .commententry .title .meta {
	width: 100%;
  }
  #user .entries .commententry .title .meta a {
	float: initial;
	font-weight: bold;
	font-size: var(--sm-font);
	margin-left: 5px;
  }
  #user .entries .commententry .title .meta a.subreddit {
	font-weight: unset;
  }
  #user .entries .commententry .title .meta .submitted {
	font-size: var(--sm-font);
	color: var(--graytext);
  }
  #user .entries .commententry .meta .title {
	margin-left: 20px;
  }
  #user #links {
	border-bottom: 1px dotted gray;
	padding-bottom: 5px;
	margin-bottom: 30px;
	margin-top: 30px;
  }
  #user #links details {
	margin-left: 25px;
	font-size: 0.8rem;
  }
  #user #links details ul {
	margin-left: 20px;
  }
  #user .entries .commententry a.comments, #user .entries .commententry a.context {
	color: gray;
	font-size: var(--sm-font);
	font-weight: bold;
  }
  #user .entries .commententry .title .meta a.comments {
	margin-left: 0;
  }
  #user .entries .commententry a.comments.t1,#user .entries .commententry a.context {
	margin-top: 0;
  }
  #user .entries .commententry a.context {
	margin-right: 10px;
  }
  /* FLAIR */
  .flair,
  #links .link .entry .title span.flair,
  #post .info .title span.flair {
	  display: inline-block;
	  border-radius: 4px;
	  color: #404040;
	  background-color: #e8e8e8;
	  font-size: x-small;
	  margin-left: 10px;
	  padding: 0 2px;
  }
  #post .comments .flair,
  #user .comment .meta .flair {
	margin-left: 0 !important;
  }
  #links .link .entry .meta p.submitted .flair,
  #user .comment .meta .flair,
  #user .entries p.submitted .flair {
	margin-right: 4px;
  }
  .flair .emoji {
	background-position: center;
	background-repeat: no-repeat;
	background-size: contain;
	display: inline-block;
	height: 16px;
	width: 16px;
	vertical-align: middle;
  }
  /* SIDEBAR */
  #sidebar {
	box-sizing: border-box;
	float: left;
	width: 25%;
	padding-left: 20px;
  }
  #sidebar .content {
	float: left;
	font-size: smaller;
	padding-right: 15px;
	width: calc(100% - 15px);
  }
  .subreddit-listing {
	margin: 8px 0;
	list-style: none;
  }
  .subreddit-listing li {
	margin: 15px 0;
  }
  #sidebar .mod-list {
	float: left;
	width: auto;
	margin-top: 25px;
  }
  #sidebar .mod-list ul {
	padding: 7px 0px 0px 20px;
  }
  #sidebar .mod-list li a {
	float: left;
  }
  #sidebar .content .description {
	margin-top: 38px;
  }
  a.sub-to-subreddit {
	color: #f9f9f9;
	background: #007900;
	font-size: var(--sm-font);
	padding: 6px 8px 6px 8px;
	margin: 0 5px 0 0;
  }
  a.sub-to-subreddit:hover,
  a.sub-to-subreddit:focus {
	color: white !important;
  }
  a.sub-to-subreddit.gray {
	background: gray;
  }
  .subscribe {
	margin: 0 0px 30px 0;
	width: 100%;
	float: left;
  }
  /* SEARCH */
  #search {
	margin-bottom: 50px;
	float: left;
	width: calc(25% - 60px);
  }
  #search.sr {
	width: calc(100% - 60px);
	margin-top: 0;
  }
  #search.sr.search-page {
	width: auto;
	margin-left: 20px;
	margin-top: 40px;
  }
  #search.explore {
	width: calc(100% - 60px);
	margin-left: 20px;
	margin-bottom: 15px;
  }
  #search form {
	max-width: 600px;
  }
  #search form div {
	float: left;
	width: 100%;
	margin-top: 5px;
	margin-bottom: 5px;
  }
  #search form input[type="text"] {
	width: 100%;
  }
  #search form label {
	float: left;
	overflow-wrap: anywhere;
  }
  #search form label input {
	float: left;
	margin-right: 10px;
  }
  #search input[type="text"] {
	padding: 4px;
	border: 1px solid #a5a5a5;
	border-radius: 0;
	margin-bottom: 11px;
  }
  /* SUGGESTED SUBREDDITS ON SEARCH PAGES */
  .suggested-subreddits {
	margin: 0px 0px 30px 1%;
  }
  .suggested-subreddits h3 {
	border-bottom: 1px solid #7d7d7d;
	max-width: 820px;
	margin: 0px 0px 16px 10px;
	padding: 0px 0px 5px 0px;
	font-size: 0.9rem;
  }
  .suggested-subreddits .sub-button {
	margin: 0px 0px 7px 0px;
  }
  .suggested-subreddits .description {
	font-size: 0.8rem;
  }
  /* REDDIT STYLES */
  .md .md-spoiler-text {
	border-radius:2px;
	transition:background ease-out 1s;
  }
  .md .md-spoiler-text>* {
	transition:opacity ease-out 1s;
  }
  .md .md-spoiler-text:not(.revealed) {
	background:#4f4f4f;
	cursor:pointer;
	color:transparent;
  }
  .md .md-spoiler-text:not(.revealed)>* {
	opacity:0;
  }
  .md .md-spoiler-text.revealed {
	background:rgba(79,79,79,0.1);
  }
  .spoiler-text-tooltip {
	border-radius:4px;
	font-size:11px;
	line-height:16px;
	pointer-events:none;
  }
  .spoiler-text-tooltip.hover-bubble {
	padding:3px 6px;
  }
  .md-container-small,.md-container {
	unicode-bidi:isolate;
	font-size:small;
  }
  .md {
	color:#222222;
	max-width:60em;
	overflow-wrap:break-word;
	word-wrap:break-word
  }
  .md .-headers,.md h1,.md h2,.md h3,.md h4,.md h5,.md h6 {
	border:0;
	color:inherit;
	-webkit-font-smoothing:antialiased;
  }
  .md .-headers code,.md h1 code,.md h2 code,.md h3 code,.md h4 code,.md h5 code,.md h6 code {
	font-size:inherit;
  }
  .md blockquote,.md del {
	color:#4f4f4f;
  }
  .md a {
	color: var(--linkcolor);
	text-decoration:none;
  }
  .md a del {
	color:inherit;
  }
  .md h6 {
	text-decoration:underline;
  }
  .md em {
	font-style:italic;
	font-weight:inherit;
  }
  .md th,.md strong,.md .-headers,.md h1,.md h2,.md h3,.md h4,.md h5,.md h6 {
	font-weight:600;
	font-style:inherit;
  }
  .md h2,.md h4 {
	font-weight:500;
  }
  .md,.md h6 {
	font-weight:400;
  }
  .md * {
	margin-left:0;
	margin-right:0;
  }
  .md tr,.md code,.md .-cells,.md .-lists,.md .-blocks,.md .-headers,.md h1,.md h2,.md h3,.md h4,.md h5,.md h6,.md th,.md td,.md ul,.md ol,.md .-lists,.md pre,.md blockquote,.md table,.md p,.md ul,.md ol {
	margin:0;
	padding:0;
  }
  .md hr {
	border:0;
	color:transparent;
	background:#c5c1ad;
	height:2px;
	padding:0;
  }
  .md blockquote {
	border-left:2px solid #c5c1ad;
  }
  .md code,.md pre {
	border:1px solid #e6e6de;
	background-color:#fcfcfb;
	border-radius:2px;
  }
  .md code {
	margin:0 2px;
	white-space:nowrap;
	word-break:normal;
  }
  .md p code {
	line-height:1em;
  }
  .md pre {
	overflow:auto;
  }
  .md pre code {
	white-space:pre;
	background-color:transparent;
	border:0;
	display:block;
	padding:0!important;
  }
  .md td,.md th {
	border:1px solid #e5e3da;
	text-align:left;
  }
  .md td[align=center],.md th[align=center] {
	text-align:center;
  }
  .md td[align=right],.md th[align=right] {
	text-align:right;
  }
  .md img {
	max-width:100%;
  }
  .md ul {
	list-style-type:disc;
  }
  .md ol {
	list-style-type:decimal;
  }
  .md blockquote {
	padding:0 8px;
	margin-left:5px;
  }
  .md code {
	padding:0 4px;
  }
  .md pre,.md .-cells,.md th,.md td {
	padding:4px 9px;
  }
  .md .-lists,.md ul,.md ol {
	padding-left:40px;
  }
  .md sup {
	font-size:0.86em;
	line-height:0;
  }
  code {
	font-family:monospace,monospace;
  }
  .md {
	font-size:1.0769230769230769em;
  }
  .md h1,.md h2 {
	font-size:1.2857142857142858em;
	line-height:1.3888888888888888em;
	margin-top:0.8333333333333334em;
	margin-bottom:0.8333333333333334em
  }
  .md h3,.md h4 {
	font-size:1.1428571428571428em;
	line-height:1.25em;
	margin-top:0.625em;
	margin-bottom:0.625em
  }
  .md h5,.md h6 {
	font-size:1em;
	line-height:1.4285714285714286em;
	margin-top:0.7142857142857143em;
	margin-bottom:0.35714285714285715em;
  }
  .md .-blocks,.md .-lists,.md pre,.md blockquote,.md table,.md p,.md ul,.md ol {
	margin-top:0.35714285714285715em;
	margin-bottom:0.35714285714285715em;
  }
  .md textarea,.md .-text,.md p,.md pre>code,.md th,.md td,.md li {
	font-size:1em;
	line-height:1.4285714285714286em;
  }
  .md-container-small .md,.side .md {
	font-size:0.9230769230769231em;
  }
  .md-container-small .md h1,.side .md h1,.md-container-small .md h2,.side .md h2 {
	font-size:1.5em;
	line-height:1.3888888888888888em;
	margin-top:0.5555555555555556em;
	margin-bottom:0.5555555555555556em;
  }
  .md-container-small .md h3,.side .md h3,.md-container-small .md h4,.side .md h4 {
	font-size:1.3333333333333333em;
	line-height:1.25em;
	margin-top:0.625em;
	margin-bottom:0.625em;
  }
  .md-container-small .md h5,.side .md h5,.md-container-small .md h6,.side .md h6 {
	font-size:1.1666666666666667em;
	line-height:1.4285714285714286em;
	margin-top:0.7142857142857143em;
	margin-bottom:0.35714285714285715em;
  }
  .md-container-small .md .-blocks,.side .md .-blocks,.md-container-small .md .-lists,.side .md .-lists,.md-container-small .md pre,.side .md pre,.md-container-small .md blockquote,.side .md blockquote,.md-container-small .md table,.side .md table,.md-container-small .md p,.side .md p,.md-container-small .md ul,.side .md ul,.md-container-small .md ol,.side .md ol {
	margin-top:0.4166666666666667em;
	margin-bottom:0.4166666666666667em;
  }
  .md-container-small .md .-text,.side .md .-text,.md-container-small .md p,.side .md p,.md-container-small .md pre>code,.side .md pre>code,.md-container-small .md th,.side .md th,.md-container-small .md td,.side .md td,.md-container-small .md li,.side .md li {
	font-size:1em;
	line-height:1.25em;
  }
  /* REDDIT WIKI STYLES */
  .wiki-content {
	margin:15px;
	float: left;
	width: calc(100% - 30px);
	font-size: 0.8rem;
  }
  .wiki-content .pagelisting {
	font-size:1.2em;
	font-weight:bold;
	color:black;
	padding-left:25px
  }
  .wiki-content .pagelisting ul {
	list-style:disc;
	padding:2px;
	padding-left:10px
  }
  .wiki-content .description {
	padding-bottom:5px
  }
  .wiki-content .description h2 {
	color:#222
  }
  .wiki-content ul.wikipagelisting {
	padding: 0px 0px 0px 30px;
  }
  .wiki-content .wikirevisionlisting .generic-table {
	width:100%
  }
  .wiki-content .wikirevisionlisting table tr td {
	padding-right:15px
  }
  .wiki-content .wikirevisionlisting .revision.deleted {
	opacity:.5;
	text-decoration:line-through
  }
  .wiki-content .wikirevisionlisting .revision.hidden {
	opacity:.5
  }
  .wiki-content .wikirevisionlisting .revision.hidden td {
	opacity:inherit
  }
  .wiki-content .wiki.md {
	max-width:none
  }
  .wiki-content .wiki>.toc>ul {
	float:right;
	padding:11px 22px;
	margin:0 0 11px 22px;
	border:1px solid #8D9CAA;
	list-style:none;
	max-width:300px
  }
  .wiki-content .wiki>.toc>ul ul {
	margin:4px 0;
	padding-left:22px;
	border-left:1px dotted #cce;
	list-style:none
  }
  .wiki-content .wiki>.toc>ul li {
	margin:0
  }
  .wiki-content .fancy-settings .toggle {
	display:inline-block;
	padding-right:15px
  }
  .wiki-content #wiki_revision_reason {
	padding:2px;
	margin-left:0;
	width:100%
  }
  .wiki-content .wiki_button {
	padding:2px
  }
  .wiki-content .throbber {
	margin-bottom:-5px
  }
  .wiki-content .discussionlink {
	display:inline-block;
	margin-left:15px;
	padding-right:50px;
	margin-top:5px
  }
  .wiki-content .discussionlink a {
	padding-left:15px
  }
  .wiki-content .markhelp {
	max-width:500px;
	font-size:1.2em;
	padding:4px;
	margin:5px 0
  }
  
  /* "CLEANED HOMEPAGE" SECTION */
  body.homepage.clean {
	margin: 0;
	width: 100vw;
	height: 100vh;
	display: flex;
	flex-direction: column;
	justify-content: center;
	align-items: center;
  }
  
  body.homepage.clean main {
	flex-grow: 1;
	display: flex;
	width: 100%;
	flex-direction: column;
	justify-content: center;
	align-items: center;
  }
  
  body.homepage.clean h1 {
	margin-bottom: 1rem;
	font-size: 3rem;
	text-align: center;
	width: 100%;
  }
  
  body.homepage.clean form {
	width: 100vw;
	max-width: 750px;
	text-align: center;
  }
  
  body.homepage.clean input[name="q"] {
	width: 90%;
	padding: 0.4rem;
	border: none;
	color: white;
	background: #555;
	margin-bottom: 1rem;
  }
  
  body.homepage.clean .sublinks {
	display: flex;
	flex-direction: row;
	flex-wrap: wrap;
	justify-content: center;
	max-width: 650px;
  }
  
  body.homepage.clean .sublinks a {
	color: gray;
	margin-right: 0.3rem;
  }
  
  .homepage.clean .top-links {
	display: none;
  }
  
  @media only screen and (max-width: 768px) {
	body.homepage.clean form, body.homepage.clean .sublinks {
	  width: 90%;
	  max-width: unset;
	}
  }
  
  /* Large gallery items */
  .gallery .item.large {
	display: flex;
	flex-direction: column;
	margin-bottom: 1rem;
	position: relative;
	margin-right: 0.3rem;
  }
  
  .gallery .item.large img {
	max-height: 90vh;
	position: relative;
  }
  
  .gallery .item.large .caption {
	position: absolute;
	width: calc(100% - 0.6rem);
	color: white;
	background: rgba(0, 0, 0, 0.7);
	padding: 0.3rem;
	bottom: 0;
  }
  
  @media only screen and (max-width: 768px) {
	.gallery .item.large img {
	  max-height: unset;
	  max-width: 100%;
	}
  }
  
  /* Fix spoiler texts not showing without JS */
  .md .md-spoiler-text:not(.revealed):active,.md .md-spoiler-text:not(.revealed):focus,.md .md-spoiler-text:not(.revealed):hover {
	color: black;
	background: #fff0;
	transition: none;
  }
  .md .md-spoiler-text:not(.revealed):active *,
  .md .md-spoiler-text:not(.revealed):focus *,
  .md .md-spoiler-text:not(.revealed):hover * {
	opacity: 1;
  }
  @media only screen and (max-width: 768px) {
	#user .info {
	  float: right;
	  width: 100%;
	  text-align: center;
	  margin: 0 0px 20px 0;
	}
	#user .entries {
	  float: left;
	  width: calc(100% - 20px);
	  min-height: 100vh;
	}
	#links .link .entry .selftext {
	  width: calc(100% - 10%);
	}
  }
  @media only screen and (max-width: 600px) {
	#sidebar {
	  width: 100%;
	}
	#sidebar .content {
	  padding-left: 20px;
	  padding-right: 20px;
	}
	#sidebar .content {
	  float: left;
	  font-size: smaller;
	  padding-right: 15px;
	  width: calc(100% - 60px);
  }
	#search {
	  margin-left: 20px;
	  margin-top: 30px;
	}
	#search.sr {
	  margin-top: 30px;
	}
	#links.sr {
	  width: calc(100% - 10px);
	}
	#search form {
	  width: 240px;
	}
	.comment {
	  padding-left: 2.5%;
	}
	.comment details > summary::before {
	  content: '[ + ]';
	  font-size: 1.25rem;
	}
	.comment details[open] > summary::before {
	  content: '[ ‒ ]';
	  font-size: 1.25rem;
	}
	.comment details summary {
	  margin-right: 20px;
	  margin-bottom: 10px;
	}
	.comment summary::-webkit-details-marker {
	  font-size: 1.25rem
	}
	#post .usertext-body {
	  margin-left: 2.5%;
	  margin-right: 2.5%;
	  width: calc(100% - 10%);
	}
	#post .image {
	  padding: 2.5%;
	}
	#post .video {
	  margin: 5px 2.5%;
	  width: 95%;
	}
	#post .video .title {
	  width: 100%;
	}
	#post .video-holder a img {
	  width: 100%;
	}
	.info .submitted {
	  margin: auto auto 2.5% 2.5%;
	}
	#post .crosspost {
	  margin: auto 2.5%;
	}
	a.sub-to-subreddit {
	  padding: 8px 10px 8px 10px;
	}
		.explore#links .link {
	  padding-left: 5px;
	}
	.explore#links .link .sub-button {
	  margin: 7px 0;
	  width: 90px;
	}
	.explore#links .link .entry {
	  width: calc(100% - 20px);
	}
	#links .link .entry details[open] .preview {
	  width: 100vw;
	  transform: translateX(-150px);
	}
	#links .link .entry .selftext {
	  width: calc(100vw - 40px);
	  transform: translateX(-150px);
	  margin-left: 10px;
	}
  }
  </style>
</head>

<body class="dark"><div id="topbar"><nav><div class="nav-item left"><a href="/"><img src="/favicon.png" alt="">unfinished business</a></div><div class="settings"><div class="icon-container"><a href="/about">[about]</a></div><div class="icon-container"><a href="/preferences">[preferences]</a></div></div></nav><div class="top-links"><a href="/r/Popular">Popular</a><a href="/r/All">All</a><a href="/saved">Saved</a><a href="/r/AskReddit">AskReddit</a><a href="/r/pics">pics</a><a href="/r/news">news</a><a href="/r/worldnews">worldnews</a><a href="/r/funny">funny</a><a href="/r/tifu">tifu</a><a href="/r/videos">videos</a><a href="/r/gaming">gaming</a><a href="/r/aww">aww</a><a href="/r/todayilearned">todayilearned</a><a href="/r/gifs">gifs</a><a href="/r/Art">Art</a><a href="/r/explainlikeimfive">explainlikeimfive</a><a href="/r/movies">movies</a><a href="/r/Jokes">Jokes</a><a href="/r/TwoXChromosomes">TwoXChromosomes</a><a href="/r/mildlyinteresting">mildlyinteresting</a><a href="/r/LifeProTips">LifeProTips</a><a href="/r/askscience">askscience</a><a href="/r/IAmA">IAmA</a><a href="/r/dataisbeautiful">dataisbeautiful</a><a href="/r/books">books</a><a href="/r/science">science</a><a href="/r/Showerthoughts">Showerthoughts</a><a href="/r/gadgets">gadgets</a><a href="/r/Futurology">Futurology</a><a href="/r/nottheonion">nottheonion</a><a href="/r/history">history</a><a href="/r/sports">sports</a><a href="/r/OldSchoolCool">OldSchoolCool</a><a href="/r/GetMotivated">GetMotivated</a><a href="/r/DIY">DIY</a><a href="/r/photoshopbattles">photoshopbattles</a><a href="/r/nosleep">nosleep</a><a href="/r/Music">Music</a><a href="/r/space">space</a><a href="/r/food">food</a><a href="/r/UpliftingNews">UpliftingNews</a><a href="/r/EarthPorn">EarthPorn</a><a href="/r/Documentaries">Documentaries</a><a href="/r/InternetIsBeautiful">InternetIsBeautiful</a><a href="/r/WritingPrompts">WritingPrompts</a><a href="/r/creepy">creepy</a><a href="/r/philosophy">philosophy</a><a href="/r/announcements">announcements</a><a href="/r/listentothis">listentothis</a><a href="/r/blog">blog</a><a href="/subreddits" id="sr-more-link">more »</a></div></div><header><a class="main" href="/"><h1>unfinished business</h1></a><div class="bottom"><ul class="tabmenu"><li class="active"><a href="/">hot</a></li><li><a href="/new">new</a></li><li><a href="/rising">rising</a></li><li><a href="downloadAll">Backup</a>

</li><li><a href="restoreAllPage">Restore</a></li></ul></div></header><div id="intro">
<h1>Welcome to Unfinished Business</h1>
<h2>the online hierarchical task manager.</h2>
</div>


<div class="sr" id="links">




   ` + taskDisplay(id, str2md5("nodes"), false,1) + `
   </div>
  <!-- 4 include the jQuery library -->
  <script src="https://cdnjs.cloudflare.com/ajax/libs/jquery/1.12.1/jquery.min.js"></script>
</style>
</div>
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
   ` + taskDisplay(id, q, true, -1) + `
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

func loadTasks(id, path string, task *Task, detailed bool, depth int) string {
	
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
			out = out + buildItem("","", path, task.Name, task.Id, task.Checked)
			/*
			fmt.Sprintf("<li><input type=\"checkbox\" "+isTaskChecked(task)+" onclick=\"$.get('toggle?path=%s')\"><a href=\"detailed?q=%s\">", path, path) + task.Name + "</a><ul>"
			*/
			tasks := task.SubTasks

			if depth != 0 {
			for _, f := range tasks {
				//log.Println("Loading task", f.Name)
				out = out + loadTasks(id, path+"/"+f.Id, f, detailed, depth-1)
			}
		}
			out = out + "</ul></li>"
		} else {
			//fmt.Println(path, "is leaf task")
			var contents = task.Text

			if detailed {
				out = out + "<li><input type=\"checkbox\"  " + isTaskChecked(task) + " onclick=\"$.get('toggle?path=" + path + "')\">" + task.Name + " <a href=\"detailed?q=" + path + "\">+</a><p style=\"margin-left: 10em\">" + string(contents) + "</p>" + "</li>"
			} else {
				out=out+buildItem("","",path, task.Name, task.Id, task.Checked)				
				//out = out + "<li><input type=\"checkbox\"  " + isTaskChecked(task) + " onclick=\"$.get('toggle?path=" + path + "')\">" + task.Name + " <a href=\"detailed?q=" + path + "\">+</a></li>"
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

func taskDisplay(id, path string, detailed bool, depth int) string {
	task := FindTask(path, LoadJson(id))
	if task == nil {
		panic("Task not found " + path)
	}
	return loadTasks(id, path, nil, detailed, depth) + `<form action="addWaypoint" method="post" ><input type="hidden" id="q" name="q" value="` + path + `"><input id="title" name="title" type="text"><input id="content" name="content" type="text"><input type="submit"  value="Add"></form>` + `<form action="deleteWaypoint" method="post"  ><input type="hidden" id="q" name="q" value="` + path + `"><input type="submit" value="Delete"></form>` + `<form action="editWaypoint" method="post"  ><input type="hidden" id="q" name="q" value="` + path + `"><input id="title" name="title" type="text" value="` + task.Name + `"><input id="content" name="content" type="text" value="` + task.Text + `"><input type="submit" value="Update"></form>`

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
