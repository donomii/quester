package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newTestApp(t *testing.T) (*App, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(store, "/quester/")
	router := gin.New()
	app.Register(router)
	return app, router
}

func TestSummaryEscapesUserContent(t *testing.T) {
	_, router := newTestApp(t)
	form := url.Values{
		"q":       {rootPath},
		"title":   {`<script>alert("x")</script>`},
		"content": {`<b>bold</b>`},
	}
	postForm(t, router, "/quester/addWaypoint", form)

	resp := performRequest(router, http.MethodGet, "/quester/summary", nil)
	defer resp.Body.Close()
	body := readBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	if strings.Contains(body, `<script>alert("x")</script>`) || strings.Contains(body, `<b>bold</b>`) {
		t.Fatalf("response contains unescaped user HTML: %s", body)
	}
	if !strings.Contains(body, `&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;`) {
		t.Fatalf("response did not contain escaped title: %s", body)
	}
}

func TestMutationsUsePostAndBadPathsDoNotPanic(t *testing.T) {
	_, router := newTestApp(t)

	getToggle := performRequest(router, http.MethodGet, "/quester/toggle?path="+rootPath, nil)
	getToggle.Body.Close()
	if getToggle.StatusCode != http.StatusNotFound {
		t.Fatalf("GET toggle status = %d, want 404", getToggle.StatusCode)
	}

	deleteMissing := postForm(t, router, "/quester/deleteWaypoint", url.Values{"q": {rootPath + "/missing"}})
	if deleteMissing.StatusCode != http.StatusNotFound {
		body := readBody(t, deleteMissing)
		t.Fatalf("delete missing status = %d, body = %s", deleteMissing.StatusCode, body)
	}
	deleteMissing.Body.Close()
}

func TestMutationRejectsCrossOriginPost(t *testing.T) {
	app, router := newTestApp(t)
	form := url.Values{
		"q":     {rootPath},
		"title": {"Blocked"},
	}
	req := httptest.NewRequest(http.MethodPost, "/quester/addWaypoint", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://example.invalid")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	resp := rec.Result()
	resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin mutation status = %d, want 403", resp.StatusCode)
	}
	root, err := app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(root.SubTasks) != 0 {
		t.Fatalf("cross-origin mutation added %d tasks, want 0", len(root.SubTasks))
	}
}

func TestMutationAllowsSameOriginPost(t *testing.T) {
	app, router := newTestApp(t)
	form := url.Values{
		"q":     {rootPath},
		"title": {"Allowed"},
	}
	req := httptest.NewRequest(http.MethodPost, "http://quester.local/quester/addWaypoint", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://quester.local")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	resp := rec.Result()
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("same-origin mutation status = %d, want 303", resp.StatusCode)
	}
	root, err := app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(root.SubTasks) != 1 || root.SubTasks[0].Name != "Allowed" {
		t.Fatalf("same-origin mutation tasks = %#v", root.SubTasks)
	}
}

func TestAddToggleDeleteFlow(t *testing.T) {
	app, router := newTestApp(t)
	postForm(t, router, "/quester/addWaypoint", url.Values{
		"q":     {rootPath},
		"title": {"First"},
	})

	root, err := app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(root.SubTasks) != 1 {
		t.Fatalf("subtasks = %d, want 1", len(root.SubTasks))
	}
	path := rootPath + "/" + root.SubTasks[0].Id

	postForm(t, router, "/quester/toggle", url.Values{"path": {path}, "next": {"/quester/summary"}})
	root, err = app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	if !root.SubTasks[0].Checked {
		t.Fatal("task was not toggled checked")
	}

	postForm(t, router, "/quester/deleteWaypoint", url.Values{"q": {path}})
	root, err = app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	if !root.SubTasks[0].Deleted {
		t.Fatal("task was not marked deleted")
	}
}

func TestEditDownloadAndRestoreFlow(t *testing.T) {
	app, router := newTestApp(t)
	postForm(t, router, "/quester/addWaypoint", url.Values{
		"q":     {rootPath},
		"title": {"Original"},
	})

	root, err := app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	path := rootPath + "/" + root.SubTasks[0].Id

	editResp := postForm(t, router, "/quester/editWaypoint", url.Values{
		"q":       {path},
		"title":   {"Updated"},
		"content": {"Edited notes"},
	})
	editResp.Body.Close()
	if editResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("edit status = %d, want 303", editResp.StatusCode)
	}

	download := performRequest(router, http.MethodGet, "/quester/downloadAll", nil)
	downloadBody := readBody(t, download)
	download.Body.Close()
	if download.StatusCode != http.StatusOK {
		t.Fatalf("download status = %d, body = %s", download.StatusCode, downloadBody)
	}
	if got := download.Header.Get("Content-Disposition"); got != `attachment; filename="tasks.json"` {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if !strings.Contains(downloadBody, `"Name": "Updated"`) || !strings.Contains(downloadBody, `"Text": "Edited notes"`) {
		t.Fatalf("download did not include edited task: %s", downloadBody)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("content", "tasks.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(`{"Name":"Restored","SubTasks":[{"Id":"restored-child","Name":"Restored child"}]}`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/quester/restoreAll", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	restore := rec.Result()
	restore.Body.Close()
	if restore.StatusCode != http.StatusSeeOther {
		t.Fatalf("restore status = %d, want 303", restore.StatusCode)
	}

	restoredRoot, err := app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredRoot.Name != "Restored" {
		t.Fatalf("restored root name = %q, want Restored", restoredRoot.Name)
	}
	if got := FindTask(rootPath+"/restored-child", restoredRoot); got == nil || got.Name != "Restored child" {
		t.Fatalf("restored child = %#v, want Restored child", got)
	}
}

func TestRootRouteRedirectsToSummary(t *testing.T) {
	_, router := newTestApp(t)

	resp := performRequest(router, http.MethodGet, "/quester/", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "/quester/summary" {
		t.Fatalf("Location = %q, want /quester/summary", got)
	}
}

func TestRootPrefixCanRegister(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(store, "/")
	router := gin.New()
	app.Register(router)

	resp := performRequest(router, http.MethodGet, "/", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "/summary" {
		t.Fatalf("Location = %q, want /summary", got)
	}
}

func TestAttachAndServeDocumentFlow(t *testing.T) {
	app, router := newTestApp(t)
	postForm(t, router, "/quester/addWaypoint", url.Values{"q": {rootPath}, "title": {"Doc task"}})

	root, err := app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	path := rootPath + "/" + root.SubTasks[0].Id

	attach := postMultipart(t, router, "/quester/attachDocument", url.Values{"q": {path}}, []testUpload{
		{"document", "notes.txt", "first version"},
	})
	attach.Body.Close()
	if attach.StatusCode != http.StatusSeeOther {
		t.Fatalf("attach status = %d, want 303", attach.StatusCode)
	}

	root, err = app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	attachments := root.SubTasks[0].Attachments
	if len(attachments) != 1 {
		t.Fatalf("attachments = %#v, want 1", attachments)
	}
	record := attachments[0]
	if record.Name != "notes.txt" || record.Size != int64(len("first version")) || !isBlobRef(record.Blob) {
		t.Fatalf("attachment record = %#v", record)
	}

	doc := performRequest(router, http.MethodGet, "/quester/document?q="+url.QueryEscape(path)+"&doc="+url.QueryEscape(record.Id), nil)
	body := readBody(t, doc)
	doc.Body.Close()
	if doc.StatusCode != http.StatusOK || body != "first version" {
		t.Fatalf("document status = %d body = %q", doc.StatusCode, body)
	}
	if got := doc.Header.Get("Content-Disposition"); !strings.HasPrefix(got, "inline") || !strings.Contains(got, "notes.txt") {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if got := doc.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := doc.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}

	detail := performRequest(router, http.MethodGet, "/quester/detailed?q="+url.QueryEscape(path), nil)
	detailBody := readBody(t, detail)
	detail.Body.Close()
	if !strings.Contains(detailBody, "notes.txt") || !strings.Contains(detailBody, "Documents at") {
		t.Fatalf("detail page missing attachment: %s", detailBody)
	}
	if !strings.Contains(detailBody, record.Blob[:8]) {
		t.Fatalf("detail page missing content ref %s: %s", record.Blob[:8], detailBody)
	}
	if !strings.Contains(detailBody, "Documents in effect: 1") {
		t.Fatalf("detail page missing in-effect line: %s", detailBody)
	}
}

func TestAddWaypointCarriesDocument(t *testing.T) {
	app, router := newTestApp(t)
	resp := postMultipart(t, router, "/quester/addWaypoint", url.Values{
		"q":     {rootPath},
		"title": {"Reply with doc"},
	}, []testUpload{{"document", "spec.md", "updated spec"}})
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("add status = %d, want 303", resp.StatusCode)
	}

	root, err := app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(root.SubTasks) != 1 || len(root.SubTasks[0].Attachments) != 1 {
		t.Fatalf("tasks = %#v", root.SubTasks)
	}
	if got := root.SubTasks[0].Attachments[0].Name; got != "spec.md" {
		t.Fatalf("attachment name = %q, want spec.md", got)
	}
}

func TestDocumentBlocksActiveContentAndUnknownIDs(t *testing.T) {
	app, router := newTestApp(t)
	resp := postMultipart(t, router, "/quester/addWaypoint", url.Values{
		"q":     {rootPath},
		"title": {"Evil"},
	}, []testUpload{{"document", "evil.html", "<script>alert(1)</script>"}})
	resp.Body.Close()

	root, err := app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	path := rootPath + "/" + root.SubTasks[0].Id
	record := root.SubTasks[0].Attachments[0]

	doc := performRequest(router, http.MethodGet, "/quester/document?q="+url.QueryEscape(path)+"&doc="+url.QueryEscape(record.Id), nil)
	doc.Body.Close()
	if got := doc.Header.Get("Content-Disposition"); !strings.HasPrefix(got, "attachment") {
		t.Fatalf("html Content-Disposition = %q, want attachment", got)
	}

	missing := performRequest(router, http.MethodGet, "/quester/document?q="+url.QueryEscape(path)+"&doc=../../etc/passwd", nil)
	missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("missing doc status = %d, want 404", missing.StatusCode)
	}

	empty := postMultipart(t, router, "/quester/attachDocument", url.Values{"q": {path}}, nil)
	empty.Body.Close()
	if empty.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty attach status = %d, want 400", empty.StatusCode)
	}
}

func TestDocumentContentTypeChoosesSafeDisposition(t *testing.T) {
	cases := []struct {
		name            string
		wantTypePrefix  string
		wantDisposition string
	}{
		{"notes.md", "text/markdown", "inline"},
		{"notes.txt", "text/plain", "inline"},
		{"photo.png", "image/png", "inline"},
		{"page.html", "text/html", "attachment"},
		{"vector.svg", "image/svg+xml", "attachment"},
		{"blob.xyz-unknown", "application/octet-stream", "attachment"},
	}
	for _, tc := range cases {
		gotType, gotDisposition := documentContentType(tc.name)
		if !strings.HasPrefix(gotType, tc.wantTypePrefix) || gotDisposition != tc.wantDisposition {
			t.Fatalf("documentContentType(%q) = %q, %q, want %s*, %s",
				tc.name, gotType, gotDisposition, tc.wantTypePrefix, tc.wantDisposition)
		}
	}
}

func TestDocumentHistoryPage(t *testing.T) {
	app, router := newTestApp(t)
	postMultipart(t, router, "/quester/addWaypoint", url.Values{
		"q":     {rootPath},
		"title": {"Doc task"},
	}, []testUpload{{"document", "spec.md", "one"}}).Body.Close()

	root, err := app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	path := rootPath + "/" + root.SubTasks[0].Id
	baseRef := root.SubTasks[0].Attachments[0].Blob[:8]

	postMultipart(t, router, "/quester/addWaypoint", url.Values{
		"q":     {path},
		"title": {"Reply"},
	}, []testUpload{{"document", "spec.md", "two-two"}}).Body.Close()

	root, err = app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	replyRef := root.SubTasks[0].SubTasks[0].Attachments[0].Blob[:8]

	baseID := root.SubTasks[0].Attachments[0].Id
	replyID := root.SubTasks[0].SubTasks[0].Attachments[0].Id
	root.SubTasks[0].SubTasks[0].Attachments[0].Replaces = baseID
	if err := app.store.Update(defaultUserID, func(stored *Task) error {
		stored.SubTasks[0].SubTasks[0].Attachments[0].Replaces = baseID
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	resp := performRequest(router, http.MethodGet, "/quester/documentHistory?q="+url.QueryEscape(path)+"&doc="+url.QueryEscape(baseID), nil)
	body := readBody(t, resp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("history status = %d, body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, baseRef) || !strings.Contains(body, replyRef) {
		t.Fatalf("history missing refs %s/%s: %s", baseRef, replyRef, body)
	}
	if !strings.Contains(body, "Later revisions") || !strings.Contains(body, replyID) {
		t.Fatalf("history missing below section: %s", body)
	}

	noDocument := performRequest(router, http.MethodGet, "/quester/documentHistory?q="+url.QueryEscape(path), nil)
	noDocument.Body.Close()
	if noDocument.StatusCode != http.StatusBadRequest {
		t.Fatalf("history without document = %d, want 400", noDocument.StatusCode)
	}

	badTask := performRequest(router, http.MethodGet, "/quester/documentHistory?q="+url.QueryEscape(rootPath+"/missing")+"&doc="+url.QueryEscape(baseID), nil)
	badTask.Body.Close()
	if badTask.StatusCode != http.StatusNotFound {
		t.Fatalf("history for missing task = %d, want 404", badTask.StatusCode)
	}
}

func TestForumAndAgentAPIFlow(t *testing.T) {
	app, router := newTestApp(t)

	forumResponse := performJSONRequest(router, http.MethodPost, "/quester/api/forums", `{"name":"Trips","description":"Private travel planning"}`, nil)
	if forumResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create forum status = %d body = %s", forumResponse.StatusCode, readBody(t, forumResponse))
	}
	var forum Forum
	if err := json.NewDecoder(forumResponse.Body).Decode(&forum); err != nil {
		t.Fatal(err)
	}
	forumResponse.Body.Close()

	postResponse := performJSONRequest(router, http.MethodPost, "/quester/api/nodes", `{"forum_id":"`+forum.Id+`","title":"Japan","body":"@trip-agent plan the trip"}`, map[string]string{
		"X-Quester-User": "planner",
		"X-Quester-Name": "Planner",
	})
	if postResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create post status = %d body = %s", postResponse.StatusCode, readBody(t, postResponse))
	}
	var post apiNode
	if err := json.NewDecoder(postResponse.Body).Decode(&post); err != nil {
		t.Fatal(err)
	}
	postResponse.Body.Close()

	replyResponse := performJSONRequest(router, http.MethodPost, "/quester/api/nodes", `{"parent_id":"`+post.ID+`","body":"I found a later flight."}`, map[string]string{
		"X-Quester-User":  "trip-agent",
		"X-Quester-Name":  "Trip agent",
		"X-Quester-Agent": "true",
	})
	if replyResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create reply status = %d body = %s", replyResponse.StatusCode, readBody(t, replyResponse))
	}
	var reply apiNode
	if err := json.NewDecoder(replyResponse.Body).Decode(&reply); err != nil {
		t.Fatal(err)
	}
	replyResponse.Body.Close()
	if reply.ForumID != forum.Id || reply.ParentID != post.ID || reply.AuthorID != "trip-agent" || reply.Status != "" {
		t.Fatalf("agent reply = %#v", reply)
	}

	moveResponse := performJSONRequest(router, http.MethodPost, "/quester/api/nodes/"+reply.ID+"/move", `{"forum_id":"general","title":"Flight option"}`, nil)
	if moveResponse.StatusCode != http.StatusOK {
		t.Fatalf("promote reply status = %d body = %s", moveResponse.StatusCode, readBody(t, moveResponse))
	}
	moveResponse.Body.Close()

	root, err := app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	moved := FindTask(reply.ID, root)
	if moved == nil || moved.ForumId != defaultForumID || findParent(root, moved) != root {
		t.Fatalf("promoted node = %#v", moved)
	}
	agent := findUser(root, "trip-agent")
	if agent == nil || !agent.IsAgent || agent.Name != "Trip agent" {
		t.Fatalf("agent user = %#v", agent)
	}
	mentions := performRequest(router, http.MethodGet, "/quester/api/mentions/trip-agent", nil)
	if mentions.StatusCode != http.StatusOK {
		t.Fatalf("mentions status = %d body = %s", mentions.StatusCode, readBody(t, mentions))
	}
	var mentionedNodes []apiNode
	if err := json.NewDecoder(mentions.Body).Decode(&mentionedNodes); err != nil {
		t.Fatal(err)
	}
	mentions.Body.Close()
	if len(mentionedNodes) != 1 || mentionedNodes[0].ID != post.ID {
		t.Fatalf("mentions = %#v", mentionedNodes)
	}

	detail := performRequest(router, http.MethodGet, "/quester/detailed?q="+reply.ID, nil)
	detailBody := readBody(t, detail)
	detail.Body.Close()
	if detail.StatusCode != http.StatusOK {
		t.Fatalf("stable node detail status = %d", detail.StatusCode)
	}
	if !strings.Contains(detailBody, "Trip agent") || !strings.Contains(detailBody, "Flight option") {
		t.Fatalf("promoted node detail missing author or title: %s", detailBody)
	}

	trips := performRequest(router, http.MethodGet, "/quester/summary?forum="+forum.Id, nil)
	tripsBody := readBody(t, trips)
	trips.Body.Close()
	if trips.StatusCode != http.StatusOK || !strings.Contains(tripsBody, "Japan") || strings.Contains(tripsBody, "Flight option") {
		t.Fatalf("trips forum status = %d body = %s", trips.StatusCode, tripsBody)
	}

	index := performRequest(router, http.MethodGet, "/quester/api/", nil)
	indexBody := readBody(t, index)
	index.Body.Close()
	if index.StatusCode != http.StatusOK || !strings.Contains(indexBody, "X-Quester-User") || !strings.Contains(indexBody, "api/mentions") {
		t.Fatalf("API index status = %d body = %s", index.StatusCode, indexBody)
	}
}

func TestAPIAttachmentRevisionUsesExplicitLink(t *testing.T) {
	app, router := newTestApp(t)
	postForm(t, router, "/quester/addWaypoint", url.Values{"q": {rootPath}, "forum": {defaultForumID}, "title": {"Documents"}}).Body.Close()
	root, err := app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	nodeID := root.SubTasks[0].Id

	first := postMultipart(t, router, "/quester/api/nodes/"+nodeID+"/attachments", nil, []testUpload{{"document", "ticket.pdf", "first"}})
	if first.StatusCode != http.StatusCreated {
		t.Fatalf("first attachment status = %d body = %s", first.StatusCode, readBody(t, first))
	}
	var firstNode apiNode
	if err := json.NewDecoder(first.Body).Decode(&firstNode); err != nil {
		t.Fatal(err)
	}
	first.Body.Close()
	firstID := firstNode.Attachments[0].ID

	second := postMultipart(t, router, "/quester/api/nodes/"+nodeID+"/attachments", url.Values{"replaces": {firstID}}, []testUpload{{"document", "boarding-pass.pdf", "second"}})
	if second.StatusCode != http.StatusCreated {
		t.Fatalf("revision attachment status = %d body = %s", second.StatusCode, readBody(t, second))
	}
	var secondNode apiNode
	if err := json.NewDecoder(second.Body).Decode(&secondNode); err != nil {
		t.Fatal(err)
	}
	second.Body.Close()
	if got := secondNode.Attachments[1].Replaces; got != firstID {
		t.Fatalf("replacement link = %q, want %q", got, firstID)
	}
}

type testUpload struct {
	field   string
	name    string
	content string
}

func postMultipart(t *testing.T, router http.Handler, target string, fields url.Values, files []testUpload) *http.Response {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, values := range fields {
		for _, value := range values {
			if err := writer.WriteField(key, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, file := range files {
		part, err := writer.CreateFormFile(file.field, file.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(file.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return performRequest(router, http.MethodPost, target, &body, writer.FormDataContentType())
}

func postForm(t *testing.T, router http.Handler, target string, form url.Values) *http.Response {
	t.Helper()
	return performRequest(router, http.MethodPost, target, strings.NewReader(form.Encode()), "application/x-www-form-urlencoded")
}

func performRequest(router http.Handler, method, target string, body io.Reader, contentType ...string) *http.Response {
	req := httptest.NewRequest(method, target, body)
	if len(contentType) > 0 {
		req.Header.Set("Content-Type", contentType[0])
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec.Result()
}

func performJSONRequest(router http.Handler, method, target, body string, headers map[string]string) *http.Response {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec.Result()
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
